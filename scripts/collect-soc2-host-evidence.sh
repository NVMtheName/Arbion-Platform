#!/usr/bin/env bash
set -Eeuo pipefail

umask 077

if [[ "${EUID}" -ne 0 ]]; then
  echo "Host evidence collection must run as root so it can inspect control status without reading secret values." >&2
  exit 1
fi

for command in awk cat date docker find hostname jq mktemp openssl stat systemctl tr uname unlink; do
  command -v "$command" >/dev/null || {
    echo "Required command not found: $command" >&2
    exit 1
  }
done

arbion_root="${ARBION_ROOT:-/opt/arbion}"
env_file="${ARBION_PRODUCTION_ENV_FILE:-$arbion_root/.env.production}"
backup_status_file="${ARBION_BACKUP_STATUS_FILE:-/var/lib/arbion-backups/last-success}"
backup_max_age_seconds="${ARBION_BACKUP_MAX_AGE_SECONDS:-129600}"

[[ -d "$arbion_root" && -f "$arbion_root/.release-sha" && ! -L "$arbion_root/.release-sha" && -r "$arbion_root/.release-sha" && -r "$env_file" ]] || {
  echo "The production release or root-owned environment file is unavailable." >&2
  exit 1
}
[[ "$backup_max_age_seconds" =~ ^[0-9]+$ && "$backup_max_age_seconds" -gt 0 && "$backup_max_age_seconds" -le 31536000 ]] || {
  echo "ARBION_BACKUP_MAX_AGE_SECONDS must be between 1 and 31536000." >&2
  exit 1
}

temporary_root="$(mktemp -d "${TMPDIR:-/tmp}/arbion-host-evidence.XXXXXX")"
cleanup() {
  if [[ -d "$temporary_root" ]]; then
    find "$temporary_root" -depth -delete
  fi
}
trap cleanup EXIT

collected_epoch="$(date -u +%s)"
collected_at="$(date -u -d "@$collected_epoch" +%Y-%m-%dT%H:%M:%SZ)"
collection_id="arbion-soc2-host-${collected_at//[-:]/}"
release_sha="INVALID"
release_marker_bytes="$(stat -c '%s' "$arbion_root/.release-sha" 2>/dev/null || true)"
if [[ "$release_marker_bytes" =~ ^[0-9]+$ && "$release_marker_bytes" -le 128 ]]; then
  release_candidate="$(tr -d '\r\n' <"$arbion_root/.release-sha")"
  [[ "$release_candidate" =~ ^[0-9a-f]{40}$ ]] && release_sha="$release_candidate"
fi
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
  next_run_clock="REALTIME"
  if [[ -z "$next_run" ]]; then
    next_run="$(systemctl show "$timer" --property=NextElapseUSecMonotonic --value 2>/dev/null || true)"
    next_run_clock="MONOTONIC"
  fi
  if [[ -z "$next_run" || "$next_run" == "infinity" || "$next_run" == "0" ]]; then
    next_run="UNAVAILABLE"
    next_run_clock="UNAVAILABLE"
  fi
  jq -n \
    --arg unit "$timer" \
    --arg load_state "${load_state:-UNAVAILABLE}" \
    --arg active_state "${active_state:-UNAVAILABLE}" \
    --arg sub_state "${sub_state:-UNAVAILABLE}" \
    --arg unit_file_state "${unit_file_state:-UNAVAILABLE}" \
    --arg next_run "${next_run:-UNAVAILABLE}" \
    --arg next_run_clock "$next_run_clock" \
    '{unit: $unit, load_state: $load_state, active_state: $active_state, sub_state: $sub_state, unit_file_state: $unit_file_state, next_run: $next_run, next_run_clock: $next_run_clock}' \
    >>"$temporary_root/timer-records.jsonl"
done
jq -s . "$temporary_root/timer-records.jsonl" >"$temporary_root/timers.json"

file_mode_record() {
  local label="$1"
  local path="$2"
  if [[ -f "$path" && ! -L "$path" ]]; then
    jq -n \
      --arg label "$label" \
      --arg owner "$(stat -c '%U' "$path")" \
      --arg group "$(stat -c '%G' "$path")" \
      --arg mode "$(stat -c '%a' "$path")" \
      '{file: $label, status: "AVAILABLE", owner: $owner, group: $group, mode: $mode}'
  elif [[ -e "$path" || -L "$path" ]]; then
    jq -n --arg label "$label" '{file: $label, status: "UNSAFE", owner: null, group: null, mode: null}'
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
if [[ -e "$backup_status_file" || -L "$backup_status_file" ]]; then
  if [[ ! -f "$backup_status_file" || -L "$backup_status_file" || ! -r "$backup_status_file" ]]; then
    backup_status="INVALID"
  else
    backup_marker_bytes="$(stat -c '%s' "$backup_status_file" 2>/dev/null || true)"
    backup_marker_lines="$(awk 'END {print NR}' "$backup_status_file" 2>/dev/null || true)"
    if [[ "$backup_marker_bytes" =~ ^[0-9]+$ && "$backup_marker_bytes" -gt 0 && "$backup_marker_bytes" -le 1024 && "$backup_marker_lines" == "1" ]]; then
      read -r backup_completed_epoch backup_key backup_extra <"$backup_status_file" || true
      if [[ "$backup_completed_epoch" =~ ^[0-9]{1,12}$ && "$backup_key" =~ ^[A-Za-z0-9][A-Za-z0-9._/-]{0,511}$ && -z "${backup_extra:-}" ]]; then
        backup_age_seconds="$((collected_epoch - backup_completed_epoch))"
        if [[ "$backup_age_seconds" -ge 0 && "$backup_age_seconds" -le "$backup_max_age_seconds" ]]; then
          backup_status="CURRENT"
        else
          backup_status="STALE_OR_FUTURE"
        fi
      else
        backup_status="INVALID"
      fi
    else
      backup_status="INVALID"
    fi
  fi
  if [[ "$backup_status" == "INVALID" ]]; then
    backup_completed_epoch=""
    backup_key=""
    backup_age_seconds=""
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
hardening_status="FAIL"
network_exposure_status="FAIL"
timer_status="FAIL"
check_status="FAIL"
permission_status="FAIL"
jq -e 'all(.[]; .status == "running" and (.health == "healthy" or .health == "NOT_CONFIGURED"))' "$temporary_root/services.json" >/dev/null && service_status="PASS"
jq -e '
  all(.[] | select(.service == "api" or .service == "ai" or .service == "web");
    .status == "running" and .read_only_root == true and .runtime_user != "IMAGE_DEFAULT" and
    .process_limit == 256 and ((.capability_drop // []) | index("ALL")) != null and
    ((.security_options // []) | index("no-new-privileges:true")) != null and
    all((.published_ports // {}) | to_entries[]?; .value == null)
  )
' "$temporary_root/services.json" >/dev/null 2>&1 && hardening_status="PASS"
jq -e '
  (.[] | select(.service == "proxy") | .published_ports) as $proxy_ports |
  all(["80/tcp", "443/tcp", "443/udp"][]; (($proxy_ports // {})[.] | type == "array" and length > 0)) and
  all(($proxy_ports // {}) | to_entries[]; ((.key | IN("80/tcp", "443/tcp", "443/udp")) or .value == null)) and
  all(.[] | select(.service != "proxy"); has("published_ports") and all(.published_ports | to_entries[]?; .value == null))
' "$temporary_root/services.json" >/dev/null 2>&1 && network_exposure_status="PASS"
jq -e 'all(.[]; .load_state == "loaded" and .active_state == "active" and .unit_file_state == "enabled" and .next_run != "UNAVAILABLE")' "$temporary_root/timers.json" >/dev/null && timer_status="PASS"
jq -e 'all(.[]; .result == "PASS")' "$temporary_root/checks.json" >/dev/null && check_status="PASS"
jq -e 'all(.[]; .status == "AVAILABLE" and .owner == "root" and .group == "root" and .mode == "600")' "$temporary_root/file-permissions.json" >/dev/null && permission_status="PASS"

collection_status="INCOMPLETE"
if [[ "$release_sha" =~ ^[0-9a-f]{40}$ && "$service_status" == "PASS" && "$hardening_status" == "PASS" && "$network_exposure_status" == "PASS" && "$timer_status" == "PASS" && "$permission_status" == "PASS" && "$check_status" == "PASS" && "$backup_status" == "CURRENT" ]]; then
  collection_status="COMPLETE_REVIEW_REQUIRED"
fi

os_id="$(awk -F= '$1 == "ID" {gsub(/\"/, "", $2); print $2}' /etc/os-release 2>/dev/null || true)"
os_version="$(awk -F= '$1 == "VERSION_ID" {gsub(/\"/, "", $2); print $2}' /etc/os-release 2>/dev/null || true)"

jq -n \
  --arg schema_version "1.2" \
  --arg collection_id "$collection_id" \
  --arg status "$collection_status" \
  --arg collected_at "$collected_at" \
  --arg host "$(hostname)" \
  --arg os_id "${os_id:-UNAVAILABLE}" \
  --arg os_version "${os_version:-UNAVAILABLE}" \
  --arg kernel "$(uname -r)" \
  --arg release_sha "$release_sha" \
  --arg service_status "$service_status" \
  --arg hardening_status "$hardening_status" \
  --arg network_exposure_status "$network_exposure_status" \
  --arg timer_status "$timer_status" \
  --arg permission_status "$permission_status" \
  --arg check_status "$check_status" \
  --slurpfile services "$temporary_root/services.json" \
  --slurpfile timers "$temporary_root/timers.json" \
  --slurpfile permissions "$temporary_root/file-permissions.json" \
  --slurpfile backup "$temporary_root/backup.json" \
  --slurpfile checks "$temporary_root/checks.json" \
  '{schema_version: $schema_version, collection_id: $collection_id, status: $status, collected_at: $collected_at, host: {name: $host, os: $os_id, os_version: $os_version, kernel: $kernel}, release_sha: $release_sha, summary: {services: $service_status, application_hardening: $hardening_status, network_exposure: $network_exposure_status, monitoring_timers: $timer_status, sensitive_file_permissions: $permission_status, checks: $check_status, backup: $backup[0].status}, services: $services[0], monitoring_timers: $timers[0], sensitive_file_permissions: $permissions[0], backup: $backup[0], read_only_checks: $checks[0], limitations: ["Point-in-time read-only host snapshot", "Contains no environment values, credentials, customer records, application records, database content, or logs", "Requires reviewer evaluation and separate alert-delivery, restore, access, and patch evidence", "Does not by itself establish operating effectiveness or SOC 2 certification"]}' \
  >"$temporary_root/payload.json"

jq -S -c . "$temporary_root/payload.json" >"$temporary_root/canonical-payload.json"
payload_digest="$(openssl dgst -sha256 "$temporary_root/canonical-payload.json" | awk '{print $NF}')"
[[ "$payload_digest" =~ ^[0-9a-f]{64}$ ]] || {
  echo "Could not calculate the host evidence payload digest." >&2
  exit 1
}
jq -S \
  --arg algorithm "SHA-256" \
  --arg canonicalization "JQ_SORTED_COMPACT_UTF8_V1" \
  --arg covers "ENTIRE_DOCUMENT_EXCLUDING_INTEGRITY" \
  --arg payload_sha256 "$payload_digest" \
  '. + {integrity: {algorithm: $algorithm, canonicalization: $canonicalization, covers: $covers, payload_sha256: $payload_sha256}}' \
  "$temporary_root/payload.json"

[[ "$collection_status" == "COMPLETE_REVIEW_REQUIRED" ]] || exit 2
