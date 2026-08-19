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

Authorized automation operates server-side; browser presence and login-session lifetime are irrelevant. The implemented opt-in scheduler evaluates only current READY, `STRATEGY_AUTONOMOUS` PAPER/SHADOW strategy versions. It uses database leases and stable event IDs, and it cannot preview or submit an order. Its reviewed NYSE calendar covers the exchange-published 2026-2028 closures and early closes, retains the conservative 9:35 a.m. open and five-minute pre-close cutoff, and fails closed with `SESSION_CALENDAR_UNAVAILABLE` outside that horizon. A mandate version may separately opt into informational email after each completed evaluation, on the first lifecycle-required observation, or on the first consecutive failure. Broader workers that request AI analysis, execute live work, monitor orders, reconcile, or expose actionable notifications remain unimplemented.

Future superadmin operations may show automation, worker, execution, provider, and reconciliation health. They must not reveal plaintext user credentials. Automation-, account-, user-, and global-level emergency controls are audited domain actions; administrative authority does not bypass user boundaries or safety policy.

## Decision Journal and explainability

Every important automated or AI-assisted decision must be reconstructable from a structured Decision Journal. A record should reference timestamp, user/account, automation, mandate version, strategy and state, AI provider/model when used, market-input reference IDs, concise structured rationale, proposed action, risk-check results, approval, resulting order and broker response, and resulting strategy state.

Arbion must not store private model chain-of-thought. It stores user-appropriate facts, assumptions, selected factors, rule results, and concise rationale. Questions such as "Why was this rejected?", "Which mandate allowed it?", and "How much capital is controlled?" are answered from journal, mandate, risk, order, and reconciliation records—not model recollection or guesswork.

## Deferred implementation decisions

Notification actions and additional transports, reservation accounting, broader mandate-diff significance, consequential approval UX, authoritative unscheduled-closure ingestion, broker-authoritative lifecycle ingestion, and live enablement remain deferred. Implemented schemas and APIs are limited to durable mandate/non-live state, manual or guarded scheduled PAPER/SHADOW evaluation, a reviewed finite NYSE calendar, explicit owner-attested PAPER Wheel lifecycle events, owner-opted informational schedule email, and read-only observations. This document authorizes no broker trading implementation.

## Relationship to read-only financial connections

The financial connector supplies durable account inventory plus read-only balance, position, capability, quote, and option-chain observations. It grants no automation or execution authority; the separate automation domain owns mandates and buckets, while the strategy domain owns isolated PAPER/SHADOW records. A disabled financial connection is ineligible for evaluation; logout does not disable it. Unknown safety-critical capabilities remain unavailable until resolved.

**Broker-reported buying power does not grant Arbion authority to deploy that capital.**

## Implemented durable mandate foundation

Migration `00006_automation_mandates.sql` evolves (renames) the original `automation_configs` table in place into `automation_mandates`; it does not create a competing active automation concept. It resolves each legacy connection-scoped row to one discovered, same-owner financial account, fails the migration when that cannot be done safely, creates a minimal migration bucket, converts its mode, and records the migrated configuration as immutable version 1 with source `SYSTEM`.

The Go `internal/automation` domain now owns typed Automation Mandates, Capital Buckets, the strategy metadata registry, lifecycle commands, structural/capability checks, entitlement checks, immutable snapshots, and expected-version concurrency. UI and future conversation must call this same service. Mandates use `DRAFT`, `READY`, `PAUSED`, `DISABLED`, and `ARCHIVED`; READY permits only separately invoked bounded evaluation and is never broker authority. PAPER and SHADOW have non-live adapters; BACKTEST and LIVE remain non-initializable configuration, and every mandate projection declares live execution incapable/disabled.

Capital buckets are account- and user-bound exact-decimal allocations. Fixed allocations can carry an explicit configured allocation limit, against which active fixed buckets are summed to detect deterministic overlap. Percentage allocations must be greater than zero and at most 100%, and may carry an absolute cap. Reserve buckets cannot be attached to a mandate. The implemented Risk/Control Engine revalidates the applicable allocation against mode-appropriate fresh facts immediately before a non-live action.

**A configured or READY Automation Mandate does not itself execute trades.**

**Broker-reported buying power is not Arbion trading authority.**

## Risk-gate integration

Every implemented manual or scheduled strategy proposal binds to the exact current immutable mandate version and enters the deterministic risk registry. `DRAFT`, `PAUSED`, `DISABLED`, and `ARCHIVED` cannot be evaluated; autonomy only changes approval semantics. A successful risk evaluation permits only the selected PAPER/SHADOW adapter and does not execute a broker trade.

## Non-live instance and journal implementation

A request can initialize and explicitly evaluate an implemented deterministic strategy from a READY `STRATEGY` mandate in PAPER or SHADOW mode. A strictly typed schedule can be enabled only for a `STRATEGY_AUTONOMOUS` PAPER/SHADOW mandate; changing it creates a new immutable DRAFT version. Initialization copies the exact mandate version and same-owner capital bucket into the instance, rejects direct reuse by another active or paused instance, and creates its operational schedule only when that version opted in. PAPER starting cash must fit within the bucket's exact allocation after any absolute limit and protected amount; a percentage bucket without an absolute limit cannot initialize an isolated PAPER ledger because its capacity cannot be determined without fresh account facts. Every evaluation reloads and requires that version to remain current. HYBRID evaluation is unsupported and does not imply AI participation.

The scheduler is disabled by default through `NONLIVE_SCHEDULER_ENABLED=false`. When enabled, it claims due work with a two-minute PostgreSQL lease, rechecks active founder entitlement, user status, effective dates, instance state, mode, mandate version, and schedule fields in the claim query, and uses an event ID derived from the durable due time. A crash retry therefore reaches the same database idempotency key. Work uses the reviewed 2026-2028 NYSE holiday and early-close calendar plus conservative 9:35 a.m. open and five-minute pre-close cutoffs; an expired horizon fails closed. PAPER states with an open option leg are recorded as `WAITING_FOR_LIFECYCLE` without contacting Schwab. After durable schedule completion, an opted-in message may be sent only to the owner’s verified address. Lifecycle-required and failure messages are edge-triggered to avoid repeated mail; delivery failure is logged with a stable kind and cannot change or retry the already completed strategy result. Messages contain a review link and safe status only—no provider error, account data, approval action, or execution action. The worker stores only stable error codes, never provider text or credentials.

An owner may progress an open PAPER Wheel leg only through the authenticated, CSRF-protected lifecycle command. The request supplies a bounded event identity, legal event type, expected state version, and an explicit PAPER-simulation acknowledgement. It cannot supply contract or accounting facts. The persistence transaction locks the instance, derives the single open simulated option, applies the deterministic transition, updates simulated cash/shares, and appends immutable lifecycle, state-transition, and Decision Journal evidence. Worthless expiry is rejected before 4:00 p.m. America/New_York on the recorded contract expiry date. Assignment and called-away outcomes remain explicit owner attestations until broker-authoritative reconciliation is separately designed. Before that acknowledgement, the automation review loads the owner-scoped simulated cash and exact durable position facts from a no-store read endpoint; if exactly one matching open contract cannot be shown, the lifecycle control fails closed.

The standard UI now supports the same bounded setup lifecycle: an entitled user can create an account-scoped capital bucket, save a mandate draft, set validated strategy parameters, configure an optional 30–1440 minute regular-session cadence, mark it READY with expected-version concurrency, pause or disable it, initialize an eligible PAPER or SHADOW strategy, and invoke a manual evaluation. Parameter or schedule edits create a new immutable version and return the mandate to DRAFT. Reserve buckets, inactive accounts, and buckets belonging to another selected account are excluded from draft selection. PAPER initialization requires explicit simulated starting cash within the selected bucket's protected capacity; SHADOW initialization creates no simulated cash and never sends a broker order. LIVE configurations remain non-initializable.

Decision Journal entries are append-only structured decision evidence: selected facts, rule outcomes, references, and resulting state. Audit events are reserved for security- or lifecycle-significant commands such as initialization, reset, pause, and exceptional lifecycle handling; routine evaluations belong in the journal rather than flooding the security audit. Neither record stores private chain-of-thought.

The authenticated Decision Journal activity center provides a reverse-chronological, owner-scoped read model across the user's strategy instances. Its opaque cursor is derived from `(created_at,id)` for stable pagination, and its database query independently constrains journal, account, instance, risk, and execution joins to the authenticated owner. The projection contains safe normalized rationale, risk reasons, PAPER simulation evidence, and SHADOW would-have-submitted evidence; it contains no credentials, raw provider payloads, account numbers, or live-order capability.
