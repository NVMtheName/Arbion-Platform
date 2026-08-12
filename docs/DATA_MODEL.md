# Persistent data model

PostgreSQL is Arbion's durable system of record. Schema changes are explicit, versioned migrations run by `cmd/migrate`; API startup never creates or changes tables.

## Tables

- `users` supplies a stable UUID and authentication fields: original and normalized email, an Argon2id password hash, display name, account status, email verification time, last login time, and timestamps. A partial unique index on normalized email provides case-insensitive uniqueness while preserving compatibility with pre-authentication rows.
- `users.role` is durable administrative authority (`user`, `admin`, or `superadmin`). `user_entitlements` independently records effective product access, its source, lifecycle, and whether billing is required. Founder rows are constrained to be non-expiring and non-billing.
- `provider_connections` records AI or financial provider identity, status, scopes, safe credential metadata, token expiry, verification time, and either authenticated ciphertext or a managed-secret reference. Provider protocols remain adapter concerns.
- `automation_configs` holds future server-side strategy and risk configuration, provider references, paper/live mode, durable enabled state, and run timestamps. This schema does not implement workers or execution.
- `audit_events` is append-oriented and records actors, actions, resources, correlation identifiers, timestamps, and non-secret JSON metadata.

Browser sessions are separate and ephemeral. Provider connections and automation state persist across logout, browser closure, Redis loss, and application restarts. Stored provider credentials are server-side only and never part of normal API models.

## Deliberately deferred

Tenancy, provider adapters and OAuth flows, automation workers, order or trading execution, audit retention/integrity controls, and the production managed-secret provider require later designs.
