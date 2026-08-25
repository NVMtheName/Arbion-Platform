# Architecture

## Purpose and scope

Arbion is a full trading platform with two equal control surfaces: traditional UI and Ask Arbion. **Arbion is UI-first and conversationally enhanced.** Both surfaces manipulate the same structured domain objects and enter the same command, mandate, control, approval, journal, and execution paths. Conversation is never the source of truth or a separate trading subsystem.

Arbion combines that user experience with a provider-independent Neural Engine, deterministic financial controls, strategy and automation domains, and external financial-provider adapters. This document describes target boundaries; it does not claim that the conceptual capabilities are implemented.

The current implementation remains a modular monolith plus one dedicated AI service. Email/password authentication, AI-provider connection verification/configuration, bounded read-only Arbion Insight analysis, a dual-provider OpenAI Shadow Engine for Coinbase and Schwab, private account connectivity, real non-executing Coinbase order previews, and durable capital-policy-bound Coinbase proposals with deterministic risk evidence and TOTP-backed owner review are implemented. The Shadow Engine can autonomously observe, abstain, or journal one mandate-bounded proposal through the deterministic risk gate, but it has no broker-write adapter. Live trading, execution approval, order submission/cancellation, tool-using AI, and legacy Flask migration remain out of scope until explicitly designed and approved.

## System model

```text
┌───────────────────────────────────────────────────────────────┐
│ Experience layer: Next.js / React / TypeScript               │
│ UI controls and Ask Arbion: trading, automation, portfolio,  │
│ research, charts, alerts, watchlists, and settings            │
└──────────────────────────────┬────────────────────────────────┘
                               │ cookie-authenticated API
┌──────────────────────────────▼────────────────────────────────┐
│ Go modular monolith                                           │
│                                                               │
│ Shared domain commands, Automation Mandates, strategy engine  │
│ deterministic control/risk, approvals, execution, journal     │
│ provider-independent financial connector adapters             │
└───────────────┬───────────────────────────────┬───────────────┘
                │ bounded requests              │ OAuth/tokens
┌───────────────▼────────────────┐       ┌──────▼───────────────┐
│ Python Neural Engine           │       │ Financial providers  │
│ provider adapters, research,   │       │ (future integrations)│
│ structured analysis, quant     │       └──────────────────────┘
└───────────────┬────────────────┘
                │ user-supplied AI provider credentials
        ┌───────▼───────────────┐
        │ AI providers          │
        └───────────────────────┘
```

PostgreSQL is the durable system of record, including provider-connection metadata and server-side automation configuration. Redis is reserved for ephemeral caching, rate limiting, queues, and worker coordination; correctness must not depend on cached data, and losing Redis must not change whether automation is enabled.

Browser/login sessions are ephemeral and distinct from durable user resources. Logging out ends only the browser session: it does not delete provider connections, revoke their server-side credentials, or disable authorized automation. Future workers will operate independently of an open browser using server-side authorization and credentials.

## Layer responsibilities

### Experience layer

`apps/web` is the Next.js App Router application and owns browser presentation and interaction for the trading dashboard, portfolios, research/chat, charts, alerts, watchlists, and settings. It must not own financial policy, store provider secrets, or make trusted execution decisions. Secrets are accepted only through secure platform flows and are never returned after storage.

Every important capability must be operable through ordinary UI controls without conversation. UI forms, trading tickets, automation builders, conversational intents, and notification actions translate into the same typed commands. They may provide different input ergonomics, but not different domain semantics.

### Control plane

`services/api` is the core platform and the sole authority for sensitive financial actions. It remains a Go modular monolith: `cmd/api` owns startup and wiring; business and platform packages belong beneath `internal`; transport handlers delegate domain behavior.

All AI output is untrusted input. Every proposed financial action must pass through deterministic authorization, permissions, risk rules, account validation, buying-power checks, order validation, approval requirements, audit logging, and execution policy. An AI response, tool call, or confidence score cannot authorize an action. Live execution requires a separate, explicit architecture and security approval before implementation.

The permanent domain design is split into focused specifications:

- [Automation Engine](AUTOMATION_ENGINE.md) defines immutable Automation Mandates, automation/autonomy types, capital buckets, server-side operation, and the Decision Journal.
- [Strategy Engine](STRATEGY_ENGINE.md) defines deterministic state machines shared across backtest, paper, shadow, and live adapters.
- [Execution Engine](EXECUTION_ENGINE.md) defines provider-independent Order Intents, lifecycle, idempotency, capability discovery, and broker reconciliation.
- [Risk and Control Engine](RISK_CONTROL_ENGINE.md) defines the authoritative deterministic gate and non-bypassable circuit breakers.
- [Market Intelligence and Command Center](MARKET_INTELLIGENCE.md) defines independent read-only market sources, feed quality, provenance, freshness, caching, and the branded command-center boundary.

### Neural Engine

`services/ai` is the Python/FastAPI boundary for AI and quantitative computation. It abstracts OpenAI, Anthropic/Claude, and Google Gemini behind a common provider interface for credential verification and model discovery. OpenAI implements the structured Arbion Insight capability and the strict, tool-free AI Shadow decision contract through the Responses API; other providers fail as unsupported for those inference routes. Go calls bounded, service-authenticated internal endpoints, and provider destinations are fixed by adapters rather than request input. Python may return only abstention or one allowlisted, size-bounded proposal; Go independently revalidates it before deterministic risk evaluation.

Arbion Insight accepts only authenticated user-supplied text, an explicit `fast`/`core`/`deep` profile, and the user's active OpenAI connection. The profile maps to a fixed Luna/Terra/Sol route with increasing reasoning, output cap, and weighted credit cost; the browser never selects an arbitrary model ID. It sends no account, portfolio, broker, credential-class, or live market data and defines no tools. The request disables provider-side response storage and requires a strict bounded JSON schema. Go enforces entitlement, ownership, active connection state, a weighted 12-credit hourly budget, encrypted credential retrieval, exact response-route validation, and metadata-only auditing. The returned analysis remains untrusted educational output and has no route to preview, authorize, or execute an order.

The Neural Engine receives only the minimum data and tools authorized for a request. It neither receives financial-provider credentials nor directly invokes financial providers. Tool requests return to Arbion-controlled systems for permission checks and execution. See [Neural Engine](NEURAL_ENGINE.md).

### Financial connector layer

Financial connectors live behind narrow provider-independent Go interfaces and adapters. Schwab implements delegated authorization plus read-only accounts, balances, positions, quotes, and option chains. Coinbase implements a founder-phase encrypted key-pair connection for Advanced Trade portfolio inventory and USD cash, Coinbase App wallet/vault totals reconciled with Advanced Trade availability, separately bounded external-fill and order-status history, a spot fee-tier snapshot, and provider order preview. Coinbase enrollment requires View, permits an explicitly recorded Trade grant, and always rejects Transfer. Before preview, the adapter reads authenticated spot-product metadata and exactly validates the requested side's size increment and bounds plus current market-IoC restrictions. The preview interface has no create, cancel, replace, or transfer method and never returns Coinbase preview IDs. The separate Go `internal/orderintent` domain binds every new proposal to an active same-owner/account capital bucket, refreshes connected cash and holdings, invokes the provider-independent deterministic risk engine, and persists immutable preview, product-rule, bucket, account-fact, and risk evidence plus proposal-review events. It has no provider-write dependency or execution state. Candidate future providers include E\*TRADE, Alpaca brokerage, and Interactive Brokers.

The connector layer is subordinate to the control plane: it translates approved operations but does not decide whether they are allowed. See [Connectors](CONNECTORS.md).

Independent market-data credentials and reads must not be forced through a user's broker connection. Delegated Schwab market observations remain explicitly account-scoped and ownership-checked, while the provider-neutral market-intelligence foundation defines source roles, exact decimals, freshness/feed quality, and fixture-tested Alpaca, CoinGecko, keyless Coinbase, and SEC EDGAR adapters. Runtime verification is isolated by source capability, so one successful Coinbase feed cannot mask another feed's failure; only safe process-local attempt/success timestamps, time-bounded current/aged state, bounded failure categories, configured cache/pacing policy, and capped aggregate request counters reach the browser. Verification expires after three cache lifetimes within a five-minute-to-one-hour bound, and neutral aging does not fabricate a provider failure. Completed provider outcomes also roll into durable five-minute PostgreSQL buckets with 30-day retention and a fixed 24-hour hourly view. Durable health contains no user, account, instrument, request, credential, provider-request, URL, or raw-error dimension; a history-write failure cannot fail a market-data read. The process counters and durable buckets are never labeled as provider quota. The branded market surfaces use Schwab for the connected user's equities/options, Coinbase for bounded single-venue crypto snapshots, connected-asset candle history, rolling venue statistics, ten-level Advanced Trade public liquidity, and 25-tick public time-and-sales, and SEC EDGAR for primary filings; no source creates an execution path. See [Market Intelligence and Command Center](MARKET_INTELLIGENCE.md).

## Controlled tool flow

The Neural Engine may conceptually request MCP-like tools such as portfolio lookup, quotes, analysis, backtesting, risk calculation, and order preview. Arbion owns the tool registry, schemas, permissions, resource limits, validation, and audit trail. No general-purpose model tool registry is implemented. The current AI trade-research route is instead a fixed Go-orchestrated request: Go selects only normalized current cash and target-position facts, Python obtains one schema-constrained proposal or abstention with no tools, and Go either stops or sends the proposal into the existing fresh preview and deterministic risk path. It grants no approval or execution authority. Tools that read or compute are not inherently trusted, and direct trade execution must not be exposed as an unrestricted AI tool.

A future MCP-compatible server could expose a deliberately approved subset of Arbion tools to external AI clients. It would remain an Arbion-controlled interface subject to the same identity, authorization, credential, policy, and audit boundaries. No MCP server is part of the current scope.

## Durable automation and execution truth

PostgreSQL will hold immutable mandate versions, exact strategy state, structured decision evidence, approval evidence, and execution/reconciliation state when those concepts are implemented. Redis may coordinate future workers but is never authoritative. Broker records are authoritative for live execution facts; a submitted request is not a fill. Automated operation is server-side and continues independently of browser sessions, subject to its durable mandate and controls.

## Credential and trust boundaries

AI-provider credentials and financial-provider credentials are separate secret classes with distinct access policies. AI vendors must never receive brokerage secrets. Financial connectors should prefer OAuth or token-based delegated authorization where supported. Stored secrets must never be returned to the browser, logged, or committed. The development vault uses authenticated encryption with an environment-supplied key and PostgreSQL ciphertext storage behind a vault interface; production should replace that adapter with a managed secret system without changing business logic. See [Security](SECURITY.md).

The Go `aiconnection` domain owns authenticated AI-provider connection lifecycle and uses the shared credential vault, provider connection and Neural preference tables, audit store, authorization policy, and narrow Python client. Its centralized registry describes `openai`, `anthropic`, and `gemini`; adding a provider extends that registry and its Python adapter rather than transport handlers. Production service-to-service traffic requires workload identity, TLS, and restrictive networking beyond the local shared-token mechanism.

## Authentication architecture

## Authorization and entitlement

**Administrative authority and product entitlement are separate concepts.** Roles form the security hierarchy `superadmin > admin > user`; they do not grant paid product capabilities. Entitlements (`free`, `pro`, `premium`, `founder`, and `internal_comped`) describe product access and do not grant administrative authority. PostgreSQL is authoritative for both, while Redis sessions merely identify a user and never copy or own persistent authorization state.

The centralized Go authorization module evaluates administrative requirements and product capability policy. Founder entitlement is non-expiring, never billing-required, and defaults to all present and future paid capabilities unless a capability is intentionally excluded. Other tier mappings remain conservative placeholders until product gates are designed.

Stripe or another billing provider may influence entitlements in the future, but external billing systems must never determine Arbion administrative roles.

### Founder bootstrap and recovery

Infrastructure operators deliberately run `FOUNDER_EMAIL=normalized@example.com /bootstrap-founder` after the account already exists. The command normalizes and looks up that email, idempotently restores `superadmin` plus the non-billing, non-expiring `founder` entitlement, and writes `authorization.founder_bootstrap` without logging the email or credentials. A missing account fails closed. It is not run during API startup.

Normal admin APIs reject founder demotion, founder entitlement replacement, and making founder access billing-required. Extraordinary recovery uses only this infrastructure-level command with explicit environment configuration and database/audit access; there is no password, URL, query parameter, or authentication bypass. Future deletion flows must likewise refuse founder deletion.

Admin routes are `GET /api/admin/me`, `GET /api/admin/users`, `GET /api/admin/users/{id}`, `PUT /api/admin/users/{id}/role`, and `PUT /api/admin/users/{id}/entitlement`. Admins can view; only superadmins can mutate. Responses use deliberately safe projections. Administrative reads and changes, founder bootstrap, and role/entitlement changes are audited with actor, target, previous/new values, time, and available request correlation metadata; audit integrity and retention remain future work.

Go owns registration, login, authenticator MFA, logout, session validation, user loading, throttling, and audit events. Transport handlers delegate to the `internal/auth` module, while Next.js is only the experience layer. The stable PostgreSQL user ID is deliberately independent of authentication methods so current TOTP factors and future passkeys, OAuth identities, and enterprise SSO can attach without replacing the user model.

PostgreSQL is authoritative for accounts, encrypted TOTP factors, TOTP replay counters, and hashed single-use MFA recovery codes. Redis stores only hashed opaque browser-session identifiers, expiry metadata, per-user session/challenge indexes, short-lived password-to-MFA login challenges, and rate-limit counters. Password changes, password resets, MFA changes, and all-session logout revoke both sessions and outstanding MFA challenges. Redis loss signs browsers out and invalidates pending MFA challenges but cannot remove or bypass a durable factor. Login issues a freshly generated 256-bit token and therefore rotates the session; an MFA-enabled account receives that session only after its second factor succeeds. Logout deletes only that token and expires its cookie.

**User authentication sessions are ephemeral. User integrations, credentials, and automation configuration are persistent server-side resources and are not tied to an active browser session.**

## Operations and evolution

Components are containerized and coordinated locally with Docker Compose. Production deployments must add TLS, authentication and authorization, managed secrets, structured observability with redaction, backups, migrations, dependency readiness checks, and restrictive network policies.

The initial production topology is a vendor-neutral Linux Docker host. Caddy is the only public container and terminates HTTPS for `www.arbion.ai`; it redirects the apex domain, routes `/api/*` and API health paths to Go, and routes all other traffic to Next.js. Go, the Neural Engine, PostgreSQL, and Redis share a non-published Docker network. PostgreSQL, Redis, and Caddy certificate state use named volumes. A one-shot migration container must succeed before API startup. See [Deployment](DEPLOYMENT.md).

Prefer a module within the Go application to a new service. Split a service only for a measured scaling, isolation, ownership, or technology constraint, and record the decision in this document or an ADR. Preserve the Next.js experience, Go control/connector, and Python AI/quantitative boundaries.

## Deferred architecture decisions

The following require later design and approval:

- identity, tenancy, roles, consent, and step-up approval flows;
- the provider/model capability contract, routing policy, fallback behavior, quotas, and cost controls;
- tool schema/versioning, data minimization, sandboxing, timeouts, and prompt-injection defenses;
- supported connector sequence, normalized financial data and order semantics, OAuth lifecycle, webhook validation, and reconciliation;
- secret-management technology, key ownership, encryption, rotation, revocation, and incident response;
- risk-policy language, approval thresholds, audit retention, idempotency, and failure recovery;
- the remaining live order, execution-approval, durable execution/automation reservation, and reconciliation schemas and APIs (short-lived manual-proposal reservations are implemented);
- strategy definition format, simulation semantics, worker topology, scheduling, concurrency, and notification transport;
- market-data licensing, freshness, provenance, and caching rules;
- the safety case and explicit design approval for any future live or automated execution; and
- whether, when, and how to expose an authenticated MCP-compatible server.

## Financial connection and preview implementation

The Go modular monolith contains a provider-independent financial connector foundation plus Schwab and Coinbase adapters. The trust path is strictly `Next.js -> authenticated Go control plane -> financial Vault class -> selected Go financial adapter -> provider`. Financial traffic and secrets never transit the Python service. Redis may hold single-use pending Schwab OAuth state; PostgreSQL and the Vault hold durable connections and discovered account inventory, so Redis or browser-session loss cannot disconnect an existing account.

The provider registry is centralized and auth-polymorphic. Schwab uses delegated OAuth authorization for read-only account and market-data operations. Coinbase uses a per-user encrypted ECDSA key pair that must report View, must not report Transfer, and may report Trade. Trade permission is persisted only inside the encrypted credential and projected as account capability; it is not Arbion approval. E\*TRADE remains a visible planned entry. Order submission and live provider execution remain unimplemented. Mandates, allocations, deterministic strategies, and the PAPER/SHADOW persistence paths are separate internal domains; scheduled AI SHADOW evaluation can read current facts and record decisions but does not add provider-write capability.

**Financial-provider credentials never enter the Neural Engine.**

The financial foundation is wired as a functional multi-provider lifecycle: thin authenticated HTTP handlers delegate to `internal/financialconnection`, provider-specific requests remain in the Schwab and Coinbase adapters, Redis stores only pending single-use OAuth state, PostgreSQL stores lifecycle/account inventory and supplies cross-instance credential locking, and the existing Vault stores encrypted tokens or key material. Account list/detail, current balance, current position, quote, standard option-chain, optional execution-history reads, and Coinbase spot preview are permissioned. The Coinbase portfolio read model composes current private holdings with separately keyless, venue-stamped Exchange tickers. It labels last-trade-based values as market observations and values USDC separately through Coinbase's documented 1:1 USD redemption reference, without presenting that reference as a trade, bid, ask, cost basis, or performance figure. Its connected-asset chart remains limited to assets with actual venue observations, preserves missing provider intervals as gaps, and labels the result venue movement rather than portfolio performance or P&L. Separate keyless adapters supply a 30-second-cached rolling venue-statistics record, a ten-level one-second-cached manual-refresh liquidity snapshot, and a 25-tick one-second-cached public tape for a currently held asset. The statistics contract has receipt-only timing because Coinbase supplies no event timestamp; none of these public views is streaming, consolidated, or executable. Its activity projections read fixed first pages of normalized spot fills and provider order state with View permission, omit provider identifiers/cursors, and label them external evidence rather than Arbion orders, cost basis, or P&L. A separate spot transaction-summary read projects only provider fee-tier evidence and makes no reporting-window, next-tier, tax, or performance claim. The authenticated CSRF-protected preview endpoint accepts only a canonical asset, BUY/SELL side, and exact positive amount; it fixes the request to a USD spot market IOC preview, normalizes Coinbase totals/fees/book evidence and safe block categories, discards the preview ID, and declares that no order was created and submission remains unavailable. Dashboard summaries do not fabricate unavailable or cross-currency values.

Financial connection lifecycle operations are isolated by durable connection and provider-account identity. A verified provider account is unique per owner and provider; reconnecting it reuses its existing connection and financial-account IDs, while a different Coinbase portfolio creates a separate connection. Resync updates only accounts belonging to that connection, and an empty discovery result fails closed without retiring the prior inventory. A transient balance, holdings, pricing, timeout, or rate-limit failure is reported as partial provider data and does not revoke the connection or substitute zeroes. Authorization and permission failures mark only the affected connection as requiring attention. Disable and disconnect fail with a conflict before credentials or account state are changed whenever a READY/PAUSED mandate or ACTIVE/PAUSED strategy uses the connection. These invariants preserve independent accounts and non-live automation through routine add, refresh, and reconnect operations.

There is still no live broker execution adapter. A future live-execution milestone must add a durable broker-order/execution/reconciliation ledger independent of credential lifecycle, suspend new submissions before connection mutation, and reconcile or explicitly cancel outstanding provider orders under an approved policy. Credential deletion can never serve as an order-cancellation mechanism.

## Implemented Automation Builder boundary

The modular Go control plane now contains `internal/automation`; authenticated HTTP handlers and the ordinary `/automations` UI both submit typed commands to that service. PostgreSQL stores account-bound mandates, immutable versions, and capital buckets independently of Redis/browser sessions, so logout cannot remove or pause them. Future Ask Arbion mutation must call this same domain command boundary and may not write mandate tables directly.

The automation domain owns configuration and lifecycle commands. A manual endpoint and an opt-in guarded server loop may evaluate a current READY PAPER or SHADOW strategy through read-only account/market facts, the deterministic strategy engine, and the Risk/Control Engine. The loop is embedded in the single API process and coordinated by PostgreSQL leases; it has no broker preview/write capability, AI automation inference call, or financial-data transfer to Python. The separate read-only Arbion Insight endpoint receives only text typed for that request and cannot access automation or financial domains. `LIVE` remains inert configuration and platform-level live execution capability is false.

**A configured or READY Automation Mandate does not itself execute trades.** **Broker-reported buying power is not Arbion trading authority.**

## Deterministic risk foundation

The risk foundation lives in `services/api/internal/risk`. It consumes normalized snapshots rather than calling financial or AI providers, evaluates an ordered provider-independent registry, and emits explainable evidence. The Risk/Control Engine authorizes or denies proposed actions; it never decides that a trade is desirable. A successful risk evaluation does not execute a trade.

## Deterministic non-live automation implementation

The Go modular monolith now contains `internal/strategy`, a pure deterministic definition/evaluation layer plus PAPER and SHADOW adapters, a guarded scheduler, and a persistence boundary. The flow is mandate/version → instance → normalized evaluation → existing ProposedAction → authoritative risk gate → non-live adapter → atomic state/history/journal persistence. Authenticated HTTP handlers expose initialization, ownership-scoped reads, explicit pause/resume/finish commands, a manual evaluation command, schedule status, and explicit owner-recorded PAPER Wheel lifecycle events. Manual and scheduled paths call the same evaluation service and require the exact current immutable mandate version and fresh provider timestamps. Durable leases plus stable scheduled event IDs make concurrent claims and crash retries duplicate-safe.

PAPER portfolios are a distinct persistence domain. When an active or paused PAPER Wheel has one open simulated option, the owner may attest only a legal worthless-expiry, assignment, or called-away event. The service derives contract and ledger values from durable PAPER records, requires the expected state version and an explicit simulation acknowledgement, then commits cash/shares, the immutable event, transition, and journal entry atomically without changing a paused instance back to active. Same-identity retries return the original event; conflicting or stale commands fail closed. SHADOW can consume normalized account facts supplied through the Go financial read boundary, but the strategy engine cannot call Schwab. Neither adapter has connector credentials or a broker-write method. There is no live adapter.

## Scalable AWS production topology

The long-term scalable production foundation retains the same modular-monolith-plus-Neural-Engine boundary. A public AWS ALB terminates ACM TLS and routes `/api/*` to private Go Fargate tasks and default traffic to private Next.js tasks. Python is private and discovered through AWS Cloud Map; token authentication remains mandatory. Private Multi-AZ RDS is durable truth and encrypted ElastiCache is ephemeral coordination/session infrastructure. Application tasks use private subnets with NAT egress for fixed provider adapters, while data subnets have no Internet route. ECR, Secrets Manager/KMS, CloudWatch, and GitHub OIDC supply image, secret, telemetry, and temporary deployment-identity boundaries. See [AWS deployment](AWS_DEPLOYMENT.md).

This infrastructure introduces no worker or new product service. Future automation or market-data workers can reuse the application/data network tiers only after their domain, safety, and operational design is approved. The Caddy single-host topology remains supported.
