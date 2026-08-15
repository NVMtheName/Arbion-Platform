#!/usr/bin/env bash
set -Eeuo pipefail

[[ "$EUID" -eq 0 ]] || {
  echo "Production container health checks must run as root." >&2
  exit 1
}

arbion_root="${ARBION_ROOT:-/opt/arbion}"
env_file="${ARBION_PRODUCTION_ENV_FILE:-$arbion_root/.env.production}"
expected_services="${ARBION_EXPECTED_SERVICES:-ai api postgres proxy redis web}"

[[ -d "$arbion_root" && -r "$arbion_root/docker-compose.prod.yml" ]] || {
  echo "Arbion production deployment files are unavailable." >&2
  exit 1
}
[[ -r "$env_file" ]] || {
  echo "Arbion production environment file is unavailable." >&2
  exit 1
}
[[ "$expected_services" =~ ^[a-z0-9][a-z0-9_-]*(\ [a-z0-9][a-z0-9_-]*)*$ ]] || {
  echo "ARBION_EXPECTED_SERVICES must be a space-separated service list." >&2
  exit 1
}
for required_command in docker jq; do
  command -v "$required_command" >/dev/null || {
    echo "Required command not found: $required_command" >&2
    exit 1
  }
done

cd "$arbion_root"
compose=(docker compose --env-file "$env_file" -f docker-compose.prod.yml)
expected_list="$(tr ' ' '\n' <<<"$expected_services" | LC_ALL=C sort)"
running_list="$("${compose[@]}" ps --status running --services | LC_ALL=C sort)"

if [[ "$running_list" != "$expected_list" ]]; then
  running_summary="${running_list//$'\n'/, }"
  [[ -n "$running_summary" ]] || running_summary="none"
  echo "Production container set is incomplete; running services: $running_summary." >&2
  exit 1
fi

unhealthy_services="$(
  "${compose[@]}" ps --format json |
    jq -r 'select(.State != "running" or ((.Health // "") != "" and .Health != "healthy")) | .Service' |
    LC_ALL=C sort -u
)"
if [[ -n "$unhealthy_services" ]]; then
  echo "Production containers are unhealthy: ${unhealthy_services//$'\n'/, }." >&2
  exit 1
fi

echo "Production container health check passed for the expected service set."
