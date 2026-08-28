#!/usr/bin/env bash
set -Eeuo pipefail

repo_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)"
test_root="$(mktemp -d "${TMPDIR:-/tmp}/arbion-release-hygiene-test.XXXXXX")"
cleanup() {
  if [[ -d "$test_root" ]]; then
    find "$test_root" -depth -delete
  fi
}
trap cleanup EXIT

clean_root="$test_root/clean"
mkdir -p "$clean_root/services/api/migrations" "$clean_root/.rollback"
: >"$clean_root/README.md"
: >"$clean_root/.rollback/._ignored-rollback-metadata"
ARBION_ROOT="$clean_root" "$repo_root/scripts/check-release-hygiene.sh" >/dev/null

: >"$clean_root/services/api/migrations/._00001_initial.sql"
if ARBION_ROOT="$clean_root" "$repo_root/scripts/check-release-hygiene.sh" >"$test_root/check-output" 2>&1; then
  echo "Release hygiene check accepted an AppleDouble file." >&2
  exit 1
fi
grep -q "Refusing deployment: release contains macOS metadata entry" "$test_root/check-output"
unlink "$clean_root/services/api/migrations/._00001_initial.sql"

mkdir "$clean_root/__MACOSX"
if ARBION_ROOT="$clean_root" "$repo_root/scripts/check-release-hygiene.sh" >/dev/null 2>&1; then
  echo "Release hygiene check accepted a __MACOSX directory." >&2
  exit 1
fi
find "$clean_root/__MACOSX" -depth -delete

deploy_root="$test_root/deploy-root"
stub_bin="$test_root/stub-bin"
command_log="$test_root/command-log"
mkdir -p "$deploy_root/scripts" "$stub_bin"
cp "$repo_root/scripts/deploy-production.sh" "$deploy_root/scripts/deploy-production.sh"
cp "$repo_root/scripts/check-release-hygiene.sh" "$deploy_root/scripts/check-release-hygiene.sh"
chmod 0755 "$deploy_root/scripts/deploy-production.sh" "$deploy_root/scripts/check-release-hygiene.sh"
: >"$deploy_root/.env.production"
: >"$deploy_root/._contaminated-release"
printf '%s\n' '#!/usr/bin/env bash' 'printf "called\\n" >>"$ARBION_TEST_COMMAND_LOG"' >"$stub_bin/docker"
printf '%s\n' '#!/usr/bin/env bash' 'printf "called\\n" >>"$ARBION_TEST_COMMAND_LOG"' >"$stub_bin/curl"
chmod 0755 "$stub_bin/docker" "$stub_bin/curl"
if PATH="$stub_bin:$PATH" ARBION_TEST_COMMAND_LOG="$command_log" \
  ARBION_PRODUCTION_ENV_FILE="$deploy_root/.env.production" \
  "$deploy_root/scripts/deploy-production.sh" >"$test_root/deploy-output" 2>&1; then
  echo "Production deploy accepted a contaminated release." >&2
  exit 1
fi
grep -q "Refusing deployment: release contains macOS metadata entry" "$test_root/deploy-output"
[[ ! -e "$command_log" ]] || {
  echo "Production deploy invoked Compose or curl before release hygiene passed." >&2
  exit 1
}

release_archive="$test_root/release.tar.gz"
"$repo_root/scripts/package-release.sh" HEAD "$release_archive" >/dev/null
expected_commit="$(git -C "$repo_root" rev-parse HEAD)"
archive_marker="$(COPYFILE_DISABLE=1 tar -xOf "$release_archive" ./.release-sha)"
[[ "$archive_marker" == "$expected_commit" ]] || {
  echo "Packaged release marker is incorrect." >&2
  exit 1
}
metadata_entry="$(
  COPYFILE_DISABLE=1 tar -tzf "$release_archive" |
    LC_ALL=C awk '/(^|\/)(\._[^\/]*|__MACOSX)(\/|$)/ && !found { print; found=1 }'
)"
[[ -z "$metadata_entry" ]] || {
  echo "Packaged release contains macOS metadata: $metadata_entry" >&2
  exit 1
}

echo "Release hygiene tests passed."
