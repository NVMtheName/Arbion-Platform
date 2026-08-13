#!/usr/bin/env bash
set -euo pipefail

base="${ARBION_PUBLIC_URL:-https://www.arbion.ai}"
[[ "$base" == "https://www.arbion.ai" ]] || { echo "Refusing unexpected public URL" >&2; exit 1; }

curl --fail --silent --show-error --max-time 10 "$base/" >/dev/null
curl --fail --silent --show-error --max-time 10 "$base/healthz" >/dev/null
curl --fail --silent --show-error --max-time 10 "$base/readyz" >/dev/null
curl --fail --silent --show-error --max-time 10 "$base/login" >/dev/null
curl --fail --silent --show-error --max-time 10 "$base/register" >/dev/null
redirect="$(curl --silent --show-error --head --max-time 10 http://arbion.ai/ | tr -d '\r' | awk 'BEGIN{IGNORECASE=1} /^location:/{print $2}')"
[[ "$redirect" == "https://www.arbion.ai/" ]] || { echo "Unexpected HTTP redirect target" >&2; exit 1; }
echo "Public production smoke checks passed."
