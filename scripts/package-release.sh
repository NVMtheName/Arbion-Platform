#!/usr/bin/env bash
set -Eeuo pipefail

if [[ "$#" -ne 2 ]]; then
  echo "Usage: $0 <git-ref> <output.tar.gz>" >&2
  exit 2
fi

git_ref="$1"
output_archive="$2"
[[ "$git_ref" != -* ]] || {
  echo "Git ref must not begin with '-'." >&2
  exit 2
}
[[ "$output_archive" == *.tar.gz ]] || {
  echo "Release output must end in .tar.gz." >&2
  exit 2
}

for command in git tar find mktemp dirname mv unlink awk; do
  command -v "$command" >/dev/null || {
    echo "Required command not found: $command" >&2
    exit 1
  }
done

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
repo_root="$(git -C "$script_dir/.." rev-parse --show-toplevel)"
commit="$(git -C "$repo_root" rev-parse --verify "$git_ref^{commit}")"
case "$output_archive" in
  /*) ;;
  *) output_archive="$PWD/$output_archive" ;;
esac
output_parent="$(dirname -- "$output_archive")"
[[ -d "$output_parent" ]] || {
  echo "Release output directory is unavailable: $output_parent" >&2
  exit 1
}
[[ ! -e "$output_archive" ]] || {
  echo "Refusing to overwrite existing release archive: $output_archive" >&2
  exit 1
}

stage_dir="$(mktemp -d "${TMPDIR:-/tmp}/arbion-release-stage.XXXXXX")"
temporary_archive="$(mktemp "$output_parent/.arbion-release.XXXXXX")"
cleanup() {
  if [[ -n "${stage_dir:-}" && -d "$stage_dir" ]]; then
    find "$stage_dir" -depth -delete
  fi
  if [[ -n "${temporary_archive:-}" && -f "$temporary_archive" ]]; then
    unlink "$temporary_archive"
  fi
}
trap cleanup EXIT

git -C "$repo_root" archive --format=tar "$commit" | tar -xf - -C "$stage_dir"
printf '%s\n' "$commit" >"$stage_dir/.release-sha"

# COPYFILE_DISABLE prevents macOS tar from serializing extended attributes as
# AppleDouble ._ files. The archive is then inspected before it can be moved to
# its requested destination.
COPYFILE_DISABLE=1 tar -czf "$temporary_archive" -C "$stage_dir" .
metadata_entry="$(
  COPYFILE_DISABLE=1 tar -tzf "$temporary_archive" |
    LC_ALL=C awk '/(^|\/)(\._[^\/]*|__MACOSX)(\/|$)/ && !found { print; found=1 }'
)"
[[ -z "$metadata_entry" ]] || {
  echo "Refusing contaminated release archive: $metadata_entry" >&2
  exit 1
}
archive_marker="$(COPYFILE_DISABLE=1 tar -xOf "$temporary_archive" ./.release-sha)"
[[ "$archive_marker" == "$commit" ]] || {
  echo "Release archive marker does not match the resolved commit." >&2
  exit 1
}

mv -- "$temporary_archive" "$output_archive"
temporary_archive=""
echo "Packaged release $commit at $output_archive"
