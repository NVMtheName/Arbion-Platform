#!/usr/bin/env bash
set -Eeuo pipefail

export LC_ALL=C

repo_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)"
verifier="$repo_root/scripts/verify-soc2-host-evidence.sh"
collector="$repo_root/scripts/collect-soc2-host-evidence.sh"
test_root="$(mktemp -d "${TMPDIR:-/tmp}/arbion-soc2-host-test.XXXXXX")"

cleanup() {
  if [[ -d "$test_root" ]]; then
    find "$test_root" -depth -delete
  fi
}
trap cleanup EXIT

fail() {
  echo "SOC 2 host evidence test failed: $*" >&2
  exit 1
}

grep -Fq -- '--arg schema_version "1.2"' "$collector" || fail "collector does not emit the reviewed schema version"
grep -Fq 'release_sha="INVALID"' "$collector" || fail "collector does not sanitize an invalid release marker"
grep -Fq 'application_hardening: $hardening_status' "$collector" || fail "collector omits application-hardening status"
grep -Fq 'network_exposure: $network_exposure_status' "$collector" || fail "collector omits network-exposure status"
grep -Fq 'sensitive_file_permissions: $permission_status' "$collector" || fail "collector omits permission status"
grep -Fq 'status: "UNSAFE"' "$collector" || fail "collector does not classify unsafe sensitive-file paths"
grep -Fq 'ENTIRE_DOCUMENT_EXCLUDING_INTEGRITY' "$collector" || fail "collector omits the canonical payload-integrity contract"

seal_host_evidence() {
  local evidence="$1"
  local payload="$test_root/payload.json"
  local canonical="$test_root/canonical.json"
  local output="$test_root/sealed.json"
  local digest
  jq 'del(.integrity)' "$evidence" >"$payload"
  jq -S -c . "$payload" >"$canonical"
  digest="$(openssl dgst -sha256 "$canonical" | awk '{print $NF}')"
  jq -S --arg digest "$digest" '. + {integrity: {
    algorithm: "SHA-256",
    canonicalization: "JQ_SORTED_COMPACT_UTF8_V1",
    covers: "ENTIRE_DOCUMENT_EXCLUDING_INTEGRITY",
    payload_sha256: $digest
  }}' "$payload" >"$output"
  mv -- "$output" "$evidence"
}

make_complete_evidence() {
  local evidence="$1"
  jq -n '
    def service($name; $index): {
      service: $name,
      container_id: ([range(0; 64) | ($index | tostring)] | join("")),
      configured_image: ("arbion-" + $name + ":verified"),
      image_id: ("sha256:" + (([range(0; 64) | (($index + 6) | tostring)] | join(""))[0:64])),
      runtime_user: (if ($name == "api") then "65532:65532" elif ($name == "ai") then "arbion" elif ($name == "web") then "nextjs" else "IMAGE_DEFAULT" end),
      status: "running",
      health: (if ($name == "proxy") then "NOT_CONFIGURED" else "healthy" end),
      started_at: "2026-09-09T15:30:00.123456789Z",
      restart_count: 0,
      read_only_root: ($name == "api" or $name == "ai" or $name == "web"),
      capability_drop: (if ($name == "api" or $name == "ai" or $name == "web") then ["ALL"] else [] end),
      security_options: (if ($name == "api" or $name == "ai" or $name == "web") then ["no-new-privileges:true"] else [] end),
      process_limit: (if ($name == "api" or $name == "ai" or $name == "web") then 256 else null end),
      published_ports: (if ($name == "proxy") then {
        "80/tcp": [{HostIp: "0.0.0.0", HostPort: "80"}],
        "443/tcp": [{HostIp: "0.0.0.0", HostPort: "443"}],
        "443/udp": [{HostIp: "0.0.0.0", HostPort: "443"}]
      } else {} end)
    };
    def timer($unit): {
      unit: $unit,
      load_state: "loaded",
      active_state: "active",
      sub_state: "waiting",
      unit_file_state: "enabled",
      next_run: "Wed 2026-09-09 16:05:00 UTC"
    };
    {
      schema_version: "1.1",
      collection_id: "arbion-soc2-host-20260909T160000Z",
      status: "COMPLETE_REVIEW_REQUIRED",
      collected_at: "2026-09-09T16:00:00Z",
      host: {name: "arbion-production", os: "ubuntu", os_version: "24.04", kernel: "6.8.0-verified"},
      release_sha: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
      summary: {
        services: "PASS",
        application_hardening: "PASS",
        network_exposure: "PASS",
        monitoring_timers: "PASS",
        sensitive_file_permissions: "PASS",
        checks: "PASS",
        backup: "CURRENT"
      },
      services: [
        service("proxy"; 1), service("postgres"; 2), service("redis"; 3),
        service("api"; 4), service("ai"; 5), service("web"; 6)
      ],
      monitoring_timers: [
        timer("arbion-postgres-backup.timer"),
        timer("arbion-postgres-backup-freshness.timer"),
        timer("arbion-host-capacity.timer"),
        timer("arbion-memory-pressure.timer"),
        timer("arbion-production-containers.timer"),
        timer("arbion-production-health.timer"),
        timer("arbion-reboot-required.timer"),
        timer("arbion-tls-certificate.timer"),
        timer("arbion-docker-build-cache-prune.timer")
      ],
      sensitive_file_permissions: [
        {file: "production-environment", status: "AVAILABLE", owner: "root", group: "root", mode: "600"},
        {file: "backup-environment", status: "AVAILABLE", owner: "root", group: "root", mode: "600"},
        {file: "alert-environment", status: "AVAILABLE", owner: "root", group: "root", mode: "600"}
      ],
      backup: {
        status: "CURRENT",
        completed_epoch: 1788966000,
        object_key: "postgres/daily/arbion-verified.dump",
        age_seconds: 3600,
        maximum_age_seconds: 129600
      },
      read_only_checks: [
        {check: "public-smoke", result: "PASS"},
        {check: "container-health", result: "PASS"},
        {check: "host-capacity", result: "PASS"},
        {check: "memory-pressure", result: "PASS"},
        {check: "tls-certificate", result: "PASS"},
        {check: "backup-freshness", result: "PASS"}
      ],
      limitations: [
        "Point-in-time read-only host snapshot",
        "Contains no environment values, credentials, customer records, application records, database content, or logs",
        "Requires reviewer evaluation and separate alert-delivery, restore, access, and patch evidence",
        "Does not by itself establish operating effectiveness or SOC 2 certification"
      ]
    }
  ' >"$evidence"
  seal_host_evidence "$evidence"
}

expect_failure() {
  local name="$1"
  local expected="$2"
  local mutate="$3"
  local directory="$test_root/$name"
  local evidence="$directory/host.json"
  mkdir -p -- "$directory"
  cp -- "$complete_evidence" "$evidence"
  "$mutate" "$evidence"
  if "$verifier" "$evidence" >"$directory/output" 2>&1; then
    fail "$name mutation verified"
  fi
  grep -Fq "$expected" "$directory/output" || fail "$name did not fail with the expected safe classification"
}

tamper_payload() {
  jq '.release_sha = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"' "$1" >"$1.tmp"
  mv -- "$1.tmp" "$1"
}

omit_backup() {
  jq 'del(.backup)' "$1" >"$1.tmp"
  mv -- "$1.tmp" "$1"
  seal_host_evidence "$1"
}

add_unexpected_field() {
  jq '.unexpected = true' "$1" >"$1.tmp"
  mv -- "$1.tmp" "$1"
  seal_host_evidence "$1"
}

duplicate_service() {
  jq '.services[5] = .services[0]' "$1" >"$1.tmp"
  mv -- "$1.tmp" "$1"
  seal_host_evidence "$1"
}

duplicate_container() {
  jq '.services[4].container_id = .services[3].container_id' "$1" >"$1.tmp"
  mv -- "$1.tmp" "$1"
  seal_host_evidence "$1"
}

add_secret_key() {
  jq '.host.client_secret = "redacted-test-value"' "$1" >"$1.tmp"
  mv -- "$1.tmp" "$1"
  seal_host_evidence "$1"
}

add_secret_value() {
  jq '.host.note = "Bearer abcdefghijklmnopqrstuvwxyz012345"' "$1" >"$1.tmp"
  mv -- "$1.tmp" "$1"
  seal_host_evidence "$1"
}

change_collection_identity() {
  jq '.collection_id = "arbion-soc2-host-20990101T000000Z"' "$1" >"$1.tmp"
  mv -- "$1.tmp" "$1"
  seal_host_evidence "$1"
}

future_collection() {
  jq '.collection_id = "arbion-soc2-host-20990101T000000Z" | .collected_at = "2099-01-01T00:00:00Z"' "$1" >"$1.tmp"
  mv -- "$1.tmp" "$1"
  seal_host_evidence "$1"
}

future_service() {
  jq '.services[0].started_at = "2099-01-01T00:00:00Z"' "$1" >"$1.tmp"
  mv -- "$1.tmp" "$1"
  seal_host_evidence "$1"
}

break_service_summary() {
  jq '.summary.services = "FAIL"' "$1" >"$1.tmp"
  mv -- "$1.tmp" "$1"
  seal_host_evidence "$1"
}

break_application_hardening() {
  jq '.services[] |= if .service == "api" then .read_only_root = false else . end' "$1" >"$1.tmp"
  mv -- "$1.tmp" "$1"
  seal_host_evidence "$1"
}

break_network_exposure() {
  jq '.services[] |= if .service == "api" then .published_ports = {"8080/tcp": [{"HostIp": "0.0.0.0", "HostPort": "8080"}]} else . end' "$1" >"$1.tmp"
  mv -- "$1.tmp" "$1"
  seal_host_evidence "$1"
}

omit_proxy_port() {
  jq '.services[] |= if .service == "proxy" then del(.published_ports["443/udp"]) else . end' "$1" >"$1.tmp"
  mv -- "$1.tmp" "$1"
  seal_host_evidence "$1"
}

duplicate_timer() {
  jq '.monitoring_timers[8] = .monitoring_timers[0]' "$1" >"$1.tmp"
  mv -- "$1.tmp" "$1"
  seal_host_evidence "$1"
}

break_timer_summary() {
  jq '.monitoring_timers[0].active_state = "inactive"' "$1" >"$1.tmp"
  mv -- "$1.tmp" "$1"
  seal_host_evidence "$1"
}

remove_timer_next_run() {
  jq '.monitoring_timers[0].next_run = "UNAVAILABLE"' "$1" >"$1.tmp"
  mv -- "$1.tmp" "$1"
  seal_host_evidence "$1"
}

break_permission_summary() {
  jq '.sensitive_file_permissions[0].mode = "640"' "$1" >"$1.tmp"
  mv -- "$1.tmp" "$1"
  seal_host_evidence "$1"
}

duplicate_permission() {
  jq '.sensitive_file_permissions[2] = .sensitive_file_permissions[0]' "$1" >"$1.tmp"
  mv -- "$1.tmp" "$1"
  seal_host_evidence "$1"
}

break_check_summary() {
  jq '.read_only_checks[0].result = "FAIL"' "$1" >"$1.tmp"
  mv -- "$1.tmp" "$1"
  seal_host_evidence "$1"
}

duplicate_check() {
  jq '.read_only_checks[5] = .read_only_checks[0]' "$1" >"$1.tmp"
  mv -- "$1.tmp" "$1"
  seal_host_evidence "$1"
}

break_backup_age() {
  jq '.backup.age_seconds = 3601' "$1" >"$1.tmp"
  mv -- "$1.tmp" "$1"
  seal_host_evidence "$1"
}

break_backup_summary() {
  jq '.summary.backup = "STALE_OR_FUTURE"' "$1" >"$1.tmp"
  mv -- "$1.tmp" "$1"
  seal_host_evidence "$1"
}

break_overall_status() {
  jq '.status = "INCOMPLETE"' "$1" >"$1.tmp"
  mv -- "$1.tmp" "$1"
  seal_host_evidence "$1"
}

break_integrity_contract() {
  jq '.integrity.canonicalization = "UNREVIEWED"' "$1" >"$1.tmp"
  mv -- "$1.tmp" "$1"
}

remove_integrity() {
  jq 'del(.integrity)' "$1" >"$1.tmp"
  mv -- "$1.tmp" "$1"
}

malform_json() {
  printf '{\n' >"$1"
}

oversize_json() {
  dd if=/dev/zero bs=2097153 count=1 2>/dev/null | tr '\000' a >"$1"
}

complete_evidence="$test_root/complete/host.json"
mkdir -p -- "${complete_evidence%/*}"
make_complete_evidence "$complete_evidence"
"$verifier" "$complete_evidence" >"$test_root/complete-output"
grep -Fq 'internal payload consistency only' "$test_root/complete-output"
grep -Fq 'certification remain required' "$test_root/complete-output"

incomplete_evidence="$test_root/incomplete/host.json"
mkdir -p -- "${incomplete_evidence%/*}"
cp -- "$complete_evidence" "$incomplete_evidence"
jq '.services[3] = {service: "api", status: "UNAVAILABLE", health: "UNAVAILABLE"} |
    .summary.services = "FAIL" | .summary.application_hardening = "FAIL" |
    .summary.network_exposure = "FAIL" | .status = "INCOMPLETE"' \
  "$incomplete_evidence" >"$incomplete_evidence.tmp"
mv -- "$incomplete_evidence.tmp" "$incomplete_evidence"
seal_host_evidence "$incomplete_evidence"
"$verifier" "$incomplete_evidence" >"$test_root/incomplete-output"

invalid_release_evidence="$test_root/invalid-release/host.json"
mkdir -p -- "${invalid_release_evidence%/*}"
cp -- "$complete_evidence" "$invalid_release_evidence"
jq '.release_sha = "INVALID" | .status = "INCOMPLETE"' "$invalid_release_evidence" >"$invalid_release_evidence.tmp"
mv -- "$invalid_release_evidence.tmp" "$invalid_release_evidence"
seal_host_evidence "$invalid_release_evidence"
"$verifier" "$invalid_release_evidence" >"$test_root/invalid-release-output"

unsafe_permission_evidence="$test_root/unsafe-permission/host.json"
mkdir -p -- "${unsafe_permission_evidence%/*}"
cp -- "$complete_evidence" "$unsafe_permission_evidence"
jq '.sensitive_file_permissions[0] = {file: "production-environment", status: "UNSAFE", owner: null, group: null, mode: null} |
    .summary.sensitive_file_permissions = "FAIL" | .status = "INCOMPLETE"' \
  "$unsafe_permission_evidence" >"$unsafe_permission_evidence.tmp"
mv -- "$unsafe_permission_evidence.tmp" "$unsafe_permission_evidence"
seal_host_evidence "$unsafe_permission_evidence"
"$verifier" "$unsafe_permission_evidence" >"$test_root/unsafe-permission-output"

expect_failure tamper 'payload checksum mismatch' tamper_payload
expect_failure omitted 'core schema' omit_backup
expect_failure unexpected 'core schema' add_unexpected_field
expect_failure duplicate-service 'service inventory or summary is inconsistent' duplicate_service
expect_failure duplicate-container 'service inventory or summary is inconsistent' duplicate_container
# New snapshots identify the clock domain; legacy 1.1 evidence above remains readable.
clock_evidence="$test_root/clock.json"
jq '.schema_version = "1.2" | .monitoring_timers |= map(. + {next_run_clock: "REALTIME"}) |
    .monitoring_timers[0].next_run_clock = "MONOTONIC" |
    .monitoring_timers[0].next_run = "2w 4d 23h 13min 16.745423s"' "$complete_evidence" >"$clock_evidence"
seal_host_evidence "$clock_evidence"
"$verifier" "$clock_evidence" >/dev/null
for invalid in '"2026-09-10T01:00:00Z"' '"infinity"' '"0"' '"0s"' '"garbage"'; do
  jq --argjson invalid "$invalid" '.monitoring_timers[0].next_run = $invalid' "$clock_evidence" >"$test_root/clock-invalid.json"
  seal_host_evidence "$test_root/clock-invalid.json"
  if "$verifier" "$test_root/clock-invalid.json" >/dev/null 2>&1; then fail "invalid monotonic time accepted"; fi
done
jq '.monitoring_timers[0].next_run_clock = "UNAVAILABLE"' "$clock_evidence" >"$test_root/clock-invalid.json"
seal_host_evidence "$test_root/clock-invalid.json"
if "$verifier" "$test_root/clock-invalid.json" >/dev/null 2>&1; then fail "mismatched clock accepted"; fi
jq '.monitoring_timers[0].next_run_clock = "UNAVAILABLE" | .monitoring_timers[0].next_run = "UNAVAILABLE" |
    .summary.monitoring_timers = "FAIL" | .status = "INCOMPLETE"' "$clock_evidence" >"$test_root/clock-unavailable.json"
seal_host_evidence "$test_root/clock-unavailable.json"
"$verifier" "$test_root/clock-unavailable.json" >/dev/null

expect_failure secret-key 'secret-like key' add_secret_key
expect_failure secret-value 'secret-like value' add_secret_value
expect_failure identity 'core schema' change_collection_identity
expect_failure future 'core schema' future_collection
expect_failure future-service 'service inventory or summary is inconsistent' future_service
expect_failure service-summary 'service inventory or summary is inconsistent' break_service_summary
expect_failure application-hardening 'service inventory or summary is inconsistent' break_application_hardening
expect_failure network-exposure 'service inventory or summary is inconsistent' break_network_exposure
expect_failure proxy-port 'service inventory or summary is inconsistent' omit_proxy_port
expect_failure duplicate-timer 'monitoring-timer inventory or summary is inconsistent' duplicate_timer
expect_failure timer-summary 'monitoring-timer inventory or summary is inconsistent' break_timer_summary
expect_failure timer-next-run 'monitoring-timer inventory or summary is inconsistent' remove_timer_next_run
expect_failure permission-summary 'sensitive-file permission evidence or summary is inconsistent' break_permission_summary
expect_failure duplicate-permission 'sensitive-file permission evidence or summary is inconsistent' duplicate_permission
expect_failure check-summary 'read-only check inventory or summary is inconsistent' break_check_summary
expect_failure duplicate-check 'read-only check inventory or summary is inconsistent' duplicate_check
expect_failure backup-age 'backup freshness evidence or summary is inconsistent' break_backup_age
expect_failure backup-summary 'backup freshness evidence or summary is inconsistent' break_backup_summary
expect_failure status 'overall host evidence status is inconsistent' break_overall_status
expect_failure integrity-contract 'integrity metadata is missing or inconsistent' break_integrity_contract
expect_failure missing-integrity 'integrity metadata is missing or inconsistent' remove_integrity
expect_failure malformed 'host evidence JSON is malformed' malform_json
expect_failure oversized 'evidence is empty or exceeds the size limit' oversize_json

unsafe_directory="$test_root/unsafe"
mkdir -p -- "$unsafe_directory"
cp -- "$complete_evidence" "$unsafe_directory/bad name.json"
if "$verifier" "$unsafe_directory/bad name.json" >"$unsafe_directory/output" 2>&1; then
  fail "unsafe evidence file name verified"
fi
grep -Fq 'file name is unsafe or noncanonical' "$unsafe_directory/output"

symlink_directory="$test_root/symlink"
mkdir -p -- "$symlink_directory"
ln -s "$complete_evidence" "$symlink_directory/host.json"
if "$verifier" "$symlink_directory/host.json" >"$symlink_directory/output" 2>&1; then
  fail "symlinked evidence verified"
fi
grep -Fq 'existing non-symlink regular file' "$symlink_directory/output"

echo "SOC 2 host evidence verification tests passed."
