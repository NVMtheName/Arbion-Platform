# Production deployment runbook

## Status, topology, and safety boundary

This repository is prepared for operation; it does **not** claim Arbion is deployed. Existing PAPER simulation, SHADOW intent recording, and read-only real Schwab data may be served. There is no scheduler, broker-write interface, order-submission adapter, live-trading implementation, or live feature flag.

```text
Internet :80/:443 -> Caddy (only published service)
  /api/*, /healthz, /readyz -> Go API :8080
  everything else           -> Next.js :3000
Private Docker network: Go, Neural Engine :8000, PostgreSQL :5432, Redis :6379
```

API, AI, PostgreSQL, and Redis have no host port mappings. Outbound provider access remains possible. Financial credentials flow only through Go/Vault to Schwab, never Python.

## DNS and host prerequisites

Use a patched Linux host with Docker Engine, Compose v2, `curl`, `jq`, `openssl`, durable disk, and inbound TCP 80/443 plus UDP 443. Point both `www.arbion.ai` and `arbion.ai` at the separately chosen host—no IP is hard-coded here. Caddy obtains/renews HTTPS certificates, redirects HTTP to HTTPS, and redirects the apex to `https://www.arbion.ai`. Preserve its certificate volume.

## Environment and secret generation

Copy `.env.production.example` to ignored `.env.production` and populate it only on the host:

- `ARBION_ENV=production`;
- PostgreSQL database/user and a strong `POSTGRES_PASSWORD`;
- `DATABASE_URL` with matching non-development credentials. Bundled private PostgreSQL may use `postgres://...@postgres:5432/arbion?sslmode=disable` only inside this host; external databases must use TLS;
- `REDIS_URL=redis://redis:6379/0`;
- `CREDENTIAL_ENCRYPTION_KEY`, generated once with `openssl rand -base64 32`;
- `AI_INTERNAL_SERVICE_TOKEN`, generated with `openssl rand -base64 48`;
- `AUTH_ALLOWED_ORIGINS=https://www.arbion.ai`;
- `REGISTRATION_ALLOWLIST`, containing the comma-separated normalized email addresses permitted to register. Production is default-deny when this value is blank;
- explicit `FOUNDER_EMAIL` only for bootstrap; and
- when Schwab is enabled, both client values and `SCHWAB_REDIRECT_URI=https://www.arbion.ai/api/connections/financial/schwab/callback`.

Compose rejects absent required values. Go rejects invalid URLs, known development database/key/token values, weak AI tokens, wildcard/noncanonical origins, and partial or misdirected Schwab settings. Python independently rejects missing/weak production internal authentication. Never print, commit, or send the environment file to support.

The encryption key must be cryptographically random, generated once, securely backed up, never committed, and never casually rotated. Losing it may make encrypted provider credentials unreadable. There is no automatic key rotation.

## Release, migrations, and deploy helper

The safe sequence is: review and back up; validate Compose; build images; start healthy PostgreSQL/Redis; run the one-shot embedded Goose migrations exactly once; start AI/API/Web; wait for health; start Caddy; verify public health. Migration failure stops deployment and the API's completed-migration dependency prevents a healthy release. Runtime schema creation is not used.

```bash
docker compose --env-file .env.production -f docker-compose.prod.yml config --quiet
ARBION_PRODUCTION_ENV_FILE=.env.production ./scripts/deploy-production.sh
./scripts/smoke-production.sh
```

The deploy script fails fast, does not echo secrets, delete volumes, or overwrite data. Never run `docker compose down -v` in production.

## Health checks and logging

Production containers use Docker's local `json-file` logging with five rotated 10 MiB files per container. This bounds routine container logs to approximately 50 MiB per service while preserving recent diagnostics; export longer-lived security or audit records to a dedicated system rather than increasing local retention indefinitely.

Install and enable `arbion-docker-build-cache-prune.service` and its timer on the single host. It runs weekly with a randomized delay and removes only build cache unused for at least seven days; it does not prune containers, application images, or volumes. Also install and enable `arbion-host-capacity.service` and its timer. Every 30 minutes it checks disk and inode utilization for the filesystem containing `/var/lib/docker` and fails at 85% utilization so the operations alert path warns before storage exhaustion. Install and enable `arbion-memory-pressure.service` and its timer to check every five minutes and alert when Linux reports less than 10% of host memory as available; its status also reports swap use without exposing process data. Install and enable `arbion-production-containers.service` and its timer to verify every five minutes that the six long-running Compose services are present and that each configured Docker health check is healthy. Install and enable `arbion-reboot-required.service` and its timer to alert daily when unattended package updates require an operator-reviewed reboot. Install and enable `arbion-tls-certificate.service` and its timer to alert daily if the public certificate cannot be read or has fewer than 14 days of validity remaining.

- Go `/healthz` reports liveness and `/readyz` checks database readiness.
- Python provides `/healthz` and startup-validated `/readyz`.
- Next.js provides `/api/health`; PostgreSQL uses `pg_isready`; Redis uses `PING`.
- Caddy waits on healthy API/Web and exposes Go health paths through the public origin.

Checks have start periods, bounded timeouts, and nonaggressive intervals. Logs remain container stdout/stderr. Never dump environments or log Schwab secrets/codes/tokens, AI keys, encryption keys, cookies, internal tokens, or database passwords.

## External host failure monitoring

Host-local checks cannot notify when the entire instance or its network is unavailable. For a Lightsail host, configure a regional email notification contact and verify it separately from the application SNS subscription. Add these Lightsail instance alarms:

- `arbion-production-status-check-failed`: `StatusCheckFailed` sum greater than `0` for two of two five-minute periods, with missing data treated as breaching;
- `arbion-production-cpu-high`: average `CPUUtilization` greater than `80%` for three of three five-minute periods, with missing data treated as not breaching; and
- `arbion-production-burst-capacity-low`: average `BurstCapacityPercentage` less than `20%` for two of two five-minute periods, with missing data treated as not breaching.

Tag every alarm for the production environment, enable email notifications for both `ALARM` and recovery to `OK`, and confirm that each initial `INSUFFICIENT_DATA` state settles to `OK`. Review recent utilization before changing performance thresholds. After the email contact is verified and an alarm is `OK`, exercise notification delivery without stopping the instance:

```bash
aws lightsail test-alarm --alarm-name arbion-production-status-check-failed --state ALARM
aws lightsail test-alarm --alarm-name arbion-production-status-check-failed --state OK
```

The alarm test changes only the simulated alarm state. Confirm receipt of both the alert and all-clear emails, then recheck the actual alarm state and production health.

## Cost guardrail

For the owner-operated Lightsail topology, configure a notification-only monthly AWS cost budget without automated actions. The current `$44` instance baseline uses a `$60` account-wide budget so ordinary backup and notification usage has headroom while unexpected resources or traffic still surface promptly. Send direct operator email notifications at:

- `80%` of actual monthly spend;
- `100%` of actual monthly spend; and
- `100%` of forecasted monthly spend.

Keep the budget account-wide so it also catches costs outside Lightsail, tag it for Arbion production, and review it whenever the hosting plan or expected usage changes. Budget data and forecasts are delayed; this is a spending warning, not a real-time limit. Do not attach an automatic shutdown action to production.

## Founder bootstrap

Register the intended account normally, explicitly configure `FOUNDER_EMAIL`, then run:

```bash
docker compose --env-file .env.production -f docker-compose.prod.yml run --rm --entrypoint /bootstrap-founder api
```

The existing command fails if the account is absent, is idempotent, promotes only the explicit address, and writes an audit event. It is never automatic.

## Registration access

Production registration is always restricted by `REGISTRATION_ALLOWLIST`. Matching is case-insensitive after trimming and normalization. A missing or blank list denies every new registration; existing accounts can still sign in. Rejected attempts are rate-limited, audited without the submitted address, and return the same generic response used for an unavailable registration. Add a tester's exact email to the host-only list and restart the API before inviting them; remove it after registration if no further account creation is expected.

## PostgreSQL backup, restore, and Redis loss

`postgres-production-data` is durable product truth. Configure encrypted, off-host, retention-managed backups before launch. The single-host deployment uses `scripts/backup-postgres.sh` from a root-owned systemd service. It creates a PostgreSQL custom-format dump with mode `0600`, validates the archive catalog, writes a SHA-256 checksum, uploads both files over TLS with S3 AES-256 server-side encryption, and removes the local staging copy. The dedicated bucket blocks public access, enforces TLS and AES-256 uploads, enables versioning, applies 35-day S3 Object Lock governance retention, and expires daily objects after 45 days. The host identity should have write-only access to the dedicated backup prefix: no object read or delete permission.

Install `deploy/systemd/arbion-postgres-backup.service` and its timer under `/etc/systemd/system`. Put the dedicated S3 writer credentials and these settings in root-owned `/etc/arbion/postgres-backup.env` with mode `0600`:

```bash
AWS_ACCESS_KEY_ID=replace-on-host
AWS_SECRET_ACCESS_KEY=replace-on-host
AWS_DEFAULT_REGION=us-east-1
AWS_REGION=us-east-1
AWS_EC2_METADATA_DISABLED=true
ARBION_BACKUP_BUCKET=replace-with-dedicated-bucket
ARBION_BACKUP_PREFIX=postgres/daily
```

Enable the backup timer and run the backup service once immediately. Each completed upload records a root-only local success marker. Also install and enable `arbion-postgres-backup-freshness.service` and its timer; it checks every six hours and enters a failed state when no successful upload has been recorded within 36 hours. Confirm that both the dump and checksum exist remotely, use the required encryption, and are covered by the bucket retention controls. Monitor failures with `systemctl --failed`, `systemctl status arbion-postgres-backup.service arbion-postgres-backup-freshness.service`, and their journals; logs contain object names but no database contents or credentials.

For external failure delivery, create a dedicated SNS topic and a confirmed operator subscription. Grant the host backup identity only `sns:Publish` to that exact topic, put `ARBION_ALERT_TOPIC_ARN=<topic-arn>` in root-owned `/etc/arbion/ops-alert.env`, and install `arbion-ops-alert@.service`. Backup, freshness-monitor, host-capacity, container-health, public-health, and Docker-cache-maintenance failures invoke the publisher with only the unit name, host name, and UTC timestamp; alerts contain no database contents or credentials.

Backups contain sensitive customer/product data. Restore only in a planned outage to an empty, version-compatible PostgreSQL instance: preserve the failed database, download with an authorized recovery identity, validate the SHA-256 checksum and archive catalog, restore with reviewed `pg_restore` arguments, rerun migrations, and verify readiness and inventory. Rehearse this process after setup and periodically thereafter. Never grant the host read/delete access, overwrite the only production copy, or delete production volumes as recovery.

After downloading a dump and its checksum with an authorized recovery identity, rehearse the isolated validation with `scripts/verify-postgres-restore.sh <backup.dump> <backup.dump.sha256>`. The script verifies the checksum, disables networking on a temporary PostgreSQL 17 container, restores with ownership and privileges excluded, checks critical tables, and removes its temporary container and volume on exit. It never connects to the production database or production Docker volumes.

Redis AOF improves continuity but Redis is ephemeral. Loss ends active sessions and pending OAuth flows must restart. Users, provider connections, mandates, strategy instances, paper portfolios, and durable automation state remain in PostgreSQL.

## Rollback

Retain the prior reviewed revision/images and a verified pre-release backup. Roll application code back without deleting volumes. Migrations are forward-managed; do not improvise destructive schema downgrades. For incompatibility, stop traffic and use the migration-specific reviewed recovery or verified backup restoration plan.

## Public and Schwab smoke tests

`scripts/smoke-production.sh` checks HTTPS root, API health/readiness, login/register pages, security headers, and the apex HTTP redirect. It never signs in to Schwab. Install and enable `arbion-production-health.service` and its timer to run these public checks every five minutes and invoke the operations alert path on failure. Inspect session cookies for `Secure`, `HttpOnly`, `SameSite=Lax`, and `Path=/`, and verify foreign origins are rejected.

Manual Schwab test (never place an order):

1. Sign into Arbion and open **Settings → Connections**.
2. Click **Connect Schwab** and confirm the redirect reaches Schwab.
3. Authenticate directly with Schwab and authorize Arbion.
4. Confirm return to `https://www.arbion.ai/api/connections/financial/schwab/callback` and a successful connection display.
5. Confirm discovered accounts, balances, and positions.
6. Run **Sync** and confirm there is no duplicate account inventory.
7. Confirm UI distinctions among PAPER, SHADOW, and real read-only Schwab data.

## Exact first-host checklist

1. Provision/patch Linux; install Docker/Compose; permit public 80/443 only (plus restricted administration).
2. Clone the reviewed commit and point apex/`www` DNS to the host.
3. Create `.env.production`; generate, store, and separately back up all secrets. Register the exact Schwab callback if enabled.
4. Establish encrypted off-host PostgreSQL backups and restoration ownership.
5. Validate Compose, then run `scripts/deploy-production.sh`.
6. Run `scripts/smoke-production.sh`; inspect Compose health and sanitized logs.
7. Register the founder, run the explicit idempotent bootstrap, and confirm its audit event.
8. Perform the read-only manual Schwab test if configured.
9. Reconfirm no broker-write routes/adapters, workers, or live toggle exist before announcing availability.

## Scalable AWS production option

The Caddy/single-host topology above remains a supported simpler deployment and is not replaced. The intended long-term scalable topology is the Terraform-prepared AWS ECS/Fargate architecture described in [AWS deployment](AWS_DEPLOYMENT.md): public ALB only, private web/API/AI tasks, private Multi-AZ RDS and ElastiCache, ECR, Secrets Manager/KMS, CloudWatch, ACM, optional existing-Route-53 integration, and GitHub OIDC. Infrastructure creation, application release, migration, and deliberate DNS cutover are separate operations. Neither deployment option enables live trading.
