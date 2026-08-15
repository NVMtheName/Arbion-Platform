#!/usr/bin/env bash
set -Eeuo pipefail

umask 077

if [[ "${EUID}" -ne 0 ]]; then
  echo "PostgreSQL backups must run as root." >&2
  exit 1
fi

for command in aws docker flock sha256sum; do
  command -v "$command" >/dev/null || {
    echo "Required command not found: $command" >&2
    exit 1
  }
done

arbion_root="${ARBION_ROOT:-/opt/arbion}"
env_file="${ARBION_PRODUCTION_ENV_FILE:-$arbion_root/.env.production}"
backup_bucket="${ARBION_BACKUP_BUCKET:?ARBION_BACKUP_BUCKET is required}"
backup_prefix="${ARBION_BACKUP_PREFIX:-postgres/daily}"
staging_dir="${ARBION_BACKUP_STAGING_DIR:-/var/lib/arbion-backups/staging}"
status_file="${ARBION_BACKUP_STATUS_FILE:-/var/lib/arbion-backups/last-success}"

[[ -f "$env_file" ]] || {
  echo "Missing production environment file: $env_file" >&2
  exit 1
}
[[ "$backup_bucket" =~ ^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$ ]] || {
  echo "ARBION_BACKUP_BUCKET is not a valid S3 bucket name." >&2
  exit 1
}
[[ "$backup_prefix" != /* && "$backup_prefix" != *..* ]] || {
  echo "ARBION_BACKUP_PREFIX must be a relative S3 prefix without '..'." >&2
  exit 1
}

install -d -o root -g root -m 0700 "$staging_dir"
install -d -o root -g root -m 0700 "$(dirname "$status_file")"
exec 9>"$staging_dir/.backup.lock"
flock -n 9 || {
  echo "Another PostgreSQL backup is already running." >&2
  exit 1
}

timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
filename="arbion-$timestamp.dump"
dump_path="$staging_dir/$filename"
checksum_path="$dump_path.sha256"
backup_key="$backup_prefix/$filename"
status_tmp=""

cleanup() {
  rm -f -- "$dump_path" "$checksum_path"
  [[ -z "$status_tmp" ]] || rm -f -- "$status_tmp"
}
trap cleanup EXIT

compose=(
  docker compose
  --project-directory "$arbion_root"
  --env-file "$env_file"
  -f "$arbion_root/docker-compose.prod.yml"
)

if ! "${compose[@]}" ps --status running --services | grep -qx postgres; then
  echo "The production PostgreSQL container is not running." >&2
  exit 1
fi

"${compose[@]}" exec -T postgres \
  sh -ceu 'exec pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc' >"$dump_path"

[[ -s "$dump_path" ]] || {
  echo "PostgreSQL produced an empty backup." >&2
  exit 1
}

"${compose[@]}" exec -T postgres pg_restore --list <"$dump_path" >/dev/null

(
  cd "$staging_dir"
  sha256sum "$filename" >"$filename.sha256"
)

aws s3 cp "$dump_path" "s3://$backup_bucket/$backup_key" \
  --sse AES256 \
  --only-show-errors
aws s3 cp "$checksum_path" "s3://$backup_bucket/$backup_key.sha256" \
  --sse AES256 \
  --only-show-errors

status_tmp="$status_file.tmp.$$"
printf '%s %s\n' "$(date -u +%s)" "$backup_key" >"$status_tmp"
chmod 0600 "$status_tmp"
mv -f -- "$status_tmp" "$status_file"

echo "Uploaded verified PostgreSQL backup to s3://$backup_bucket/$backup_key"
