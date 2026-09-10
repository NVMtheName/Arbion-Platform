#!/usr/bin/env bash
set -Eeuo pipefail

repo_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)"
test_root="$(mktemp -d "${TMPDIR:-/tmp}/arbion-soc2-controls-test.XXXXXX")"
cleanup() {
  if [[ -d "$test_root" ]]; then
    find "$test_root" -depth -delete
  fi
}
trap cleanup EXIT

"$repo_root/scripts/check-soc2-controls.sh" "$test_root/report.json" >/dev/null
jq -e '
  .schema_version == "1.0" and
  .status == "READINESS_BASELINE_NOT_CERTIFIED" and
  .checks == "PASSED" and
  .control_count >= 15 and
  .status_counts.implemented > 0 and
  .status_counts.partial > 0 and
  .status_counts.operational_action_required > 0 and
  .status_counts.external_configuration_required > 0
' "$test_root/report.json" >/dev/null

jq 'del(.controls[] | select(.id == "IR-01"))' \
  "$repo_root/compliance/soc2-controls.json" >"$test_root/missing-control.json"
if ARBION_SOC2_CONTROL_CATALOG="$test_root/missing-control.json" \
  "$repo_root/scripts/check-soc2-controls.sh" >"$test_root/missing-output" 2>&1; then
  echo "SOC 2 checker accepted a catalog without required IR-01." >&2
  exit 1
fi
grep -q 'required control is missing: IR-01' "$test_root/missing-output"

jq '.program_status = "SOC2_CERTIFIED"' \
  "$repo_root/compliance/soc2-controls.json" >"$test_root/false-claim.json"
if ARBION_SOC2_CONTROL_CATALOG="$test_root/false-claim.json" \
  "$repo_root/scripts/check-soc2-controls.sh" >"$test_root/claim-output" 2>&1; then
  echo "SOC 2 checker accepted an unsupported certification status." >&2
  exit 1
fi
grep -q 'catalog schema, scope, or control fields are invalid' "$test_root/claim-output"

if bash "$repo_root/scripts/collect-soc2-external-evidence.sh" "$repo_root" >"$test_root/collector-output" 2>&1; then
  echo "SOC 2 collector accepted an evidence destination inside the repository." >&2
  exit 1
fi
grep -q 'evidence output must be outside the Git repository' "$test_root/collector-output"

echo "SOC 2 control baseline tests passed."
