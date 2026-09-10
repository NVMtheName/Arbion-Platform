#!/usr/bin/env bash
set -Eeuo pipefail

umask 077

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
repository_root="$(cd -- "$script_dir/.." && pwd -P)"
output_parent_input="${1:-}"

fail() {
  echo "SOC 2 external evidence collection failed: $*" >&2
  exit 1
}

[[ -n "$output_parent_input" ]] || fail "usage: $0 <existing-output-parent-outside-the-repository>"
for command in awk aws gh git jq mv openssl; do
  command -v "$command" >/dev/null || fail "required command not found: $command"
done
[[ -d "$output_parent_input" ]] || fail "output parent does not exist: $output_parent_input"
output_parent="$(cd -- "$output_parent_input" && pwd -P)"
case "$output_parent/" in
  "$repository_root/"*) fail "evidence output must be outside the Git repository" ;;
esac

collected_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
collection_id="arbion-soc2-${collected_at//[-:]/}"
collection_dir="$output_parent/$collection_id"
[[ ! -e "$collection_dir" ]] || fail "collection path already exists: $collection_dir"
mkdir -m 700 -- "$collection_dir"

declare -a unavailable=()

record_unavailable() {
  local name="$1"
  local system="$2"
  local reason="$3"
  unavailable+=("$name")
  jq -n \
    --arg status "UNAVAILABLE" \
    --arg system "$system" \
    --arg reason "$reason" \
    --arg collected_at "$collected_at" \
    '{status: $status, system: $system, reason: $reason, collected_at: $collected_at}' \
    >"$collection_dir/$name.json"
}

capture_json() {
  local name="$1"
  local system="$2"
  shift 2
  local temporary="$collection_dir/.$name.tmp"

  if "$@" >"$temporary" 2>/dev/null && jq -e . "$temporary" >/dev/null 2>&1; then
    jq -S . "$temporary" >"$collection_dir/$name.json"
    unlink "$temporary"
    return 0
  fi

  [[ ! -e "$temporary" ]] || unlink "$temporary"
  record_unavailable "$name" "$system" "The read-only API call failed, returned non-JSON, or the control is not configured. Review authentication and the external setting."
  return 0
}

capture_github_collaborators() {
  local temporary="$collection_dir/.github-collaborators.tmp"
  if gh api --paginate --slurp \
    "repos/$github_repository/collaborators?affiliation=all&per_page=100" \
    >"$temporary" 2>/dev/null &&
    jq -e 'type == "array" and all(.[]; type == "array")' "$temporary" >/dev/null 2>&1; then
    jq -S '[.[][] | {login: .login, id: .id, role_name: .role_name, permissions: .permissions}]' \
      "$temporary" >"$collection_dir/github-collaborators.json"
    unlink "$temporary"
    return 0
  fi

  [[ ! -e "$temporary" ]] || unlink "$temporary"
  record_unavailable github-collaborators GITHUB "The paginated read-only collaborator export failed or returned non-JSON. Review authentication and repository administration access."
  return 0
}

capture_guardduty_status() {
  local source="$collection_dir/aws-guardduty-detectors.json"
  local details="$collection_dir/.guardduty-details.tmp"
  local response="$collection_dir/.guardduty-response.tmp"
  local combined="$collection_dir/.guardduty-combined.tmp"
  local detector_id
  capture_json aws-guardduty-detectors AWS aws guardduty list-detectors --region "$aws_region" --output json
  if ! jq -e '(.DetectorIds | type == "array") and
    all(.DetectorIds[]; type == "string" and test("^[0-9a-f]{32}$")) and
    ((.DetectorIds | unique | length) == (.DetectorIds | length)) and
    (.DetectorIds | length <= 100)' "$source" >/dev/null; then
    # Keep API failures explicit; never turn an incomplete inventory into an empty one.
    if ! jq -e '.status == "UNAVAILABLE"' "$source" >/dev/null; then
      record_unavailable aws-guardduty-detectors AWS "Detector inventory was malformed or exceeded the bounded review limit."
    fi
    return 0
  fi
  : >"$details"
  while IFS= read -r detector_id; do
    if ! aws guardduty get-detector --detector-id "$detector_id" --region "$aws_region" \
      --query '{Status:Status}' --output json >"$response" 2>/dev/null ||
      ! jq -e '.Status | IN("ENABLED", "DISABLED")' "$response" >/dev/null 2>&1; then
      unlink "$details"
      [[ ! -e "$response" ]] || unlink "$response"
      record_unavailable aws-guardduty-detectors AWS "A detector status could not be verified. Detector existence is not evidence that monitoring is enabled."
      return 0
    fi
    jq --arg id "$detector_id" '{DetectorId: $id, Status: .Status}' "$response" >>"$details"
  done < <(jq -r '.DetectorIds[]' "$source")
  jq -s . "$details" >"$combined"
  jq -S --slurpfile detectors "$combined" '. + {Detectors: $detectors[0]}' "$source" >"$details"
  mv -- "$details" "$source"
  unlink "$combined"
  [[ ! -e "$response" ]] || unlink "$response"
}

github_repository="${ARBION_GITHUB_REPOSITORY:-}"
if gh auth status >/dev/null 2>&1; then
  if [[ -z "$github_repository" ]]; then
    github_remote="$(git -C "$repository_root" config --get remote.origin.url 2>/dev/null || true)"
    case "$github_remote" in
      git@github.com:*) github_repository="${github_remote#git@github.com:}" ;;
      https://github.com/*) github_repository="${github_remote#https://github.com/}" ;;
      *) github_repository="" ;;
    esac
    github_repository="${github_repository%.git}"
  fi
  if [[ "$github_repository" =~ ^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$ ]]; then
    capture_json github-identity GITHUB gh api user --jq '{login: .login, id: .id}'
    capture_json github-repository GITHUB gh api "repos/$github_repository" --jq '{full_name: .full_name, visibility: .visibility, default_branch: .default_branch, archived: .archived, web_commit_signoff_required: .web_commit_signoff_required, security_and_analysis: .security_and_analysis}'
    capture_json github-main-protection GITHUB gh api "repos/$github_repository/branches/main/protection"
    capture_json github-rulesets GITHUB gh api "repos/$github_repository/rulesets?includes_parents=true"
    capture_json github-production-environment GITHUB gh api "repos/$github_repository/environments/production"
    capture_json github-actions-permissions GITHUB gh api "repos/$github_repository/actions/permissions/workflow"
    capture_github_collaborators
  else
    record_unavailable github GITHUB "Could not resolve an exact owner/repository name. Set ARBION_GITHUB_REPOSITORY."
  fi
else
  record_unavailable github GITHUB "GitHub CLI authentication is unavailable. Run gh auth login with read access to repository administration and security settings."
fi

aws_region="${AWS_REGION:-${AWS_DEFAULT_REGION:-}}"
if [[ -z "$aws_region" ]]; then
  aws_region="$(aws configure get region 2>/dev/null || true)"
fi
aws_region="${aws_region:-us-east-1}"
aws_identity="$collection_dir/.aws-identity.tmp"

if aws sts get-caller-identity --output json >"$aws_identity" 2>/dev/null && jq -e '.Account and .Arn' "$aws_identity" >/dev/null 2>&1; then
  jq -S '{Account, Arn, UserId}' "$aws_identity" >"$collection_dir/aws-identity.json"
  aws_account_id="$(jq -r .Account "$aws_identity")"
  unlink "$aws_identity"

  environment_name="${ARBION_ENVIRONMENT:-production}"
  resource_prefix="arbion-$environment_name"
  trail_name="$resource_prefix-management"
  event_rule_name="$resource_prefix-guardduty-findings"
  alarm_topic_arn="arn:aws:sns:$aws_region:$aws_account_id:$resource_prefix-alarms"
  audit_bucket="$resource_prefix-$aws_account_id-$aws_region-audit"
  backup_bucket="arbion-production-backups-$aws_account_id-$aws_region"

  capture_json aws-cloudtrail-trails AWS aws cloudtrail describe-trails --include-shadow-trails --region "$aws_region" --output json
  capture_json aws-cloudtrail-status AWS aws cloudtrail get-trail-status --name "$trail_name" --region "$aws_region" --output json
  capture_json aws-cloudtrail-selectors AWS aws cloudtrail get-event-selectors --trail-name "$trail_name" --region "$aws_region" --output json
  capture_json aws-config-recorders AWS aws configservice describe-configuration-recorders --region "$aws_region" --output json
  capture_json aws-config-recorder-status AWS aws configservice describe-configuration-recorder-status --region "$aws_region" --output json
  capture_json aws-config-delivery-channels AWS aws configservice describe-delivery-channels --region "$aws_region" --output json
  capture_guardduty_status
  capture_json aws-access-analyzers AWS aws accessanalyzer list-analyzers --type ACCOUNT --region "$aws_region" --output json
  capture_json aws-security-event-rule AWS aws events describe-rule --name "$event_rule_name" --region "$aws_region" --output json
  capture_json aws-security-event-targets AWS aws events list-targets-by-rule --rule "$event_rule_name" --region "$aws_region" --output json
  capture_json aws-alarm-topic-subscriptions AWS aws sns list-subscriptions-by-topic --topic-arn "$alarm_topic_arn" --region "$aws_region" --query 'Subscriptions[].{SubscriptionArn:SubscriptionArn,Protocol:Protocol,Owner:Owner}' --output json
  capture_json aws-cloudwatch-alarms AWS aws cloudwatch describe-alarms --alarm-name-prefix "$resource_prefix" --region "$aws_region" --query 'MetricAlarms[].{AlarmName:AlarmName,StateValue:StateValue,ActionsEnabled:ActionsEnabled,AlarmActions:AlarmActions,MetricName:MetricName,Namespace:Namespace,Updated:AlarmConfigurationUpdatedTimestamp}' --output json
  capture_json aws-lightsail-instances AWS aws lightsail get-instances --region "$aws_region" --query 'instances[].{name:name,arn:arn,state:state.name,blueprintId:blueprintId,bundleId:bundleId,createdAt:createdAt,isStaticIp:isStaticIp}' --output json
  capture_json aws-lightsail-alarms AWS aws lightsail get-alarms --region "$aws_region" --query 'alarms[].{name:name,state:state,metricName:metricName,notificationTriggers:notificationTriggers,notificationEnabled:notificationEnabled,contactProtocols:contactProtocols,createdAt:createdAt,monitoredResourceInfo:monitoredResourceInfo}' --output json

  for bucket_role in audit backup; do
    if [[ "$bucket_role" == "audit" ]]; then
      bucket_name="$audit_bucket"
    else
      bucket_name="$backup_bucket"
    fi
    capture_json "aws-$bucket_role-bucket-public-access" AWS aws s3api get-public-access-block --bucket "$bucket_name" --region "$aws_region" --output json
    capture_json "aws-$bucket_role-bucket-encryption" AWS aws s3api get-bucket-encryption --bucket "$bucket_name" --region "$aws_region" --output json
    capture_json "aws-$bucket_role-bucket-versioning" AWS aws s3api get-bucket-versioning --bucket "$bucket_name" --region "$aws_region" --output json
    capture_json "aws-$bucket_role-bucket-object-lock" AWS aws s3api get-object-lock-configuration --bucket "$bucket_name" --region "$aws_region" --output json
    capture_json "aws-$bucket_role-bucket-lifecycle" AWS aws s3api get-bucket-lifecycle-configuration --bucket "$bucket_name" --region "$aws_region" --output json
  done
else
  [[ ! -e "$aws_identity" ]] || unlink "$aws_identity"
  record_unavailable aws AWS "AWS CLI authentication is unavailable. Refresh the approved production SSO profile before collecting evidence."
fi

if ((${#unavailable[@]} == 0)); then
  collection_status="COMPLETE_REVIEW_REQUIRED"
  unavailable_json='[]'
else
  collection_status="INCOMPLETE"
  unavailable_json="$(printf '%s\n' "${unavailable[@]}" | jq -Rsc 'split("\n")[:-1]')"
fi

jq -n \
  --arg schema_version "1.0" \
  --arg collection_id "$collection_id" \
  --arg collected_at "$collected_at" \
  --arg status "$collection_status" \
  --arg repository "${github_repository:-UNAVAILABLE}" \
  --arg aws_region "$aws_region" \
  --argjson unavailable "$unavailable_json" \
  '{schema_version: $schema_version, collection_id: $collection_id, collected_at: $collected_at, status: $status, repository: $repository, aws_region: $aws_region, unavailable_sources: $unavailable, limitations: ["Read-only point-in-time configuration snapshot", "Requires reviewer evaluation", "Contains no secret-scanning alert payloads, notification endpoints, customer records, credentials, or CloudTrail event bodies", "Does not by itself establish operating effectiveness or SOC 2 certification"]}' \
  >"$collection_dir/collection-summary.json"

(
  cd -- "$collection_dir"
  export LC_ALL=C
  for evidence_file in ./*.json; do
    evidence_digest="$(openssl dgst -sha256 "$evidence_file" | awk '{print $NF}')"
    [[ "$evidence_digest" =~ ^[0-9a-f]{64}$ ]] || fail "could not calculate a canonical SHA-256 digest"
    printf '%s  %s\n' "$evidence_digest" "${evidence_file#./}"
  done
) >"$collection_dir/SHA256SUMS"

echo "SOC 2 external evidence snapshot: $collection_dir"
echo "Collection status: $collection_status"
[[ "$collection_status" == "COMPLETE_REVIEW_REQUIRED" ]] || exit 2
