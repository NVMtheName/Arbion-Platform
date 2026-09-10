#!/usr/bin/env bash
set -Eeuo pipefail

umask 077
export LC_ALL=C

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
repository_root="$(cd -- "$script_dir/.." && pwd -P)"
evidence_input="${1:-}"
maximum_evidence_bytes=2097152

fail() {
  echo "SOC 2 host evidence verification failed: $*" >&2
  exit 1
}

[[ "$#" -eq 1 && -n "$evidence_input" ]] || fail "usage: $0 <saved-host-evidence.json>"
for command in awk find jq mktemp openssl tr wc; do
  command -v "$command" >/dev/null || fail "required command not found: $command"
done
[[ -f "$evidence_input" && ! -L "$evidence_input" ]] || fail "evidence must be an existing non-symlink regular file"
evidence_parent="$(cd -- "$(dirname -- "$evidence_input")" && pwd -P)"
evidence="$evidence_parent/$(basename -- "$evidence_input")"
case "$evidence/" in
  "$repository_root/"*) fail "host evidence must remain outside the Git repository" ;;
esac
evidence_name="${evidence##*/}"
[[ "$evidence_name" =~ ^[A-Za-z0-9][A-Za-z0-9._-]*\.json$ ]] || fail "evidence file name is unsafe or noncanonical"

evidence_bytes="$(wc -c <"$evidence" | tr -d '[:space:]')"
[[ "$evidence_bytes" =~ ^[0-9]+$ && "$evidence_bytes" -gt 0 && "$evidence_bytes" -le "$maximum_evidence_bytes" ]] ||
  fail "evidence is empty or exceeds the size limit"
jq -e . "$evidence" >/dev/null 2>&1 || fail "host evidence JSON is malformed"

if ! jq -e '
  [
    .. | objects | keys[] |
    ascii_downcase | gsub("[^a-z0-9]"; "") |
    select(IN(
      "password", "passwd", "clientsecret", "secretaccesskey", "accesskeyid",
      "sessiontoken", "accesstoken", "refreshtoken", "idtoken", "authorization",
      "cookie", "privatekey", "recoverycode", "mfasecret", "environmentvalue"
    ))
  ] | length == 0
' "$evidence" >/dev/null 2>&1; then
  fail "host evidence contains a secret-like key"
fi
if ! jq -e '
  [
    .. | strings |
    select(test("-----BEGIN [A-Z ]*PRIVATE KEY-----|(^|[^A-Z0-9])AKIA[0-9A-Z]{16}([^A-Z0-9]|$)|(^|[[:space:]])Bearer[[:space:]]+[A-Za-z0-9._~+/=-]{16,}"; "i"))
  ] | length == 0
' "$evidence" >/dev/null 2>&1; then
  fail "host evidence contains a secret-like value"
fi

temporary_root="$(mktemp -d "${TMPDIR:-/tmp}/arbion-soc2-host-verify.XXXXXX")"
cleanup() {
  if [[ -d "$temporary_root" ]]; then
    find "$temporary_root" -depth -delete
  fi
}
trap cleanup EXIT

jq -e '
  (.integrity | type == "object") and
  ((.integrity | keys | sort) == ["algorithm", "canonicalization", "covers", "payload_sha256"]) and
  .integrity.algorithm == "SHA-256" and
  .integrity.canonicalization == "JQ_SORTED_COMPACT_UTF8_V1" and
  .integrity.covers == "ENTIRE_DOCUMENT_EXCLUDING_INTEGRITY" and
  (.integrity.payload_sha256 | type == "string" and test("^[0-9a-f]{64}$"))
' "$evidence" >/dev/null || fail "integrity metadata is missing or inconsistent"
jq -S -c 'del(.integrity)' "$evidence" >"$temporary_root/canonical-payload.json"
claimed_digest="$(jq -r '.integrity.payload_sha256' "$evidence")"
computed_digest="$(openssl dgst -sha256 "$temporary_root/canonical-payload.json" | awk '{print $NF}')"
[[ "$computed_digest" =~ ^[0-9a-f]{64}$ && "$computed_digest" == "$claimed_digest" ]] ||
  fail "host evidence payload checksum mismatch"

jq -e '
  ((keys | sort) == [
    "backup", "collected_at", "collection_id", "host", "integrity", "limitations",
    "monitoring_timers", "read_only_checks", "release_sha", "schema_version",
    "sensitive_file_permissions", "services", "status", "summary"
  ]) and
  (.schema_version | IN("1.1", "1.2")) and
  (.status | IN("COMPLETE_REVIEW_REQUIRED", "INCOMPLETE")) and
  (.collection_id | type == "string" and test("^arbion-soc2-host-[0-9]{8}T[0-9]{6}Z$")) and
  (.collected_at | type == "string" and test("^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$")) and
  ((.collected_at | fromdateiso8601) <= now) and
  ("arbion-soc2-host-" + (.collected_at | gsub("[-:]"; ""))) == .collection_id and
  (.host | type == "object") and
  ((.host | keys | sort) == ["kernel", "name", "os", "os_version"]) and
  all(.host[]; type == "string" and length > 0 and length <= 255 and test("^[A-Za-z0-9_.+-]+$")) and
  (.release_sha | type == "string" and (test("^[0-9a-f]{40}$") or . == "INVALID")) and
  (.summary | type == "object") and
  ((.summary | keys | sort) == ["application_hardening", "backup", "checks", "monitoring_timers", "network_exposure", "sensitive_file_permissions", "services"]) and
  (.summary.services | IN("PASS", "FAIL")) and
  (.summary.application_hardening | IN("PASS", "FAIL")) and
  (.summary.network_exposure | IN("PASS", "FAIL")) and
  (.summary.monitoring_timers | IN("PASS", "FAIL")) and
  (.summary.sensitive_file_permissions | IN("PASS", "FAIL")) and
  (.summary.checks | IN("PASS", "FAIL")) and
  (.summary.backup | IN("CURRENT", "STALE_OR_FUTURE", "INVALID", "UNAVAILABLE")) and
  .limitations == [
    "Point-in-time read-only host snapshot",
    "Contains no environment values, credentials, customer records, application records, database content, or logs",
    "Requires reviewer evaluation and separate alert-delivery, restore, access, and patch evidence",
    "Does not by itself establish operating effectiveness or SOC 2 certification"
  ]
' "$evidence" >/dev/null || fail "host evidence identity, core schema, time, or limitations are inconsistent"

jq -e '
  (.collected_at | fromdateiso8601) as $collected_epoch |
  def full_service:
    ((keys | sort) == [
      "capability_drop", "configured_image", "container_id", "health", "image_id",
      "process_limit", "published_ports", "read_only_root", "restart_count", "runtime_user",
      "security_options", "service", "started_at", "status"
    ]) and
    (.container_id | type == "string" and test("^[0-9a-f]{64}$")) and
    (.configured_image | type == "string" and length > 0 and length <= 512 and test("^[A-Za-z0-9][A-Za-z0-9._/@:+-]*$")) and
    (.image_id | type == "string" and test("^sha256:[0-9a-f]{64}$")) and
    (.runtime_user | type == "string" and length > 0 and length <= 128 and test("^[A-Za-z0-9_.:-]+$")) and
    (.status | type == "string" and length > 0 and length <= 64 and test("^[A-Za-z0-9_.-]+$")) and
    (.health | type == "string" and length > 0 and length <= 64 and test("^[A-Za-z0-9_.-]+$")) and
    (.started_at | type == "string" and test("^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\\.[0-9]+)?Z$")) and
    (.restart_count | type == "number" and floor == . and . >= 0) and
    (.read_only_root | type == "boolean") and
    (.capability_drop | type == "array" and all(.[]; type == "string" and length > 0 and length <= 64) and ((unique | length) == length)) and
    (.security_options | type == "array" and all(.[]; type == "string" and length > 0 and length <= 128) and ((unique | length) == length)) and
    ((.process_limit == null) or (.process_limit | type == "number" and floor == . and . >= 0)) and
    (.published_ports | type == "object");
  def unavailable_service:
    ((keys | sort) == ["health", "service", "status"]) and
    .status == "UNAVAILABLE" and .health == "UNAVAILABLE";
  (.services | type == "array" and length == 6) and
  (([.services[].service] | sort) == ["ai", "api", "postgres", "proxy", "redis", "web"]) and
  (([.services[].service] | unique | length) == 6) and
  (([.services[] | select(has("container_id")) | .container_id] | unique | length) ==
   ([.services[] | select(has("container_id"))] | length)) and
  all(.services[];
    (.service | type == "string") and
    ((full_service and ((.started_at | sub("\\.[0-9]+Z$"; "Z") | fromdateiso8601) <= $collected_epoch)) or unavailable_service)
  ) and
  ((all(.services[]; .status == "running" and (.health == "healthy" or .health == "NOT_CONFIGURED"))) as $passing |
   .summary.services == (if $passing then "PASS" else "FAIL" end)) and
  ((all(.services[] | select(.service == "api" or .service == "ai" or .service == "web");
      .status == "running" and .read_only_root == true and .runtime_user != "IMAGE_DEFAULT" and
      .process_limit == 256 and ((.capability_drop // []) | index("ALL")) != null and
      ((.security_options // []) | index("no-new-privileges:true")) != null and
      all((.published_ports // {}) | to_entries[]?; .value == null)
    )) as $passing |
   .summary.application_hardening == (if $passing then "PASS" else "FAIL" end)) and
  (((.services[] | select(.service == "proxy") | .published_ports) as $proxy_ports |
      all(["80/tcp", "443/tcp", "443/udp"][]; (($proxy_ports // {})[.] | type == "array" and length > 0)) and
      all(($proxy_ports // {}) | to_entries[]; ((.key | IN("80/tcp", "443/tcp", "443/udp")) or .value == null)) and
      all(.services[] | select(.service != "proxy");
        has("published_ports") and all(.published_ports | to_entries[]?; .value == null)
      )
    ) as $passing |
   .summary.network_exposure == (if $passing then "PASS" else "FAIL" end))
' "$evidence" >/dev/null || fail "service inventory or summary is inconsistent"

jq -e '
  .schema_version as $schema |
  (.monitoring_timers | type == "array" and length == 9) and
  (([.monitoring_timers[].unit] | sort) == [
    "arbion-docker-build-cache-prune.timer", "arbion-host-capacity.timer",
    "arbion-memory-pressure.timer", "arbion-postgres-backup-freshness.timer",
    "arbion-postgres-backup.timer", "arbion-production-containers.timer",
    "arbion-production-health.timer", "arbion-reboot-required.timer",
    "arbion-tls-certificate.timer"
  ]) and
  (([.monitoring_timers[].unit] | unique | length) == 9) and
  all(.monitoring_timers[];
    ((keys | sort) == (if $schema == "1.2" then ["active_state", "load_state", "next_run", "next_run_clock", "sub_state", "unit", "unit_file_state"] else ["active_state", "load_state", "next_run", "sub_state", "unit", "unit_file_state"] end)) and
    (if $schema == "1.2" then
      (.next_run_clock | IN("REALTIME", "MONOTONIC", "UNAVAILABLE")) and
      ((.next_run == "UNAVAILABLE") == (.next_run_clock == "UNAVAILABLE")) and
      (if .next_run_clock == "MONOTONIC" then (.next_run | test("[1-9]") and test("^[0-9]+(\\.[0-9]+)?(us|ms|s|min|h|d|w|month|y)( [0-9]+(\\.[0-9]+)?(us|ms|s|min|h|d|w|month|y))*$")) else true end)
    else true end) and
    all(.[]; type == "string" and length > 0 and length <= 512)
  ) and
  ((all(.monitoring_timers[]; .load_state == "loaded" and .active_state == "active" and .unit_file_state == "enabled" and .next_run != "UNAVAILABLE")) as $passing |
   .summary.monitoring_timers == (if $passing then "PASS" else "FAIL" end))
' "$evidence" >/dev/null || fail "monitoring-timer inventory or summary is inconsistent"

jq -e '
  (.sensitive_file_permissions | type == "array" and length == 3) and
  (([.sensitive_file_permissions[].file] | sort) == ["alert-environment", "backup-environment", "production-environment"]) and
  (([.sensitive_file_permissions[].file] | unique | length) == 3) and
  all(.sensitive_file_permissions[];
    ((keys | sort) == ["file", "group", "mode", "owner", "status"]) and
    (.status | IN("AVAILABLE", "UNAVAILABLE", "UNSAFE")) and
    (if .status == "AVAILABLE" then
      (.owner | type == "string" and length > 0 and length <= 128 and test("^[A-Za-z0-9_.-]+$")) and
      (.group | type == "string" and length > 0 and length <= 128 and test("^[A-Za-z0-9_.-]+$")) and
      (.mode | type == "string" and test("^[0-7]{3,4}$"))
    else
      .owner == null and .group == null and .mode == null
    end)
  ) and
  ((all(.sensitive_file_permissions[]; .status == "AVAILABLE" and .owner == "root" and .group == "root" and .mode == "600")) as $passing |
   .summary.sensitive_file_permissions == (if $passing then "PASS" else "FAIL" end))
' "$evidence" >/dev/null || fail "sensitive-file permission evidence or summary is inconsistent"

jq -e '
  (.read_only_checks | type == "array" and length == 6) and
  (([.read_only_checks[].check] | sort) == [
    "backup-freshness", "container-health", "host-capacity", "memory-pressure", "public-smoke", "tls-certificate"
  ]) and
  (([.read_only_checks[].check] | unique | length) == 6) and
  all(.read_only_checks[];
    ((keys | sort) == ["check", "result"]) and
    (.check | type == "string") and (.result | IN("PASS", "FAIL"))
  ) and
  ((all(.read_only_checks[]; .result == "PASS")) as $passing |
   .summary.checks == (if $passing then "PASS" else "FAIL" end))
' "$evidence" >/dev/null || fail "read-only check inventory or summary is inconsistent"

jq -e '
  (.collected_at | fromdateiso8601) as $collected_epoch |
  (.backup | type == "object") and
  ((.backup | keys | sort) == ["age_seconds", "completed_epoch", "maximum_age_seconds", "object_key", "status"]) and
  (.backup.status | IN("CURRENT", "STALE_OR_FUTURE", "INVALID", "UNAVAILABLE")) and
  (.backup.maximum_age_seconds | type == "number" and floor == . and . > 0 and . <= 31536000) and
  (if (.backup.status == "CURRENT" or .backup.status == "STALE_OR_FUTURE") then
    (.backup.completed_epoch | type == "number" and floor == . and . >= 0) and
    (.backup.object_key | type == "string" and length > 0 and length <= 512 and test("^[A-Za-z0-9][A-Za-z0-9._/-]*$")) and
    (.backup.age_seconds | type == "number" and floor == .) and
    .backup.age_seconds == ($collected_epoch - .backup.completed_epoch) and
    (if .backup.status == "CURRENT" then
      .backup.age_seconds >= 0 and .backup.age_seconds <= .backup.maximum_age_seconds
    else
      .backup.age_seconds < 0 or .backup.age_seconds > .backup.maximum_age_seconds
    end)
  else
    .backup.completed_epoch == null and .backup.object_key == null and .backup.age_seconds == null
  end) and
  .summary.backup == .backup.status
' "$evidence" >/dev/null || fail "backup freshness evidence or summary is inconsistent"

jq -e '
  ((.release_sha | test("^[0-9a-f]{40}$")) and
   .summary.services == "PASS" and
   .summary.application_hardening == "PASS" and
   .summary.network_exposure == "PASS" and
   .summary.monitoring_timers == "PASS" and
   .summary.sensitive_file_permissions == "PASS" and
   .summary.checks == "PASS" and
   .summary.backup == "CURRENT") as $complete |
  .status == (if $complete then "COMPLETE_REVIEW_REQUIRED" else "INCOMPLETE" end)
' "$evidence" >/dev/null || fail "overall host evidence status is inconsistent"

echo "SOC 2 host evidence verification passed; internal payload consistency only."
echo "Independent collection, reviewer evaluation, immutable retention, operating effectiveness, and certification remain required."
