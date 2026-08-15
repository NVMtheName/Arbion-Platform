#!/usr/bin/env bash
set -Eeuo pipefail

usage() {
  echo "Usage: $0 <backup.dump> <backup.dump.sha256>" >&2
  exit 2
}

[[ "$#" -eq 2 ]] || usage
dump_path="$1"
checksum_path="$2"

[[ -f "$dump_path" && -f "$checksum_path" ]] || {
  echo "Backup dump and checksum must both exist." >&2
  exit 1
}

for command in docker openssl sha256sum; do
  command -v "$command" >/dev/null || {
    echo "Required command not found: $command" >&2
    exit 1
  }
done

expected_checksum="$(awk 'NR == 1 {print $1}' "$checksum_path")"
actual_checksum="$(sha256sum "$dump_path" | awk '{print $1}')"
[[ "$expected_checksum" =~ ^[[:xdigit:]]{64}$ && "$actual_checksum" == "$expected_checksum" ]] || {
  echo "Backup checksum verification failed." >&2
  exit 1
}

suffix="$(openssl rand -hex 6)"
container="arbion-restore-drill-$suffix"
volume="arbion-restore-drill-$suffix-data"
restore_password="$(openssl rand -hex 24)"

cleanup() {
  docker container inspect "$container" >/dev/null 2>&1 && docker rm -f "$container" >/dev/null
  docker volume inspect "$volume" >/dev/null 2>&1 && docker volume rm "$volume" >/dev/null
}
trap cleanup EXIT

docker volume create "$volume" >/dev/null
docker run -d \
  --name "$container" \
  --network none \
  -e POSTGRES_PASSWORD="$restore_password" \
  -e POSTGRES_DB=arbion_restore \
  -v "$volume:/var/lib/postgresql/data" \
  postgres:17-alpine >/dev/null

for attempt in {1..40}; do
  if docker exec "$container" pg_isready -U postgres -d arbion_restore >/dev/null 2>&1; then
    break
  fi
  sleep 2
done
docker exec "$container" pg_isready -U postgres -d arbion_restore >/dev/null

docker cp "$dump_path" "$container:/tmp/backup.dump"
docker exec "$container" pg_restore --list /tmp/backup.dump >/dev/null
docker exec "$container" pg_restore \
  --exit-on-error \
  --no-owner \
  --no-privileges \
  -U postgres \
  -d arbion_restore \
  /tmp/backup.dump

docker exec -i "$container" psql -U postgres -d arbion_restore -v ON_ERROR_STOP=1 >/dev/null <<'SQL'
DO $$
DECLARE
  required_table text;
BEGIN
  FOREACH required_table IN ARRAY ARRAY[
    'users',
    'user_entitlements',
    'provider_connections',
    'financial_accounts',
    'automation_mandates',
    'risk_circuit_breakers',
    'strategy_instances',
    'audit_events',
    'goose_db_version'
  ]
  LOOP
    IF to_regclass('public.' || required_table) IS NULL THEN
      RAISE EXCEPTION 'required table is missing: %', required_table;
    END IF;
  END LOOP;
END
$$;
SQL

table_count="$(docker exec "$container" psql -U postgres -d arbion_restore -Atc \
  "select count(*) from information_schema.tables where table_schema = 'public'")"
echo "PostgreSQL restore drill passed with $table_count public tables."
