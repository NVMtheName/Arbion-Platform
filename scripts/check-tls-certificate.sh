#!/usr/bin/env bash
set -Eeuo pipefail

tls_host="${ARBION_TLS_HOST:-www.arbion.ai}"
minimum_valid_seconds="${ARBION_TLS_MIN_VALID_SECONDS:-1209600}"

[[ "$tls_host" == "www.arbion.ai" ]] || {
  echo "Refusing unexpected TLS host." >&2
  exit 1
}
[[ "$minimum_valid_seconds" =~ ^[0-9]+$ && "$minimum_valid_seconds" -gt 0 ]] || {
  echo "ARBION_TLS_MIN_VALID_SECONDS must be a positive integer." >&2
  exit 1
}

certificate="$(openssl s_client -servername "$tls_host" -connect "$tls_host:443" </dev/null 2>/dev/null)"
[[ -n "$certificate" ]] || {
  echo "Could not retrieve the production TLS certificate." >&2
  exit 1
}

expiry="$(printf '%s\n' "$certificate" | openssl x509 -noout -enddate)" || {
  echo "Could not parse the production TLS certificate." >&2
  exit 1
}

if ! printf '%s\n' "$certificate" | openssl x509 -noout -checkend "$minimum_valid_seconds" >/dev/null; then
  echo "The production TLS certificate expires within the configured warning window: $expiry" >&2
  exit 1
fi

echo "Production TLS certificate validity check passed: $expiry"
