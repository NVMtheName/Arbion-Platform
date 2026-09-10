# Arbion System Description

| Field | Value |
| --- | --- |
| Document owner | Security and Compliance Owner |
| Version | 0.1 |
| Approval status | PENDING_MANAGEMENT_APPROVAL |
| Review cadence | At least annually and after material architecture change |

## Service commitments

Arbion provides an owner-scoped trading command center that connects financial and AI providers, displays portfolio and market context, and evaluates bounded autonomous strategies. Current production execution is limited to Paper simulation and Shadow would-have-submitted evidence. Live broker execution is unavailable.

The proposed SOC 2 scope addresses protection of customer credentials and confidential financial information, reliable availability of the command center and scheduled non-live engines, controlled changes, and traceable security operations. Exact contractual availability and notification commitments remain pending management and legal approval.

## System boundary

- **Browser application:** Next.js user interface served over HTTPS. The browser is untrusted and cannot authorize financial actions.
- **Control plane:** Go API owns authentication, authorization, account isolation, credential access, immutable audit records, schedules, deterministic risk checks, and connector orchestration.
- **Neural Engine:** Python service performs bounded provider calls and structured inference. It receives no financial-provider credential and has no broker execution path.
- **Data services:** PostgreSQL stores durable application and audit data. Redis stores ephemeral sessions, rate limits, locks, and coordination state.
- **Edge and hosting:** Current owner-operated production uses Caddy and containerized services. The repository also defines an AWS ECS/Fargate target architecture with private app/data tiers, ALB TLS termination, RDS, ElastiCache, ECR, KMS, Secrets Manager, CloudWatch, and OIDC deployment roles.
- **Delivery:** GitHub source control and Actions run tests, security checks, infrastructure validation, and manual production deployments from `main`.
- **Operations:** Encrypted PostgreSQL backups, restore validation, health/capacity/TLS/reboot checks, and alert delivery support availability and recovery.

## Data classes and flows

1. A user authenticates through the web and receives an opaque secure session cookie; only its hash is retained in Redis.
2. Financial and AI credentials cross the Go API boundary over TLS, are encrypted before storage, and are disclosed only to the adapter that needs them.
3. Financial adapters obtain owner-scoped holdings and market context. The Go control plane minimizes and normalizes facts before sending permitted context to the Neural Engine.
4. Model output is untrusted. Go revalidates identity, mandate, symbols, amounts, timestamps, capital, authorization, and deterministic risk limits.
5. Paper writes only to an isolated simulation ledger; Shadow records only hypothetical evidence. Neither path reaches a broker-write adapter.
6. Security, scheduler, decision, risk, and execution evidence is stored as owner-scoped immutable or append-only records and displayed through bounded read projections.

## People, procedures, and subservice organizations

The Security and Compliance Owner, System Owner, Engineering Owner, and Incident Commander roles operate the controls. One person may temporarily hold multiple roles, but the assignment and any compensating review must be documented.

Material subservice organizations include AWS, GitHub, OpenAI and other configured AI providers, Coinbase, Schwab, email delivery, DNS/domain, and any monitoring or support service that processes scoped information. Their controls are complementary; Arbion remains responsible for vendor selection, configuration, access, monitoring, incident coordination, and customer commitments.

## Exclusions and assumptions

- No live trading capability is represented as implemented or approved.
- Customer endpoints, financial institutions, and AI-provider internal controls are outside Arbion's direct control but are addressed through user responsibilities and vendor management.
- Source code contains no production secrets or customer records. Sensitive operating evidence belongs in a restricted evidence repository, not this public repository.
- Privacy and Processing Integrity criteria are not part of the initial readiness scope until management and the auditor document their applicability.

## Significant-change triggers

Reassess scope and controls before enabling live execution, adding a broker-write adapter, changing identity architecture, introducing a new data region, materially changing hosting, using customer data for model training, adding a material vendor, or changing customer security/availability commitments.
