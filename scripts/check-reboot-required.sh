#!/usr/bin/env bash
set -Eeuo pipefail

reboot_required_file="${ARBION_REBOOT_REQUIRED_FILE:-/var/run/reboot-required}"

[[ "$reboot_required_file" == /* ]] || {
  echo "ARBION_REBOOT_REQUIRED_FILE must be an absolute path." >&2
  exit 1
}

if [[ -e "$reboot_required_file" ]]; then
  echo "The production host requires a reboot after package updates." >&2
  exit 1
fi

echo "The production host does not require a reboot."
