# Financial Connectors

## Role and scope

The financial connector layer will translate Arbion's provider-independent financial operations into external provider APIs. Candidate providers include Schwab, E\*TRADE, Coinbase, Alpaca, Interactive Brokers, and future providers. This document defines boundaries only; it does not implement or claim support for a provider.

Connectors belong in the Go modular monolith because Go owns the control plane and financial integration boundary. A connector is an adapter, not a source of authorization or risk policy.

## Provider abstraction

Platform modules should depend on narrow capability interfaces rather than provider SDKs or wire formats. Provider-specific authentication, endpoints, identifiers, pagination, rate limits, errors, asset conventions, and order semantics stay behind adapters.

Conceptual capabilities may eventually include:

- accounts and account metadata;
- balances and buying power;
- positions;
- orders and order status;
- quotes and market data;
- transactions;
- order preview;
- order placement; and
- order cancellation.

Interfaces should be capability-based because providers do not offer identical products or semantics. Adapters must report unsupported capabilities explicitly rather than emulate unsafe behavior. Normalized models must preserve provider identifiers, timestamps, precision, currency, asset class, provenance, freshness, and raw status needed for reconciliation.

Order placement and cancellation are listed only to reserve the abstraction boundary. Live or automated trading remains prohibited until an explicit future architecture and security review approves its authorization, risk, approval, audit, idempotency, reconciliation, and failure behavior.

## Control-plane relationship

All connector calls originate from an authorized Go use case. Before a sensitive financial operation reaches an adapter, the control plane must deterministically evaluate applicable identity, tenant/account ownership, permission, risk, account status, buying power, order validity, approval, and execution policy. The connector must not accept an AI assertion as evidence that these checks passed.

Read operations are also permissioned and audited as appropriate. Connector responses are external input and require schema validation, normalization, provenance, and safe failure handling.

The Python Neural Engine does not call connectors directly. AI-requested reads or previews use controlled tools mediated by Go. AI-generated order proposals remain untrusted input.

## Credential isolation

Financial-provider credentials form a separate trust domain from AI-provider credentials. They must be available only to the minimum Go connector components that require them. They must never be included in AI prompts, tool payloads, browser responses, analytics, error messages, or logs, and must never be committed to Git.

After storage, the platform returns only non-secret connection metadata such as provider name, connection state, scopes, and safe account labels. Production credentials will require managed secret storage or envelope encryption, access auditing, rotation, revocation, and strict network and process access policies.

## OAuth and token lifecycle

Use OAuth or provider-supported delegated token authorization instead of collecting reusable usernames and passwords wherever possible. A future implementation must define:

- authorization-code flow and PKCE where applicable;
- exact requested scopes and incremental consent;
- state and redirect validation;
- encrypted access/refresh-token storage;
- refresh concurrency and token rotation;
- expiry, revocation, disconnect, and reauthorization behavior;
- webhook authenticity and replay protection; and
- redaction of headers, URLs, payloads, and provider errors.

OAuth does not remove the need for Arbion authorization: possession of a provider token must not by itself authorize a platform action.

## Reliability and audit expectations

Future adapters should use explicit deadlines, bounded retries with jitter, rate-limit handling, circuit protection where justified, and idempotency controls for side effects. The platform must distinguish accepted, rejected, pending, unknown, and reconciled outcomes. Audit events should record actor, policy decision, account, requested operation, provider correlation identifiers, and outcome without recording secrets.

## Deferred decisions

Later design must choose the initial providers and capabilities, canonical account/asset/order models, decimal and currency rules, market-data sources and licensing, polling/webhook strategy, OAuth scope sets, secret storage, reconciliation and idempotency schemes, sandbox certification, error taxonomy, provider outage behavior, and the approval model for any live execution.
