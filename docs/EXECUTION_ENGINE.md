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

The PostgreSQL claim boundary locks the account, requires an injected transaction-scoped current-authority evaluator, rechecks authorization expiry using database wall time after all waits, and atomically records one attempt. The initial confirm-each `OwnerAuthority` and saved provider preflight verifier are implemented but remain unwired; missing authority or verifier denies. There is at most one attempt per order and one unresolved attempt per account. A commit error returns no dispatch receipt, even if the database actually committed. A new worker/process recovers the persisted record but never acquires a second send claim.

Broker acknowledgements are private immutable correlation only, not fills or settlement. Unknown outcomes and acknowledged orders retain the account's active submission slot. Immutable attempts are permanent; migration50 replaces permanent account uniqueness with a guarded active slot, seeded from existing attempts and claimed atomically on every new attempt. A terminal order can never be submitted again, even after its account slot is released. Preparing an order does not reserve capital. Migration51 atomically reserves the exact maximum all-in BUY debit or SELL base quantity with every attempt; no expiry, acknowledgement, or terminal report releases that capital.

The private reconciliation boundary now records exact dispatch-bound base quantities, USD prices, gross values and fees, deduplicated by order/trade identity. Equivalent decimal spellings and later polling times do not create new fills; changed economic facts conflict. Price, quantity, fee allowance, all-in BUY debit and nonnegative SELL proceeds are checked exactly. Unknown/malformed, future, pre-attempt, over-limit, conflicting, or late new fills fail closed. Out-of-order arrival of valid fills is allowed; completeness is not inferred from delivery order.

Only a final FILLED/CANCELLED/REJECTED/EXPIRED report with explicit complete fill pagination, exact saved count/quantity/gross/fee totals, and consistent timestamps may release that order's submission slot. Incomplete or unmatched terminal reports retain the slot and can be retried after the missing fills arrive. FILLED requires the full requested quantity; REJECTED requires zero fills. This is order-level reconciliation, **not cash/position reconciliation or release of financial capital**. The future provider adapter must establish actual broker attribution, fee units, fill completeness and final status; the existing bounded history reader is not sufficient evidence.

Conflicting corrections and new fills arriving after a final report durably block the entire account, even when a newer order already holds its slot. No automatic correction, unblock, reset, replacement, or liquidation path exists. Rejected observations are retained in bounded private records (oversized payloads retain only the account stop). No accepted historical fill is rewritten. The final receipt and slot release are atomic; a lost commit response is recovered by replay/read without double settlement. SQL guards independently serialize same-account mutations, bind payloads to relational facts, preserve immutable fills/receipts/blocks, and reject slot deletion without a matching terminal receipt. Late evidence cannot undo a network request already in flight; the future sender must check blocks at its final send boundary.

Claim now locks current owner/founder entitlement before account, connection and fixed-USD bucket controls, validates the current credential generation, and serializes GLOBAL/USER/ACCOUNT stops using the existing shared scope locks. The SQL insertion guard repeats validation after the authority callback with database wall time. A distinct capital fence persists after order-level reconciliation and conservatively prevents another order on that account until cash/position reconciliation is implemented. Active manual previews and non-Paper strategy allocations exclude a claim; reciprocal guards prevent new competing reservations. Paper money remains separate. A shared fence-row write rejects stale SERIALIZABLE snapshots after advisory waits. Claims require READ COMMITTED snapshots. These guards do not establish available broker cash/inventory, credential permissions, live approval or deterministic risk clearance.

### Exact owner authorization and available funding

The first controlled-order authority uses a separate immutable `EXACT_ORDER_CONFIRM_EACH` approval, never a proposal review or an autonomous mandate grant. A distinct execution step-up consumes a one-use TOTP against the exact enrolled factor; recovery codes and a legacy store without atomic factor comparison cannot satisfy it. Approval binds the immutable order digest, owner and current financial credential generation, lasts at most five minutes, and can be revoked through an append-only record. Claim locks the approval and factor so revocation or factor removal cannot race past claim commit. Financial credential payload/reference changes now advance the generation exactly once, independently of the credential writer; staged replacements and metadata do not invalidate active material.

`OwnerAuthority` requires a private provider preflight verifier on the same transaction. It checks proof freshness (at most 30 seconds), exact account/connection/order/key generation, and the latest enforced MATCHED/CLEAR reconciliation. USD cash and available cash, complete single-pair inventory and available quantity must match that reconciliation exactly; totals or buying power cannot replace available funds. Reconciliation child inserts serialize with the account lock, so incomplete or changing inventory fails closed. Competing financial reservations deny. The unchanged deterministic Risk Engine must independently return ALLOW with manual approval still required and platform execution still unavailable. The short-lived authorization records exact preflight and risk evidence and can commit only with its matching durable attempt.

This package is not connected to HTTP, the scheduler, AI, or production runtime. Tests use synthetic preflight evidence and mocked provider responses. A successful claim is only the durable pre-send record, not permission to send outside the one-shot coordinator below. Exact account settlement can release internal capital only through its separate unwired coordinator; no autonomous live-mandate authority exists.

### Private Coinbase preflight adapter

The Coinbase collector uses fresh key permissions (View and Trade, explicitly no Transfer), complete portfolio-scoped account pagination, product restrictions/increments/size bounds, a timestamped product book, and an exact price-bounded `sor_limit_ioc` preview. Its only POST is the non-executing preview endpoint. No submit/cancel method is added to the existing financial read interface. The key's permissioned portfolio must match the saved account; supplying a portfolio ID in a preview is not proof of key scope. Nonzero foreign assets, external holds, incomplete responses, missing safety fields, duplicate JSON keys, changed preview size, fee overruns, redirects, pagination loops and stale quotes fail closed. Unknown zero-balance assets cannot contribute cash or inventory. No market-order fallback exists.

The server-only capture service retrieves the encrypted financial key and reads providers outside transactions. A versioned vault read returns the generation from the same database snapshot as the encrypted material; old repeatable-read material cannot be relabeled with a newer control-store generation. It checks that version before provider reads and under save locks; a replacement during collection invalidates the result. Legacy unversioned vault reads cannot satisfy this boundary. Migration54 persists immutable private order/digest/account/key-bound evidence. `SavedPreflightVerifier` pins one exact evidence ID and revalidates it using only the claim transaction, with no HTTP client or vault. Provider-read and reconciliation timestamps remain distinct: both must be at most 30 seconds old, the quote at most 10 seconds old, and complete funding facts must agree exactly. A newer reconciliation requires new evidence. No snapshot is relabeled as fresh, and no drift is auto-cleared.

These tests establish local/mock contract behavior, not live provider compatibility. Preview estimates are not settlement; the pilot's conservative preview-consistency rules must be confirmed with authenticated read/preview evidence before activation. The eventual sender still needs current open-order/account checks, exact private preview correlation and a revocation-safe send boundary. Provider and preview IDs stay private. Reference: Coinbase [preview](https://docs.cdp.coinbase.com/api-reference/advanced-trade-api/rest-api/orders/preview-orders), [product](https://docs.cdp.coinbase.com/api-reference/advanced-trade-api/rest-api/products/get-product), and [product book](https://docs.cdp.coinbase.com/api-reference/advanced-trade-api/rest-api/products/get-product-book) contracts.

### One-shot send admission (unwired)

`SendConfirmed` loads the exact credential material and its generation outside authorization locks, then calls the real owner authority with a pinned saved preflight. Only this invocation's successful durable claim can reach the final send transaction. No API accepts a recovered attempt as send authority, and no callback receives a refreshed preview or replacement client identity. A lost claim commit response prevents the callback entirely. A crash before any network activity still leaves an unresolved attempt, intentionally requiring reconciliation rather than retry.

The final transaction reuses the current-control lock order, additionally locks the exact attempt, and revalidates owner approval/revocation, MFA enrollment, credentials, stops, account/bucket state, complete funding and deterministic risk. Only its own exact capital reservation is excluded from competing reservations; both own holds must remain, and any existing acknowledgement, fill, terminal or account block denies. The immutable authorization's exact preflight, reconciliation, generation and expiry must match. A final database wall-clock/control check bounds a monotonic local deadline by the original authorization, current evidence/approval, entitlement/connection expiry and a five-second maximum. The entire final-query round trip counts against that budget; scheduling delay cannot renew it.

The separate Coinbase execution adapter is synchronous, honors cancellation, attempts at most one exact price-bounded IOC submission, and disables redirects, transport/application retries and credential reloads. A valid acknowledgement saves only exact client/product/side/provider-order correlation, never a fill or capital release. Callback errors (including unclassified rejection), timeout, malformed correlation and ambiguous acknowledgement commit leave the durable attempt and both holds intact. GET-only recovery can discover exact correlation but never resubmit. Strict provider-reported rejection is immutable evidence separate from unknown outcomes, not a fabricated broker order or release.

Revocation committed before final locks prevents admission. Revocation arriving after admission waits for the bounded callback and cannot retract an in-flight order. **Losing the database session can release those locks while the network request continues.** This provides local serialization and durable no-resend protection, not strict broker-side fencing or guaranteed cancellation. The dedicated runtime security review must explicitly resolve the concrete transport's failure semantics, unknown-outcome lookup and cancellation coordination before activation; mocks do not certify them.

### Once-only cancellation and exact settlement (unwired)

`CancelBrokerOrder` claims one immutable cancellation attempt for the original known broker order. Only the invocation that successfully commits a new claim may enter the final provider boundary. It rechecks current scoped owner/account/connection access and credential generation under ordered locks, bounds the original five-second window by current access expiry, and charges all final database-response delay. Expired/revoked order approvals, stops and reconciliation blocks do not prevent a risk-reducing cancel; disabled owner/account/connection access still does. A terminal order is not a cancellation target. No replacement, resubmission or automatic retry follows a lost response or process crash.

The private Coinbase adapter requires fresh View/Trade/no-Transfer portfolio permissions and a matching unresolved order detail before a singleton `batch_cancel` request. It uses the existing isolated no-retry HTTP/1 transport. Coinbase describes this operation as initiating cancel requests in its [official SDK](https://github.com/coinbase/coinbase-advanced-py/blob/master/coinbase/rest/orders.py); an accepted response is not terminal evidence. Immutable ACCEPTED/NOT_ACCEPTED/UNKNOWN receipts never release the submission slot or reserved capital. Restart reads the same receipt, or UNKNOWN if none committed, without another cancellation request.

Complete GET-only final status/fill reconciliation must still match exact saved identities, quantities, gross and fees before releasing the submission slot. Separate account settlement then reconciles the original pinned opening cash/base against saved fills and USD fees plus two matching complete zero-hold account reads. Stable observations are not broker-atomic snapshots. Only an exact immutable settlement receipt can release internal capital once; unexplained changes retain the reservation. Original nanosecond evidence stays intact; relational timestamp columns use the database's own cast of that evidence. Neither operation changes real holdings or clears generic risk, reconciliation, stop or approval controls.

### Bounded security review of the inert slice

The assembled implementation at `d429462` received separate read-only reviews of
the execution store/authorization boundary and Coinbase adapter/credential/transport
boundary, plus a runtime reachability and stop-control review. No actionable
high/medium defect was found within that scope. This approves the inert component
boundary only, not authenticated runtime integration, real provider compatibility
or live activation. Existing proofs were inspected rather than rerun as an audit.

- `postgres_send_test.go` covers claim-to-send revocation, synchronous callback
  locking, monotonic deadline expiry and database-session loss. The additional
  `postgres_send_stop_test.go` specifically covers the first GLOBAL/USER/ACCOUNT
  stop after claim, a GLOBAL stop waiting on the admitted callback's actual
  advisory lock, and cancellation after that stop without releasing either hold.
- `postgres_recovery_test.go`, `postgres_cancellation_test.go` and
  `postgres_settlement_test.go` cover current scoped access, changed credentials,
  ambiguous commits, duplicate/restart behavior and receipt-only capital release.
  Coinbase synthetic integration covers the full cancel/fill race through exact
  account settlement; transport tests cover lost responses, timeout, redirects,
  disabled retries, wrong scope/terms and malformed evidence.
- `cmd/api/main.go` still constructs only the read/preview `Client`, not the
  `ExecutionAdapter` or execution coordinators. Existing financial interfaces
  contain no submit/cancel capability. Browser and model inputs cannot supply
  execution authority or obtain financial credentials through these interfaces.

Required integration constraints remain explicit: construct the runtime client
only from the fixed Coinbase HTTPS destination and trusted standard TLS/transport
configuration; do not accept owner/model-provided base URLs, proxy/dial hooks or
TLS settings. The adapter's isolated transport prevents replay but is not itself
a destination/TLS-policy boundary. Database locks cannot retract an in-flight
broker request after session loss. Stable account reads are not a broker-atomic
snapshot or protection against independent portfolio mutation.

An operational completion gap also remains: a saved provider-reported submission
rejection has no broker order ID, `loadRecoveryContext` returns
`ErrSubmissionRejected`, and terminal/account settlement requires a known broker
order. Its account and capital holds therefore remain indefinitely, as do unknown
attempts for which no positive broker identity can be recovered. This is
conservative containment, not successful lifecycle completion. Before the pilot,
the owner workflow needs a separately reviewed resolution policy/path for these
states; a rejection, empty history scan, expired attempt or owner acknowledgement
alone must never unlock capital or authorize resubmission. No remediation writer
or activation permission is implied by this review.

## Remaining execution work

The narrow durable dispatch, owner authority, Coinbase preflight/submit/recovery/status/fill/cancel adapters and exact account settlement above are implemented but not runtime-wired. The bounded inert-component review above is complete; final fault acceptance and authenticated owner-control/runtime integration still require their own evidence and approval. Resolve the rejected/unknown-attempt operational gate without weakening containment before the separately authorized pilot. Unattended execution additionally requires reviewed live-mandate authority; confirm-each approval cannot grant it. No broker-write job or live runtime exists. The existing `order_intents`, proposal reviews, and expiring preview reservations must not be promoted into execution authority. The [private fill observation store](PRIVATE_FILL_EVIDENCE.md) remains read-only history, not dispatch-bound settlement. Options, replacement orders, multi-leg execution, and other brokers are outside the personal pilot.

The separate [offline lifecycle laboratory](SIMULATION_LIFECYCLE.md) implements executable fixture state transitions and durable local replay for testing these mechanics now. Its fictional attempts, fills, and cash movements never enter production accounts or the Paper/Shadow scheduler. The fixture configuration is not risk approval, the synthetic provider labels do not certify broker compatibility, and passing the scenarios does not satisfy the live-execution approval gates.

## Proposed-action boundary

The implemented `ProposedAction` is not an Order Intent or broker payload. After structured risk evaluation it may reach only a PAPER simulation or SHADOW record; it cannot reach Schwab or another broker-write interface. No role, model, strategy, UI, or conversation may bypass the Risk/Control Engine.

## Implemented non-live adapters

The provider-independent non-live adapter boundary accepts the existing Risk Engine `ProposedAction`, a successful `RiskEvaluation`, supplied market facts, and an expected deterministic state. PAPER uses a conservative deterministic bid-based option-credit fixture and rejects missing/invalid price data. SHADOW records `WOULD_HAVE_SUBMITTED` and an expected transition but mutates neither paper nor real holdings.

Non-live statuses are `PROPOSED`, `RISK_DENIED`, `SIMULATED_FILLED`, `SIMULATED_REJECTED`, `WOULD_HAVE_SUBMITTED`, `CANCELED`, and `ERROR`; they are not broker order states. Durable event identities and unique idempotency keys prevent duplicate simulated fills. The PostgreSQL implementation claims the evaluation event and commits risk evidence, execution evidence, paper accounting, journal evidence, and optimistic state transitions in one transaction, avoiding both duplicate effects and abandoned pre-claims.

**Paper execution is simulation and never represents a broker fill.**

**Shadow mode records what Arbion would have attempted but never submits an order.**
