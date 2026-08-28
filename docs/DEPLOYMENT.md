# Production deployment runbook

## Status, topology, and safety boundary

This repository supports PAPER simulation, SHADOW intent recording, read-only real Schwab data, private Coinbase account reads, non-executing Coinbase order previews, bounded OpenAI-generated Coinbase proposals, durable non-executing Coinbase proposals with TOTP-backed owner review, and an opt-in guarded non-live scheduler. Coinbase enrollment requires View, may record Trade, and rejects Transfer. There is no provider-write interface, order-submission adapter, live-trading implementation, or live feature flag.

```text
Internet :80/:443 -> Caddy (only published service)
  /api/*, /healthz, /readyz -> Go API :8080
  everything else           -> Next.js :3000
Private Docker network: Go, Neural Engine :8000, PostgreSQL :5432, Redis :6379
```

API, AI, PostgreSQL, and Redis have no host port mappings. Outbound provider access remains possible. Financial credentials flow only through Go/Vault to the selected Schwab or Coinbase adapter, never Python.

## DNS and host prerequisites

Use a patched Linux host with Docker Engine, Compose v2, `curl`, `jq`, `openssl`, durable disk, and inbound TCP 80/443 plus UDP 443. Point both `www.arbion.ai` and `arbion.ai` at the separately chosen host—no IP is hard-coded here. Caddy obtains/renews HTTPS certificates, redirects HTTP to HTTPS, and redirects the apex to `https://www.arbion.ai`. Preserve its certificate volume.

## Environment and secret generation

Copy `.env.production.example` to ignored `.env.production` and populate it only on the host:

- `ARBION_ENV=production`;
- `NONLIVE_SCHEDULER_ENABLED=false` for the initial migration and smoke test. Change it to `true` only after the current schema (20) is confirmed and the guarded scheduler validation below passes;
- PostgreSQL database/user and a strong `POSTGRES_PASSWORD`;
- `DATABASE_URL` with matching non-development credentials. Bundled private PostgreSQL may use `postgres://...@postgres:5432/arbion?sslmode=disable` only inside this host; external databases must use TLS;
- `REDIS_URL=redis://redis:6379/0`;
- `CREDENTIAL_ENCRYPTION_KEY`, generated once with `openssl rand -base64 32`;
- `AI_INTERNAL_SERVICE_TOKEN`, generated with `openssl rand -base64 48`;
- `AUTH_ALLOWED_ORIGINS=https://www.arbion.ai`;
- `REGISTRATION_ALLOWLIST`, containing the comma-separated normalized email addresses permitted to register. Production is default-deny when this value is blank;
- keep `EMAIL_DELIVERY_MODE=disabled` and `EMAIL_VERIFICATION_REQUIRED=false` until the sender identity and SMTP credentials have been verified. Then configure the canonical public origin, sender, regional SMTP endpoint/port, and credentials before enabling both values;
- explicit `FOUNDER_EMAIL` only for bootstrap; and
- when Schwab is enabled, both client values and `SCHWAB_REDIRECT_URI=https://www.arbion.ai/api/connections/financial/schwab/callback`.
- when independent market intelligence is enabled, provide both `ALPACA_MARKET_DATA_KEY_ID` and `ALPACA_MARKET_DATA_SECRET_KEY`, pin `ALPACA_EQUITY_FEED=iex` unless a reviewed SIP entitlement exists, and provide an authenticated `COINGECKO_API_KEY` with the matching `COINGECKO_API_TIER=demo|pro`;
- `COINBASE_MARKET_DATA_BASE_URL=https://api.exchange.coinbase.com` enables the bounded keyless crypto venue board by default; it is not a secret and must not be redirected to an unapproved host;
- set `SEC_EDGAR_USER_AGENT=Arbion market intelligence admin@arbion.ai` to enable primary-source filing discovery. This contact identity is not a secret. Alpaca and CoinGecko credentials are secrets and must be backed up and installed through the same owner-only secret workflow as other provider credentials.

Compose rejects absent required values. Go rejects invalid URLs, known development database/key/token values, weak AI tokens, wildcard/noncanonical origins, and partial or misdirected Schwab settings. Python independently rejects missing/weak production internal authentication. Never print, commit, or send the environment file to support.

The encryption key must be cryptographically random, generated once, securely backed up, never committed, and never casually rotated. Losing it may make encrypted provider credentials unreadable. There is no automatic key rotation.

## Release, migrations, and deploy helper

The safe sequence is: review and back up; validate Compose; build images; start healthy PostgreSQL/Redis; run the one-shot embedded Goose migrations exactly once; start AI/API/Web; wait for health; start Caddy; verify public health. Migration failure stops deployment and the API's completed-migration dependency prevents a healthy release. Runtime schema creation is not used.

Package a reviewed Git commit with the repository helper rather than a desktop archive utility. The helper resolves the exact commit, adds the `.release-sha` marker, disables macOS extended-attribute serialization, refuses to overwrite an existing archive, and rejects AppleDouble or `__MACOSX` entries before publishing the bundle:

```bash
./scripts/package-release.sh <reviewed-git-sha> /path/to/arbion-<reviewed-git-sha>.tar.gz
```

```bash
docker compose --env-file .env.production -f docker-compose.prod.yml config --quiet
ARBION_PRODUCTION_ENV_FILE=.env.production ./scripts/deploy-production.sh
./scripts/smoke-production.sh
```

For the current owner-operated Lightsail production host, package and transport
the reviewed release with the host-safe helper from the operator workstation:

```bash
./scripts/deploy-lightsail-release.sh <reviewed-git-sha> <ssh-user>@<lightsail-public-ip> <ssh-private-key>
```

The helper verifies the release marker and checksum, preserves the root-owned
`.env.production`, creates a code-only rollback archive, replaces only the
release tree, runs the migration and Compose readiness gates, checks the public
smoke contract, and confirms the expected six-service set. It never deletes
named data volumes and never invokes a broker operation. Keep the SSH key
outside the repository and use the host's restricted administration firewall.

The repository also contains a separate manual `Deploy application to AWS
ECS/Fargate` workflow for the Terraform-prepared scalable topology. Do not run
that workflow for Lightsail: it requires an already-provisioned ECS cluster,
ECR repositories, private networking, and task definitions. Provision and
review that topology separately before using it.

The deploy script runs the release-hygiene gate before Compose validation, image builds, migrations, or container replacement. It rejects AppleDouble `._*` files and `__MACOSX` directories outside ignored rollback and Git storage. Docker build contexts exclude the same metadata, and only version-prefixed SQL files can enter the embedded migration filesystem. The deploy script otherwise fails fast, does not echo secrets, delete volumes, or overwrite data. Never run `docker compose down -v` in production.

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

Email verification, password recovery, and optional non-live schedule notices share the configured SMTP delivery boundary. Enable them only after verifying `EMAIL_FROM_ADDRESS` with the provider and successfully exercising delivery. Production requires `EMAIL_PUBLIC_BASE_URL=https://www.arbion.ai`; verification links expire after 24 hours and reset links after 30 minutes by default. The database stores only SHA-256 token hashes, each link is single-use, and browser-facing links carry the token in the URL fragment so it does not reach access logs. Reset completion revokes every existing session. Request endpoints always use generic responses. Schedule notices must be explicitly selected in an immutable PAPER/SHADOW schedule version, go only to the verified owner address, and contain no action button or broker data. Never print SMTP credentials, recipient addresses, or emailed links during operational checks.

For an SES SMTP endpoint, production access and the sender identity must both be verified in `us-east-1`. Store the generated SMTP username and password only in the encrypted production parameter and the owner-only host environment. Do not reuse AWS access keys as SMTP credentials. Activate in this order: configure the verified sender and SMTP values with delivery still disabled; restart and validate configuration; enable delivery; send a test to the owner; then enable required verification for new registrations. Existing active accounts remain usable when required verification is enabled.

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
7. During an open market session, configure a bounded PAPER or SHADOW strategy and manually evaluate it; confirm quote/standard option-chain reads succeed, the response says no broker order was sent, and no Schwab order appears.
8. Confirm stale/closed-market data fails closed and the UI distinctions among PAPER, SHADOW, and real read-only Schwab data remain clear.

Manual Coinbase connection and proposal test (never enable Transfer and never place an order):

1. In Coinbase Developer Platform, create a Secret API Key restricted to the intended portfolio and production host IP, select ECDSA, enable View and Trade, and leave Transfer off.
2. Sign into Arbion, open **Settings → Connections → Coinbase**, and enter the full `organizations/.../apiKeys/...` key name plus the ECDSA private key beginning `-----BEGIN EC PRIVATE KEY-----`. Downloaded JSON is optional. A short one-line secret is an Ed25519 or expired legacy credential and must be replaced with a new ECDSA Secret API Key. Literal PEM lines, quoted values, and escaped `\n` line breaks are accepted.
3. Confirm Arbion accepts the connection, creates one masked Coinbase portfolio account, records the Trade grant only as provider capability, and never redisplays the key.
4. Confirm USD cash and nonzero crypto holdings load, then run **Sync** and confirm no duplicate portfolio record appears.
5. On the connected account, request a small real Coinbase preview and confirm the page shows the current product status and exact Coinbase size increment, says no order was created, and exposes no provider preview ID. Confirm a size that violates the displayed increment is blocked.
6. Create or select an active, non-reserve USD Capital Bucket bound to this Coinbase account. Save the preview as a durable proposal and verify the saved Coinbase re-quote, product status, exact proposed notional, available cash, selected policy, and deterministic check results. Confirm an over-capacity BUY or over-holding SELL is retained as `BLOCKED` and cannot consume MFA.
7. For an allowed proposal, review it with a fresh authenticator code. Confirm the result is `USER_APPROVED_NONEXECUTABLE`, scope is `PROPOSAL_REVIEW_ONLY`, and no Coinbase order appears.
8. In the Arbion Neural Engine card, select the same capital policy, enter one bounded objective, and treat the displayed amount as a fixed maximum. Confirm the model either abstains without a provider preview or proposes a size no greater than that maximum. For a proposal, confirm the saved record has source `AI`, fresh Coinbase and deterministic risk evidence, and every execution-capability flag remains false. Confirm the browser states that only normalized cash/target-position facts were shared and that no financial credential left the broker boundary.
9. Disable and re-enable the connection to confirm re-verification; disconnect it and separately revoke the key in Coinbase only when intentionally testing teardown.

Guarded scheduler activation (never place an order):

1. Confirm migrations `00012_nonlive_strategy_scheduler.sql` through `00016_paper_options_simulation_attestation.sql` completed and keep every existing mandate schedule disabled.
2. Set `NONLIVE_SCHEDULER_ENABLED=true` in the owner-only production environment and redeploy the API. With no opted-in mandate version, the scheduler remains idle.
3. In the UI, enable a 30-minute or longer schedule on a `STRATEGY_AUTONOMOUS` PAPER or SHADOW mandate. Confirm this creates a new DRAFT version; review it, mark that exact version READY, and initialize it.
   For PAPER, confirm starting cash above the selected bucket's allocation after its protected amount/absolute limit is rejected, and confirm a second active instance cannot reuse either that bucket or another bucket on the same financial account.
   Pause the initialized instance and confirm manual/scheduled evaluation stops while the account claim remains occupied. Resume only after the exact bound mandate version is still current and READY; confirm a changed or non-ready mandate is rejected. If PAPER has an open simulated option, confirm its lifecycle can be resolved while paused and that the instance stays paused.
   Confirm the explicit finish control rejects PAPER with an open simulated option or share position. After all simulated quantities are zero, confirm finish terminally completes only the non-live instance, preserves its history/portfolio, releases the financial-account claim, and creates no Schwab activity.
4. During the regular session, confirm one due run creates only PAPER/SHADOW decision evidence and no Schwab order. Confirm the schedule status advances and contains only a stable result code.
5. For PAPER, confirm an open option leg changes the schedule result to `WAITING_FOR_LIFECYCLE` without another provider read. Outside the 9:35 a.m.–3:55 p.m. America/New_York window, on a published NYSE holiday, or after the 12:55 p.m. safety cutoff on a published early-close day, confirm it records `OUTSIDE_SESSION` and moves to the next published session.
6. The checked-in calendar is authoritative only for NYSE-published 2026-2028 dates. Before the 2029 calendar year, review the current official NYSE holiday page, extend the tested horizon, and deploy it. If the horizon expires first, confirm due schedules fail closed with `SESSION_CALENDAR_UNAVAILABLE` and make no provider call.
7. To stop all scheduled claims without altering mandate history, set `NONLIVE_SCHEDULER_ENABLED=false` and restart the API. To stop one strategy, disable its schedule or pause/disable its mandate.

## Exact first-host checklist

1. Provision/patch Linux; install Docker/Compose; permit public 80/443 only (plus restricted administration).
2. Clone the reviewed commit and point apex/`www` DNS to the host.
3. Create `.env.production`; generate, store, and separately back up all secrets. Register the exact Schwab callback if enabled.
4. Establish encrypted off-host PostgreSQL backups and restoration ownership.
5. Validate Compose, then run `scripts/deploy-production.sh`.
6. Run `scripts/smoke-production.sh`; inspect Compose health and sanitized logs.
7. Register the founder, run the explicit idempotent bootstrap, and confirm its audit event.
8. Perform the read-only Schwab, non-executing Coinbase proposal, and guarded scheduler tests if configured.
9. Reconfirm no broker-write routes/adapters, live-trading worker, or live toggle exists before announcing availability.

## Scalable AWS production option

The Caddy/single-host topology above remains a supported simpler deployment and is not replaced. The intended long-term scalable topology is the Terraform-prepared AWS ECS/Fargate architecture described in [AWS deployment](AWS_DEPLOYMENT.md): public ALB only, private web/API/AI tasks, private Multi-AZ RDS and ElastiCache, ECR, Secrets Manager/KMS, CloudWatch, ACM, optional existing-Route-53 integration, and GitHub OIDC. Infrastructure creation, application release, migration, and deliberate DNS cutover are separate operations. Neither deployment option enables live trading.
