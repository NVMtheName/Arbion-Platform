# Architecture

## Goals and current boundary

Arbion starts as a small, deployable platform with clear internal boundaries. It is a foundation, not a trading implementation: live trading, order execution, automated strategies, broker connections, and legacy Flask migration are explicitly out of scope.

```text
Browser -> Next.js web -> Go API -> PostgreSQL
                         |      -> Redis
                         +-----> Python AI service (future, explicit calls)
```

## Components

### Web application

`apps/web` is a Next.js App Router application. It owns presentation and browser-facing routes, but not business rules. Its health route is independent of downstream services so orchestration can distinguish process health from dependency readiness.

### Core API

`services/api` is the system's primary backend and is intentionally a modular monolith. `cmd/api` owns process startup and dependency wiring. `internal/platform` contains shared technical infrastructure, while future business modules should use separate packages beneath `internal` with their own models, use cases, and repository interfaces. HTTP handlers must delegate domain behavior rather than become the domain layer.

PostgreSQL is the durable system of record. Redis is reserved for ephemeral caching, rate limiting, and coordination; correctness must not depend on cached data. Neither dependency is connected merely for the process liveness endpoint.

### AI service

`services/ai` is a small Python/FastAPI boundary for future model-dependent workloads. Separating Python avoids forcing ML dependencies into the core API. It does not make trading decisions or execute trades. Add endpoints only when a concrete AI use case exists.

## Operations and security

All components are containerized and coordinated locally by Docker Compose. Services expose liveness endpoints, run as non-root users, and receive configuration through environment variables. Example values are for local development only; production credentials belong in a managed secret store. Production deployments should add TLS, authentication/authorization, structured observability, backups, migrations, dependency readiness probes, and restrictive network policies before exposing functionality.

## Evolution rules

Prefer adding a module inside the Go application over creating a service. Split a service only after a measured scaling, isolation, ownership, or technology constraint appears. Record significant decisions here (or in an ADR) and preserve the rule that broker and execution integrations require explicit future design and approval.

