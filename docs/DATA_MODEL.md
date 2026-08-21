# Persistent data model

PostgreSQL is Arbion's durable system of record. Schema changes are explicit, versioned migrations run by `cmd/migrate`; API startup never creates or changes tables.

## Tables

- `users` supplies a stable UUID and authentication fields: original and normalized email, an Argon2id password hash, display name, account status, email verification time, last login time, and timestamps. A partial unique index on normalized email provides case-insensitive uniqueness while preserving compatibility with pre-authentication rows.
- `auth_email_tokens` stores only unique 32-byte SHA-256 token hashes with a user, purpose, expiry, creation time, and optional consumption time. A partial unique index permits one active verification and one active reset token per user; replacement invalidates the older link and consumption is atomic with verification or password replacement.
- `auth_totp_factors` stores one optional authenticator factor per user as user-bound AES-256-GCM ciphertext, its pending/active lifecycle, and the greatest successfully used 30-second step for replay prevention. `auth_mfa_recovery_codes` stores ten independently generated recovery codes only as unique 32-byte hashes with one-time usage timestamps; deleting the factor cascades only its recovery material.
- `users.role` is durable administrative authority (`user`, `admin`, or `superadmin`). `user_entitlements` independently records effective product access, its source, lifecycle, and whether billing is required. Founder rows are constrained to be non-expiring and non-billing.
- `provider_connections` records AI or financial provider identity, status, scopes, safe credential metadata, token expiry, verification time, and either authenticated ciphertext or a managed-secret reference. Provider protocols remain adapter concerns.
- `neural_engine_preferences` stores one user's default active AI provider connection and discovered model ID. It is an ordinary durable preference, not authority for future automations; its restrictive connection foreign key prevents silent deletion.

AI rows always use `provider_category = 'ai'`. Their safe credential metadata may contain only the masked display hint; ciphertext remains in the vault-owned storage column and is excluded from application/API connection models. Status `pending` means stored but not externally verified, while `disabled` preserves the encrypted credential but prevents future selection. Durable references block explicit deletion rather than cascading silently.

- `automation_mandates`, `automation_mandate_versions`, and `capital_buckets` hold durable configuration and explicit allocation authority; their presence does not imply broker execution.
- `audit_events` is append-oriented and records actors, actions, resources, correlation identifiers, timestamps, and non-secret JSON metadata.

Browser sessions are separate and ephemeral. Provider connections, Neural Engine preferences, and automation state persist across logout, browser closure, Redis loss, and application restarts. Stored provider credentials are server-side only and never part of normal API models.

## Target domain records

The permanent architecture requires normalized durable concepts for Automation Mandates and immutable versions, capital buckets and reservations, strategy definitions/instances/state transitions, structured Arbion Intents, provider-independent Order Intents and execution events, approvals, account capabilities, reconciliation observations, circuit breakers, and the Decision Journal. Mandates, buckets, non-live instances/transitions, circuit breakers, risk evaluations, non-live execution evidence, and Decision Journal entries are implemented; live orders, approvals, reconciliation, and reservations remain conceptual.

References between a decision, order, strategy transition, and mandate must identify the exact immutable mandate version. Significant changes append a version rather than overwriting history. Execution history is append-oriented and preserves provider evidence: `SUBMITTED` never implies `FILLED`, and broker reconciliation supplies authoritative live status.

Capital records must distinguish observed balance/buying power from explicit Arbion allocations, protected/reserve amounts, and pending reservations. The model must prevent unintended double allocation across concurrent mandates and retain absolute limits even when a future allocation is percentage-based.

Decision records store structured rationale, input references, rule outcomes, approvals, and resulting facts. They must not store private model chain-of-thought. PostgreSQL remains authoritative; Redis coordination and browser sessions do not own any of these facts. See [Automation Engine](AUTOMATION_ENGINE.md), [Strategy Engine](STRATEGY_ENGINE.md), [Execution Engine](EXECUTION_ENGINE.md), and [Risk and Control Engine](RISK_CONTROL_ENGINE.md).

## Deliberately deferred

Live-order/approval/reconciliation schemas, broader tenancy, automation workers, broker trading execution, audit retention/integrity controls, retention and partitioning, and the production managed-secret provider require later designs.

## Financial account inventory

Migration `00005_financial_accounts.sql` adds the durable `financial_accounts` inventory. Each row is owned by a user, linked restrictively to a same-owner `provider_category='financial'` connection, and stores provider name, a server-only opaque provider account identifier, masked/display labels, account type, base currency, lifecycle status, tri-state capability JSON, discovery/sync times, and ordinary timestamps. `(provider_connection_id, provider_account_id)` is unique so synchronization upserts rather than duplicates and a connection can contain multiple accounts. The trigger guards against cross-user and wrong-category links.

The normal API model omits `user_id`, opaque provider account identity, and provider instrument identifiers. It never stores or returns full account numbers. Financial values are provider-precision decimal strings, not binary floats. Balance observations and buying power are read-only provider facts and remain separate from later-introduced allocation, mandate, and non-live strategy records.

**Broker-reported buying power does not grant Arbion authority to deploy that capital.**

## Schwab read-only connection records

`provider_connections` remains the sole durable connection table. Schwab rows use category `financial`, provider `schwab`, one stable display name per user, safe status/expiry/verification timestamps, and encrypted-database credential storage. Token plaintext is present only inside the Vault payload. Reauthorization upserts the existing row instead of creating duplicates.

`financial_accounts` stores the owning user and connection, Schwab's server-only account hash, a masked display label, normalized type/currency/status/capabilities, and discovery/sync timestamps. `(provider_connection_id, provider_account_id)` makes repeat discovery idempotent. Ownership is present in every account query and enforced again by the database trigger. Missing accounts become `unavailable`; disconnect retires them as `closed` rather than destroying inventory or audit history. Full account numbers, balances, and positions are not persisted.

## Coinbase connection records

Coinbase reuses the same provider-neutral tables without a schema migration. A financial `provider_connections` row uses provider `coinbase`; its structured Coinbase App key name, ECDSA private key, and verified portfolio ID exist only inside the Vault ciphertext. API connection models expose none of those values. The durable `financial_accounts` row represents the permissioned Coinbase portfolio with a server-only `portfolio:{provider UUID}` identity, a masked label, USD display currency, explicit crypto-holdings capabilities, and unsupported order/transfer capabilities. Individual wallet balances and holdings are fetched on demand and are not persisted.

Coinbase portfolio valuation is also an on-demand read model, not a durable ledger. Current quantities come from the authenticated view-only account adapter; last trade, bid, ask, venue, and timestamps come from the independent keyless Exchange market adapter. The API derives bounded exact-decimal observed values in memory and returns per-position pricing status and aggregate coverage. It does not persist price snapshots, observed totals, inferred pegs, cost basis, or unavailable-asset estimates.

Connected-asset history is likewise an ephemeral observation model. A series contains one canonical asset and USD quote currency, a fixed fifteen-minute granularity, an expected count of 96, only the candles Coinbase Exchange actually returned, and source provenance for the newest interval. Each candle preserves exact low, high, open, close, and volume decimals plus its provider bucket time. Arbion retains the normalized series only in a bounded one-minute memory cache; it does not persist candles, fill gaps, infer trades, replay current holdings across old prices, or derive a portfolio return.

Connected Coinbase execution history is an ephemeral owner-scoped projection, not a durable Arbion ledger. Each normalized fill contains only product/base/quote symbols, BUY or SELL side, exact provider price and size with an explicit size unit, exact commission/currency, execution time, and normalized liquidity. The response carries source/retrieval metadata and only a boolean indicating that more provider history exists. Coinbase entry, trade, order, user, portfolio, and cursor identifiers are deliberately omitted. Arbion does not persist these fills or derive orders, tax lots, cost basis, realized/unrealized results, or strategy state from them.

Connected Coinbase order history is a separate ephemeral observation model. Each row contains external provider status and safe classification plus exact normalized fill progress/value/fee evidence and timestamps, but no identity that can address an order at Coinbase. Only a `has_more` boolean survives pagination. The projection is neither persisted nor joined to Arbion strategies, fills, mandates, journals, tax lots, or positions; it cannot authorize a state transition or mutate the provider.

Connected Coinbase trading costs are another ephemeral owner-scoped projection. The normalized snapshot contains only spot scope, a bounded provider tier label, exact maker/taker rates, exact USD Advanced Trade volume and fees, exact provider total fees, the provider pricing-model flag, and retrieval time. It is never persisted or joined to orders, strategies, positions, fills, tax lots, or journal records. Arbion does not infer the provider's reporting window, calculate next-tier progress, forecast an order fee, or turn the summary into an order preview.

## Durable automation authorization records

Migration `00006_automation_mandates.sql` replaces the early placeholder in place: `automation_configs` is renamed and normalized as `automation_mandates`, so there are not two active automation concepts. Stable security fields are columns; validated extensible strategy/risk/universe/condition documents remain structured JSON. Each mandate binds a same-owner `financial_accounts` row and `capital_buckets` row and has a lifecycle status, effective interval, and monotonically increasing current version.

`capital_buckets` stores exact PostgreSQL numeric allocation values, currency, allocation basis, optional absolute limit/protected amount, reserve semantics, and durable status. It is explicit Arbion authorization, distinct from transient broker balances. Referenced buckets cannot be silently deleted.

`automation_mandate_versions` is append-only and protected by a database trigger against update/delete. Its complete credential-free JSON snapshot records account, type, strategy, AI references/model, bucket, autonomy, mode, lifecycle, parameters, risk, universes, margin/options policy, conditions, effective dates, capability warning, PAPER-only options-simulation attestation, and `execution_capable=false`. Expected-version updates atomically advance the mandate and append the next version, otherwise returning a conflict.

**A configured or READY Automation Mandate does not itself execute trades.**

**Broker-reported buying power is not Arbion trading authority.**

## Risk and control records

Migration `00007_risk_control.sql` adds durable `risk_circuit_breakers` for global, user, account, and automation scopes and compact `risk_evaluations` linked by current non-live Decision Journal entries. Evaluation rows preserve references, exact mandate version, decision, approval requirement, stable reason codes, deterministic checks, and timestamp while omitting portfolio snapshots, credentials, broker payloads, and model reasoning. A constraint records `platform_execution_available=false` for this milestone.

## Durable non-live strategy records

Migration `00008_nonlive_strategy.sql` adds strategy instances bound by a composite foreign key to an immutable mandate version; append-only state transitions with unique `(instance,state_version)` optimistic concurrency; durable evaluation event identities; separate exact-decimal paper portfolios and positions; generic non-live execution records; and structured Decision Journal entries. Paper data never enters `financial_accounts` or provider position records.

History update/delete triggers make state transitions and journal evidence immutable. Execution idempotency and per-instance event identities are database-enforced. Browser logout changes only Redis session state and cannot remove instances, paper portfolios, executions, decisions, or state history.

Migration `00011_decision_journal_feed.sql` adds the `(user_id,created_at DESC,id DESC)` index used by the owner-wide Decision Journal projection. Cursor pagination uses both timestamp and UUID so equal timestamps do not skip or duplicate entries, while ownership remains the leading database predicate.

Migration `00012_nonlive_strategy_scheduler.sql` adds one optional operational schedule per strategy instance. Each row is bound to the owner and exact immutable mandate version, constrains cadence to 30–1440 minutes and the U.S. regular-session policy, and records next-run, lease, stable result code, and consecutive-failure status. Leases are paired and time-bounded; they grant no live capability and do not replace the immutable mandate or evaluation-event idempotency record.

Migration `00013_paper_lifecycle_events.sql` adds immutable, owner-scoped event identities for explicit PAPER Wheel outcomes: worthless expiry, assignment, and called-away shares. Each event binds to one strategy instance and resulting state version, stores only normalized simulated contract/accounting evidence, and is protected by the same history-mutation trigger as state transitions and Decision Journal entries. Applying an event locks the instance, derives the one open simulated option from `paper_positions`, updates the PAPER cash/share ledger, appends the lifecycle event, transition, and journal evidence, and advances the optimistic state version in one transaction. No request field can supply a strike, quantity, cash amount, provider identity, or broker instruction.

Migration `00014_strategy_capital_bucket_binding.sql` backfills every strategy instance from its exact immutable mandate-version snapshot, then makes the same-owner capital bucket a required foreign key. A partial unique index allows at most one `ACTIVE` or `PAUSED` strategy instance per owner/bucket pair. This prevents direct reuse of one bucket by concurrent non-live instances; it does not claim to solve aggregate reservation accounting across distinct buckets, which remains deferred.

Migration `00015_nonlive_account_exclusivity.sql` adds a conservative partial unique index allowing at most one `ACTIVE` or `PAUSED` strategy instance per owner/financial-account pair. This prevents separate buckets from concurrently claiming one account while the full reservation ledger remains deferred. Paused instances intentionally retain the claim; completed or errored instances release it.

Migration `00016_paper_options_simulation_attestation.sql` adds a default-false mandate field constrained to options-enabled PAPER strategy configurations. The value is copied into every new immutable version snapshot. It records an owner decision to permit simulation when provider capability is `UNKNOWN`; it neither changes provider capability nor applies to a confirmed `UNSUPPORTED` account, SHADOW, LIVE, or broker execution.
