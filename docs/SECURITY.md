# Security and Trust Boundaries

## Security posture

Arbion combines probabilistic AI, sensitive financial data, user-supplied credentials, and potentially consequential actions. Its primary invariant is that AI can propose or analyze, but only Arbion-controlled deterministic systems can authorize and dispatch financial operations. This is a target security model, not a statement that integrations or execution are implemented. Authentication is implemented; live and automated trading remain prohibited without a separate explicit task and safety approval.

## Trust zones

### Browser and experience layer

The browser is untrusted. It must not receive stored provider secrets or make authoritative claims about identity, permission, approval, account ownership, risk, or order validity. Client-side checks improve usability but never replace server-side enforcement.

Traditional UI and Ask Arbion are equivalent untrusted inputs to shared structured commands. Natural-language interpretation, conversation history, and actionable notification buttons cannot create a privileged execution path.

### Go control plane

The Go modular monolith is the authentication authority and policy enforcement point. It validates opaque Redis-backed sessions and loads the PostgreSQL user before protected handlers run. Transport handlers and connector adapters cannot bypass domain policy.

## Browser authentication controls

Passwords are accepted only in bounded JSON requests, never logged or serialized, and hashed with encapsulated Argon2id parameters (64 MiB memory, three iterations, two lanes, 16-byte random salt, 32-byte output). Passwords must be 12–1024 bytes; passphrases are encouraged and no composition rule is imposed. Encoded hashes carry their parameters to allow rehash upgrades.

The browser receives a random opaque 256-bit token in an `HttpOnly`, `Path=/`, `SameSite=Lax` cookie. Production also sets `Secure`; localhost development intentionally does not. Only SHA-256 token hashes are Redis keys. Sessions expire after an environment-defined TTL, are rotated at authentication, can be individually deleted, and are indexed for future all-session revocation.

Cookie-authenticated state changes use an explicit trusted-origin check in addition to `SameSite=Lax`; requests without an allowed `Origin` are rejected. This is the selected CSRF strategy and does not rely on CORS. Login and registration are protected too. JSON decoding rejects unknown fields and trailing values, bodies are capped at 4 KiB, common security headers are applied, and API errors do not expose internals. Redis sliding-window counters currently throttle registration by IP and login by IP plus normalized account identifier without permanent account lockout.

Audit events record registration, successful and failed login, and logout outcomes without passwords, hashes, tokens, or provider secrets.

## Required authentication follow-up

Before public launch, Arbion must integrate an email delivery provider and require email verification. The schema already records verification time, but development registration is not blocked. Password recovery is intentionally absent until that trusted delivery channel exists; no temporary or insecure reset path is provided. MFA, passkeys, external OAuth identities, enterprise SSO, recovery codes, administrative session revocation UI, adaptive throttling, and step-up authentication remain future security work.

### Python Neural Engine and external AI providers

The Neural Engine is a constrained computational zone. Model output, structured responses, reasoning, tool arguments, and instructions embedded in retrieved content are untrusted. The engine receives only authorized tools and minimized data. It never receives financial-provider credentials and has no direct connector or execution path.

External AI providers are third parties with separate security, privacy, availability, and retention characteristics. Provider selection cannot weaken platform authorization. User-supplied AI credentials are revealed only to the applicable AI adapter/provider flow.

### Financial connectors and providers

Financial providers are external systems. Their data and callbacks require authentication, schema validation, provenance, freshness checks, and replay defenses. Go connector adapters receive financial credentials only when needed and only after control-plane authorization. A provider's acceptance of a request does not replace Arbion audit and reconciliation.

### Data infrastructure

PostgreSQL is the durable system of record. Redis is ephemeral and must not be the sole store for authorization, approvals, credential state, or audit facts. Production infrastructure requires encrypted transport, encryption at rest, backups, least-privilege identities, network restrictions, and monitored access.

Login sessions are short-lived browser authorization state, not ownership of stored resources. Logout must not delete persistent provider connections or disable authorized, server-side automation. Automation enablement and configuration remain authoritative in PostgreSQL and survive browser closure, logout, process restarts, and Redis loss.

## Credential classes

AI-provider and financial-provider credentials must be stored, authorized, audited, rotated, and revoked as distinct secret classes. Access to one class never implies access to the other.

| Secret class                       | Permitted consumers                                                 | Must not reach                                                                                          |
| ---------------------------------- | ------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------- |
| AI-provider credentials            | Secret-management boundary and selected AI adapter/provider request | Financial providers, unrelated adapters, logs, analytics, or browser responses after storage            |
| Financial-provider credentials     | Secret-management boundary and selected Go connector adapter        | Neural Engine, AI providers, prompts, tool results, logs, analytics, or browser responses after storage |
| Platform keys and session material | Narrow platform infrastructure components                           | AI/financial providers, application payloads, logs, or client-visible configuration                     |

Secrets must never be committed to Git. Configuration examples use safe development values or blank placeholders. Production storage must use managed secret storage or strong application-level encryption with separated key management. Logs, traces, metrics, support tooling, exception reporting, and backups need consistent redaction and access controls.

The development credential vault encrypts each payload with AES-256-GCM and binds ciphertext authentication to its user, connection, and AI-versus-financial credential class. Only encrypted payloads are persisted in PostgreSQL; normal provider models contain safe metadata only. The vault interface allows a future managed KMS or secrets-manager adapter. Production key custody, rotation, and managed-secret selection remain deferred.

AI connection endpoints authenticate every request, scope every lookup and mutation by user ID and the `ai` credential class, enforce trusted origins on mutations, and cap JSON bodies and API keys. Foreign and missing connection identifiers are indistinguishable. Audit events contain provider/connection identifiers and state transitions but no request body, key, ciphertext, nonce, or vault details. Logout removes only the session and has no connection lifecycle side effect.

For verification and model discovery, Go checks entitlement, ownership, and state before retrieving plaintext through the Vault. It sends the credential in a bounded JSON body over an internal-only, bearer-authenticated service route; it never uses an environment variable or query string. Python holds it only for request scope, selects a fixed adapter destination, bounds time and response size, performs no uncontrolled retry, and returns only normalized results or errors. Raw provider bodies are never relayed. Local Compose does not publish the AI port; production additionally requires encrypted transport, workload identity/rotation, restrictive network policy, and trace/log redaction.

**AI provider verification does not grant trading authority. AI provider credentials never cross into financial-provider connectors.**

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

The active immutable Automation Mandate version is an additional authorization ceiling for automation. Available broker buying power is not consent to deploy capital; explicit capital buckets, reservations, protected amounts, and absolute limits constrain use. Autonomy changes human-approval requirements but never weakens the deterministic gate.

Hard circuit breakers cover loss/deployment limits, reserves, concentration/frequency, connectivity, stale or abnormal data, volatility, execution failures, reconciliation mismatch, capability changes, and scoped kill switches. They fail closed for affected new action and cannot be overridden or reset by AI. See [Risk and Control Engine](RISK_CONTROL_ENGINE.md).

Future submission must be idempotent and duplicate-resistant. Submission or provider acknowledgement is not a fill. Broker truth is authoritative for execution state, and reconciliation must detect partial fills, cancellations, rejections, external trades, drift, and lost acknowledgements before dependent strategy state advances. See [Execution Engine](EXECUTION_ENGINE.md).

## Tool boundary

Internal MCP-like tools are deny-by-default capabilities, not raw network or database access. Each invocation must bind to an identity, tenant, account, purpose, schema version, permission set, deadline, and audit correlation identifier. Inputs and outputs are validated and minimized. Tool descriptions and retrieved content are data, not policy instructions.

A future MCP-compatible server for external clients would introduce a new public trust boundary and requires separate threat modeling and approval. It must reuse, not bypass, the control plane.

## Audit and observability

Administrative endpoints enforce centralized, deny-by-default role checks. Product entitlement is never interpreted as an administrative role, and administrative role is never interpreted as product access. Founder protections are domain invariants in the mutation service and database constraints, not client-side controls. Rejected privileged changes return a generic forbidden response without exposing sensitive account state.

Security-relevant events should capture actor, tenant, decision, policy version, resource, time, correlation identifiers, approvals, and outcome. Audit records must avoid secrets and unnecessary sensitive payloads, be access-controlled, have defined retention, and eventually support integrity protection. Operational telemetry should default to metadata rather than prompts, model responses, order payloads, or provider bodies.

Automated and AI-assisted actions also require a structured Decision Journal linking the exact mandate version, strategy state, market references, concise rationale, risk checks, approval, order, broker response, and resulting state. Arbion does not store private model chain-of-thought. Explainability uses these records rather than model guesswork.

## Threats to address before integrations

- prompt injection and tool-confusion attacks;
- cross-tenant or cross-account access;
- excessive OAuth scopes and token theft;
- secret leakage through logs, traces, errors, URLs, support exports, or model context;
- forged or replayed OAuth callbacks and webhooks;
- stale, manipulated, or incorrectly normalized financial data;
- duplicate, reordered, ambiguous, or partially completed side effects;
- provider compromise, outage, rate limiting, and dependency substitution; and
- unauthorized automation or approval bypass;
- capital double allocation or stale mandate/version use;
- circuit-breaker bypass or unaudited emergency reset; and
- execution-state confusion caused by treating submission as a fill.

## Required future security decisions

Before product integration work, Arbion must choose its identity and tenancy model, authorization policy system, consent and step-up approval design, secret manager and cryptographic ownership, data classification and retention policy, audit integrity controls, AI-provider privacy requirements, connector OAuth scopes, incident-response process, and formal threat models for tools, connectors, MCP, and any execution workflow.
