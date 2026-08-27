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

The broader policy language, production numerical defaults, jurisdictional rules, realized-loss calculation, cross-account aggregation, durable execution and automation reservation accounting, breaker reset workflows beyond the owner-controlled automation, account, and owner-wide stops described below, administrative dual control, audit integrity, and formal live-trading safety case remain deferred. Manual Coinbase proposals now account for immutable one-minute cash/asset reservations and fail closed on a concurrent snapshot change. The implemented deterministic gate authorizes only current manual or guarded scheduled PAPER/SHADOW evaluation; it has no broker trading capability.

## Implemented mandate risk-policy foundation

Automation Mandate versions now preserve validated exact-decimal configuration for maximum deployed capital, single-position amount and percentage, daily loss, minimum cash reserve, maximum trades per day, margin policy, options policy, structured allowed/prohibited universes, and conditions. The deterministic risk evaluator consumes these fields during manual or guarded scheduled PAPER/SHADOW evaluation. Missing facts required by a configured rule deny the action, and configured allocation checks do not claim real-time broker sufficiency.

**A configured or READY Automation Mandate does not itself execute trades.** **Broker-reported buying power is not Arbion trading authority.**

## Implemented deterministic evaluation foundation

The Go `internal/risk` package defines provider-independent `ProposedAction`, normalized account and daily-activity snapshots, stable reason codes, structured `RiskEvaluation` evidence, and an ordered rule registry. Exact decimal arithmetic avoids binary floating point, and security-critical unknowns fail closed. Migration `00007_risk_control.sql` adds durable scoped circuit-breaker and compact evaluation evidence tables. `LIVE` remains configuration metadata and every result reports platform execution unavailable.

Autonomous AI SHADOW proposals additionally carry a bounded list of prior same-instance `WOULD_HAVE_SUBMITTED` actions from the last hour. The deterministic gate denies an identical symbol and side until that one-hour cooldown expires, records `REPEAT_ACTION_COOLDOWN_ACTIVE`, and fails closed when the required recent-action snapshot is missing or invalid. Other symbols and the opposite side remain independently evaluable. The guard limits repetitive model churn; it does not decide that any action is desirable and does not create broker authority.

**The Risk/Control Engine authorizes or denies proposed actions; it never decides that a trade is desirable.**

**A successful risk evaluation does not execute a trade.**

**No role, model, strategy, UI, or conversation may bypass the Risk/Control Engine.**

The package imports no connector or Neural Engine code and exposes no broker-write operation. The authenticated evaluation service builds normalized snapshots, loads applicable durable breakers, invokes the gate, and atomically persists its evidence with the non-live result. Both manual and guarded scheduled evaluation call this same service. An authenticated founder can engage and release durable automation-, account-, and owner-wide user-scoped emergency stops through owner-facing APIs and UI. Every stop requires an explicit confirmation and operator reason, is owner-scoped and audited, and never requests a broker action. An account stop denies new actions across every Arbion automation and non-executing proposal tied to that exact financial account while leaving other accounts independent. The owner-wide stop denies new actions across all accounts and automations belonging to the authenticated owner. Monitoring and immutable evidence may continue under either stop. Global breaker mutation workflows, administrative dual control, realized daily P/L ingestion, and broader orchestration remain deferred.

The same engine now evaluates durable manual Coinbase spot proposals through the Go order-intent service. Every new proposal selects an active non-reserve USD capital bucket owned by the same user and account. The service re-quotes Coinbase, refreshes exact available cash and holdings, loads open global/user/account breakers, and checks BUY capital consumption or SELL holding sufficiency before persisting an immutable result. An ALLOW requires later TOTP proposal review; a DENY remains a review-ineligible blocked record. Neither result can submit an order, and no provider-write method exists.

## Non-live strategy integration

The deterministic strategy orchestrator emits the existing `ProposedAction` and invokes this engine unchanged for PAPER and SHADOW. A DENY records denial and cannot fill or advance execution-dependent state. Circuit breakers, stale data, exact mandate version, account options capability, allocation, reserve, and autonomy rules remain authoritative; simulation has no risk bypass. When options capability is `UNKNOWN`, an explicit immutable owner attestation may convert only the PAPER options-capability check into a durable warning so later capital and policy rules still run. It never applies to `UNSUPPORTED`, SHADOW, or LIVE and never changes the account capability record. The persistence boundary also binds each instance to the exact same-owner capital bucket, caps PAPER starting cash before the first evaluation, and conservatively allows only one active or paused non-live strategy per financial account until aggregate reservation accounting exists.

**Every strategy action must pass the Risk/Control Engine.** An ALLOW result permits only the selected non-live adapter in this milestone and is not broker authorization.
