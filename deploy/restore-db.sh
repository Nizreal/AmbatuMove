#!/usr/bin/env bash
set -Eeuo pipefail
umask 077

usage() { echo "Usage: $0 BACKUP.sqlite --confirm" >&2; exit 2; }
[[ $# == 2 && $2 == --confirm ]] || usage
backup_input=$1
repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
deploy_dir="$repo_root/deploy"
install_root=${AMBATUMOVE_INSTALL_ROOT:-/mnt/storage/ambatumove-deployment}
env_file=${AMBATUMOVE_CONFIG_FILE:-$install_root/config/ambatumove.env}
state_root="$install_root/state"
[[ -f "$env_file" ]] || { echo "Missing $env_file." >&2; exit 1; }
[[ -s "$state_root/current-version" ]] || { echo 'No active release is recorded; refusing database restore.' >&2; exit 1; }
for tool in docker curl realpath findmnt flock awk chown cp mv date seq; do command -v "$tool" >/dev/null || { echo "Required command not found: $tool" >&2; exit 1; }; done
docker compose version >/dev/null

data_dir=$(awk -F= '$1=="DATA_HOST_DIR" {value=$2} END {print value}' "$env_file")
data_dir=${data_dir:-/mnt/storage/ambatumove}
[[ $data_dir == /* ]] || { echo 'DATA_HOST_DIR must be an absolute path.' >&2; exit 1; }
data_root=$(realpath -e "$data_dir")
mount_target=$(findmnt -n -o TARGET -T "$data_root")
[[ $mount_target == /mnt/storage && -w "$data_root" ]] || { echo "Expected writable persistent /mnt/storage mount, found '$mount_target'." >&2; exit 1; }
backup=$(realpath -e "$backup_input")
case "$backup" in "$data_root"/backups/*) ;; *) echo 'Backup must be stored in DATA_HOST_DIR/backups on the persistent storage mount.' >&2; exit 1 ;; esac
[[ -s "$backup" ]] || { echo 'Backup file is empty.' >&2; exit 1; }

mkdir -p "$state_root"
export AMBATUMOVE_CONFIG_FILE="$env_file"
exec 9>"$state_root/deploy.lock"
flock -n 9 || { echo 'A deployment or database recovery is already running.' >&2; exit 1; }
version=$(<"$state_root/current-version")
[[ $version =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]] || { echo 'Recorded active release version is invalid.' >&2; exit 1; }
export AMBATUMOVE_IMAGE="ambatumove-core:$version"
compose=(docker compose --project-name ambatumove --env-file "$env_file" --file "$deploy_dir/compose.yaml")
"${compose[@]}" config --quiet

db="$data_root/ambatumove.sqlite"
[[ -s "$db" ]] || { echo "Live SQLite database not found: $db" >&2; exit 1; }
ts=$(date -u +%Y%m%dT%H%M%SZ)
mkdir -p "$data_root/backups"
snapshot="$data_root/backups/pre-restore-current-$ts.sqlite"
restore_tmp="$data_root/.ambatumove-restore-$ts.sqlite"
current_saved="$data_root/backups/pre-restore-live-$ts.sqlite"
failed_saved="$data_root/backups/failed-restore-$ts.sqlite"
for path in "$snapshot" "$snapshot-wal" "$snapshot-shm" "$restore_tmp" "$restore_tmp-wal" "$restore_tmp-shm" "$current_saved" "$current_saved-wal" "$current_saved-shm" "$failed_saved" "$failed_saved-wal" "$failed_saved-shm"; do
  [[ ! -e "$path" ]] || { echo "Refusing to overwrite existing recovery file: $path" >&2; exit 1; }
done

# Stage an owned copy of the selected snapshot and validate it before touching
# the live database. The original backup remains in place.
cp -- "$backup" "$restore_tmp"
chown 10001:10001 "$restore_tmp"
"${compose[@]}" run --rm --no-deps core-service verify-backup --path "/data/.ambatumove-restore-$ts.sqlite"
# Take a second consistent online snapshot of the current database so rollback
# is possible if activation fails.
"${compose[@]}" run --rm --no-deps core-service backup --output "/data/backups/pre-restore-current-$ts.sqlite"
"${compose[@]}" run --rm --no-deps core-service verify-backup --path "/data/backups/pre-restore-current-$ts.sqlite"

port=$(awk -F= '$1=="GATEWAY_PORT" {value=$2} END {print value}' "$env_file")
port=${port:-18080}
old_wal="$db-wal"; old_shm="$db-shm"
saved_wal="$current_saved-wal"; saved_shm="$current_saved-shm"
tmp_wal="$restore_tmp-wal"; tmp_shm="$restore_tmp-shm"

restore_previous_on_failure() {
  status=$?
  trap - ERR
  if [[ -e "$current_saved" ]]; then
    "${compose[@]}" stop core-service >/dev/null 2>&1 || true
    if [[ -e "$db" ]]; then mv -- "$db" "$failed_saved"; fi
    if [[ -e "$saved_wal" ]]; then
      [[ ! -e "$old_wal" ]] || mv -- "$old_wal" "$failed_saved-wal"
      mv -- "$saved_wal" "$old_wal"
    fi
    if [[ -e "$saved_shm" ]]; then
      [[ ! -e "$old_shm" ]] || mv -- "$old_shm" "$failed_saved-shm"
      mv -- "$saved_shm" "$old_shm"
    fi
    mv -- "$current_saved" "$db"
  fi
  if "${compose[@]}" up -d --wait --wait-timeout 60; then
    echo 'Database restore failed; the previous database and service were restored.' >&2
  else
    echo 'Automatic database rollback did not become healthy; inspect logs and recovery files immediately.' >&2
  fi
  exit "$status"
}

trap restore_previous_on_failure ERR
"${compose[@]}" stop core-service
mv -- "$db" "$current_saved"
[[ ! -e "$old_wal" ]] || mv -- "$old_wal" "$saved_wal"
[[ ! -e "$old_shm" ]] || mv -- "$old_shm" "$saved_shm"
mv -- "$restore_tmp" "$db"
[[ ! -e "$tmp_wal" ]] || mv -- "$tmp_wal" "$db-wal"
[[ ! -e "$tmp_shm" ]] || mv -- "$tmp_shm" "$db-shm"
"${compose[@]}" up -d --wait --wait-timeout 60
ready=0
for _ in $(seq 1 30); do
  if curl --fail --silent "http://127.0.0.1:$port/ready" >/dev/null; then ready=1; break; fi
  sleep 2
done
[[ $ready == 1 ]] || { echo 'Readiness failed after restoring the selected snapshot.' >&2; false; }
trap - ERR
echo "Restored verified database $backup. Previous live database is preserved at $current_saved."
