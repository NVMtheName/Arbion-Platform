# Strategy Engine

## Deterministic strategy contract

Strategies are deterministic, versioned state machines—not loose prompts. A strategy definition owns states, events, guards, legal transitions, required account capabilities, invariant checks, and the data used to produce a provider-independent proposed action. The engine rejects undefined transitions and records the definition version and before/after states.

For a conceptual Wheel strategy:

```text
CASH -> SHORT_PUT -> (assignment) -> LONG_SHARES
  ^                                      |
  └────── (called away) <- SHORT_CALL <──┘
```

Expiration, closing, rejection, partial fills, and exceptional recovery require explicit events and transitions in the eventual specification; they must not be inferred from this simplified diagram. Initial future definitions may cover Wheel, Covered Call, Cash-Secured Put, and Collar, followed by vertical spreads and other multi-leg options strategies.

In `HYBRID` mode, AI may propose bounded decision variables—such as an allowed underlying, strike, or expiration—inside the current state's permitted transition. It cannot add a transition, skip a guard, reinterpret a fill, or mutate strategy state directly. The state engine, reconciled broker facts, mandate, and control engine remain authoritative.

## One engine, four execution modes

Core strategy evaluation must be identical across:

- `BACKTEST`: historical inputs and a historical/simulation execution adapter;
- `PAPER`: current inputs with simulated orders and fills;
- `SHADOW`: real account state, market data, strategy logic, and applicable AI decisions, but no order submission; and
- `LIVE`: approved real-provider execution, only after a separate architecture/security approval and explicit implementation task.

```text
                    Strategy Engine
                          |
        +-----------------+------------------+
        |                 |                  |
 Historical adapter  Paper adapter   Shadow adapter   Live adapter
```

Mode adapters supply clocks, market/account data, and execution observations. They must not fork or weaken strategy rules. Mode, adapter, strategy-definition version, and input provenance are journaled so results are comparable without implying that simulated fills predict live fills.

Shadow Mode must never submit an order. It records the intended order, expected capital impact, decision results, and risk results using real connected-account state and market data. Shadow observations do not reserve live capital or alter broker state, but still respect capability and safety validation so the user sees realistic behavior.

## Evaluation and state advancement

The implemented manual or guarded scheduled evaluation follows this path, with AI, live preview, and broker submission intentionally absent:

1. loads the active immutable mandate version and reconciled strategy state;
2. obtains mode-appropriate, provenance-bearing account and market inputs;
3. determines legal candidate transitions;
4. rejects Hybrid/AI evaluation in this milestone;
5. emits a structured proposal;
6. submits it to the authoritative control/risk engine;
7. records any approval requirement without creating a broker preview;
8. sends an allowed proposal only to the PAPER or SHADOW non-live adapter; and
9. advances durable state only from mode-appropriate authoritative facts, recording the journal entry.

For live execution, submission is not a fill. Strategy state that depends on execution advances only from reconciled broker events. Partial fills, external/manual trades, assignment, exercise, corrections, and unknown outcomes require explicit deterministic handling.

## Invariants

- A strategy instance binds to an exact definition version, mandate version, account, capital bucket, and execution mode.
- Re-evaluation and event delivery must tolerate duplicates; transition application is idempotent and concurrency-controlled.
- Strategy rules cannot authorize capital, instruments, margin, or account capabilities absent from the mandate.
- Paused/disabled automation and active circuit breakers prevent new action while preserving observable state.
- Mode switching is an explicit reviewed command, not an adapter toggle; whether state can carry across modes is deferred.

See [Automation Engine](AUTOMATION_ENGINE.md), [Risk and Control Engine](RISK_CONTROL_ENGINE.md), and [Execution Engine](EXECUTION_ENGINE.md).

## Deferred decisions

The remaining strategy-definition format, complete transition/event taxonomy, production-grade fill simulation, corporate actions, broker-authoritative assignment/exercise ingestion, multi-leg atomicity, authoritative unscheduled-closure ingestion and calendar-horizon maintenance, deterministic replay requirements, state migration, mode-switch policy, broader orchestration, and backtester remain for later design. The current implementation is deliberately limited to explicit manual or opt-in scheduled PAPER/SHADOW evaluation plus owner-attested PAPER Wheel lifecycle events.

## Implemented configuration registry

The Go automation domain exposes metadata for `wheel`, `covered_call`, `cash_secured_put`, and `collar`, including capability and parameter-schema hints. The registry itself remains configuration metadata; implemented strategy definitions, evaluation, the guarded scheduler, and non-live adapters live in `internal/strategy`. No order preview or live adapter exists. Definitively unsupported required options capability rejects configuration; `UNKNOWN` is saved only with `capability_unverified=true` and fails closed during action evaluation.

## Implemented deterministic non-live foundation

The existing automation registry is now the `StrategyDefinition` catalog: implemented definitions carry a definition version and initial state. A durable `StrategyInstance` is separate user state bound permanently to one account, execution mode, definition version, and exact immutable mandate version. Editing a mandate never retargets an instance.

Cash-Secured Put and Covered Call use explicit legal states and provider-independent normalized inputs. Wheel composes their shared option lifecycle transitions. Candidate selection filters symbol, option type, DTE, delta, and premium, then deterministically ranks by distance to target delta, earliest expiration, and lowest strike. Required missing data fails closed. Covered calls conservatively require 100 normalized equity shares per standard contract.

`StrategyEvaluationInput`, `MarketSnapshot`, and `OptionCandidate` use exact decimal strings and provider-derived timestamps. The engine has no Schwab, broker, market-vendor, or AI dependency. The surrounding service obtains read-only inputs through normalized financial interfaces and rejects missing or stale market timestamps. Explicit `EXPIRE_WORTHLESS`, `ASSIGNED`, and `CALLED_AWAY` events drive lifecycle state; no probability is invented.

Strategy parameters and schedule conditions are validated and normalized server-side. Saving either creates a new immutable mandate version and returns the mandate to DRAFT; the user must review and mark that version READY before initializing or evaluating its exact matching strategy instance. Initialization persists the immutable version's same-owner capital bucket and permits only one active or paused instance to hold either that bucket or its financial account. PAPER starting cash must fit within its protected absolute capacity; this is simulation-only and grants no broker authority. Paused strategies retain the account claim. Manual requests supply an event ID; scheduled runs derive one from the durable due time. The database atomically claims that identity with risk evidence, non-live execution evidence, the Decision Journal entry, and any PAPER-only state/accounting mutation.

Owner-scoped pause and resume commands use the current state version and append immutable same-state transition evidence. Pause immediately makes both evaluation paths ineligible without releasing capital. Resume requires explicit non-live confirmation and the exact bound mandate version to remain current and `READY`; a changed, paused, disabled, or archived mandate fails closed. An existing PAPER Wheel option may still receive an owner-attested lifecycle outcome while paused, after which the instance remains paused.

An owner-confirmed finish command terminally marks an active or paused non-live instance `COMPLETED` under expected-version concurrency. PAPER finish first requires every simulated option and share quantity to be zero. It appends a same-state `FINISHED` history record, preserves the PAPER portfolio and all evidence, disables future evaluation through the terminal instance status, and releases the conservative financial-account claim. It has no execution adapter or provider call.

For an open PAPER Wheel option, the owner can record only the legal deterministic lifecycle outcomes for the current state: worthless expiry or assignment for a put, and worthless expiry or called-away shares for a call. The command cannot provide a symbol, strike, quantity, or cash effect; those values are loaded from the one durable open simulated option. The database transaction closes that option, applies any simulated cash/share effect, advances the optimistic state version, and appends immutable lifecycle, transition, and journal evidence. Exact event-identity retries are idempotent. This records a PAPER fact only and does not inspect or change Schwab.

**The same deterministic strategy definition is intended to serve paper, shadow, and future live execution.** Future live execution remains unavailable and requires separate approval.
