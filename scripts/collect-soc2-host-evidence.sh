#!/usr/bin/env bash
set -Eeuo pipefail

umask 077

if [[ "${EUID}" -ne 0 ]]; then
  echo "Host evidence collection must run as root so it can inspect control status without reading secret values." >&2
  exit 1
fi

for command in awk cat date docker find hostname jq mktemp stat systemctl uname unlink; do
  command -v "$command" >/dev/null || {
    echo "Required command not found: $command" >&2
    exit 1
  }
done

arbion_root="${ARBION_ROOT:-/opt/arbion}"
env_file="${ARBION_PRODUCTION_ENV_FILE:-$arbion_root/.env.production}"
backup_status_file="${ARBION_BACKUP_STATUS_FILE:-/var/lib/arbion-backups/last-success}"
backup_max_age_seconds="${ARBION_BACKUP_MAX_AGE_SECONDS:-129600}"

[[ -d "$arbion_root" && -r "$arbion_root/.release-sha" && -r "$env_file" ]] || {
  echo "The production release or root-owned environment file is unavailable." >&2
  exit 1
}
[[ "$backup_max_age_seconds" =~ ^[0-9]+$ && "$backup_max_age_seconds" -gt 0 ]] || {
  echo "ARBION_BACKUP_MAX_AGE_SECONDS must be a positive integer." >&2
  exit 1
}

temporary_root="$(mktemp -d "${TMPDIR:-/tmp}/arbion-host-evidence.XXXXXX")"
cleanup() {
  if [[ -d "$temporary_root" ]]; then
    find "$temporary_root" -depth -delete
  fi
}
trap cleanup EXIT

collected_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
collected_epoch="$(date -u +%s)"
release_sha="$(cat "$arbion_root/.release-sha")"
compose=(docker compose --project-directory "$arbion_root" --env-file "$env_file" -f "$arbion_root/docker-compose.prod.yml")

services=(proxy postgres redis api ai web)
: >"$temporary_root/service-records.jsonl"
for service in "${services[@]}"; do
  container_id="$("${compose[@]}" ps -q "$service" 2>/dev/null || true)"
  if [[ -z "$container_id" ]]; then
    jq -n --arg service "$service" '{service: $service, status: "UNAVAILABLE", health: "UNAVAILABLE"}' \
      >>"$temporary_root/service-records.jsonl"
    continue
  fi

  docker inspect "$container_id" | jq --arg service "$service" '.[0] | {
    service: $service,
    container_id: .Id,
    configured_image: .Config.Image,
    image_id: .Image,
    runtime_user: (if .Config.User == "" then "IMAGE_DEFAULT" else .Config.User end),
    status: .State.Status,
    health: (.State.Health.Status // "NOT_CONFIGURED"),
    started_at: .State.StartedAt,
    restart_count: .RestartCount,
    read_only_root: .HostConfig.ReadonlyRootfs,
    capability_drop: (.HostConfig.CapDrop // []),
    security_options: (.HostConfig.SecurityOpt // []),
    process_limit: .HostConfig.PidsLimit,
    published_ports: (.NetworkSettings.Ports // {})
  }' >>"$temporary_root/service-records.jsonl"
done
jq -s . "$temporary_root/service-records.jsonl" >"$temporary_root/services.json"

timers=(
  arbion-postgres-backup.timer
  arbion-postgres-backup-freshness.timer
  arbion-host-capacity.timer
  arbion-memory-pressure.timer
  arbion-production-containers.timer
  arbion-production-health.timer
  arbion-reboot-required.timer
  arbion-tls-certificate.timer
  arbion-docker-build-cache-prune.timer
)
: >"$temporary_root/timer-records.jsonl"
for timer in "${timers[@]}"; do
  load_state="$(systemctl show "$timer" --property=LoadState --value 2>/dev/null || true)"
  active_state="$(systemctl show "$timer" --property=ActiveState --value 2>/dev/null || true)"
  sub_state="$(systemctl show "$timer" --property=SubState --value 2>/dev/null || true)"
  unit_file_state="$(systemctl show "$timer" --property=UnitFileState --value 2>/dev/null || true)"
  next_run="$(systemctl show "$timer" --property=NextElapseUSecRealtime --value 2>/dev/null || true)"
  jq -n \
    --arg unit "$timer" \
    --arg load_state "${load_state:-UNAVAILABLE}" \
    --arg active_state "${active_state:-UNAVAILABLE}" \
    --arg sub_state "${sub_state:-UNAVAILABLE}" \
    --arg unit_file_state "${unit_file_state:-UNAVAILABLE}" \
    --arg next_run "${next_run:-UNAVAILABLE}" \
    '{unit: $unit, load_state: $load_state, active_state: $active_state, sub_state: $sub_state, unit_file_state: $unit_file_state, next_run: $next_run}' \
    >>"$temporary_root/timer-records.jsonl"
done
jq -s . "$temporary_root/timer-records.jsonl" >"$temporary_root/timers.json"

file_mode_record() {
  local label="$1"
  local path="$2"
  if [[ -e "$path" ]]; then
    jq -n \
      --arg label "$label" \
      --arg owner "$(stat -c '%U' "$path")" \
      --arg group "$(stat -c '%G' "$path")" \
      --arg mode "$(stat -c '%a' "$path")" \
      '{file: $label, status: "AVAILABLE", owner: $owner, group: $group, mode: $mode}'
  else
    jq -n --arg label "$label" '{file: $label, status: "UNAVAILABLE", owner: null, group: null, mode: null}'
  fi
}

{
  file_mode_record production-environment "$env_file"
  file_mode_record backup-environment /etc/arbion/postgres-backup.env
  file_mode_record alert-environment /etc/arbion/ops-alert.env
} | jq -s . >"$temporary_root/file-permissions.json"

backup_status="UNAVAILABLE"
backup_completed_epoch=""
backup_key=""
backup_extra=""
backup_age_seconds=""
if [[ -r "$backup_status_file" ]]; then
  read -r backup_completed_epoch backup_key backup_extra <"$backup_status_file" || true
  if [[ "$backup_completed_epoch" =~ ^[0-9]+$ && -n "$backup_key" && -z "${backup_extra:-}" ]]; then
    backup_age_seconds="$((collected_epoch - backup_completed_epoch))"
    if [[ "$backup_age_seconds" -ge 0 && "$backup_age_seconds" -le "$backup_max_age_seconds" ]]; then
      backup_status="CURRENT"
    else
      backup_status="STALE_OR_FUTURE"
    fi
  else
    backup_status="INVALID"
  fi
fi
jq -n \
  --arg status "$backup_status" \
  --arg completed_epoch "$backup_completed_epoch" \
  --arg object_key "$backup_key" \
  --arg age_seconds "$backup_age_seconds" \
  --argjson maximum_age_seconds "$backup_max_age_seconds" \
  '{status: $status, completed_epoch: (if $completed_epoch == "" then null else ($completed_epoch | tonumber) end), object_key: (if $object_key == "" then null else $object_key end), age_seconds: (if $age_seconds == "" then null else ($age_seconds | tonumber) end), maximum_age_seconds: $maximum_age_seconds}' \
  >"$temporary_root/backup.json"

run_check() {
  local name="$1"
  shift
  if "$@" >/dev/null 2>&1; then
    jq -n --arg check "$name" '{check: $check, result: "PASS"}'
  else
    jq -n --arg check "$name" '{check: $check, result: "FAIL"}'
  fi
}

{
  run_check public-smoke "$arbion_root/scripts/smoke-production.sh"
  run_check container-health "$arbion_root/scripts/check-production-containers.sh"
  run_check host-capacity "$arbion_root/scripts/check-host-capacity.sh"
  run_check memory-pressure "$arbion_root/scripts/check-memory-pressure.sh"
  run_check tls-certificate "$arbion_root/scripts/check-tls-certificate.sh"
  run_check backup-freshness "$arbion_root/scripts/check-postgres-backup-freshness.sh"
} | jq -s . >"$temporary_root/checks.json"

service_status="FAIL"
timer_status="FAIL"
check_status="FAIL"
jq -e 'all(.[]; .status == "running" and (.health == "healthy" or .health == "NOT_CONFIGURED"))' "$temporary_root/services.json" >/dev/null && service_status="PASS"
jq -e 'all(.[]; .load_state == "loaded" and .active_state == "active" and .unit_file_state == "enabled")' "$temporary_root/timers.json" >/dev/null && timer_status="PASS"
jq -e 'all(.[]; .result == "PASS")' "$temporary_root/checks.json" >/dev/null && check_status="PASS"

collection_status="INCOMPLETE"
if [[ "$release_sha" =~ ^[0-9a-f]{40}$ && "$service_status" == "PASS" && "$timer_status" == "PASS" && "$check_status" == "PASS" && "$backup_status" == "CURRENT" ]]; then
  collection_status="COMPLETE_REVIEW_REQUIRED"
fi

os_id="$(awk -F= '$1 == "ID" {gsub(/\"/, "", $2); print $2}' /etc/os-release 2>/dev/null || true)"
os_version="$(awk -F= '$1 == "VERSION_ID" {gsub(/\"/, "", $2); print $2}' /etc/os-release 2>/dev/null || true)"

jq -n \
  --arg schema_version "1.0" \
  --arg status "$collection_status" \
  --arg collected_at "$collected_at" \
  --arg host "$(hostname)" \
  --arg os_id "${os_id:-UNAVAILABLE}" \
  --arg os_version "${os_version:-UNAVAILABLE}" \
  --arg kernel "$(uname -r)" \
  --arg release_sha "$release_sha" \
  --arg service_status "$service_status" \
  --arg timer_status "$timer_status" \
  --arg check_status "$check_status" \
  --slurpfile services "$temporary_root/services.json" \
  --slurpfile timers "$temporary_root/timers.json" \
  --slurpfile permissions "$temporary_root/file-permissions.json" \
  --slurpfile backup "$temporary_root/backup.json" \
  --slurpfile checks "$temporary_root/checks.json" \
  '{schema_version: $schema_version, status: $status, collected_at: $collected_at, host: {name: $host, os: $os_id, os_version: $os_version, kernel: $kernel}, release_sha: $release_sha, summary: {services: $service_status, monitoring_timers: $timer_status, checks: $check_status, backup: $backup[0].status}, services: $services[0], monitoring_timers: $timers[0], sensitive_file_permissions: $permissions[0], backup: $backup[0], read_only_checks: $checks[0], limitations: ["Point-in-time read-only host snapshot", "Contains no environment values, credentials, customer records, application records, database content, or logs", "Requires reviewer evaluation and separate alert-delivery, restore, access, and patch evidence", "Does not by itself establish operating effectiveness or SOC 2 certification"]}'

[[ "$collection_status" == "COMPLETE_REVIEW_REQUIRED" ]] || exit 2
