#!/usr/bin/env bash
set -Eeuo pipefail

umask 077
export LC_ALL=C

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
repository_root="$(cd -- "$script_dir/.." && pwd -P)"
snapshot_input="${1:-}"

maximum_file_bytes=2097152
maximum_total_bytes=16777216
maximum_file_count=128
maximum_manifest_bytes=65536

fail() {
  echo "SOC 2 evidence snapshot verification failed: $*" >&2
  exit 1
}

[[ "$#" -eq 1 && -n "$snapshot_input" ]] || fail "usage: $0 <external-evidence-snapshot-directory>"
for command in awk cmp find jq mktemp openssl sort tr wc; do
  command -v "$command" >/dev/null || fail "required command not found: $command"
done
[[ -d "$snapshot_input" && ! -L "$snapshot_input" ]] || fail "snapshot must be an existing non-symlink directory"
snapshot="$(cd -- "$snapshot_input" && pwd -P)"
case "$snapshot/" in
  "$repository_root/"*) fail "evidence snapshot must remain outside the Git repository" ;;
esac

snapshot_name="${snapshot##*/}"
[[ "$snapshot_name" =~ ^arbion-soc2-[0-9]{8}T[0-9]{6}Z$ ]] || fail "collection directory name is unsafe or noncanonical"
[[ -f "$snapshot/collection-summary.json" && ! -L "$snapshot/collection-summary.json" ]] ||
  fail "collection-summary.json is missing or unsafe"
[[ -f "$snapshot/SHA256SUMS" && ! -L "$snapshot/SHA256SUMS" ]] || fail "SHA256SUMS is missing or unsafe"
manifest_bytes="$(wc -c <"$snapshot/SHA256SUMS" | tr -d '[:space:]')"
[[ "$manifest_bytes" =~ ^[0-9]+$ && "$manifest_bytes" -gt 0 && "$manifest_bytes" -le "$maximum_manifest_bytes" ]] ||
  fail "checksum manifest is empty or exceeds its size limit"

unsafe_entry="$(find "$snapshot" -mindepth 1 \( -type l -o -type d -o ! -type f \) -print -quit)"
[[ -z "$unsafe_entry" ]] || fail "snapshot contains a symlink, nested directory, or non-regular entry"

temporary_root="$(mktemp -d "${TMPDIR:-/tmp}/arbion-soc2-evidence-verify.XXXXXX")"
cleanup() {
  if [[ -d "$temporary_root" ]]; then
    find "$temporary_root" -depth -delete
  fi
}
trap cleanup EXIT

: >"$temporary_root/actual-files"
file_count=0
total_bytes=0
while IFS= read -r -d '' path; do
  name="${path##*/}"
  if [[ "$name" == "SHA256SUMS" ]]; then
    continue
  fi
  [[ "$name" =~ ^[A-Za-z0-9][A-Za-z0-9._-]*\.json$ ]] || fail "snapshot contains an unexpected or unsafe file name"
  bytes="$(wc -c <"$path" | tr -d '[:space:]')"
  [[ "$bytes" =~ ^[0-9]+$ && "$bytes" -gt 0 && "$bytes" -le "$maximum_file_bytes" ]] ||
    fail "JSON evidence file is empty or exceeds the per-file size limit: $name"
  file_count=$((file_count + 1))
  total_bytes=$((total_bytes + bytes))
  [[ "$file_count" -le "$maximum_file_count" && "$total_bytes" -le "$maximum_total_bytes" ]] ||
    fail "snapshot exceeds the bounded file-count or total-size limit"
  jq -e . "$path" >/dev/null 2>&1 || fail "JSON evidence is malformed: $name"
  if ! jq -e '
    [
      .. | objects | keys[] |
      ascii_downcase | gsub("[^a-z0-9]"; "") |
      select(IN(
        "password", "passwd", "clientsecret", "secretaccesskey", "accesskeyid",
        "sessiontoken", "accesstoken", "refreshtoken", "idtoken", "authorization",
        "cookie", "privatekey", "recoverycode", "mfasecret"
      ))
    ] | length == 0
  ' "$path" >/dev/null 2>&1; then
    fail "JSON evidence contains a secret-like key: $name"
  fi
  if ! jq -e '
    [
      .. | strings |
      select(test("-----BEGIN [A-Z ]*PRIVATE KEY-----|(^|[^A-Z0-9])AKIA[0-9A-Z]{16}([^A-Z0-9]|$)|(^|[[:space:]])Bearer[[:space:]]+[A-Za-z0-9._~+/=-]{16,}"; "i"))
    ] | length == 0
  ' "$path" >/dev/null 2>&1; then
    fail "JSON evidence contains a secret-like value: $name"
  fi
  printf '%s\n' "$name" >>"$temporary_root/actual-files"
done < <(find "$snapshot" -mindepth 1 -maxdepth 1 -type f -print0)
[[ "$file_count" -gt 0 ]] || fail "snapshot contains no JSON evidence"
sort -o "$temporary_root/actual-files" "$temporary_root/actual-files"

: >"$temporary_root/expected-files"
printf '%s\n' collection-summary.json >>"$temporary_root/expected-files"
if [[ -f "$snapshot/github.json" ]]; then
  printf '%s\n' github.json >>"$temporary_root/expected-files"
else
  github_evidence=(
    github-actions-permissions.json
    github-collaborators.json
    github-identity.json
    github-main-protection.json
    github-production-environment.json
    github-repository.json
    github-rulesets.json
  )
  printf '%s\n' "${github_evidence[@]}" >>"$temporary_root/expected-files"
fi
if [[ -f "$snapshot/aws.json" ]]; then
  printf '%s\n' aws.json >>"$temporary_root/expected-files"
else
  aws_evidence=(
    aws-access-analyzers.json
    aws-alarm-topic-subscriptions.json
    aws-audit-bucket-encryption.json
    aws-audit-bucket-lifecycle.json
    aws-audit-bucket-object-lock.json
    aws-audit-bucket-public-access.json
    aws-audit-bucket-versioning.json
    aws-backup-bucket-encryption.json
    aws-backup-bucket-lifecycle.json
    aws-backup-bucket-object-lock.json
    aws-backup-bucket-public-access.json
    aws-backup-bucket-versioning.json
    aws-cloudtrail-selectors.json
    aws-cloudtrail-status.json
    aws-cloudtrail-trails.json
    aws-cloudwatch-alarms.json
    aws-config-delivery-channels.json
    aws-config-recorder-status.json
    aws-config-recorders.json
    aws-guardduty-detectors.json
    aws-identity.json
    aws-lightsail-alarms.json
    aws-lightsail-instances.json
    aws-security-event-rule.json
    aws-security-event-targets.json
  )
  printf '%s\n' "${aws_evidence[@]}" >>"$temporary_root/expected-files"
  if jq -e '.schema_version == "1.1"' "$snapshot/collection-summary.json" >/dev/null; then
    printf '%s\n' aws-operations-alarm-topic.json >>"$temporary_root/expected-files"
  fi
fi
sort -o "$temporary_root/expected-files" "$temporary_root/expected-files"
cmp -s "$temporary_root/actual-files" "$temporary_root/expected-files" ||
  fail "snapshot file inventory does not match the bounded collector contract"

: >"$temporary_root/manifest-files"
manifest_count=0
previous_name=""
manifest_pattern='^([0-9a-f]{64})  ([A-Za-z0-9][A-Za-z0-9._-]*\.json)$'
while IFS= read -r manifest_line || [[ -n "$manifest_line" ]]; do
  [[ "$manifest_line" =~ $manifest_pattern ]] ||
    fail "checksum manifest contains a malformed line"
  claimed_digest="${BASH_REMATCH[1]}"
  name="${BASH_REMATCH[2]}"
  if [[ -n "$previous_name" && "$name" < "$previous_name" ]]; then
    fail "checksum manifest entries are not in deterministic order"
  fi
  [[ "$name" != "$previous_name" ]] || fail "checksum manifest contains a duplicate file entry"
  [[ -f "$snapshot/$name" && ! -L "$snapshot/$name" ]] || fail "checksum manifest references missing or unsafe evidence"
  recomputed_digest="$(openssl dgst -sha256 "$snapshot/$name" | awk '{print $NF}')"
  [[ "$recomputed_digest" =~ ^[0-9a-f]{64}$ && "$claimed_digest" == "$recomputed_digest" ]] ||
    fail "checksum mismatch: $name"
  printf '%s\n' "$name" >>"$temporary_root/manifest-files"
  previous_name="$name"
  manifest_count=$((manifest_count + 1))
done <"$snapshot/SHA256SUMS"
[[ "$manifest_count" -eq "$file_count" ]] || fail "checksum manifest file count does not match the snapshot"
cmp -s "$temporary_root/actual-files" "$temporary_root/manifest-files" ||
  fail "checksum manifest coverage does not exactly match the JSON evidence"

summary="$snapshot/collection-summary.json"
jq -e --arg collection_id "$snapshot_name" '
  (.schema_version | IN("1.0", "1.1")) and
  .collection_id == $collection_id and
  (.collected_at | type == "string" and test("^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$")) and
  ((.collected_at | fromdateiso8601) <= now) and
  ("arbion-soc2-" + (.collected_at | gsub("[-:]"; ""))) == .collection_id and
  (.status | IN("COMPLETE_REVIEW_REQUIRED", "INCOMPLETE")) and
  (.repository | type == "string" and length > 0) and
  (.aws_region | type == "string" and test("^[a-z]{2}(-[a-z]+)+-[0-9]+$")) and
  (.unavailable_sources | type == "array") and
  all(.unavailable_sources[]; type == "string" and test("^[A-Za-z0-9][A-Za-z0-9._-]*$")) and
  ((.unavailable_sources | unique | length) == (.unavailable_sources | length)) and
  (((.unavailable_sources | length) == 0 and .status == "COMPLETE_REVIEW_REQUIRED") or
   ((.unavailable_sources | length) > 0 and .status == "INCOMPLETE")) and
  (.limitations | type == "array" and length > 0 and all(.[]; type == "string" and length > 0))
' "$summary" >/dev/null || fail "collection summary identity, status, time, or schema is inconsistent"

collected_at="$(jq -r '.collected_at' "$summary")"
: >"$temporary_root/actual-unavailable"
for path in "$snapshot"/*.json; do
  name="${path##*/}"
  [[ "$name" != "collection-summary.json" ]] || continue
  if jq -e '.status? == "UNAVAILABLE"' "$path" >/dev/null 2>&1; then
    jq -e --arg collected_at "$collected_at" '
      .status == "UNAVAILABLE" and
      (.system | type == "string" and length > 0) and
      (.reason | type == "string" and length > 0) and
      .collected_at == $collected_at
    ' "$path" >/dev/null || fail "unavailable evidence metadata is inconsistent: $name"
    printf '%s\n' "${name%.json}" >>"$temporary_root/actual-unavailable"
  fi
done
sort -o "$temporary_root/actual-unavailable" "$temporary_root/actual-unavailable"
jq -r '.unavailable_sources[]' "$summary" | sort >"$temporary_root/claimed-unavailable"
cmp -s "$temporary_root/actual-unavailable" "$temporary_root/claimed-unavailable" ||
  fail "summary unavailable-source set does not match the saved evidence"

echo "SOC 2 evidence snapshot verification passed: $file_count files; internal checksum consistency only."
echo "Reviewer evaluation, immutable external retention, operating effectiveness, and independent certification remain required."
