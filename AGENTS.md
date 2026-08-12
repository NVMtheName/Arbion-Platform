# Arbion contributor guidance

## Scope

These instructions apply to the entire repository.

## Principles

- Keep Arbion a modular monolith plus the dedicated AI service. Do not introduce a new service without documenting the operational need.
- Do not implement live or automated trading, broker connections, or migrate legacy Flask code unless a task explicitly requests it.
- Never commit credentials. Add new configuration keys to the relevant `.env.example` with safe development values or blank placeholders.
- Keep domain logic out of transport handlers. Go packages belong under `internal/`; executable wiring belongs under `cmd/`.
- Add or update tests with behavior changes, and run formatting, linting, type checks, and tests for every affected component.

## Conventions

- Web: TypeScript, ESLint, Prettier, and colocated `*.test.ts(x)` tests.
- API: standard Go formatting and small internal packages with tests.
- AI: typed Python formatted/linted by Ruff and tested with pytest.
- Update `docs/ARCHITECTURE.md` when changing component boundaries or infrastructure.

