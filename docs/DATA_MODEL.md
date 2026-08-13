# Persistent data model

PostgreSQL is Arbion's durable system of record. Schema changes are explicit, versioned migrations run by `cmd/migrate`; API startup never creates or changes tables.

## Tables

- `users` supplies a stable UUID and authentication fields: original and normalized email, an Argon2id password hash, display name, account status, email verification time, last login time, and timestamps. A partial unique index on normalized email provides case-insensitive uniqueness while preserving compatibility with pre-authentication rows.
- `users.role` is durable administrative authority (`user`, `admin`, or `superadmin`). `user_entitlements` independently records effective product access, its source, lifecycle, and whether billing is required. Founder rows are constrained to be non-expiring and non-billing.
- `provider_connections` records AI or financial provider identity, status, scopes, safe credential metadata, token expiry, verification time, and either authenticated ciphertext or a managed-secret reference. Provider protocols remain adapter concerns.

AI rows always use `provider_category = 'ai'`. Their safe credential metadata may contain only the masked display hint; ciphertext remains in the vault-owned storage column and is excluded from application/API connection models. Status `pending` means stored but not externally verified, while `disabled` preserves the encrypted credential but prevents future selection. Durable references block explicit deletion rather than cascading silently.

- `automation_configs` is the currently existing placeholder for future server-side configuration. It must not be treated as the permanent Automation Mandate model or used to infer that execution exists.
- `audit_events` is append-oriented and records actors, actions, resources, correlation identifiers, timestamps, and non-secret JSON metadata.

Browser sessions are separate and ephemeral. Provider connections and automation state persist across logout, browser closure, Redis loss, and application restarts. Stored provider credentials are server-side only and never part of normal API models.

## Target domain records (not implemented)

The permanent architecture requires normalized durable concepts for Automation Mandates and immutable versions, capital buckets and reservations, strategy definitions/instances/state transitions, structured Arbion Intents, provider-independent Order Intents and execution events, approvals, account capabilities, reconciliation observations, circuit breakers, and the Decision Journal. This is a conceptual inventory, not a request to add tables.

References between a decision, order, strategy transition, and mandate must identify the exact immutable mandate version. Significant changes append a version rather than overwriting history. Execution history is append-oriented and preserves provider evidence: `SUBMITTED` never implies `FILLED`, and broker reconciliation supplies authoritative live status.

Capital records must distinguish observed balance/buying power from explicit Arbion allocations, protected/reserve amounts, and pending reservations. The model must prevent unintended double allocation across concurrent mandates and retain absolute limits even when a future allocation is percentage-based.

Decision records store structured rationale, input references, rule outcomes, approvals, and resulting facts. They must not store private model chain-of-thought. PostgreSQL remains authoritative; Redis coordination and browser sessions do not own any of these facts. See [Automation Engine](AUTOMATION_ENGINE.md), [Strategy Engine](STRATEGY_ENGINE.md), [Execution Engine](EXECUTION_ENGINE.md), and [Risk and Control Engine](RISK_CONTROL_ENGINE.md).

## Deliberately deferred

Concrete schemas, tenancy, provider adapters and OAuth flows, automation workers, order or trading execution, audit retention/integrity controls, retention and partitioning, and the production managed-secret provider require later designs.
