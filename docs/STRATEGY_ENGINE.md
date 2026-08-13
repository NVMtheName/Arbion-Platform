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

A future evaluation conceptually:

1. loads the active immutable mandate version and reconciled strategy state;
2. obtains mode-appropriate, provenance-bearing account and market inputs;
3. determines legal candidate transitions;
4. optionally requests bounded AI assistance for a Hybrid decision;
5. emits a structured proposal;
6. submits it to the authoritative control/risk engine;
7. previews and obtains approval as required by autonomy and mode;
8. sends an authorized intent to the applicable execution adapter; and
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

The strategy-definition format, transition/event taxonomy, pricing and fill simulation, corporate actions, assignment/exercise semantics, multi-leg atomicity, calendars, deterministic replay requirements, state migration, and mode-switch policy remain for later design. No engine or backtester is implemented here.
