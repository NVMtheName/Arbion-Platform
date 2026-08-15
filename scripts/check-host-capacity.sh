#!/usr/bin/env bash
set -Eeuo pipefail

capacity_path="${ARBION_CAPACITY_PATH:-/var/lib/docker}"
threshold_percent="${ARBION_CAPACITY_THRESHOLD_PERCENT:-85}"

[[ "$threshold_percent" =~ ^[0-9]+$ && "$threshold_percent" -ge 1 && "$threshold_percent" -le 100 ]] || {
  echo "ARBION_CAPACITY_THRESHOLD_PERCENT must be an integer from 1 through 100." >&2
  exit 1
}
[[ -d "$capacity_path" ]] || {
  echo "Capacity path does not exist or is not a directory: $capacity_path" >&2
  exit 1
}

disk_percent="$(df -P "$capacity_path" | awk 'NR == 2 {gsub(/%/, "", $5); print $5}')"
inode_percent="$(df -Pi "$capacity_path" | awk 'NR == 2 {gsub(/%/, "", $5); print $5}')"

[[ "$disk_percent" =~ ^[0-9]+$ && "$inode_percent" =~ ^[0-9]+$ ]] || {
  echo "Could not determine filesystem capacity for $capacity_path." >&2
  exit 1
}

if [[ "$disk_percent" -ge "$threshold_percent" || "$inode_percent" -ge "$threshold_percent" ]]; then
  printf 'Host capacity threshold reached for %s: disk=%s%%, inodes=%s%%, threshold=%s%%.\n' \
    "$capacity_path" "$disk_percent" "$inode_percent" "$threshold_percent" >&2
  exit 1
fi

printf 'Host capacity check passed for %s: disk=%s%%, inodes=%s%%, threshold=%s%%.\n' \
  "$capacity_path" "$disk_percent" "$inode_percent" "$threshold_percent"
