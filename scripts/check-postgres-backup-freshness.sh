#!/usr/bin/env bash
set -Eeuo pipefail

status_file="${ARBION_BACKUP_STATUS_FILE:-/var/lib/arbion-backups/last-success}"
max_age_seconds="${ARBION_BACKUP_MAX_AGE_SECONDS:-129600}"

[[ "$max_age_seconds" =~ ^[0-9]+$ && "$max_age_seconds" -gt 0 ]] || {
  echo "ARBION_BACKUP_MAX_AGE_SECONDS must be a positive integer." >&2
  exit 1
}
[[ -r "$status_file" ]] || {
  echo "No successful PostgreSQL backup has been recorded." >&2
  exit 1
}

read -r completed_at backup_key extra <"$status_file" || true
[[ "$completed_at" =~ ^[0-9]+$ && -n "$backup_key" && -z "${extra:-}" ]] || {
  echo "The PostgreSQL backup status file is invalid." >&2
  exit 1
}

now="$(date -u +%s)"
age=$((now - completed_at))
[[ "$age" -ge 0 && "$age" -le "$max_age_seconds" ]] || {
  echo "The latest successful PostgreSQL backup is stale." >&2
  exit 1
}

echo "PostgreSQL backup freshness check passed."
