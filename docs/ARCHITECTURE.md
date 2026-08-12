# Architecture

## Purpose and scope

Arbion is intended to be an AI-powered financial orchestration platform. It combines a user experience, a provider-independent Neural Engine, deterministic financial controls, and external financial-provider adapters. This document describes the target boundaries; it does not claim that the conceptual capabilities are implemented.

The current implementation remains a modular monolith plus one dedicated AI service. Live or automated trading, order execution, broker connections, AI-provider integrations, authentication, and legacy Flask migration are out of scope until explicitly designed and approved.

## System model

```text
┌───────────────────────────────────────────────────────────────┐
│ Experience layer: Next.js / React / TypeScript               │
│ Dashboard, portfolios, research/chat, charts, alerts,        │
│ watchlists, and settings                                     │
└──────────────────────────────┬────────────────────────────────┘
                               │ authenticated API (future)
┌──────────────────────────────▼────────────────────────────────┐
│ Go modular monolith                                           │
│                                                               │
│ Control plane                 Financial connector layer       │
│ authorization and permissions provider-independent adapters   │
│ risk and account validation   accounts, positions, orders,    │
│ buying power and order policy quotes, and transactions        │
│ approvals and audit                                           │
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

PostgreSQL is the durable system of record. Redis is reserved for ephemeral caching, rate limiting, and coordination; correctness must not depend on cached data.

## Layer responsibilities

### Experience layer

`apps/web` is the Next.js App Router application and owns browser presentation and interaction for the trading dashboard, portfolios, research/chat, charts, alerts, watchlists, and settings. It must not own financial policy, store provider secrets, or make trusted execution decisions. Secrets are accepted only through secure platform flows and are never returned after storage.

### Control plane

`services/api` is the core platform and the sole authority for sensitive financial actions. It remains a Go modular monolith: `cmd/api` owns startup and wiring; business and platform packages belong beneath `internal`; transport handlers delegate domain behavior.

All AI output is untrusted input. Every proposed financial action must pass through deterministic authorization, permissions, risk rules, account validation, buying-power checks, order validation, approval requirements, audit logging, and execution policy. An AI response, tool call, or confidence score cannot authorize an action. Live execution requires a separate, explicit architecture and security approval before implementation.

### Neural Engine

`services/ai` is the Python/FastAPI boundary for AI and quantitative computation. It will eventually abstract OpenAI, Anthropic/Claude, and Google Gemini behind a common provider interface for chat, reasoning, research, structured analysis, streaming, and tool calling. Task-to-provider and task-to-model selection may evolve without changing callers.

The Neural Engine receives only the minimum data and tools authorized for a request. It neither receives financial-provider credentials nor directly invokes financial providers. Tool requests return to Arbion-controlled systems for permission checks and execution. See [Neural Engine](NEURAL_ENGINE.md).

### Financial connector layer

Financial connectors live behind provider-independent Go interfaces and adapters. Candidate providers include Schwab, E\*TRADE, Coinbase, Alpaca, Interactive Brokers, and future providers. Possible capabilities include accounts, balances, positions, orders, quotes, transactions, order preview, placement, and cancellation, but none are implemented by this architecture task.

The connector layer is subordinate to the control plane: it translates approved operations but does not decide whether they are allowed. See [Connectors](CONNECTORS.md).

## Controlled tool flow

The Neural Engine may conceptually request MCP-like tools such as portfolio lookup, quotes, analysis, backtesting, risk calculation, and order preview. Arbion owns the tool registry, schemas, permissions, resource limits, validation, and audit trail. Tools that read or compute are not inherently trusted, and direct trade execution must not be exposed as an unrestricted AI tool.

A future MCP-compatible server could expose a deliberately approved subset of Arbion tools to external AI clients. It would remain an Arbion-controlled interface subject to the same identity, authorization, credential, policy, and audit boundaries. No MCP server is part of the current scope.

## Credential and trust boundaries

AI-provider credentials and financial-provider credentials are separate secret classes with distinct access policies. AI vendors must never receive brokerage secrets. Financial connectors should prefer OAuth or token-based delegated authorization where supported. Stored secrets must never be returned to the browser, logged, or committed, and production storage will require managed encryption or a secret manager. See [Security](SECURITY.md).

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
- market-data licensing, freshness, provenance, and caching rules;
- the safety case and explicit design approval for any future live or automated execution; and
- whether, when, and how to expose an authenticated MCP-compatible server.
