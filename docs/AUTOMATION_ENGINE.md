# Automation Engine

## Status and product principle

This document defines the target architecture and identifies the bounded portions already implemented. **Arbion is UI-first and conversationally enhanced.** Traditional controls and Ask Arbion are equal control surfaces over the same domain. Neither is privileged, and neither may bypass authorization, mandates, risk, approval, audit, or execution policy.

A UI automation builder and the sentence "Run the wheel on my Schwab account with $20,000 using Claude, conservative risk, and don't use margin" must produce the same validated command and the same Automation Mandate. Conversation history is context, not truth; durable structured Arbion objects are truth.

```text
UI controls ───────────┐
                      ├─> Structured Arbion Command/Intent
Ask Arbion interpreter ┘             │
                                     v
                        Domain validation and authorization
                                     │
                         Mandate / risk / approval path
```

## Automation Mandate

The **Automation Mandate** is the authoritative durable object describing exactly what Arbion is permitted to control. A model, UI, conversation, strategy, or worker may narrow a mandate but never exceed its active version. PostgreSQL is authoritative; Redis may coordinate work but cannot own configuration or enablement.

Conceptually a mandate contains:

- user and financial account;
- automation type and, when applicable, strategy and strategy parameters;
- AI connection and model when applicable;
- capital allocation, reserve capital, and optional capital-bucket reference;
- allowed instrument universe and explicitly prohibited instruments;
- margin and options permissions;
- autonomy level and execution mode;
- risk limits and schedule/operating conditions;
- enabled or paused state and effective dates; and
- created and updated timestamps.

Every meaningful change creates a new immutable version. For example, version 12 may authorize $10,000 while version 13 authorizes $20,000. Historical versions are never overwritten. Every decision, strategy transition, and order produced under a mandate records the exact version, so later reconstruction does not depend on present configuration.

## Automation types

| Type            | User selects                                                                         | AI role                                                                                                            |
| --------------- | ------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------ |
| `AI_AUTONOMOUS` | Account, AI provider/model, allocation, autonomy, risk, universe, and execution mode | May propose or select opportunities within the mandate; deterministic controls validate every action               |
| `STRATEGY`      | Account, deterministic strategy, allocation, parameters, risk, and execution mode    | Not required                                                                                                       |
| `HYBRID`        | Deterministic strategy plus AI connection/model and bounded decision parameters      | May choose values such as underlying, strike, or expiration only inside legal strategy transitions and the mandate |

Examples of deterministic strategies include Wheel, Covered Call, Cash-Secured Put, Collar, and future spread strategies. The detailed state-machine contract is in [Strategy Engine](STRATEGY_ENGINE.md).

## Autonomy levels

Autonomy controls approval behavior, never risk enforcement:

- `RESEARCH_ONLY`: analysis only; no order proposal requiring execution.
- `SUGGEST`: trade ideas only; executable action requires user initiation.
- `CONFIRM_EACH`: Arbion may build and preview an order; the user approves every trade.
- `STRATEGY_AUTONOMOUS`: autonomous execution is permitted only within the selected deterministic strategy and mandate.
- `FULL_AUTONOMOUS`: AI-directed automation may execute without per-order approval only inside the mandate and deterministic control system.

An autonomy level cannot waive policy, account capabilities, risk limits, or circuit breakers. Live behavior remains prohibited until a separate implementation task and safety approval.

## Capital buckets

Broker balance and buying power describe provider capacity; they do not grant Arbion permission. Users explicitly allocate capital to named buckets, including protected and reserve buckets. For a $50,000 balance, a user might allocate $20,000 to AI Autonomous, $15,000 to Wheel, $10,000 to protected holdings, and $5,000 to reserve.

Mandates may reserve all or part of a bucket. The control engine accounts for concurrent reservations and prevents automations from unintentionally double-allocating capital. Future percentage rules such as 25% of available cash or 50% of buying power are evaluated against fresh account state, but remain capped by explicit absolute limits and minimum reserves. Allocation reductions must account for deployed and pending capital rather than creating impossible authorization.

## Equal user experiences

The future standard UI automation builder must expose account, automation type, strategy, AI provider/model, capital, autonomy, risk, margin, execution mode, and **Review & Enable** without requiring conversation. Its review produces a structured mandate command and shows consequential fields before activation.

Ask Arbion accepts equivalent natural-language requests to create, modify, pause, inspect, or explain automation. Natural language first becomes a typed Arbion Intent; ambiguous or consequential fields require resolution or approval. Notification actions such as **Continue Automatically**, **Review**, and **Pause** also map to the same commands rather than embedding special notification logic.

```text
Natural language -> intent interpretation -> Structured Arbion Intent
 -> authorization -> mandate validation -> risk/control validation
 -> preview/approval when required -> execution boundary
```

Future manual trading is equally UI-complete: account, instrument, side, quantity, order type, limit/stop, time in force, options contract where applicable, preview, and execute. The path is Trade Ticket -> Structured Order Intent -> control/risk validation -> preview -> required confirmation -> execution. Conversation is optional. See [Execution Engine](EXECUTION_ENGINE.md).

## Server-side operation and administration

Authorized automation operates server-side; browser presence and login-session lifetime are irrelevant. The implemented opt-in scheduler evaluates the immutable READY version pinned to each active `STRATEGY_AUTONOMOUS` PAPER/SHADOW or `AI_AUTONOMOUS` SHADOW instance. A newer mutable DRAFT may be reviewed without changing or interrupting that pinned runtime; explicitly PAUSED, DISABLED, or ARCHIVED mandates still stop eligibility. The scheduler uses database leases and stable event IDs, and it cannot preview or submit an order. Its reviewed NYSE calendar covers the exchange-published 2026-2028 closures and early closes, retains the conservative 9:35 a.m. open and five-minute pre-close cutoff, and fails closed with `SESSION_CALENDAR_UNAVAILABLE` outside that horizon. A mandate version may separately opt into informational email after each completed evaluation, on the first lifecycle-required observation, on the first consecutive failure, or—only for AI Shadow—when a new blocking tradable-inventory reconciliation needs owner review. After a successful AI Shadow schedule is durably completed, an existing completed-evaluation email is enriched from the owner-scoped scorecard with either collecting or reviewable evidence-gate language. Missing or unrecognized scorecard state produces the generic completion message; it does not add a notification, infer readiness, or change any schedule or execution state. The deterministic risk engine consumes the latest immutable broker reconciliation for AI-autonomous proposals. Missing, legacy-advisory, baseline-only, incomplete, blocking-drift, future-dated, or older-than-24-hour evidence produces an immutable risk-held decision. A complete Coinbase snapshot may remain `MATCHED` while retaining an exact non-blocking change only when available-to-trade is unchanged and both total/available/unavailable equations reconcile; this records provider truth without inventing its cause or treating inaccessible quantity as sell authority. The scheduler and existing positions remain active, and no reconciliation state can grant execution authority. The implemented drift email links to the existing account review surface and has no action control; broader workers that execute live work, automatically acknowledge blocking reconciliation drift, or expose actionable notification commands remain unimplemented.

A superadmin-only production operations snapshot now shows credential-free aggregate AI Shadow engine counts, schedule failures, reconciliation blockers or staleness, unavailable attached financial connections, open emergency stops, and explicit Shadow-only execution-boundary violations. Both the session role and the current active PostgreSQL role are checked, and the read is `no-store`; it exposes no user, account, symbol, provider identifier, credential, or raw error. Broader worker and provider telemetry remains future work. Automation-, account-, and owner-wide user-level emergency controls are implemented as audited owner actions. The account control applies only to the selected owner/account boundary, while the owner-wide control applies only to accounts and automations belonging to the authenticated owner. A separate superadmin-only platform stop is durable and applies to every risk-gated action; it can be engaged immediately with an explicit reason and confirmation, while release requires a fresh non-replayable TOTP code. No operations read or emergency control closes positions, changes broker state, or grants trading authority.

Every authenticated owner also receives an in-app Attention Center derived from durable current state rather than email delivery. It reports active non-live schedule failures, enforced portfolio drift or incomplete-evidence blockers, unavailable financial/AI connections and accounts, and applicable open safety stops. A condition disappears only when its authoritative source state clears. This is not an unread-message or acknowledgement ledger: the bounded endpoint and dashboard are read-only, expose fixed safe codes instead of raw diagnostics or portfolio/provider data, and link back to the existing review surfaces. An unavailable query is displayed as unavailable and never treated as a healthy scheduler or cleared control.

## Decision Journal and explainability

Every important automated or AI-assisted decision must be reconstructable from a structured Decision Journal. A record should reference timestamp, user/account, automation, mandate version, strategy and state, AI provider/model when used, market-input reference IDs, concise structured rationale, proposed action, risk-check results, approval, resulting order and broker response, and resulting strategy state.

Arbion must not store private model chain-of-thought. It stores user-appropriate facts, assumptions, selected factors, rule results, and concise rationale. Questions such as "Why was this rejected?", "Which mandate allowed it?", and "How much capital is controlled?" are answered from journal, mandate, risk, order, and reconciliation records—not model recollection or guesswork.

## Deferred implementation decisions

Actionable notification commands and additional transports, reservation accounting, broader mandate-diff significance, consequential approval UX, authoritative unscheduled-closure ingestion, broker-authoritative lifecycle ingestion, and live enablement remain deferred. Implemented schemas and APIs are limited to durable mandate/non-live state, manual or guarded scheduled PAPER/SHADOW evaluation, an immutable owner-scoped scheduler-run ledger, a reviewed finite NYSE calendar, explicit owner-attested PAPER Wheel lifecycle events, owner-opted informational schedule email—including a link-only, evidence-ID-deduplicated drift-review alert—read-only observations, and audited owner-controlled automation, account, and owner-wide emergency stops. The stops are enforced by the deterministic risk gate and do not alter Coinbase or Schwab state. This document authorizes no broker trading implementation.

Each completed scheduler claim now advances the current schedule row and appends one immutable run record in the same transaction. The ledger preserves the exact due/start/completion/next-run times, pinned mandate version, PAPER/SHADOW mode and state, safe `SUCCEEDED`/`SKIPPED`/`FAILED` result, safe error code, resulting failure streak, duplicate-recovery flag, optional AI abstain/propose plus non-live adapter disposition, and same-account reconciliation reference when one was observed. It never stores raw provider errors or responses. The bounded owner-scoped `GET /api/strategy-instances/{id}/schedule-runs` route uses opaque stable pagination, `no-store`, and an instance ownership check; the automation page renders the record as an operational timeline and labels every entry non-live.

## Relationship to read-only financial connections

The financial connector supplies durable account inventory plus read-only balance, position, capability, quote, and option-chain observations. It also owns immutable owner/account-scoped reconciliation snapshots so deterministic controls can distinguish a first baseline, exact tradable-inventory continuity, exact Coinbase unavailable-only movement, broker-reported blocking changes, and missing coverage without inferring a cause. Enforced reconciliation evidence may deny a new AI proposal but grants no automation or execution authority; the separate automation domain owns mandates and buckets, while the strategy domain owns isolated PAPER/SHADOW records. A disabled financial connection is ineligible for evaluation; logout does not disable it. Unknown safety-critical capabilities fail closed except for a separately confirmed, immutable options-simulation attestation that applies only to PAPER; this warning-bearing simulation exception does not assert broker support or permit SHADOW/LIVE execution.

**Broker-reported buying power does not grant Arbion authority to deploy that capital.**

## Implemented durable mandate foundation

Migration `00006_automation_mandates.sql` evolves (renames) the original `automation_configs` table in place into `automation_mandates`; it does not create a competing active automation concept. It resolves each legacy connection-scoped row to one discovered, same-owner financial account, fails the migration when that cannot be done safely, creates a minimal migration bucket, converts its mode, and records the migrated configuration as immutable version 1 with source `SYSTEM`.

The Go `internal/automation` domain now owns typed Automation Mandates, Capital Buckets, the strategy metadata registry, lifecycle commands, structural/capability checks, entitlement checks, immutable snapshots, and expected-version concurrency. UI and future conversation must call this same service. Mandates use `DRAFT`, `READY`, `PAUSED`, `DISABLED`, and `ARCHIVED`; READY permits only separately invoked bounded evaluation and is never broker authority. PAPER and SHADOW have non-live adapters; BACKTEST and LIVE remain non-initializable configuration, and every mandate projection declares live execution incapable/disabled.

Capital buckets are account- and user-bound exact-decimal allocations. Fixed allocations can carry an explicit configured allocation limit, against which active fixed buckets are summed to detect deterministic overlap. Percentage allocations must be greater than zero and at most 100%, and may carry an absolute cap. Reserve buckets cannot be attached to a mandate. The implemented Risk/Control Engine revalidates the applicable allocation against mode-appropriate fresh facts immediately before a non-live action.

**A configured or READY Automation Mandate does not itself execute trades.**

**Broker-reported buying power is not Arbion trading authority.**

## Risk-gate integration

Every implemented manual or scheduled strategy proposal binds to the exact immutable READY mandate version pinned to its strategy instance and enters the deterministic risk registry. A newer DRAFT cannot initialize a replacement instance or affect that pinned runtime, while explicit PAUSED, DISABLED, and ARCHIVED status remains ineligible. Autonomy only changes approval semantics. A successful risk evaluation permits only the selected PAPER/SHADOW adapter and does not execute a broker trade.

## Non-live instance and journal implementation

A request can initialize and explicitly evaluate an implemented deterministic strategy from a READY `STRATEGY` mandate in PAPER or SHADOW mode, or an AI decision loop from a READY `AI_AUTONOMOUS` mandate in SHADOW mode. Deterministic schedules require `STRATEGY_AUTONOMOUS`; AI schedules require `FULL_AUTONOMOUS`, remain SHADOW-only, and use `CONTINUOUS` for Coinbase or `US_EQUITIES_REGULAR` for Schwab. Changing a schedule creates a new immutable DRAFT version. When an older AI SHADOW mandate predates the required daily-action ceiling, its first schedule edit adds a conservative one-action-per-day limit to that new reviewable version and records the upgrade in audit evidence. Initialization copies the exact mandate version and same-owner capital bucket into the instance, rejects direct bucket reuse, and conservatively permits only one active or paused non-live instance per financial account until aggregate reservations exist. PAPER starting cash must fit within the bucket's exact allocation after any absolute limit and protected amount; AI SHADOW creates no simulated cash. A paused instance keeps its account claim. Every evaluation reloads the exact pinned immutable version; the mutable mandate may remain READY or advance to a newer DRAFT, but an explicit PAUSED, DISABLED, or ARCHIVED state blocks evaluation. HYBRID evaluation remains unsupported.

The owner can pause an active instance with its expected state version, immediately removing it from both manual and scheduled evaluation while preserving its account claim and durable history. Resume requires explicit acknowledgement and succeeds only when the pinned immutable mandate version remains a valid READY snapshot and the mutable mandate has not been explicitly stopped; a newer DRAFT alone does not invalidate the pinned instance. Pause and resume append immutable transition and audit evidence. A paused PAPER Wheel can record a legal lifecycle outcome for an already-open simulated option without becoming active again, preventing pause from trapping unresolved simulation exposure.

The owner may explicitly finish an active or paused PAPER/SHADOW instance by submitting its expected state version and a non-live confirmation. PAPER finish fails closed while any simulated option or share quantity remains open. The transaction locks the instance, verifies zero PAPER exposure, advances its state version, sets the terminal `COMPLETED` status, appends immutable `FINISHED` transition evidence, and thereby releases the account claim. This operation is irreversible, preserves all simulation and journal history, records a user audit event, and has no broker or provider side effect.

The scheduler is disabled by default through `NONLIVE_SCHEDULER_ENABLED=false`. When enabled, it claims due work with a two-minute PostgreSQL lease, rechecks active founder entitlement, user status, effective dates, instance state, mode, mandate version, and schedule fields in the claim query, and uses an event ID derived from the durable due time. A crash retry therefore reaches the same database idempotency key. Work uses the reviewed 2026-2028 NYSE holiday and early-close calendar plus conservative 9:35 a.m. open and five-minute pre-close cutoffs; an expired horizon fails closed. PAPER states with an open option leg are recorded as `WAITING_FOR_LIFECYCLE` without contacting Schwab. Immediately before an actionable AI SHADOW evaluation, the scheduler asks the financial connector to ensure immutable reconciliation evidence is fresh. That check can use only balance and position reads, never order preview or broker writes. Missing evidence creates a baseline, baseline confirmation waits 30 minutes, healthy matches refresh after 12 hours, incomplete evidence retries after one hour, and confirmed drift remains untouched for explicit owner review. A hard reconciliation error prevents model evaluation and records only `RECONCILIATION_REFRESH_FAILED`; an appended incomplete or baseline snapshot proceeds into the deterministic reconciliation gate and therefore produces no allowed proposal. AI schedule failures distinguish the platform's exhausted decision window from an upstream provider throttle, while keeping raw provider text private. After durable schedule completion, an opted-in message may be sent only to the owner’s verified address. Lifecycle-required and failure messages are edge-triggered to avoid repeated mail. An opted-in AI Shadow drift alert is additionally keyed to the current blocking reconciliation ID, recorded only after successful delivery, and retried after delivery failure; it never acknowledges the evidence. Messages contain a review link and safe status only—no provider error, symbol, quantity, account data, approval action, or execution action. The worker stores only stable error codes, never provider text or credentials.

An owner may progress an open PAPER Wheel leg only through the authenticated, CSRF-protected lifecycle command. The request supplies a bounded event identity, legal event type, expected state version, and an explicit PAPER-simulation acknowledgement. It cannot supply contract or accounting facts. The persistence transaction locks the instance, derives the single open simulated option, applies the deterministic transition, updates simulated cash/shares, and appends immutable lifecycle, state-transition, and Decision Journal evidence. Worthless expiry is rejected before 4:00 p.m. America/New_York on the recorded contract expiry date. Assignment and called-away outcomes remain explicit owner attestations until broker-authoritative reconciliation is separately designed. Before that acknowledgement, the automation review loads the owner-scoped simulated cash and exact durable position facts from a no-store read endpoint; if exactly one matching open contract cannot be shown, the lifecycle control fails closed.

The standard UI now supports the same bounded setup lifecycle: an entitled user can create an account-scoped capital bucket, revise the attached active non-reserve bucket's allocation without changing broker permissions, save a mandate draft, set validated strategy parameters, configure an optional 30–1440 minute regular-session cadence, mark it READY with expected-version concurrency, pause or disable it, initialize an eligible PAPER or SHADOW strategy, and invoke a manual evaluation. A new AI SHADOW draft requires an immutable daily action ceiling between 1 and 48 and may add exact minimum-cash, total-exposure, single-position amount, and concentration limits; the review surface explains those server-enforced values without treating absent values as inferred policy. AI SHADOW rejects a realized daily-loss setting until broker-authoritative realized profit and loss exists. Parameter or schedule edits create a new immutable version and return the mandate to DRAFT. Reserve buckets, inactive accounts, and buckets belonging to another selected account are excluded from draft selection. PAPER initialization requires explicit simulated starting cash within the selected bucket's protected capacity; SHADOW initialization creates no simulated cash and never sends a broker order. LIVE configurations remain non-initializable.

Decision Journal entries are append-only structured decision evidence: selected facts, rule outcomes, references, and resulting state. Audit events are reserved for security- or lifecycle-significant commands such as initialization, reset, pause, and exceptional lifecycle handling; routine evaluations belong in the journal rather than flooding the security audit. Neither record stores private chain-of-thought.

The authenticated Decision Journal activity center provides a reverse-chronological, owner-scoped read model across the user's strategy instances. Its opaque cursor is derived from `(created_at,id)` for stable pagination, and its database query independently constrains journal, account, instance, risk, and execution joins to the authenticated owner. The projection contains safe normalized rationale, risk reasons, PAPER simulation evidence, and SHADOW would-have-submitted evidence; it contains no credentials, raw provider payloads, account numbers, or live-order capability.

Each automation review also exposes an immutable runtime-evidence ledger through two separate bounded projections. State transitions are reverse-ordered by `(state_version,id)` and PAPER/SHADOW execution results by `(created_at,id)`; both are owner/instance-scoped, capped at 50 per request, served `no-store`, and loaded 16 at a time. The projection omits persistence-only identity, idempotency, correlation, and metadata fields. Reading or paginating this evidence cannot evaluate a strategy, contact a provider, mutate state, or authorize a broker action.
