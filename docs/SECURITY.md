# Security and Trust Boundaries

## Security posture

Arbion combines probabilistic AI, sensitive financial data, user-supplied credentials, and potentially consequential actions. Its primary invariant is that AI can propose or analyze, but only Arbion-controlled deterministic systems can authorize and dispatch financial operations. This is a target security model, not a statement that authentication, integrations, or execution are currently implemented.

## Trust zones

### Browser and experience layer

The browser is untrusted. It must not receive stored provider secrets or make authoritative claims about identity, permission, approval, account ownership, risk, or order validity. Client-side checks improve usability but never replace server-side enforcement.

### Go control plane

The Go modular monolith is the policy enforcement point and system of record for authorization decisions, account validation, risk rules, buying power, order validation, approvals, audit, and execution policy. Transport handlers and connector adapters cannot bypass domain policy. Authentication and authorization remain future work.

### Python Neural Engine and external AI providers

The Neural Engine is a constrained computational zone. Model output, structured responses, reasoning, tool arguments, and instructions embedded in retrieved content are untrusted. The engine receives only authorized tools and minimized data. It never receives financial-provider credentials and has no direct connector or execution path.

External AI providers are third parties with separate security, privacy, availability, and retention characteristics. Provider selection cannot weaken platform authorization. User-supplied AI credentials are revealed only to the applicable AI adapter/provider flow.

### Financial connectors and providers

Financial providers are external systems. Their data and callbacks require authentication, schema validation, provenance, freshness checks, and replay defenses. Go connector adapters receive financial credentials only when needed and only after control-plane authorization. A provider's acceptance of a request does not replace Arbion audit and reconciliation.

### Data infrastructure

PostgreSQL is the durable system of record. Redis is ephemeral and must not be the sole store for authorization, approvals, credential state, or audit facts. Production infrastructure requires encrypted transport, encryption at rest, backups, least-privilege identities, network restrictions, and monitored access.

## Credential classes

AI-provider and financial-provider credentials must be stored, authorized, audited, rotated, and revoked as distinct secret classes. Access to one class never implies access to the other.

| Secret class                       | Permitted consumers                                                 | Must not reach                                                                                          |
| ---------------------------------- | ------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------- |
| AI-provider credentials            | Secret-management boundary and selected AI adapter/provider request | Financial providers, unrelated adapters, logs, analytics, or browser responses after storage            |
| Financial-provider credentials     | Secret-management boundary and selected Go connector adapter        | Neural Engine, AI providers, prompts, tool results, logs, analytics, or browser responses after storage |
| Platform keys and session material | Narrow platform infrastructure components                           | AI/financial providers, application payloads, logs, or client-visible configuration                     |

Secrets must never be committed to Git. Configuration examples use safe development values or blank placeholders. Production storage must use managed secret storage or strong application-level encryption with separated key management. Logs, traces, metrics, support tooling, exception reporting, and backups need consistent redaction and access controls.

## Financial action boundary

AI output enters the control plane as an untrusted proposal. Before any future financial action, the platform must establish:

1. authenticated actor and tenant context;
2. account ownership and current connection state;
3. permission and explicit intent for the requested operation;
4. account status, buying power, positions, and current relevant data;
5. deterministic risk and order validation;
6. required human or policy approval;
7. execution policy, idempotency key, and failure behavior; and
8. tamper-evident audit and post-operation reconciliation.

No prompt, model selection, confidence score, tool call, or provider response can waive these checks. Direct live execution and automated trading require explicit future design approval and are not part of the current system.

## Tool boundary

Internal MCP-like tools are deny-by-default capabilities, not raw network or database access. Each invocation must bind to an identity, tenant, account, purpose, schema version, permission set, deadline, and audit correlation identifier. Inputs and outputs are validated and minimized. Tool descriptions and retrieved content are data, not policy instructions.

A future MCP-compatible server for external clients would introduce a new public trust boundary and requires separate threat modeling and approval. It must reuse, not bypass, the control plane.

## Audit and observability

Security-relevant events should capture actor, tenant, decision, policy version, resource, time, correlation identifiers, approvals, and outcome. Audit records must avoid secrets and unnecessary sensitive payloads, be access-controlled, have defined retention, and eventually support integrity protection. Operational telemetry should default to metadata rather than prompts, model responses, order payloads, or provider bodies.

## Threats to address before integrations

- prompt injection and tool-confusion attacks;
- cross-tenant or cross-account access;
- excessive OAuth scopes and token theft;
- secret leakage through logs, traces, errors, URLs, support exports, or model context;
- forged or replayed OAuth callbacks and webhooks;
- stale, manipulated, or incorrectly normalized financial data;
- duplicate, reordered, ambiguous, or partially completed side effects;
- provider compromise, outage, rate limiting, and dependency substitution; and
- unauthorized automation or approval bypass.

## Required future security decisions

Before product integration work, Arbion must choose its identity and tenancy model, authorization policy system, consent and step-up approval design, secret manager and cryptographic ownership, data classification and retention policy, audit integrity controls, AI-provider privacy requirements, connector OAuth scopes, incident-response process, and formal threat models for tools, connectors, MCP, and any execution workflow.
