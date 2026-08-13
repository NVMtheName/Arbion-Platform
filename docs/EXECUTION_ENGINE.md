# Execution Engine

## Boundary

The future Execution Engine accepts only provider-independent Order Intents that have traversed the same domain path regardless of source: UI, conversation, automation, or strategy. It does not accept prose or model tool calls as execution authority. Connector adapters translate an authorized intent; they do not authorize it.

An Order Intent should eventually identify the Arbion order ID, user, account, exact mandate version when applicable, strategy instance when applicable, source, normalized legs and order terms, provider order ID when known, timestamps, approval evidence, idempotency identity, and current execution state.

The manual UI path is:

```text
Trade Ticket -> Structured Order Intent -> Control/Risk Validation
 -> Preview -> User confirmation when required -> Execution
```

Ask Arbion and automation converge on the same Order Intent and downstream services. There is no chat-specific or UI-specific order logic.

## Lifecycle

The conceptual lifecycle vocabulary is:

```text
PROPOSED -> VALIDATED -> PREVIEWED -> AUTHORIZED -> SUBMITTED
 -> ACKNOWLEDGED -> PARTIALLY_FILLED -> FILLED
                         |                 |
                         +-> CANCEL_PENDING -> CANCELED

Any applicable stage -> REJECTED or FAILED
```

This diagram is not a promise that every order visits every state or that states alone capture ambiguous provider outcomes. Validation means Arbion checks passed at a point in time; authorization records approval; submission means only that a request was attempted. **Submitted never means filled.** A timeout after submission is an unknown outcome requiring reconciliation, not evidence of failure safe to retry blindly.

Broker truth is authoritative for live execution state. Arbion preserves provider status and provenance while mapping it into the normalized lifecycle. Strategy state and user-facing positions must not assume a fill based on request success or acknowledgement.

## Idempotency and duplicate protection

Every eventual side-effecting request must carry an Arbion-owned idempotency key derived from a stable operation identity, not generated anew on retry. Arbion records submission attempts and provider correlation identifiers before/around dispatch using a failure-safe protocol. Provider-native idempotency is used where available, but never relied on as the sole defense. Concurrent dispatch, worker redelivery, user retries, timeouts, and failover must not create duplicate orders.

Exact key format, persistence protocol, retry windows, and provider-specific behavior remain deferred. When outcome is uncertain, execution stops and reconciles rather than issuing a semantically duplicate order.

## Reconciliation

Reconciliation continuously compares Arbion records with broker-authoritative orders, fills, positions, balances, and capabilities. It detects fills, partial fills, cancellations, rejections, external/manual trades, position drift, balance drift, lost acknowledgements, and provider corrections. Differences generate structured events, journal entries, operational visibility, and—where safety requires—a circuit breaker or manual review.

Reconciliation must be restartable, idempotent, ordered by provider evidence where possible, and safe under delayed or duplicate webhooks/polls. External trades are facts to incorporate, not transactions to overwrite. A reconciliation mismatch cannot be resolved by AI assertion.

## Capability-aware adapters

Connected accounts expose discovered, freshness-bearing capabilities such as equities, options and options level, margin, short selling, crypto, fractional shares, extended hours, and supported order types. Domain validation prevents unsupported intents, and UI/conversation should avoid offering impossible actions. Capability changes invalidate stale previews and may trip a circuit breaker.

The same strategy core uses Historical, Paper, Shadow, or future Live adapters as described in [Strategy Engine](STRATEGY_ENGINE.md). Shadow produces records but has no submission capability. The deterministic gate is specified in [Risk and Control Engine](RISK_CONTROL_ENGINE.md).

## Deferred decisions

No order tables, endpoints, adapters, SDKs, reconciliation jobs, or provider calls are added by this design. Canonical order/leg schemas, precision rules, preview expiry, provider mapping, webhook/poll strategy, cancellation/replacement semantics, correction handling, multi-leg guarantees, and live retry protocols require future provider-specific threat modeling and approval.
