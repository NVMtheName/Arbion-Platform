#!/usr/bin/env bash
set -Eeuo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
arbion_root_input="${ARBION_ROOT:-$script_dir/..}"
catalog_input="${ARBION_SOC2_CONTROL_CATALOG:-$arbion_root_input/compliance/soc2-controls.json}"
report_path="${1:-}"

fail() {
  echo "SOC 2 baseline check failed: $*" >&2
  exit 1
}

for command in git grep jq; do
  command -v "$command" >/dev/null || fail "required command not found: $command"
done

[[ -d "$arbion_root_input" ]] || fail "repository root is unavailable: $arbion_root_input"
arbion_root="$(cd -- "$arbion_root_input" && pwd -P)"
[[ -f "$catalog_input" ]] || fail "control catalog is unavailable: $catalog_input"
catalog="$(cd -- "$(dirname -- "$catalog_input")" && pwd -P)/$(basename -- "$catalog_input")"

jq -e '
  .schema_version == "1.0" and
  .program_status == "READINESS_BASELINE_NOT_CERTIFIED" and
  (.criteria_scope | sort == ["AVAILABILITY", "CONFIDENTIALITY", "SECURITY"]) and
  (.controls | type == "array" and length >= 15) and
  all(.controls[];
    (.id | type == "string" and test("^[A-Z]+-[0-9]{2}$")) and
    (.title | type == "string" and length > 0) and
    (.criteria | type == "array" and length > 0) and
    (.owner_role | type == "string" and length > 0) and
    (.cadence | type == "string" and length > 0) and
    (.implementation_status | IN("IMPLEMENTED", "PARTIAL", "OPERATIONAL_ACTION_REQUIRED", "EXTERNAL_CONFIGURATION_REQUIRED")) and
    (.repository_evidence | type == "array" and length > 0) and
    (.operational_evidence_required | type == "array" and length > 0)
  )
' "$catalog" >/dev/null || fail "catalog schema, scope, or control fields are invalid"

duplicate_ids="$(jq -r '.controls | group_by(.id)[] | select(length != 1) | .[0].id' "$catalog")"
[[ -z "$duplicate_ids" ]] || fail "duplicate control IDs: $duplicate_ids"

required_controls=(
  GOV-01 GOV-02 RISK-01 HR-01 AC-01 AC-02 SEC-01 SEC-02 DATA-01
  CHG-01 SDLC-01 MON-01 IR-01 AV-01 DR-01 CONF-01 TPRM-01 AI-01 EVID-01
)
for control_id in "${required_controls[@]}"; do
  jq -e --arg id "$control_id" 'any(.controls[]; .id == $id)' "$catalog" >/dev/null ||
    fail "required control is missing: $control_id"
done

while IFS= read -r evidence_path; do
  [[ "$evidence_path" != /* && "$evidence_path" != *".."* ]] ||
    fail "repository evidence path must remain inside the repository: $evidence_path"
  [[ -e "$arbion_root/$evidence_path" ]] || fail "repository evidence is missing: $evidence_path"
done < <(jq -r '.controls[].repository_evidence[]' "$catalog" | LC_ALL=C sort -u)

policy_files=(
  docs/compliance/README.md
  docs/compliance/SOC2_READINESS.md
  docs/compliance/SYSTEM_DESCRIPTION.md
  docs/compliance/EVIDENCE_RUNBOOK.md
  docs/compliance/OPERATING_EVIDENCE_WORKBOOK.md
  docs/compliance/INFORMATION_SECURITY_POLICY.md
  docs/compliance/ACCESS_CONTROL_POLICY.md
  docs/compliance/CHANGE_MANAGEMENT_POLICY.md
  docs/compliance/INCIDENT_RESPONSE_PLAN.md
  docs/compliance/BUSINESS_CONTINUITY_AND_DISASTER_RECOVERY.md
  docs/compliance/DATA_CLASSIFICATION_RETENTION_POLICY.md
  docs/compliance/VENDOR_RISK_MANAGEMENT_POLICY.md
  docs/compliance/RISK_MANAGEMENT_POLICY.md
)
for policy_file in "${policy_files[@]}"; do
  [[ -s "$arbion_root/$policy_file" ]] || fail "required policy is missing or empty: $policy_file"
  grep -q 'PENDING_MANAGEMENT_APPROVAL' "$arbion_root/$policy_file" ||
    fail "policy does not disclose pending approval: $policy_file"
done

[[ -s "$arbion_root/scripts/collect-soc2-external-evidence.sh" ]] ||
  fail "external evidence collector is missing or empty"
grep -q -- 'gh api --paginate --slurp' "$arbion_root/scripts/collect-soc2-external-evidence.sh" ||
  fail "external evidence collector does not paginate the GitHub collaborator population"
[[ -s "$arbion_root/scripts/collect-soc2-host-evidence.sh" ]] ||
  fail "production host evidence collector is missing or empty"
[[ -x "$arbion_root/scripts/verify-soc2-host-evidence.sh" ]] ||
  fail "production host evidence verifier is missing or not executable"
grep -q 'internal payload consistency only' "$arbion_root/scripts/verify-soc2-host-evidence.sh" ||
  fail "host evidence verification must disclose its consistency-only limitation"
[[ -x "$arbion_root/scripts/verify-soc2-evidence-snapshot.sh" ]] ||
  fail "external evidence snapshot verifier is missing or not executable"
grep -q 'internal checksum consistency only' "$arbion_root/scripts/verify-soc2-evidence-snapshot.sh" ||
  fail "external evidence verification must disclose its consistency-only limitation"
[[ -x "$arbion_root/scripts/review-soc2-external-evidence.py" ]] ||
  fail "external evidence review-draft generator is missing or not executable"
grep -q 'REVIEW_DRAFT_NOT_OPERATING_EVIDENCE' "$arbion_root/scripts/review-soc2-external-evidence.py" ||
  fail "external evidence review draft must disclose its non-authoritative status"
grep -q 'AUTHENTICATED_EVIDENCE_NOT_YET_COLLECTED' "$arbion_root/docs/compliance/EXTERNAL_CONTROL_VERIFICATION.md" ||
  fail "external control status must fail closed until authenticated evidence is retained"

while IFS= read -r workflow_file; do
  while IFS= read -r uses_line; do
    [[ "$uses_line" =~ uses:[[:space:]]+[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+(/[A-Za-z0-9_.-]+)?@[0-9a-f]{40}([[:space:]]*#[[:space:]].*)?$ ]] ||
      fail "GitHub Action is not pinned to an immutable 40-character commit: ${workflow_file#"$arbion_root/"}:$uses_line"
  done < <(grep -E '^[[:space:]]*-[[:space:]]+uses:' "$workflow_file" || true)
done < <(find "$arbion_root/.github/workflows" -type f \( -name '*.yml' -o -name '*.yaml' \) -print | LC_ALL=C sort)

grep -q 'github/codeql-action/init@' "$arbion_root/.github/workflows/security.yml" ||
  fail "CodeQL initialization is not configured"
grep -q 'actions/dependency-review-action@' "$arbion_root/.github/workflows/security.yml" ||
  fail "pull-request dependency review is not configured"
grep -q 'aquasecurity/trivy-action@' "$arbion_root/.github/workflows/security.yml" ||
  fail "production container vulnerability scanning is not configured"
grep -Eq 'scanners:[[:space:]]+secret' "$arbion_root/.github/workflows/security.yml" ||
  fail "independent repository secret scanning is not configured"
grep -Eq 'scan-type:[[:space:]]+config' "$arbion_root/.github/workflows/security.yml" ||
  fail "infrastructure misconfiguration scanning is not configured"
grep -Eq 'version:[[:space:]]+v0\.74\.0' "$arbion_root/.github/workflows/security.yml" ||
  fail "the reviewed Trivy scanner version is not pinned"
for lock_file in services/ai/requirements.lock services/ai/requirements-dev.lock; do
  [[ -s "$arbion_root/$lock_file" ]] || fail "Python dependency lock is missing or empty: $lock_file"
  grep -q -- '--hash=sha256:' "$arbion_root/$lock_file" ||
    fail "Python dependency lock does not contain package hashes: $lock_file"
done

extract_python_requirements() {
  local array_name="$1"
  awk -v target="$array_name" '
    index($0, target " = [") == 1 { in_array = 1; next }
    in_array && /^]/ { exit }
    in_array {
      requirement = $0
      sub(/^[[:space:]]*"/, "", requirement)
      sub(/",?[[:space:]]*$/, "", requirement)
      if (requirement != "") print requirement
    }
  ' "$arbion_root/services/ai/pyproject.toml"
}

while IFS= read -r requirement; do
  [[ "$requirement" =~ ^[A-Za-z0-9_.-]+==[A-Za-z0-9_.+-]+$ ]] ||
    fail "Python runtime dependency is not exactly pinned: $requirement"
  grep -Fq "$requirement \\" "$arbion_root/services/ai/requirements.lock" ||
    fail "Python runtime lock is stale or missing direct dependency: $requirement"
done < <(extract_python_requirements dependencies)

while IFS= read -r requirement; do
  [[ "$requirement" =~ ^[A-Za-z0-9_.-]+==[A-Za-z0-9_.+-]+$ ]] ||
    fail "Python development dependency is not exactly pinned: $requirement"
  grep -Fq "$requirement \\" "$arbion_root/services/ai/requirements-dev.lock" ||
    fail "Python development lock is stale or missing direct dependency: $requirement"
done < <(extract_python_requirements dev)

grep -q -- '--require-hashes -r requirements.lock' "$arbion_root/services/ai/Dockerfile" ||
  fail "AI production image does not enforce its hash-locked dependency closure"
grep -q -- '--require-hashes -r requirements-dev.lock' "$arbion_root/.github/workflows/ci.yml" ||
  fail "AI CI does not enforce its hash-locked dependency closure"
grep -q 'pip-audit --requirement requirements-dev.lock' "$arbion_root/.github/workflows/ci.yml" ||
  fail "AI CI does not audit the exact locked dependency closure"
for ecosystem in npm gomod pip docker github-actions terraform; do
  grep -Eq "package-ecosystem:[[:space:]]+$ecosystem" "$arbion_root/.github/dependabot.yml" ||
    fail "Dependabot does not cover $ecosystem"
done

for owned_path in '/.github/' '/compliance/' '/docs/compliance/' '/infrastructure/' '/services/api/internal/auth/' '/services/api/internal/credential/' '/services/api/internal/risk/' '/services/api/internal/liveexecution/'; do
  grep -Fq "$owned_path" "$arbion_root/.github/CODEOWNERS" || fail "CODEOWNERS omits $owned_path"
done

grep -q '^\.env\.production$' "$arbion_root/.gitignore" || fail ".env.production is not ignored"
grep -q '^\*\.tfstate\.\*$' "$arbion_root/.gitignore" || fail "Terraform state variants are not ignored"
grep -q 'readonlyRootFilesystem = true' "$arbion_root/infrastructure/terraform/modules/ecs/main.tf" ||
  fail "ECS read-only root filesystems are not enforced"
for service in migrate api ai web; do
  docker_hardening="$(awk -v target="  $service:" '
    $0 == target { in_service = 1; next }
    in_service && /^  [A-Za-z0-9_-]+:/ { exit }
    in_service { print }
  ' "$arbion_root/docker-compose.prod.yml")"
  if ! grep -q '<<: \*app-hardening' <<<"$docker_hardening"; then
    fail "production Compose hardening is not applied to $service"
  fi
done
grep -q 'image_tag_mutability = "IMMUTABLE"' "$arbion_root/infrastructure/terraform/modules/ecr/main.tf" ||
  fail "ECR image tags are not immutable"
grep -q 'scan_on_push = true' "$arbion_root/infrastructure/terraform/modules/ecr/main.tf" ||
  fail "ECR scan-on-push is not enabled"
grep -q -- '--sse AES256' "$arbion_root/scripts/backup-postgres.sh" ||
  fail "off-host backup upload does not request server-side encryption"
grep -q 'Strict-Transport-Security' "$arbion_root/deploy/Caddyfile" || fail "HSTS is not configured"
grep -q 'USER nextjs' "$arbion_root/apps/web/Dockerfile" || fail "web runtime is not explicitly non-root"
grep -q 'USER 65532:65532' "$arbion_root/services/api/Dockerfile" || fail "API runtime is not explicitly non-root"
grep -q 'USER arbion' "$arbion_root/services/ai/Dockerfile" || fail "AI runtime is not explicitly non-root"
while IFS= read -r container_line; do
  [[ "$container_line" =~ @sha256:[0-9a-f]{64}([[:space:]]|$) ]] ||
    fail "container reference is not pinned by digest: $container_line"
done < <(
  {
    grep -hE '^FROM[[:space:]][^[:space:]]*:' "$arbion_root/apps/web/Dockerfile" "$arbion_root/services/api/Dockerfile" "$arbion_root/services/ai/Dockerfile"
    grep -hE '^[[:space:]]+image:[[:space:]]+' "$arbion_root/docker-compose.prod.yml"
  } | sed -E 's/[[:space:]]+AS[[:space:]].*$//'
)

tracked_sensitive_file="$({
  git -C "$arbion_root" ls-files | grep -E '(^|/)(\.env|\.env\.production|id_rsa|id_ed25519)$|\.(pem|p12|pfx)$' || true
} | head -n 1)"
[[ -z "$tracked_sensitive_file" ]] || fail "sensitive file type is tracked: $tracked_sensitive_file"

grep -q 'kms_master_key_id = aws_kms_key.audit.arn' "$arbion_root/infrastructure/terraform/modules/audit/main.tf" ||
  fail "immutable audit storage is not protected by its customer-managed KMS key"
grep -q 's3_kms_key_arn.*aws_kms_key.audit.arn' "$arbion_root/infrastructure/terraform/modules/audit/main.tf" ||
  fail "AWS Config delivery does not require the audit KMS key"
grep -q 'kms_key_id.*aws_kms_key.audit.arn' "$arbion_root/infrastructure/terraform/modules/audit/main.tf" ||
  fail "CloudTrail or its security log group does not require the audit KMS key"
grep -q 'kms_master_key_id = aws_kms_key.alarms.arn' "$arbion_root/infrastructure/terraform/modules/observability/main.tf" ||
  fail "the operational and security alarm topic is not KMS encrypted"

exception_count=0
exception_today="$(date -u +%F)"
while IFS= read -r exception_line; do
  exception_expiry="${exception_line##*:exp:}"
  [[ "$exception_expiry" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}$ ]] ||
    fail "Trivy infrastructure exception has no valid expiry: $exception_line"
  [[ "$exception_expiry" > "$exception_today" ]] ||
    fail "Trivy infrastructure exception is expired: $exception_line"
  exception_count=$((exception_count + 1))
done < <(grep -Rh '^#trivy:ignore:.*:exp:' "$arbion_root/infrastructure/terraform" || true)
[[ "$exception_count" -eq 2 ]] ||
  fail "expected exactly two documented, time-bounded infrastructure design exceptions"

control_count="$(jq '.controls | length' "$catalog")"
implemented_count="$(jq '[.controls[] | select(.implementation_status == "IMPLEMENTED")] | length' "$catalog")"
partial_count="$(jq '[.controls[] | select(.implementation_status == "PARTIAL")] | length' "$catalog")"
operational_count="$(jq '[.controls[] | select(.implementation_status == "OPERATIONAL_ACTION_REQUIRED")] | length' "$catalog")"
external_count="$(jq '[.controls[] | select(.implementation_status == "EXTERNAL_CONFIGURATION_REQUIRED")] | length' "$catalog")"
repository_commit="${GITHUB_SHA:-$(git -C "$arbion_root" rev-parse HEAD)}"
generated_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

if [[ -n "$report_path" ]]; then
  report_parent="$(dirname -- "$report_path")"
  [[ -d "$report_parent" ]] || fail "report directory is unavailable: $report_parent"
  report_tmp="$(mktemp "$report_parent/.soc2-control-baseline.XXXXXX")"
  cleanup() {
    [[ ! -f "$report_tmp" ]] || unlink "$report_tmp"
  }
  trap cleanup EXIT
  jq -n \
    --arg schema_version "1.0" \
    --arg status "READINESS_BASELINE_NOT_CERTIFIED" \
    --arg generated_at "$generated_at" \
    --arg repository_commit "$repository_commit" \
    --argjson control_count "$control_count" \
    --argjson implemented_count "$implemented_count" \
    --argjson partial_count "$partial_count" \
    --argjson operational_action_required_count "$operational_count" \
    --argjson external_configuration_required_count "$external_count" \
    '{
      schema_version: $schema_version,
      status: $status,
      generated_at: $generated_at,
      repository_commit: $repository_commit,
      checks: "PASSED",
      control_count: $control_count,
      status_counts: {
        implemented: $implemented_count,
        partial: $partial_count,
        operational_action_required: $operational_action_required_count,
        external_configuration_required: $external_configuration_required_count
      }
    }' >"$report_tmp"
  mv -- "$report_tmp" "$report_path"
fi

echo "SOC 2 readiness baseline passed: $control_count controls; status READINESS_BASELINE_NOT_CERTIFIED."
