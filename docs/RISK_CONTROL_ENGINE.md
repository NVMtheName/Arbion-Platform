# Risk and Control Engine

## Authority and ordering

The deterministic Go Control Engine is the policy enforcement point for every proposed financial action. AI output, tool arguments, browser values, market data, and connector responses are untrusted input. No AI provider, consensus result, interface, strategy, adapter, worker, or administrator may bypass the engine or override a circuit breaker.

At minimum, the future gate evaluates authenticated identity, authorization, entitlement, account ownership, active mandate version, automation state, account capabilities, buying power, explicit capital allocation, reserved cash, concentration, daily loss, maximum trade and position size, trade frequency, strategy constraints, allowed/prohibited instruments, options permissions, margin policy, market/session conditions, data freshness, approval requirements, and circuit-breaker state.

Checks use authoritative, freshness-qualified inputs and produce structured allow/deny results with rule identifiers, policy versions, facts, limits, and concise user-safe explanations. A preview is time-bound; execution revalidates mutable conditions to prevent stale approval from bypassing changed risk.

## Layered limits

Effective permission is the intersection of platform policy, user/account policy, capital bucket, mandate version, strategy invariants, account capabilities, execution-mode rules, and current circuit-breaker state. The most restrictive applicable rule wins. Broker buying power is merely an upper observation and never expands explicit allocation.

Autonomy decides whether human approval is needed after validation. It never suppresses a check. Superadmin visibility and emergency authority do not grant access to plaintext credentials or permission to trade outside a user's mandate.

## Hard controls and circuit breakers

Mandatory hard controls remain outside AI authority. The future policy set includes:

- maximum daily loss, capital deployed, order size, position concentration, and trades per day;
- minimum cash reserve and explicit allocation boundaries;
- broker connectivity failure, stale or abnormal market data, and extreme volatility;
- repeated execution failure and reconciliation mismatch;
- account-capability change; and
- automation-, account-, user-, and Arbion-global kill switches.

A breaker has a scope, reason, source, activation time, state, and reset policy. Activation fails closed for affected new actions, is durable where safety requires, and is auditable. Reset requires an authorized structured command and satisfied recovery conditions; a model cannot activate a workaround or clear a breaker. Handling of cancellation or risk-reducing actions while a breaker is active requires explicit future policy rather than a blanket allow.

## Capital safety

The engine treats capital buckets and pending commitments as a ledger of permission. It prevents overlapping mandates from reserving the same funds and includes open orders, unsettled funds, strategy obligations, options assignment exposure, fees, and configured reserves as eventually defined. Percentage allocations remain bounded by absolute caps. Negative or stale headroom fails safely.

## Decision evidence and operations

Each evaluation contributes structured evidence to the [Decision Journal](AUTOMATION_ENGINE.md#decision-journal-and-explainability) and links to mandate/version, intent, account snapshot, market-input references, policy/rule versions, approvals, breaker state, and eventual order. Private model chain-of-thought is never required or stored.

Operational controls should expose aggregate automation, worker, broker, and reconciliation health without secrets. Global emergency actions and resets require strong authorization, correlation, immutable audit evidence, and future step-up/dual-control decisions.

See [Execution Engine](EXECUTION_ENGINE.md) for lifecycle and reconciliation and [Security](SECURITY.md) for trust boundaries.

## Deferred decisions

The policy language, numerical defaults, jurisdictional rules, loss calculation, cross-account aggregation, reservation accounting, snapshot freshness thresholds, breaker reset workflows, administrative dual control, audit integrity, and formal live-trading safety case remain deferred. This document implements no risk service or trading capability.

## Implemented mandate risk-policy foundation

Automation Mandate versions now preserve validated exact-decimal configuration for maximum deployed capital, single-position amount and percentage, daily loss, minimum cash reserve, maximum trades per day, margin policy, options policy, structured allowed/prohibited universes, and conditions. These fields are authorization inputs only: no risk evaluator or execution path is implemented. Configured allocation checks do not claim real-time broker sufficiency.

**A configured or READY Automation Mandate does not itself execute trades.** **Broker-reported buying power is not Arbion trading authority.**

## Implemented deterministic evaluation foundation

The Go `internal/risk` package defines provider-independent `ProposedAction`, normalized account and daily-activity snapshots, stable reason codes, structured `RiskEvaluation` evidence, and an ordered rule registry. Exact decimal arithmetic avoids binary floating point, and security-critical unknowns fail closed. Migration `00007_risk_control.sql` adds durable scoped circuit-breaker and compact evaluation evidence tables. `LIVE` remains configuration metadata and every result reports platform execution unavailable.

**The Risk/Control Engine authorizes or denies proposed actions; it never decides that a trade is desirable.**

**A successful risk evaluation does not execute a trade.**

**No role, model, strategy, UI, or conversation may bypass the Risk/Control Engine.**

The package imports no connector or Neural Engine code and exposes no broker-write operation. Authenticated persistence services and production breaker mutation/evaluation HTTP wiring remain deferred; the safety view is informational and disabled rather than pretending client-side state is authoritative.

## Non-live strategy integration

The deterministic strategy orchestrator emits the existing `ProposedAction` and invokes this engine unchanged for PAPER and SHADOW. A DENY records denial and cannot fill or advance execution-dependent state. Circuit breakers, stale data, exact mandate version, account options capability, allocation, reserve, and autonomy rules remain authoritative; simulation has no risk bypass.

**Every strategy action must pass the Risk/Control Engine.** An ALLOW result permits only the selected non-live adapter in this milestone and is not broker authorization.
