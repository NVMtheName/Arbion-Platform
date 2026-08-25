# Arbion Platform

Production-oriented foundation for Arbion. Coinbase and Schwab supply owner-scoped read-only portfolio context to a shared AI Shadow Engine, Arbion Insight provides bounded educational analysis through explicit Fast/Core/Deep model profiles, and execution remains limited to PAPER and SHADOW. The AI engine can autonomously observe, abstain, or propose one mandate-bounded action; every proposal crosses the deterministic Risk/Control Engine and becomes only Decision Journal evidence. An opt-in server-side scheduler can invoke the same non-live path. No live broker-write adapter or live-trading worker exists.

## Repository layout

- `apps/web` — Next.js/React TypeScript user interface
- `services/api` — Go modular-monolith API
- `services/ai` — Python FastAPI service for provider connectivity and bounded structured insight
- `docs` — architectural decisions and boundaries

See [the architecture guide](docs/ARCHITECTURE.md) for component responsibilities.

## Production readiness

The repository includes a single-host production Compose topology with Caddy-managed HTTPS for `www.arbion.ai`, private application/data services, durable volumes, one-shot migrations, fail-closed configuration, and operator scripts. It prepares deployment but does **not** claim Arbion is deployed. See [the production deployment runbook](docs/DEPLOYMENT.md).

## Prerequisites

- Docker with Docker Compose (recommended), or
- Node.js 22+, Go 1.25.13+, Python 3.13+, PostgreSQL 17, and Redis 7

## Run with Docker Compose

```bash
cp .env.example .env
docker compose up --build
```

Health checks are available at:

- Web: <http://localhost:3000/api/health>
- API: <http://localhost:8080/healthz>
- API readiness: <http://localhost:8080/readyz>
- AI: <http://localhost:8000/healthz>

Stop the stack with `docker compose down`. Use `docker compose down -v` to also delete local database data.

## Run services directly

Copy each service's example environment file before starting it. PostgreSQL and Redis can still be started with `docker compose up postgres redis`.

```bash
# Web
cd apps/web && cp .env.example .env.local && npm ci && npm run dev

# Go API
cd services/api && cp .env.example .env && go run ./cmd/migrate && go run ./cmd/api

# Python AI service
cd services/ai && cp .env.example .env && python -m venv .venv
source .venv/bin/activate && pip install -e '.[dev]'
uvicorn app.main:app --reload
```

Environment files are examples only. Replace development defaults through your deployment platform's secret manager in production.

## Quality checks

```bash
cd apps/web && npm run format:check && npm run lint && npm run typecheck && npm test
cd services/api && test -z "$(gofmt -l .)" && go vet ./... && go test ./...
cd services/ai && ruff format --check . && ruff check . && mypy app && pytest
docker compose config --quiet
```

CI runs the same component checks on every push and pull request.

## Production deployment options

Arbion retains the supported Docker Compose/Caddy single-host deployment for simpler operations. The intended scalable production foundation is AWS ECS/Fargate with a public ALB and private application/data tiers, defined in Terraform without applying resources. Start with [the AWS deployment runbook](docs/AWS_DEPLOYMENT.md); it covers one-time state/OIDC bootstrap, operator-populated Secrets Manager values, manual application and infrastructure workflows, migrations, DNS cutover, rollback, monitoring, and costs. Neither topology enables live trading.
