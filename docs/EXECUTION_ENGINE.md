# Execution Engine

## Boundary

The target Execution Engine accepts only provider-independent Order Intents that have traversed the same domain path regardless of source: UI, conversation, automation, or strategy. It does not accept prose or model tool calls as execution authority. Connector adapters translate an authorized intent; they do not authorize it.

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

## Implemented Coinbase preview boundary

Arbion now has a narrow provider-independent `OrderPreviewProvider` implemented by the Coinbase adapter. It accepts only an authenticated owner-scoped account plus a canonical crypto symbol, BUY/SELL side, and exact positive amount. Before preview, the adapter reads authenticated Coinbase product metadata and exactly validates the product identity, spot type, base/quote increments and bounds, status, and current market-IoC restriction flags. BUY amounts are USD quote size; SELL amounts are base-asset size. Invalid metadata fails closed; a safe restriction produces a blocked proposal.

This operation calls Coinbase's real Advanced Trade preview endpoint and cannot submit an order. The immediate preview response contains normalized totals, commission, best bid/ask, estimated average fill, safe warning/block categories, and whether the encrypted key currently has Coinbase Trade permission. It excludes the Coinbase preview ID and every create/cancel/replace/transfer method. Browser responses state `order_created=false`, `submission_available=false`, `ai_execution_authority=false`, and `live_execution_available=false`.

An owner may now save that normalized evidence as a durable, owner-scoped, non-executing Order Intent. The proposal records its UI or internal AI source, exact request hash, one-minute evidence expiry, immutable preview revision, append-only events, and an idempotency key. A fresh non-replayable TOTP step may change `REVIEW_REQUIRED` to `USER_APPROVED_NONEXECUTABLE`, but the stored review scope is permanently `PROPOSAL_REVIEW_ONLY`. This is review of the proposal the owner saw—not risk approval, a live-order authorization, a provider submission attempt, or permission to reuse stale evidence. The internal AI proposal method is not wired to the Neural Engine or a public AI tool.

Coinbase key enrollment requires View, permits Trade, and rejects Transfer. A provider Trade grant is a capability fact—not Arbion approval, a capital allocation, a risk decision, or execution authority.

## Coinbase live-execution approval gates

Before a Coinbase `Create Order` adapter may exist, the following must be implemented and reviewed together:

1. complete the current durable non-executing intent/review/event foundation with canonical live legs, execution approvals, dispatch attempts, provider correlation, and reconciliation records;
2. preserve the implemented exact product/precision evidence and bounded preview expiry through every later risk, approval, and dispatch transition;
3. deterministic Risk/Control evaluation against current account, reserve, mandate, breaker, and market facts;
4. explicit owner approval with step-up authentication for manual live orders, plus an immutable mandate path for any later automation;
5. an Arbion-owned stable idempotency key used as Coinbase `client_order_id`, with a transactionally claimed dispatch attempt;
6. unknown-outcome handling that stops and reconciles instead of blindly retrying;
7. broker-authoritative order/fill polling, drift detection, scoped kill switches, and operational alerts; and
8. a dedicated security review proving that neither the browser nor Neural Engine can receive financial credentials, preview IDs, provider order IDs, or direct dispatch authority.

The AI-facing tool set may eventually include structured proposal and preview tools. It must not contain an unrestricted `place_order` tool. An AI proposal enters the same durable Order Intent and deterministic control path as a UI ticket and cannot satisfy its own approval requirement.

## Deferred decisions

No live-order, dispatch-attempt, provider-correlation, broker-write, or reconciliation table/interface/job exists. The implemented `order_intents`, preview/product-evidence, proposal-review, and event tables deliberately cannot represent provider submission or execution approval. Canonical live order/leg schemas, provider mapping, webhook/poll strategy, cancellation/replacement semantics, correction handling, multi-leg guarantees, and live retry protocols require the approval gates above.

## Proposed-action boundary

The implemented `ProposedAction` is not an Order Intent or broker payload. After structured risk evaluation it may reach only a PAPER simulation or SHADOW record; it cannot reach Schwab or another broker-write interface. No role, model, strategy, UI, or conversation may bypass the Risk/Control Engine.

## Implemented non-live adapters

The provider-independent non-live adapter boundary accepts the existing Risk Engine `ProposedAction`, a successful `RiskEvaluation`, supplied market facts, and an expected deterministic state. PAPER uses a conservative deterministic bid-based option-credit fixture and rejects missing/invalid price data. SHADOW records `WOULD_HAVE_SUBMITTED` and an expected transition but mutates neither paper nor real holdings.

Non-live statuses are `PROPOSED`, `RISK_DENIED`, `SIMULATED_FILLED`, `SIMULATED_REJECTED`, `WOULD_HAVE_SUBMITTED`, `CANCELED`, and `ERROR`; they are not broker order states. Durable event identities and unique idempotency keys prevent duplicate simulated fills. The PostgreSQL implementation claims the evaluation event and commits risk evidence, execution evidence, paper accounting, journal evidence, and optimistic state transitions in one transaction, avoiding both duplicate effects and abandoned pre-claims.

**Paper execution is simulation and never represents a broker fill.**

**Shadow mode records what Arbion would have attempted but never submits an order.**
