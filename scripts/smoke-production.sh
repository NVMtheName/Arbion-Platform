#!/usr/bin/env bash
set -euo pipefail

base="${ARBION_PUBLIC_URL:-https://www.arbion.ai}"
[[ "$base" == "https://www.arbion.ai" ]] || { echo "Refusing unexpected public URL" >&2; exit 1; }

curl --fail --silent --show-error --max-time 10 "$base/" >/dev/null
curl --fail --silent --show-error --max-time 10 "$base/healthz" >/dev/null
curl --fail --silent --show-error --max-time 10 "$base/readyz" >/dev/null
curl --fail --silent --show-error --max-time 10 "$base/login" >/dev/null
curl --fail --silent --show-error --max-time 10 "$base/register" >/dev/null
curl --fail --silent --show-error --max-time 10 "$base/brand/arbion-wordmark.svg" >/dev/null
curl --fail --silent --show-error --max-time 10 "$base/icon.svg" >/dev/null

journal_status="$(curl --silent --show-error --output /dev/null --write-out '%{http_code}' --max-time 10 "$base/api/decision-journal")"
[[ "$journal_status" == "401" ]] || { echo "Decision Journal endpoint is not protected" >&2; exit 1; }

headers="$(curl --fail --silent --show-error --head --max-time 10 "$base/login" | tr -d '\r')"
require_header() {
  local name="$1" expected="$2" actual
  actual="$(awk -v name="$name" 'tolower($1) == tolower(name ":") {sub(/^[^:]+:[[:space:]]*/, ""); print; exit}' <<<"$headers")"
  [[ "$actual" == "$expected" ]] || { echo "Missing or unexpected $name header" >&2; exit 1; }
}

require_header "Content-Security-Policy" "frame-ancestors 'none'; base-uri 'self'; form-action 'self'"
require_header "Permissions-Policy" "camera=(), microphone=(), geolocation=(), payment=()"
require_header "Referrer-Policy" "strict-origin-when-cross-origin"
require_header "Strict-Transport-Security" "max-age=31536000; includeSubDomains"
require_header "X-Content-Type-Options" "nosniff"
require_header "X-Frame-Options" "DENY"
require_header "X-Permitted-Cross-Domain-Policies" "none"
if grep -Eiq '^(server|via|x-powered-by):' <<<"$headers"; then
  echo "Production response exposes an identifying server header" >&2
  exit 1
fi

redirect="$(curl --silent --show-error --head --max-time 10 http://arbion.ai/ | tr -d '\r' | awk 'tolower($1) == "location:" {print $2}')"
[[ "$redirect" == "https://www.arbion.ai/" ]] || { echo "Unexpected HTTP redirect target" >&2; exit 1; }
echo "Public production smoke checks passed."
