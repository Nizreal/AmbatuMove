#!/usr/bin/env bash
set -Eeuo pipefail
umask 077

usage() { echo "Usage: $0 VERSION [--publish]" >&2; exit 2; }
[[ $# -ge 1 && $# -le 2 ]] || usage
version=$1
publish=${2:-}
[[ $version =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]] || { echo "Invalid semantic version: $version" >&2; exit 2; }
[[ -z $publish || $publish == --publish ]] || usage
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
backend="$root/backend"
dist="$root/dist"
archive="ambatumove-core-$version-linux-arm64.tar.gz"
stage="$dist/$version-linux-arm64"
source_manifest() {
  local release_version=$1 files inputs_hash file file_hash
  files=$(find . -type f \( -name '*.go' ! -name '*_test.go' -o -name '*.sql' -o -name '*.json' -o -name 'go.mod' -o -name 'go.sum' \) | LC_ALL=C sort)
  inputs_hash=$(while IFS= read -r file; do
    [[ -n $file ]] || continue
    file_hash=$(sha256sum "$file" | awk '{print $1}')
    printf '%s  %s\n' "$file_hash" "${file#./}"
  done <<< "$files" | sha256sum | awk '{print $1}')
  printf 'version=%s\ngo=%s\ninputs_sha256=%s\n' "$release_version" "$("$go" version)" "$inputs_hash"
}
reuse_existing=0
has_output=0
[[ -e "$dist/$archive" || -e "$dist/$archive.sha256" || -e "$stage" ]] && has_output=1
if [[ $has_output == 1 ]]; then
  if [[ $publish != --publish || ! -s "$dist/$archive" || ! -s "$dist/$archive.sha256" || ! -s "$stage/server" || ! -s "$stage/release.manifest" ]]; then
    echo "Release output already exists or is incomplete for $version; artifacts are preserved. Use --publish only to verify and publish a complete existing build, or choose a new version." >&2
    exit 1
  fi
  reuse_existing=1
fi
go=$(command -v go || true)
if [[ -z $go && -x "$root/.tools/go/go/bin/go.exe" ]]; then go="$root/.tools/go/go/bin/go.exe"; fi
[[ -n $go ]] || { echo 'Go was not found on PATH or at .tools/go/go/bin/go.exe.' >&2; exit 1; }
command -v tar >/dev/null || { echo 'tar is required.' >&2; exit 1; }
command -v sha256sum >/dev/null || { echo 'sha256sum is required.' >&2; exit 1; }

if [[ $publish == --publish ]]; then
  gh=$(command -v gh || true)
  [[ -n $gh ]] || { echo 'GitHub CLI is required when --publish is requested.' >&2; exit 1; }
  gh auth status || { echo 'GitHub CLI is not authenticated; run gh auth login.' >&2; exit 1; }
fi

cd "$backend"
"$go" fmt ./... || { echo 'go fmt failed.' >&2; exit 1; }
"$go" vet ./... || { echo 'go vet failed.' >&2; exit 1; }
"$go" test ./... || { echo 'go test failed.' >&2; exit 1; }

mkdir -p "$dist"
if [[ $reuse_existing == 1 ]]; then
  verify_dir=$(mktemp -d "$dist/.${version}.verify.XXXXXX")
  trap 'rm -rf -- "$verify_dir"' EXIT
  CGO_ENABLED=0 GOOS=linux GOARCH=arm64 "$go" build -trimpath -ldflags "-s -w -X main.version=$version" -o "$verify_dir/server-compiled" ./cmd/server || { echo 'Linux ARM64 cross-build failed.' >&2; exit 1; }
  current_manifest=$(source_manifest "$version")
  existing_manifest=$(cat "$stage/release.manifest")
  [[ $existing_manifest == "$current_manifest" ]] || { echo 'Existing release was built from different Go sources or a different Go toolchain; choose a new version.' >&2; exit 1; }
  staged_hash=$(sha256sum "$stage/server" | awk '{print $1}')
  entries=$(tar -tzf "$dist/$archive") || { echo 'Cannot inspect the existing release archive.' >&2; exit 1; }
  [[ $entries == server ]] || { echo 'Existing release archive does not contain exactly the server executable.' >&2; exit 1; }
  (cd "$dist" && sha256sum --check "$archive.sha256") || { echo 'Existing release checksum did not verify.' >&2; exit 1; }
  tar -xOzf "$dist/$archive" server > "$verify_dir/server-archived" || { echo 'Could not extract the existing release binary for verification.' >&2; exit 1; }
  archived_hash=$(sha256sum "$verify_dir/server-archived" | awk '{print $1}')
  [[ $archived_hash == "$staged_hash" ]] || { echo 'Existing archive does not contain the staged binary.' >&2; exit 1; }
  echo "Verified existing release $dist/$archive"
else
  mkdir "$stage"
  CGO_ENABLED=0 GOOS=linux GOARCH=arm64 "$go" build -trimpath -ldflags "-s -w -X main.version=$version" -o "$stage/server" ./cmd/server || { echo 'Linux ARM64 cross-build failed.' >&2; exit 1; }
  source_manifest "$version" > "$stage/release.manifest"
  tar -czf "$dist/$archive" -C "$stage" server || { echo 'Release archive creation failed.' >&2; exit 1; }
  (cd "$dist" && sha256sum "$archive" > "$archive.sha256") || { echo 'SHA-256 generation failed.' >&2; exit 1; }
  entries=$(tar -tzf "$dist/$archive") || { echo 'Cannot inspect the generated release archive.' >&2; exit 1; }
  [[ $entries == server ]] || { echo 'Generated release archive does not contain exactly the server executable.' >&2; exit 1; }
  (cd "$dist" && sha256sum --check "$archive.sha256") || { echo 'Generated release checksum did not verify.' >&2; exit 1; }
  echo "Built $dist/$archive"
  cat "$dist/$archive.sha256"
fi

if [[ $publish == --publish ]]; then
  origin=$(git -C "$root" remote get-url origin 2>/dev/null || true)
  if [[ $origin =~ github\.com[:/]([^/]+/[^/]+)(\.git)?$ ]]; then
    repository=${BASH_REMATCH[1]%.git}
  else
    echo 'Could not derive OWNER/REPOSITORY from the GitHub origin.' >&2
    exit 1
  fi
  gh release create "$version" --repo "$repository" --verify-tag --title "AmbatuMove $version" --generate-notes "$dist/$archive" "$dist/$archive.sha256" || { echo 'GitHub Release publishing failed.' >&2; exit 1; }
fi
