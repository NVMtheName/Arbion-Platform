# Architecture

## Purpose and scope

Arbion is a full trading platform with two equal control surfaces: traditional UI and Ask Arbion. **Arbion is UI-first and conversationally enhanced.** Both surfaces manipulate the same structured domain objects and enter the same command, mandate, control, approval, journal, and execution paths. Conversation is never the source of truth or a separate trading subsystem.

Arbion combines that user experience with a provider-independent Neural Engine, deterministic financial controls, strategy and automation domains, and external financial-provider adapters. This document describes target boundaries; it does not claim that the conceptual capabilities are implemented.

The current implementation remains a modular monolith plus one dedicated AI service. Email/password authentication and AI-provider connection verification/configuration are implemented. Live or automated trading, order execution, broker connections, inference, and legacy Flask migration remain out of scope until explicitly designed and approved.

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
        │ AI providers (future) │
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

### Neural Engine

`services/ai` is the Python/FastAPI boundary for AI and quantitative computation. It now abstracts OpenAI, Anthropic/Claude, and Google Gemini behind a common provider interface for credential verification and model discovery, with capability contracts reserving future generation, reasoning, structured analysis, streaming, and tool calling. Go calls bounded, service-authenticated internal endpoints; provider destinations are fixed by adapters rather than request input.

The Neural Engine receives only the minimum data and tools authorized for a request. It neither receives financial-provider credentials nor directly invokes financial providers. Tool requests return to Arbion-controlled systems for permission checks and execution. See [Neural Engine](NEURAL_ENGINE.md).

### Financial connector layer

Financial connectors live behind provider-independent Go interfaces and adapters. Candidate providers include Schwab, E\*TRADE, Coinbase, Alpaca, Interactive Brokers, and future providers. Possible capabilities include accounts, balances, positions, orders, quotes, transactions, order preview, placement, and cancellation, but none are implemented by this architecture task.

The connector layer is subordinate to the control plane: it translates approved operations but does not decide whether they are allowed. See [Connectors](CONNECTORS.md).

## Controlled tool flow

The Neural Engine may conceptually request MCP-like tools such as portfolio lookup, quotes, analysis, backtesting, risk calculation, and order preview. Arbion owns the tool registry, schemas, permissions, resource limits, validation, and audit trail. Tools that read or compute are not inherently trusted, and direct trade execution must not be exposed as an unrestricted AI tool.

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

Go owns registration, login, logout, session validation, user loading, throttling, and audit events. Transport handlers delegate to the `internal/auth` module, while Next.js is only the experience layer. The stable PostgreSQL user ID is deliberately independent of authentication methods so MFA factors, passkeys, OAuth identities, and enterprise SSO can later be attached without replacing the user model.

PostgreSQL is authoritative for accounts. Redis stores only hashed opaque browser-session identifiers, expiry metadata, per-user session indexes for future bulk revocation, and short-lived rate-limit counters. Redis loss signs browsers out but cannot alter durable resources. Login issues a freshly generated 256-bit token and therefore rotates the session; logout deletes only that token and expires its cookie.

**User authentication sessions are ephemeral. User integrations, credentials, and automation configuration are persistent server-side resources and are not tied to an active browser session.**

## Operations and evolution

Components are containerized and coordinated locally with Docker Compose. Production deployments must add TLS, authentication and authorization, managed secrets, structured observability with redaction, backups, migrations, dependency readiness checks, and restrictive network policies.

Prefer a module within the Go application to a new service. Split a service only for a measured scaling, isolation, ownership, or technology constraint, and record the decision in this document or an ADR. Preserve the Next.js experience, Go control/connector, and Python AI/quantitative boundaries.

## Deferred architecture decisions

The following require later design and approval:

- identity, tenancy, roles, consent, and step-up approval flows;
- the provider/model capability contract, routing policy, fallback behavior, quotas, and cost controls;
- tool schema/versioning, data minimization, sandboxing, timeouts, and prompt-injection defenses;
- supported connector sequence, normalized financial data and order semantics, OAuth lifecycle, webhook validation, and reconciliation;
- secret-management technology, key ownership, encryption, rotation, revocation, and incident response;
- risk-policy language, approval thresholds, audit retention, idempotency, and failure recovery;
- the concrete schemas and APIs for mandates, capital buckets, strategy instances, decision journals, orders, and reconciliation;
- strategy definition format, simulation semantics, worker topology, scheduling, concurrency, and notification transport;
- market-data licensing, freshness, provenance, and caching rules;
- the safety case and explicit design approval for any future live or automated execution; and
- whether, when, and how to expose an authenticated MCP-compatible server.

## Read-only financial connection implementation

The Go modular monolith now contains a provider-independent, read-only financial connector foundation and a Schwab Trader API adapter. The trust path is strictly `Next.js -> authenticated Go control plane -> financial Vault class -> selected Go broker adapter -> Schwab`. Financial traffic and secrets never transit the Python service. Redis may hold single-use pending OAuth state; PostgreSQL and the Vault hold durable connections and discovered account inventory, so Redis or browser-session loss cannot disconnect an existing account.

The provider registry is centralized and auth-polymorphic. Schwab is implemented for delegated authorization and reads; E*TRADE and Coinbase are visible planned entries with no external calls. This changes the earlier connector boundary from wholly conceptual to read-only connectivity only. Order, execution, strategy, mandate, allocation, paper, shadow, live, and automated trading remain unimplemented.

**Financial-provider credentials never enter the Neural Engine.**

The financial foundation is now wired as a functional lifecycle: thin authenticated HTTP handlers delegate to `internal/financialconnection`, provider-specific requests remain in the existing Schwab adapter, Redis stores only pending single-use OAuth state, PostgreSQL stores lifecycle/account inventory and supplies cross-instance refresh locking, and the existing Vault stores encrypted token material. Account list/detail, current balance, and current position screens are read-only. Dashboard summaries report inventory only and do not fabricate unavailable or cross-currency values.

## Implemented Automation Builder boundary

The modular Go control plane now contains `internal/automation`; authenticated HTTP handlers and the ordinary `/automations` UI both submit typed commands to that service. PostgreSQL stores account-bound mandates, immutable versions, and capital buckets independently of Redis/browser sessions, so logout cannot remove or pause them. Future Ask Arbion mutation must call this same domain command boundary and may not write mandate tables directly.

The implementation is deliberately configuration-only: there is no execution endpoint, worker, strategy state machine, broker write/preview call, AI inference call, or financial-data transfer to Python. `LIVE` and `READY` are inert persisted values while the platform-level execution capability is false.

**A configured or READY Automation Mandate does not itself execute trades.** **Broker-reported buying power is not Arbion trading authority.**
