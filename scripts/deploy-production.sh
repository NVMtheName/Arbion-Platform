#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."
env_file="${ARBION_PRODUCTION_ENV_FILE:-.env.production}"
compose=(docker compose --env-file "$env_file" -f docker-compose.prod.yml)

[[ -f "$env_file" ]] || { echo "Missing production environment file: $env_file" >&2; exit 1; }
for command in docker curl; do command -v "$command" >/dev/null || { echo "Required command not found: $command" >&2; exit 1; }; done

# Compose expands and validates required variables without printing their values.
"${compose[@]}" config --quiet
"${compose[@]}" build
"${compose[@]}" up -d postgres redis
"${compose[@]}" up --no-deps migrate
"${compose[@]}" up -d ai api web proxy

for attempt in {1..30}; do
  if curl --fail --silent --show-error --max-time 5 https://www.arbion.ai/readyz >/dev/null; then
    echo "Production readiness check passed."
    exit 0
  fi
  sleep 10
done
echo "Production readiness check failed; inspect container logs without exposing secrets." >&2
exit 1
