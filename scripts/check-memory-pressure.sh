#!/usr/bin/env bash
set -Eeuo pipefail

threshold_percent="${ARBION_MEMORY_AVAILABLE_THRESHOLD_PERCENT:-10}"

[[ "$threshold_percent" =~ ^[0-9]+$ && "$threshold_percent" -ge 1 && "$threshold_percent" -le 100 ]] || {
  echo "ARBION_MEMORY_AVAILABLE_THRESHOLD_PERCENT must be an integer from 1 through 100." >&2
  exit 1
}

total_kib="$(awk '$1 == "MemTotal:" {print $2}' /proc/meminfo)"
available_kib="$(awk '$1 == "MemAvailable:" {print $2}' /proc/meminfo)"
swap_total_kib="$(awk '$1 == "SwapTotal:" {print $2}' /proc/meminfo)"
swap_free_kib="$(awk '$1 == "SwapFree:" {print $2}' /proc/meminfo)"

[[ "$total_kib" =~ ^[0-9]+$ && "$total_kib" -gt 0 && "$available_kib" =~ ^[0-9]+$ ]] || {
  echo "Could not determine available host memory from /proc/meminfo." >&2
  exit 1
}
[[ "$swap_total_kib" =~ ^[0-9]+$ && "$swap_free_kib" =~ ^[0-9]+$ ]] || {
  echo "Could not determine host swap usage from /proc/meminfo." >&2
  exit 1
}

available_percent="$((available_kib * 100 / total_kib))"
available_mib="$((available_kib / 1024))"
total_mib="$((total_kib / 1024))"
swap_used_mib="$(((swap_total_kib - swap_free_kib) / 1024))"
swap_total_mib="$((swap_total_kib / 1024))"

if [[ "$available_percent" -lt "$threshold_percent" ]]; then
  printf 'Host memory pressure threshold reached: available=%s MiB/%s MiB (%s%%), threshold=%s%%, swap-used=%s MiB/%s MiB.\n' \
    "$available_mib" "$total_mib" "$available_percent" "$threshold_percent" "$swap_used_mib" "$swap_total_mib" >&2
  exit 1
fi

printf 'Host memory pressure check passed: available=%s MiB/%s MiB (%s%%), threshold=%s%%, swap-used=%s MiB/%s MiB.\n' \
  "$available_mib" "$total_mib" "$available_percent" "$threshold_percent" "$swap_used_mib" "$swap_total_mib"
