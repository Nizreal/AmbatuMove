#!/usr/bin/env bash
set -Eeuo pipefail
umask 077

usage() { echo "Usage: $0 deploy VERSION | rollback [VERSION]" >&2; exit 2; }
[[ $# -ge 1 && $# -le 2 ]] || usage
action=$1
[[ $action == deploy || $action == rollback ]] || usage
version=${2:-}
repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
deploy_dir="$repo_root/deploy"
install_root=${AMBATUMOVE_INSTALL_ROOT:-/mnt/storage/ambatumove-deployment}
config_file=${AMBATUMOVE_CONFIG_FILE:-$install_root/config/ambatumove.env}
release_root="$install_root/releases"
downloads_root="$install_root/downloads"
state_root="$install_root/state"

for tool in gh docker curl sha256sum tar flock awk seq mktemp chmod chown git findmnt realpath sort rm; do
  command -v "$tool" >/dev/null || { echo "Required command not found: $tool" >&2; exit 1; }
done
docker compose version >/dev/null
[[ $install_root == /* && $config_file == /* ]] || { echo 'Installation and configuration paths must be absolute.' >&2; exit 1; }
mount_target=$(findmnt -n -o TARGET -T /mnt/storage 2>/dev/null || true)
[[ $mount_target == /mnt/storage && -d /mnt/storage && -w /mnt/storage ]] || {
  echo "Persistent /mnt/storage mount is unavailable or not writable (found '${mount_target:-none}'); refusing deployment." >&2
  exit 1
}

mkdir -p "$install_root/config" "$release_root" "$downloads_root" "$state_root"
[[ -f "$config_file" && -s "$config_file" ]] || {
  echo "Missing $config_file. Create it from deploy/.env.example, configure secrets, and chmod 600." >&2
  exit 1
}
chmod 600 "$config_file"
exec 9>"$state_root/deploy.lock"
flock -n 9 || { echo 'Another AmbatuMove deployment is running.' >&2; exit 1; }

if [[ $action == rollback && -z $version ]]; then
  [[ -s "$state_root/previous-version" ]] || { echo 'No previous successful version is recorded.' >&2; exit 1; }
  version=$(<"$state_root/previous-version")
fi
[[ $version =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]] || { echo "Invalid release version: $version" >&2; exit 2; }

data_dir=$(awk -F= '$1=="DATA_HOST_DIR" {value=$2} END {print value}' "$config_file")
data_dir=${data_dir:-/mnt/storage/ambatumove}
[[ $data_dir == /mnt/storage/* && $data_dir != *".."* ]] || { echo 'DATA_HOST_DIR must be a directory beneath /mnt/storage.' >&2; exit 1; }
data_parent=$(dirname "$data_dir")
data_mount=$(findmnt -n -o TARGET -T "$data_parent" 2>/dev/null || true)
[[ $data_mount == /mnt/storage && -w "$data_parent" ]] || { echo "DATA_HOST_DIR parent is not on the writable /mnt/storage mount: $data_parent" >&2; exit 1; }
mkdir -p "$data_dir"
[[ -w "$data_dir" ]] || { echo "DATA_HOST_DIR is not writable: $data_dir" >&2; exit 1; }
export AMBATUMOVE_CONFIG_FILE="$config_file"

github_repo=${GH_REPOSITORY:-}
if [[ -z $github_repo ]]; then
  origin=$(git -C "$repo_root" remote get-url origin 2>/dev/null || true)
  if [[ $origin =~ github\.com[:/]([^/]+/[^/.]+)(\.git)?$ ]]; then github_repo=${BASH_REMATCH[1]}; fi
fi
[[ $github_repo =~ ^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$ ]] || { echo 'Set GH_REPOSITORY=OWNER/REPOSITORY (or configure a GitHub origin).' >&2; exit 1; }

archive="ambatumove-core-$version-linux-arm64.tar.gz"
checksum="$archive.sha256"
download_dir="$downloads_root/$version"
release_dir="$release_root/$version"
mkdir -p "$download_dir"
if [[ -e "$download_dir/$archive" || -e "$download_dir/$checksum" ]]; then
  [[ -s "$download_dir/$archive" && -s "$download_dir/$checksum" ]] || { echo "Incomplete cached release assets in $download_dir; preserve and inspect them." >&2; exit 1; }
else
  download_tmp=$(mktemp -d "$downloads_root/.${version}.download.XXXXXX")
  if ! gh release download "$version" --repo "$github_repo" --pattern "$archive" --pattern "$checksum" --dir "$download_tmp"; then
    rm -rf -- "$download_tmp"
    echo 'Could not download release assets. For a private repository, authenticate GitHub CLI with gh auth login.' >&2
    exit 1
  fi
  [[ -s "$download_tmp/$archive" && -s "$download_tmp/$checksum" ]] || { rm -rf -- "$download_tmp"; echo 'Release archive or checksum is missing.' >&2; exit 1; }
  (cd "$download_tmp" && sha256sum --check "$checksum") || { rm -rf -- "$download_tmp"; echo 'Release SHA-256 verification failed.' >&2; exit 1; }
  mv -- "$download_tmp/$archive" "$download_dir/$archive"
  mv -- "$download_tmp/$checksum" "$download_dir/$checksum"
  rmdir -- "$download_tmp"
fi
(cd "$download_dir" && sha256sum --check "$checksum") || { echo 'Cached GitHub Release checksum verification failed.' >&2; exit 1; }
entries=$(tar -tzf "$download_dir/$archive")
[[ $entries == server ]] || { echo 'Archive must contain exactly one top-level server executable.' >&2; exit 1; }

staging="$release_root/.${version}.verified.$$"
[[ ! -e "$staging" ]] || { echo "Refusing to overwrite existing staging path $staging." >&2; exit 1; }
mkdir -m 0755 "$staging"
if ! tar -xzf "$download_dir/$archive" -C "$staging" --no-same-owner; then
  echo "Release archive extraction failed; inspect preserved staging directory $staging." >&2
  exit 1
fi
[[ -f "$staging/server" && ! -L "$staging/server" ]] || { echo 'Extracted server file is invalid.' >&2; exit 1; }
chmod 0755 "$staging/server"
(cd "$staging" && sha256sum server > server.sha256)
if [[ -e "$release_dir" ]]; then
  [[ -x "$release_dir/server" && -s "$release_dir/server.sha256" ]] || { echo "Refusing to use incomplete release directory $release_dir." >&2; exit 1; }
  (cd "$release_dir" && sha256sum --check --status server.sha256) || { echo "Cached executable integrity check failed: $release_dir/server" >&2; exit 1; }
  staged_hash=$(awk '{print $1}' "$staging/server.sha256")
  cached_hash=$(awk '{print $1}' "$release_dir/server.sha256")
  [[ $staged_hash == "$cached_hash" ]] || { echo 'Cached executable differs from the binary inside the verified release archive.' >&2; exit 1; }
  rm -rf -- "$staging"
else
  mv -- "$staging" "$release_dir"
fi

image="ambatumove-core:$version"
docker build --platform linux/arm64 --file "$deploy_dir/Dockerfile.runtime" --tag "$image" "$release_dir"
current=$(cat "$state_root/current-version" 2>/dev/null || true)
export AMBATUMOVE_IMAGE="$image"
compose=(docker compose --project-name ambatumove --env-file "$config_file" --file "$deploy_dir/compose.yaml")
"${compose[@]}" config --quiet

# A verified online SQLite backup is a deployment gate. Keep every backup;
# retention is operator-controlled so a recovery point is never removed here.
db="$data_dir/ambatumove.sqlite"
old_schema=''
if [[ -n $current ]]; then
  [[ $current =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]] || { echo 'Recorded active release version is invalid.' >&2; exit 1; }
  [[ -s "$db" ]] || { echo "An active release is recorded but the persistent database is missing or empty at $db; refusing to initialize a replacement database." >&2; exit 1; }
  # The new binary provides schema-version/backup tools even when the
  # previously deployed version predates these operational subcommands.
  old_schema=$("${compose[@]}" run --rm --no-deps core-service schema-version)
fi
if [[ -s "$db" ]]; then
  mkdir -p "$data_dir/backups"
  chown 10001:10001 "$data_dir/backups"
  timestamp=$(date -u +%Y%m%dT%H%M%SZ)
  backup_name="pre-upgrade-${current:-untracked}-to-$version-$timestamp.sqlite"
  backup_host="$data_dir/backups/$backup_name"
  [[ ! -e "$backup_host" ]] || { echo "Refusing to overwrite recovery backup $backup_host" >&2; exit 1; }
  backup_container="/data/backups/$backup_name"
  if ! "${compose[@]}" run --rm --no-deps core-service backup --output "$backup_container"; then
    echo 'Pre-upgrade SQLite backup failed; deployment stopped before activation.' >&2
    exit 1
  fi
  verification=$("${compose[@]}" run --rm --no-deps core-service verify-backup --path "$backup_container")
  [[ $verification =~ schema\ version\ ([0-9]+) ]] || { echo 'SQLite backup verification did not report a schema version; deployment stopped.' >&2; exit 1; }
  backup_schema=${BASH_REMATCH[1]}
  (cd "$data_dir/backups" && sha256sum "$backup_name" > "$backup_name.sha256")
  manifest="$backup_host.manifest"
  {
    printf 'created_utc=%s\n' "$timestamp"
    printf 'previous_release=%s\n' "${current:-unknown}"
    printf 'target_release=%s\n' "$version"
    printf 'schema_version=%s\n' "$backup_schema"
    printf 'backup=%s\n' "$backup_host"
    printf 'checksum=%s\n' "$backup_host.sha256"
  } > "$manifest"
  echo "Verified pre-upgrade SQLite backup: $backup_host (schema $backup_schema)."
fi

rollback_after_failure() {
  status=$?
  trap - ERR
  if [[ -n $current ]]; then
    if [[ -s "$db" ]]; then
      export AMBATUMOVE_IMAGE="$image"
      new_schema=$("${compose[@]}" run --rm --no-deps core-service schema-version 2>/dev/null || true)
      if [[ ! $old_schema =~ ^[0-9]+$ || ! $new_schema =~ ^[0-9]+$ || $new_schema -gt $old_schema ]]; then
        printf '%s\n' "$version" > "$state_root/upgrade-failed-version"
        echo "Activation failed and database compatibility is uncertain (schema ${old_schema:-unknown} -> ${new_schema:-unknown}). The old binary was not started against the changed database. Preserve the verified backup and inspect the failed release; database restore requires the explicit restore-db.sh command." >&2
        exit "$status"
      fi
    fi
    echo "Activation failed without a schema advance; restoring previously active $current." >&2
    export AMBATUMOVE_IMAGE="ambatumove-core:$current"
    if "${compose[@]}" up -d --wait --wait-timeout 60; then
      echo 'Previous deployment is healthy again.' >&2
    else
      echo 'Automatic application rollback did not become healthy; inspect Docker Compose logs. Persistent data and verified backups remain in place.' >&2
    fi
  else
    echo 'First activation failed; no previous application exists. Persistent data and any verified backup were preserved.' >&2
  fi
  exit "$status"
}

prune_release_artifacts() {
  local root candidate name root_real target_real index=0
  local -A keep=()
  local -a versions=()
  keep["$version"]=1
  [[ -z $current ]] || keep["$current"]=1
  [[ -z ${previous:-} ]] || keep["$previous"]=1
  mapfile -t versions < <(
    for candidate in "$release_root"/v*; do
      [[ -d $candidate && ! -L $candidate ]] || continue
      name=${candidate##*/}
      [[ $name =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]] || continue
      printf '%s\n' "$name"
    done | sort -V -r
  )
  for name in "${versions[@]}"; do
    if (( index < 3 )); then keep["$name"]=1; fi
    ((index+=1))
  done
  for root in "$release_root" "$downloads_root"; do
    root_real=$(realpath -e "$root") || continue
    for candidate in "$root"/v*; do
      [[ -d $candidate && ! -L $candidate ]] || continue
      name=${candidate##*/}
      [[ $name =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]] || continue
      [[ ${keep[$name]:-} == 1 ]] && continue
      target_real=$(realpath -e "$candidate") || continue
      case "$target_real" in
        "$root_real"/*) ;;
        *) echo "Refusing to prune a release path outside $root_real: $target_real" >&2; continue ;;
      esac
      if ! rm -rf -- "$target_real"; then
        echo "Could not prune inactive release artifacts at $target_real; review available storage." >&2
      fi
    done
  done
}

export AMBATUMOVE_IMAGE="$image"
trap rollback_after_failure ERR
"${compose[@]}" up -d --wait --wait-timeout 60
port=$(awk -F= '$1=="GATEWAY_PORT" {value=$2} END {print value}' "$config_file")
port=${port:-18080}
ready=0
for _ in $(seq 1 30); do
  if curl --fail --silent --show-error "http://127.0.0.1:$port/ready" >/dev/null; then ready=1; break; fi
  sleep 2
done
[[ $ready == 1 ]] || { echo 'Gateway readiness check failed.' >&2; false; }

if [[ -n $current && $current != "$version" ]]; then printf '%s\n' "$current" > "$state_root/previous-version.tmp"; mv "$state_root/previous-version.tmp" "$state_root/previous-version"; fi
printf '%s\n' "$version" > "$state_root/current-version.tmp"
mv "$state_root/current-version.tmp" "$state_root/current-version"
rm -f -- "$state_root/upgrade-failed-version"
previous=$(cat "$state_root/previous-version" 2>/dev/null || true)
prune_release_artifacts || echo 'Release artifact retention could not complete; inspect deployment storage.' >&2
trap - ERR
echo "AmbatuMove $version is healthy at http://127.0.0.1:$port (image $image)."
