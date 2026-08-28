#!/usr/bin/env bash
set -Eeuo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
arbion_root_input="${ARBION_ROOT:-$script_dir/..}"

[[ -d "$arbion_root_input" ]] || {
  echo "Release root is unavailable: $arbion_root_input" >&2
  exit 1
}
command -v find >/dev/null || {
  echo "Required command not found: find" >&2
  exit 1
}

arbion_root="$(cd -- "$arbion_root_input" && pwd -P)"
metadata_entry="$(
  find "$arbion_root" -xdev \
    \( -path "$arbion_root/.git" -o -path "$arbion_root/.rollback" \) -prune -o \
    \( -name '._*' -o -name '__MACOSX' \) -print -quit
)"

if [[ -n "$metadata_entry" ]]; then
  relative_entry="${metadata_entry#"$arbion_root"/}"
  echo "Refusing deployment: release contains macOS metadata entry $relative_entry" >&2
  exit 1
fi

echo "Release hygiene check passed."
