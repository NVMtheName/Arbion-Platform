#!/usr/bin/env bash
set -Eeuo pipefail

[[ "$#" -eq 1 ]] || {
  echo "Usage: $0 <failed-systemd-unit>" >&2
  exit 2
}

failed_unit="$1"
topic_arn="${ARBION_ALERT_TOPIC_ARN:?ARBION_ALERT_TOPIC_ARN is required}"

command -v aws >/dev/null || {
  echo "Required command not found: aws" >&2
  exit 1
}
[[ "$failed_unit" =~ ^[A-Za-z0-9@_.:-]+$ ]] || {
  echo "Invalid failed unit name." >&2
  exit 1
}
[[ "$topic_arn" =~ ^arn:aws:sns:[a-z0-9-]+:[0-9]{12}:[A-Za-z0-9_-]+$ ]] || {
  echo "ARBION_ALERT_TOPIC_ARN is not a valid SNS topic ARN." >&2
  exit 1
}

message="$(printf 'Arbion production service failure\nUnit: %s\nHost: %s\nTime: %s' \
  "$failed_unit" "$(hostname)" "$(date -u +%Y-%m-%dT%H:%M:%SZ)")"

aws sns publish \
  --topic-arn "$topic_arn" \
  --subject "Arbion production alert" \
  --message "$message" \
  --output text \
  --query MessageId >/dev/null

echo "Published Arbion production alert for $failed_unit."
