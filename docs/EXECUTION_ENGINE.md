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

An owner may now save that normalized evidence as a durable, owner-scoped, non-executing Order Intent. The proposal records its UI or internal AI source, exact request hash, one-minute evidence expiry, immutable preview revision, append-only events, and an idempotency key. A fresh non-replayable TOTP step may change `REVIEW_REQUIRED` to `USER_APPROVED_NONEXECUTABLE`, but the stored review scope is permanently `PROPOSAL_REVIEW_ONLY`. This is review of the proposal the owner saw—not risk approval, a live-order authorization, a provider submission attempt, or permission to reuse stale evidence.

The implemented Neural Engine bridge is intentionally narrower than a trading tool. The owner fixes the Coinbase account, asset, side, maximum exact size, and capital policy. OpenAI receives only the objective and normalized cash/target-position facts, may recommend a smaller size or abstain, and receives no financial credential, provider account identifier, or provider preview identifier. Go revalidates the structured response and routes a proposed size into the same fresh Coinbase preview, reservation, deterministic risk, and durable Order Intent path. Abstention never contacts the preview endpoint. No model-facing tool, provider submission method, or AI approval path exists.

Every new reviewable proposal also commits one immutable reservation for the same one-minute evidence window. BUY reserves the previewed order total plus commission in USD; SELL reserves the requested base-asset quantity. Deterministic risk subtracts existing account/bucket cash reservations or target-asset reservations before allowing another proposal. A per-account PostgreSQL transaction lock rechecks the exact reservation snapshot at commit, so concurrent UI or future AI proposals cannot both rely on the same capital facts. Blocked proposals never reserve resources, expiration releases capacity without deleting evidence, and the reservation grants no execution authority.

Coinbase key enrollment requires View, permits Trade, and rejects Transfer. A provider Trade grant is a capability fact—not Arbion approval, a capital allocation, a risk decision, or execution authority.

## Coinbase live-execution approval gates

Before a Coinbase `Create Order` adapter may exist, the following must be implemented and reviewed together:

1. complete the current durable non-executing intent/review/event foundation with canonical live legs, execution approvals, dispatch attempts, provider correlation, and reconciliation records;
2. preserve the implemented exact product/precision evidence and bounded preview expiry through every later risk, approval, and dispatch transition;
3. deterministic Risk/Control evaluation against current account, reserve, mandate, breaker, and market facts;
4. explicit owner approval with step-up authentication for manual live orders, plus an immutable mandate path for any later automation;
5. an Arbion-owned stable idempotency key used as Coinbase `client_order_id`, with a transactionally claimed dispatch attempt;
6. unknown-outcome handling that stops and reconciles instead of blindly retrying;
7. broker-authoritative order/fill polling, drift detection, the implemented automation/account/owner/platform kill-switch hierarchy, and operational alerts; and
8. a dedicated security review proving that neither the browser nor Neural Engine can receive financial credentials, preview IDs, provider order IDs, or direct dispatch authority.

The AI-facing tool set may eventually include structured proposal and preview tools. It must not contain an unrestricted `place_order` tool. An AI proposal enters the same durable Order Intent and deterministic control path as a UI ticket and cannot satisfy its own approval requirement.

## Approved personal pilot implementation — durable dispatch foundation

On September 28, 2026 the owner approved the proposed Coinbase-only architecture/security scope for implementation, not live activation or broker orders. The scope is one owner, an isolated portfolio, one spot USD pair, price-bounded IOC limits, and one unresolved order at a time. Capital limits and actual activation remain separate explicit decisions. Existing proposal-review approvals remain permanently non-executable.

`internal/execution` now persists immutable execution requests separately from non-executable preview intents. Exact account, connection, owner, bucket, client UUID, order terms, and all-in BUY debit bounds are digest-bound; identity reuse with changed terms fails. Composite foreign keys and insertion guards preserve owner/account/provider attribution.

The PostgreSQL claim boundary locks the account, requires an injected transaction-scoped current-authority evaluator, rechecks authorization expiry using database wall time after all waits, and atomically records one attempt. No production authority implementation exists: missing authority denies. There is at most one attempt per order and one unresolved attempt per account. A commit error returns no dispatch receipt, even if the database actually committed. A new worker/process recovers the persisted record but never acquires a second send claim.

Broker acknowledgements are private immutable correlation only, not fills or settlement. Unknown outcomes and acknowledged orders retain the account's active submission slot. Immutable attempts are permanent; migration50 replaces permanent account uniqueness with a guarded active slot, seeded from existing attempts and claimed atomically on every new attempt. A terminal order can never be submitted again, even after its account slot is released. Preparing an order does not reserve capital; the future authority implementation must commit a durable capital reservation with the attempt, coordinating the existing account/bucket controls.

The private reconciliation boundary now records exact dispatch-bound base quantities, USD prices, gross values and fees, deduplicated by order/trade identity. Equivalent decimal spellings and later polling times do not create new fills; changed economic facts conflict. Price, quantity, fee allowance, all-in BUY debit and nonnegative SELL proceeds are checked exactly. Unknown/malformed, future, pre-attempt, over-limit, conflicting, or late new fills fail closed. Out-of-order arrival of valid fills is allowed; completeness is not inferred from delivery order.

Only a final FILLED/CANCELLED/REJECTED/EXPIRED report with explicit complete fill pagination, exact saved count/quantity/gross/fee totals, and consistent timestamps may release that order's submission slot. Incomplete or unmatched terminal reports retain the slot and can be retried after the missing fills arrive. FILLED requires the full requested quantity; REJECTED requires zero fills. This is order-level reconciliation, **not cash/position reconciliation or release of financial capital**. The future provider adapter must establish actual broker attribution, fee units, fill completeness and final status; the existing bounded history reader is not sufficient evidence.

Conflicting corrections and new fills arriving after a final report durably block the entire account, even when a newer order already holds its slot. No automatic correction, unblock, reset, replacement, or liquidation path exists. Rejected observations are retained in bounded private records (oversized payloads retain only the account stop). No accepted historical fill is rewritten. The final receipt and slot release are atomic; a lost commit response is recovered by replay/read without double settlement. SQL guards independently serialize same-account mutations, bind payloads to relational facts, preserve immutable fills/receipts/blocks, and reject slot deletion without a matching terminal receipt. Late evidence cannot undo a network request already in flight; the future sender must check blocks at its final send boundary.

This package is not connected to HTTP, the scheduler, AI, provider writes, or production runtime. Its tests use a test-only authority and no financial providers. A successful claim is only the durable pre-send record, not a complete live authorization protocol: the future sender must serialize revocation/kill switches through the actual send boundary, and cannot use an expired receipt. The account-lock ordering must be reviewed with the existing control-plane writers before wiring.

## Remaining execution work

The durable dispatch and order-reconciliation foundation above is implemented but not wired. Still required: the transactionally current live-authority and durable capital-reservation adapter; exact LIMIT_IOC product/preview verification; a revocation-safe send boundary; the narrow Coinbase write/read adapter with complete fill pagination; account cash/position reconciliation and financial-capital settlement; cancellation/kill-switch coordination; and the separately approved bounded pilot. No broker-write job or live runtime exists. The existing `order_intents`, proposal reviews, and expiring preview reservations must not be promoted into execution authority. The [private fill observation store](PRIVATE_FILL_EVIDENCE.md) remains read-only history, not dispatch-bound settlement. Options, replacement orders, multi-leg execution, and other brokers are outside the personal pilot.

The separate [offline lifecycle laboratory](SIMULATION_LIFECYCLE.md) implements executable fixture state transitions and durable local replay for testing these mechanics now. Its fictional attempts, fills, and cash movements never enter production accounts or the Paper/Shadow scheduler. The fixture configuration is not risk approval, the synthetic provider labels do not certify broker compatibility, and passing the scenarios does not satisfy the live-execution approval gates.

## Proposed-action boundary

The implemented `ProposedAction` is not an Order Intent or broker payload. After structured risk evaluation it may reach only a PAPER simulation or SHADOW record; it cannot reach Schwab or another broker-write interface. No role, model, strategy, UI, or conversation may bypass the Risk/Control Engine.

## Implemented non-live adapters

The provider-independent non-live adapter boundary accepts the existing Risk Engine `ProposedAction`, a successful `RiskEvaluation`, supplied market facts, and an expected deterministic state. PAPER uses a conservative deterministic bid-based option-credit fixture and rejects missing/invalid price data. SHADOW records `WOULD_HAVE_SUBMITTED` and an expected transition but mutates neither paper nor real holdings.

Non-live statuses are `PROPOSED`, `RISK_DENIED`, `SIMULATED_FILLED`, `SIMULATED_REJECTED`, `WOULD_HAVE_SUBMITTED`, `CANCELED`, and `ERROR`; they are not broker order states. Durable event identities and unique idempotency keys prevent duplicate simulated fills. The PostgreSQL implementation claims the evaluation event and commits risk evidence, execution evidence, paper accounting, journal evidence, and optimistic state transitions in one transaction, avoiding both duplicate effects and abandoned pre-claims.

**Paper execution is simulation and never represents a broker fill.**

**Shadow mode records what Arbion would have attempted but never submits an order.**
