# Arbion contributor guidance

## Scope

These instructions apply to the entire repository.

## Principles

- Keep Arbion a modular monolith plus the dedicated AI service. Do not introduce a new service without documenting the operational need.
- Do not implement live or automated trading, broker connections, or migrate legacy Flask code unless a task explicitly requests it.
- Never commit credentials. Add new configuration keys to the relevant `.env.example` with safe development values or blank placeholders.
- Keep domain logic out of transport handlers. Go packages belong under `internal/`; executable wiring belongs under `cmd/`.
- Add or update tests with behavior changes, and run formatting, linting, type checks, and tests for every affected component.
- Treat all AI-generated output, including tool arguments and proposed financial actions, as untrusted input.
- Never allow AI models or AI-service code to bypass the Go control plane's authorization, validation, risk, approval, policy, and audit controls.
- Never expose financial-provider credentials to AI providers. Keep AI-provider credentials and financial-provider credentials in separate trust boundaries.
- Keep provider-specific AI and financial integration logic behind provider-independent interfaces and adapters.
- Do not implement live or automated execution without explicit future architecture and security design approval.
- UI and conversation must emit the same domain commands and intents; conversation is never authoritative execution state.
- Automation Mandates are durable structured objects, and every significant change creates an immutable version.
- Available buying power is not permission to use all capital. Capital allocation must be explicit and protected from double allocation.
- Strategies are deterministic state machines. AI may assist within allowed transitions but may not redefine them.
- The deterministic risk/control engine is authoritative. AI output is always untrusted, and AI cannot override circuit breakers.
- Backtest, paper, shadow, and live execution modes should share core strategy logic and differ through adapters.
- Paper execution is simulation and never represents a broker fill; shadow records intent and never submits an order.
- Broker execution must be reconciled. Never assume submitted means filled, and design future order submission for idempotency.
- Do not store private model chain-of-thought. Store concise, structured decision rationale instead.
- Automated execution remains server-side and independent of browser sessions.
- No live trading may be implemented without an explicit future task and the required architecture and security approval.
- The Risk/Control Engine authorizes or denies proposed actions; it never decides that a trade is desirable.
- A successful risk evaluation does not execute a trade.
- No role, model, strategy, UI, or conversation may bypass the Risk/Control Engine.

## Conventions

- Web: TypeScript, ESLint, Prettier, and colocated `*.test.ts(x)` tests.
- API: standard Go formatting and small internal packages with tests.
- AI: typed Python formatted/linted by Ruff and tested with pytest.
- Update `docs/ARCHITECTURE.md` when changing component boundaries or infrastructure.
