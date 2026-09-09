#!/usr/bin/env bash
set -Eeuo pipefail

export LC_ALL=C

repo_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)"
verifier="$repo_root/scripts/verify-soc2-evidence-snapshot.sh"
test_root="$(mktemp -d "${TMPDIR:-/tmp}/arbion-soc2-evidence-test.XXXXXX")"

cleanup() {
  if [[ -d "$test_root" ]]; then
    find "$test_root" -depth -delete
  fi
}
trap cleanup EXIT

fail() {
  echo "SOC 2 evidence snapshot test failed: $*" >&2
  exit 1
}

seal_snapshot() {
  local snapshot="$1"
  local temporary="$snapshot/.SHA256SUMS.tmp"
  (
    cd -- "$snapshot"
    for evidence_file in ./*.json; do
      digest="$(openssl dgst -sha256 "$evidence_file" | awk '{print $NF}')"
      printf '%s  %s\n' "$digest" "${evidence_file#./}"
    done
  ) >"$temporary"
  mv -- "$temporary" "$snapshot/SHA256SUMS"
}

make_complete_snapshot() {
  local snapshot="$1"
  mkdir -m 700 -- "$snapshot"
  jq -n '{
    schema_version: "1.0",
    collection_id: "arbion-soc2-20260909T160000Z",
    collected_at: "2026-09-09T16:00:00Z",
    status: "COMPLETE_REVIEW_REQUIRED",
    repository: "example/arbion",
    aws_region: "us-east-1",
    unavailable_sources: [],
    limitations: [
      "Read-only point-in-time configuration snapshot",
      "Requires reviewer evaluation",
      "Does not by itself establish operating effectiveness or SOC 2 certification"
    ]
  }' >"$snapshot/collection-summary.json"
  jq -n '{full_name: "example/arbion", default_branch: "main", security_and_analysis: {secret_scanning: {status: "enabled"}}}' \
    >"$snapshot/github-repository.json"
  jq -n '{Account: "111122223333", Arn: "arn:aws:iam::111122223333:role/read-only", UserId: "AROATEST"}' \
    >"$snapshot/aws-identity.json"
  expected_evidence=(
    github-actions-permissions github-collaborators github-identity github-main-protection
    github-production-environment github-repository github-rulesets
    aws-access-analyzers aws-alarm-topic-subscriptions aws-audit-bucket-encryption
    aws-audit-bucket-lifecycle aws-audit-bucket-object-lock aws-audit-bucket-public-access
    aws-audit-bucket-versioning aws-backup-bucket-encryption aws-backup-bucket-lifecycle
    aws-backup-bucket-object-lock aws-backup-bucket-public-access aws-backup-bucket-versioning
    aws-cloudtrail-selectors aws-cloudtrail-status aws-cloudtrail-trails aws-cloudwatch-alarms
    aws-config-delivery-channels aws-config-recorder-status aws-config-recorders
    aws-guardduty-detectors aws-identity aws-lightsail-alarms aws-lightsail-instances
    aws-security-event-rule aws-security-event-targets
  )
  for evidence_name in "${expected_evidence[@]}"; do
    [[ -f "$snapshot/$evidence_name.json" ]] && continue
    jq -n --arg source "$evidence_name" '{source: $source, result: "AVAILABLE"}' >"$snapshot/$evidence_name.json"
  done
  seal_snapshot "$snapshot"
}

make_incomplete_snapshot() {
  local snapshot="$1"
  mkdir -m 700 -- "$snapshot"
  jq -n '{
    schema_version: "1.0",
    collection_id: "arbion-soc2-20260909T160100Z",
    collected_at: "2026-09-09T16:01:00Z",
    status: "INCOMPLETE",
    repository: "example/arbion",
    aws_region: "us-east-1",
    unavailable_sources: ["aws", "github"],
    limitations: ["Requires reviewer evaluation", "Does not establish SOC 2 certification"]
  }' >"$snapshot/collection-summary.json"
  jq -n '{status: "UNAVAILABLE", system: "AWS", reason: "Read-only authentication unavailable.", collected_at: "2026-09-09T16:01:00Z"}' \
    >"$snapshot/aws.json"
  jq -n '{status: "UNAVAILABLE", system: "GITHUB", reason: "Read-only authentication unavailable.", collected_at: "2026-09-09T16:01:00Z"}' \
    >"$snapshot/github.json"
  seal_snapshot "$snapshot"
}

expect_failure() {
  local name="$1"
  local expected="$2"
  local mutate="$3"
  local parent="$test_root/$name"
  local snapshot="$parent/arbion-soc2-20260909T160000Z"
  mkdir -p -- "$parent"
  cp -R "$complete_snapshot" "$snapshot"
  "$mutate" "$snapshot"
  if "$verifier" "$snapshot" >"$parent/output" 2>&1; then
    fail "$name mutation verified"
  fi
  grep -Fq "$expected" "$parent/output" || fail "$name did not fail with the expected safe classification"
}

fake_bin="$test_root/fake-bin"
collector_parent="$test_root/collector"
mkdir -p -- "$fake_bin" "$collector_parent"
printf '%s\n' '#!/usr/bin/env bash' 'if [[ "$1" == "configure" && "$2" == "get" && "$3" == "region" ]]; then echo us-east-1; exit 0; fi' 'exit 1' >"$fake_bin/aws"
printf '%s\n' '#!/usr/bin/env bash' 'exit 1' >"$fake_bin/gh"
chmod 755 "$fake_bin/aws" "$fake_bin/gh"
if PATH="$fake_bin:$PATH" "$repo_root/scripts/collect-soc2-external-evidence.sh" "$collector_parent" >"$test_root/collector-output" 2>&1; then
  fail "mocked unavailable external sources unexpectedly produced a complete snapshot"
fi
collector_snapshots=("$collector_parent"/arbion-soc2-*)
[[ "${#collector_snapshots[@]}" -eq 1 && -d "${collector_snapshots[0]}" ]] || fail "collector did not create exactly one bounded snapshot"
"$verifier" "${collector_snapshots[0]}" >"$test_root/collector-verification-output"
grep -Fq 'internal checksum consistency only' "$test_root/collector-verification-output"

paginated_bin="$test_root/paginated-bin"
paginated_parent="$test_root/paginated-collector"
mkdir -p -- "$paginated_bin" "$paginated_parent"
printf '%s\n' \
  '#!/usr/bin/env bash' \
  'if [[ "$1" == "configure" && "$2" == "get" && "$3" == "region" ]]; then echo us-east-1; exit 0; fi' \
  'exit 1' >"$paginated_bin/aws"
printf '%s\n' \
  '#!/usr/bin/env bash' \
  'if [[ "$1" == "auth" && "$2" == "status" ]]; then exit 0; fi' \
  '[[ "$1" == "api" ]] || exit 1' \
  'endpoint=""; saw_paginate=false; saw_slurp=false' \
  'for argument in "$@"; do' \
  '  [[ "$argument" == "--paginate" ]] && saw_paginate=true' \
  '  [[ "$argument" == "--slurp" ]] && saw_slurp=true' \
  '  case "$argument" in user|repos/example/arbion*) endpoint="$argument" ;; esac' \
  'done' \
  'case "$endpoint" in' \
  '  user) printf "%s\\n" '\''{"login":"collector","id":1}'\'' ;;' \
  '  repos/example/arbion) printf "%s\\n" '\''{"full_name":"example/arbion"}'\'' ;;' \
  '  repos/example/arbion/branches/main/protection) printf "%s\\n" '\''{}'\'' ;;' \
  '  "repos/example/arbion/rulesets?includes_parents=true") printf "%s\\n" '\''[]'\'' ;;' \
  '  repos/example/arbion/environments/production) printf "%s\\n" '\''{}'\'' ;;' \
  '  repos/example/arbion/actions/permissions) printf "%s\\n" '\''{}'\'' ;;' \
  '  "repos/example/arbion/collaborators?affiliation=all&per_page=100")' \
  '    [[ "$saw_paginate" == true && "$saw_slurp" == true ]] || exit 2' \
  '    printf "%s\\n" '\''[[{"login":"first","id":1,"role_name":"admin","permissions":{"admin":true}}],[{"login":"second","id":2,"role_name":"write","permissions":{"push":true}}]]'\''' \
  '    ;;' \
  '  *) exit 1 ;;' \
  'esac' >"$paginated_bin/gh"
chmod 755 "$paginated_bin/aws" "$paginated_bin/gh"
if ARBION_GITHUB_REPOSITORY=example/arbion PATH="$paginated_bin:$PATH" \
  "$repo_root/scripts/collect-soc2-external-evidence.sh" "$paginated_parent" \
  >"$test_root/paginated-output" 2>&1; then
  fail "partially available mocked collector unexpectedly reported a complete snapshot"
fi
paginated_snapshots=("$paginated_parent"/arbion-soc2-*)
[[ "${#paginated_snapshots[@]}" -eq 1 && -d "${paginated_snapshots[0]}" ]] ||
  fail "paginated collector did not create exactly one bounded snapshot"
"$verifier" "${paginated_snapshots[0]}" >"$test_root/paginated-verification-output"
jq -e '
  length == 2 and
  .[0].login == "first" and
  .[1].login == "second"
' "${paginated_snapshots[0]}/github-collaborators.json" >/dev/null ||
  fail "collector did not retain the complete paginated collaborator population"
jq -e '
  .status == "INCOMPLETE" and
  .unavailable_sources == ["aws"]
' "${paginated_snapshots[0]}/collection-summary.json" >/dev/null ||
  fail "paginated partial collection did not preserve the exact unavailable source"

tamper_json() {
  local snapshot="$1"
  jq '.tampered = true' "$snapshot/github-repository.json" >"$snapshot/.tmp"
  mv -- "$snapshot/.tmp" "$snapshot/github-repository.json"
}

omit_json() {
  unlink "$1/github-repository.json"
}

duplicate_manifest_entry() {
  local snapshot="$1"
  awk 'NR == 1 { print; print; next } { print }' "$snapshot/SHA256SUMS" >"$snapshot/.tmp"
  mv -- "$snapshot/.tmp" "$snapshot/SHA256SUMS"
}

malform_manifest() {
  local snapshot="$1"
  awk 'NR == 1 { sub(/^[0-9a-f]+/, "not-a-digest") } { print }' "$snapshot/SHA256SUMS" >"$snapshot/.tmp"
  mv -- "$snapshot/.tmp" "$snapshot/SHA256SUMS"
}

omit_manifest_entry() {
  local snapshot="$1"
  awk 'NR > 1 { print }' "$snapshot/SHA256SUMS" >"$snapshot/.tmp"
  mv -- "$snapshot/.tmp" "$snapshot/SHA256SUMS"
}

reverse_manifest() {
  local snapshot="$1"
  sort -k2,2r "$snapshot/SHA256SUMS" >"$snapshot/.tmp"
  mv -- "$snapshot/.tmp" "$snapshot/SHA256SUMS"
}

add_secret_key() {
  local snapshot="$1"
  jq '.client_secret = "redacted-test-value"' "$snapshot/github-repository.json" >"$snapshot/.tmp"
  mv -- "$snapshot/.tmp" "$snapshot/github-repository.json"
  seal_snapshot "$snapshot"
}

add_secret_value() {
  local snapshot="$1"
  jq '.note = "Bearer abcdefghijklmnopqrstuvwxyz012345"' "$snapshot/github-repository.json" >"$snapshot/.tmp"
  mv -- "$snapshot/.tmp" "$snapshot/github-repository.json"
  seal_snapshot "$snapshot"
}

add_symlink() {
  ln -s collection-summary.json "$1/copied-summary.json"
}

add_unsafe_name() {
  printf '{}\n' >"$1/bad name.json"
}

add_unexpected_file() {
  printf 'not evidence\n' >"$1/notes.txt"
}

add_hidden_file() {
  printf '{}\n' >"$1/.hidden.json"
}

add_nested_directory() {
  mkdir -- "$1/nested"
}

add_oversized_json() {
  local snapshot="$1"
  dd if=/dev/zero bs=2097153 count=1 2>/dev/null | tr '\000' a >"$snapshot/.large"
  jq -Rs '{value: .}' "$snapshot/.large" >"$snapshot/oversized.json"
  unlink "$snapshot/.large"
  seal_snapshot "$snapshot"
}

oversize_manifest() {
  dd if=/dev/zero bs=65537 count=1 2>/dev/null | tr '\000' a >"$1/SHA256SUMS"
}

malform_json() {
  printf '{\n' >"$1/github-repository.json"
}

change_collection_id() {
  local snapshot="$1"
  jq '.collection_id = "arbion-soc2-20990101T000000Z"' "$snapshot/collection-summary.json" >"$snapshot/.tmp"
  mv -- "$snapshot/.tmp" "$snapshot/collection-summary.json"
  seal_snapshot "$snapshot"
}

change_collection_time() {
  local snapshot="$1"
  jq '.collected_at = "2026-09-09T16:00:01Z"' "$snapshot/collection-summary.json" >"$snapshot/.tmp"
  mv -- "$snapshot/.tmp" "$snapshot/collection-summary.json"
  seal_snapshot "$snapshot"
}

change_status() {
  local snapshot="$1"
  jq '.status = "INCOMPLETE"' "$snapshot/collection-summary.json" >"$snapshot/.tmp"
  mv -- "$snapshot/.tmp" "$snapshot/collection-summary.json"
  seal_snapshot "$snapshot"
}

complete_snapshot="$test_root/complete/arbion-soc2-20260909T160000Z"
mkdir -p -- "${complete_snapshot%/*}"
make_complete_snapshot "$complete_snapshot"
"$verifier" "$complete_snapshot" >"$test_root/complete-output"
grep -Fq 'internal checksum consistency only' "$test_root/complete-output"
grep -Fq 'independent certification remain required' "$test_root/complete-output"

incomplete_snapshot="$test_root/incomplete/arbion-soc2-20260909T160100Z"
mkdir -p -- "${incomplete_snapshot%/*}"
make_incomplete_snapshot "$incomplete_snapshot"
"$verifier" "$incomplete_snapshot" >"$test_root/incomplete-output"

expect_failure tampered 'checksum mismatch: github-repository.json' tamper_json
expect_failure omitted 'snapshot file inventory does not match the bounded collector contract' omit_json
expect_failure duplicate 'duplicate file entry' duplicate_manifest_entry
expect_failure malformed-manifest 'malformed line' malform_manifest
expect_failure missing-checksum 'file count does not match' omit_manifest_entry
expect_failure order 'not in deterministic order' reverse_manifest
expect_failure secret-key 'secret-like key: github-repository.json' add_secret_key
expect_failure secret-value 'secret-like value: github-repository.json' add_secret_value
expect_failure symlink 'symlink, nested directory, or non-regular entry' add_symlink
expect_failure unsafe-name 'unexpected or unsafe file name' add_unsafe_name
expect_failure unexpected-file 'unexpected or unsafe file name' add_unexpected_file
expect_failure hidden-file 'unexpected or unsafe file name' add_hidden_file
expect_failure nested 'symlink, nested directory, or non-regular entry' add_nested_directory
expect_failure oversized 'per-file size limit: oversized.json' add_oversized_json
expect_failure oversized-manifest 'checksum manifest is empty or exceeds its size limit' oversize_manifest
expect_failure malformed-json 'JSON evidence is malformed: github-repository.json' malform_json
expect_failure collection-id 'collection summary identity, status, time, or schema is inconsistent' change_collection_id
expect_failure collection-time 'collection summary identity, status, time, or schema is inconsistent' change_collection_time
expect_failure status 'collection summary identity, status, time, or schema is inconsistent' change_status

incomplete_bad_time="$test_root/incomplete-time/arbion-soc2-20260909T160100Z"
mkdir -p -- "${incomplete_bad_time%/*}"
cp -R "$incomplete_snapshot" "$incomplete_bad_time"
jq '.collected_at = "2026-09-09T16:02:00Z"' "$incomplete_bad_time/aws.json" >"$incomplete_bad_time/.tmp"
mv -- "$incomplete_bad_time/.tmp" "$incomplete_bad_time/aws.json"
seal_snapshot "$incomplete_bad_time"
if "$verifier" "$incomplete_bad_time" >"$test_root/incomplete-time-output" 2>&1; then
  fail "mismatched unavailable-evidence time verified"
fi
grep -Fq 'unavailable evidence metadata is inconsistent: aws.json' "$test_root/incomplete-time-output"

incomplete_duplicate="$test_root/incomplete-duplicate/arbion-soc2-20260909T160100Z"
mkdir -p -- "${incomplete_duplicate%/*}"
cp -R "$incomplete_snapshot" "$incomplete_duplicate"
jq '.unavailable_sources += ["aws"]' "$incomplete_duplicate/collection-summary.json" >"$incomplete_duplicate/.tmp"
mv -- "$incomplete_duplicate/.tmp" "$incomplete_duplicate/collection-summary.json"
seal_snapshot "$incomplete_duplicate"
if "$verifier" "$incomplete_duplicate" >"$test_root/incomplete-duplicate-output" 2>&1; then
  fail "duplicate unavailable-source claim verified"
fi
grep -Fq 'collection summary identity, status, time, or schema is inconsistent' "$test_root/incomplete-duplicate-output"

future_snapshot="$test_root/future/arbion-soc2-20990101T000000Z"
mkdir -p -- "${future_snapshot%/*}"
cp -R "$complete_snapshot" "$future_snapshot"
jq '.collection_id = "arbion-soc2-20990101T000000Z" | .collected_at = "2099-01-01T00:00:00Z"' \
  "$future_snapshot/collection-summary.json" >"$future_snapshot/.tmp"
mv -- "$future_snapshot/.tmp" "$future_snapshot/collection-summary.json"
seal_snapshot "$future_snapshot"
if "$verifier" "$future_snapshot" >"$test_root/future-output" 2>&1; then
  fail "future-dated collection verified"
fi
grep -Fq 'collection summary identity, status, time, or schema is inconsistent' "$test_root/future-output"

echo "SOC 2 evidence snapshot verification tests passed."
