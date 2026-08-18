# Security and Trust Boundaries

## Production deployment controls

Production startup rejects missing or development-placeholder database credentials, credential encryption keys, and internal AI tokens. Trusted origins are explicit and restricted to `https://www.arbion.ai`; wildcards and reflected Host/Origin trust are forbidden. Session cookies are `HttpOnly`, `SameSite=Lax`, scoped to `/`, and forced `Secure` in production while localhost development remains usable.

`CREDENTIAL_ENCRYPTION_KEY` is a cryptographically random, base64-encoded 32-byte key generated once per environment. Back it up in a restricted secret store; never commit or casually rotate it. Losing it can make encrypted provider credentials unreadable. Automatic rotation is intentionally out of scope.

Container logs use stdout/stderr, but operators must never log environment dumps or secret-bearing requests. Schwab secrets/codes/tokens, AI API keys, encryption keys, session cookies, internal tokens, and database passwords are prohibited from logs and support bundles. Backups contain sensitive customer and product data and require encryption and restricted access.

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

The browser receives a random opaque 256-bit token in an `HttpOnly`, `Path=/`, `SameSite=Lax` cookie. Production also sets `Secure`; localhost development intentionally does not. Only SHA-256 token hashes are Redis keys. Sessions expire after an environment-defined TTL, are rotated at authentication, can be individually deleted, and are indexed for immediate self-service all-session revocation.

Cookie-authenticated state changes use an explicit trusted-origin check in addition to `SameSite=Lax`; requests without an allowed `Origin` are rejected. This is the selected CSRF strategy and does not rely on CORS. Login and registration are protected too. JSON decoding rejects unknown fields and trailing values, bodies are capped at 4 KiB, common security headers are applied, and API errors do not expose internals. Redis sliding-window counters currently throttle registration by IP and login by IP plus normalized account identifier without permanent account lockout.

Authenticated users can change their password only after re-entering the current password. The replacement uses the same bounded Argon2id policy, must differ from the existing password, and is rate-limited. Arbion fails closed when the session store is unavailable: it revokes every current session before committing the new password with an optimistic hash check, then clears the current browser cookie. A durable database failure may therefore require the user to sign in again with the unchanged password, but cannot leave old sessions active after a successful password change. The separate **Sign Out All Sessions** control revokes all browser sessions without deleting financial or AI connections or changing automation state.

Audit events record registration, successful and failed login, MFA enrollment/enable/disable and challenge outcomes, logout, all-session logout, and successful or rejected password-change outcomes without passwords, factors, recovery codes, hashes, tokens, or provider secrets.

Email verification and password recovery use a provider-independent sender boundary. Raw 256-bit tokens exist only in the outbound message and browser URL fragment; PostgreSQL stores a unique SHA-256 hash, purpose, expiry, and consumption time. Creating a new link invalidates the prior active link for that user and purpose. Confirmation is atomic and single-use. Password reset revokes all sessions before committing the new Argon2id password hash. Email request responses are generic for existing, missing, ineligible, and already-verified accounts, and request/confirmation paths are rate-limited. SMTP delivery requires STARTTLS with TLS 1.2 or newer and is disabled by default.

Optional authenticator-app MFA uses interoperable RFC 6238 TOTP with a 30-second period, six digits, and one-step clock skew. The Go authentication domain owns enrollment, challenge completion, recovery-code replacement, disablement, and audit behavior. A password login for an MFA-enabled account creates only a random five-minute Redis challenge; it creates no authenticated session until the second factor succeeds. PostgreSQL stores the TOTP secret only as AES-256-GCM ciphertext bound to the user and authentication factor through associated data. Ten 80-bit recovery codes are displayed once, stored only as domain-separated SHA-256 hashes, and atomically consumed. An authenticated user can replace the entire recovery set only after re-entering the current password and a valid second factor; all old codes become invalid in the same database statement. Successful TOTP steps advance a durable counter so a code cannot be replayed, including the code used to confirm enrollment. Enabling or disabling MFA revokes every existing browser session before the durable change. Setup and disablement require the current password, all state changes require a trusted origin, and failures use generic bounded responses. Arbion does not send MFA codes by email or SMS.

## Required authentication follow-up

The application boundary for verified email and password recovery is implemented, but production delivery must remain disabled until SES production access, a verified sender identity, encrypted SMTP credentials, and an end-to-end owner test are complete. Required verification applies only to newly registered accounts after activation; it does not retroactively lock existing active accounts. MFA is opt-in; a future policy may require it for privileged or high-risk actions only after recovery and support procedures are reviewed. Passkeys, external OAuth identities, enterprise SSO, administrative session revocation UI, adaptive throttling, and broader step-up authentication remain future security work.

### Python Neural Engine and external AI providers

The Neural Engine is a constrained computational zone. Model output, structured responses, reasoning, tool arguments, and instructions embedded in retrieved content are untrusted. The engine receives only authorized tools and minimized data. It never receives financial-provider credentials and has no direct connector or execution path.

External AI providers are third parties with separate security, privacy, availability, and retention characteristics. Provider selection cannot weaken platform authorization. User-supplied AI credentials are revealed only to the applicable AI adapter/provider flow.

The implemented Arbion Insight flow is intentionally data-isolated. An authenticated entitled user can submit at most 2,000 bytes of text using an active saved OpenAI connection. The request includes no financial account, portfolio, broker, quote, news, or live-market context and exposes no tools. The browser chooses only `fast`, `core`, or `deep`; it cannot supply a model ID. Go and Python bind those profiles to Luna, Terra, and Sol respectively, reject unknown routes, and verify returned route metadata. OpenAI requests set `store: false`, use profile-specific explicit reasoning and output caps, and require a strict JSON schema. A Redis counter enforces a 12-credit hourly budget with weights of one, three, and six; counter failure denies the request. Audit events retain provider/profile/model identifiers, charged credits, byte and token counts, outcome, latency, and provider request ID only; prompt text, model output, credentials, and private chain-of-thought are excluded.

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

For verification, model discovery, and bounded insight, Go checks entitlement, ownership, and state before retrieving plaintext through the Vault. It sends the credential in a bounded JSON body over an internal-only, bearer-authenticated service route; it never uses an environment variable or query string. Python holds it only for request scope, selects a fixed adapter destination, bounds time and response size, performs no uncontrolled retry, and returns only normalized results or errors. Raw provider bodies are never relayed. Local Compose does not publish the AI port; production additionally requires encrypted transport, workload identity/rotation, restrictive network policy, and trace/log redaction.

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

## Financial authorization implementation controls

Schwab app credentials are infrastructure secrets; user OAuth grants are per-user financial Vault records authenticated to user, connection, and secret class. Tokens are never connection metadata, database plaintext columns, audit metadata, URLs, browser storage/responses, or Neural Engine input. Only the selected Go adapter receives decrypted financial credentials for a bounded request. Conversely, AI Vault records are retrieved only by the AI connection domain and can never be selected by a financial locator. Tests cover this AAD-enforced class separation and safe JSON projections.

Pending OAuth state is random, expiring, user-bound, and single-use. A mismatched attempt consumes the state to stop swapping attempts. Provider redirects are fixed configuration, not request parameters. Read bodies and durations are bounded; authentication failures are not retried. Concurrent token refresh must be serialized by connection, encrypted replacement must complete before expiry metadata/status changes, and terminal authorization failure fails closed without leaking the provider body.

Disable preserves authorization and inventory while preventing use. Disconnect removes Vault material and retires the Arbion connection while preserving audit history. Logout only revokes an ephemeral browser session: it does not delete tokens, accounts, or provider authorization.

**Financial-provider credentials never enter the Neural Engine.** Financial credentials flow `Go/Vault -> broker only`; AI credentials flow `Go/Vault -> Neural Engine -> selected AI provider only`. Neither credential class may cross into the other provider domain.

The Schwab callback is intentionally independent of a surviving browser session: the single-use Redis record identifies the initiating user and contains no secret or redirect. The post-callback destination is selected only from configured trusted origins. OAuth codes are exchanged server-side and never returned. PostgreSQL advisory locks serialize token refresh across API instances; encrypted replacement is stored before safe expiry metadata advances. Logout deletes only Redis session data and cannot delete the durable connection, account inventory, or encrypted financial credential.

## Automation authorization controls implemented

Automation mutations require centralized `CanUseAutomation` and financial-account entitlement checks; AI-bearing mandates additionally require `CanUseNeuralEngine`. Administrative roles do not grant these product capabilities. Stores scope account, bucket, mandate, version, and AI references to the user; active AI/model preference checks occur without invoking AI or revealing credentials. Version snapshots and audit metadata contain identifiers and safe configuration only.

The API exposes configuration and lifecycle routes plus one authenticated, CSRF-protected manual PAPER/SHADOW strategy-evaluation route. That route accepts only an instance ID and bounded idempotency event ID, reloads ownership and the exact current immutable mandate version, uses fresh read-only inputs, and passes every proposal through the Risk/Control Engine. There are no order, broker preview/write, live execution, worker, or AI-portfolio routes. `execution_capable=false` remains the permanent live-execution safeguard returned with mandate configuration.

**A configured or READY Automation Mandate does not itself execute trades.** **Broker-reported buying power is not Arbion trading authority.**

## Implemented risk trust boundary

No role, model, strategy, UI, or conversation may bypass the Risk/Control Engine. Applicable durable breakers are evaluated before mandate and financial rules, unknown safety-critical inputs fail closed, and neither administrative status nor AI output can override a breaker. Evidence contains normalized references and deterministic messages, never credentials or private chain-of-thought.

## Non-live strategy security boundary

Strategy state, transitions, market fixtures, paper records, and Decision Journal rationale contain only normalized safe data. They exclude financial/AI credentials, full account numbers, raw provider payloads, and private chain-of-thought. Ownership and `CanUseAutomation`/financial entitlement are checked server-side for instance APIs.

PAPER and SHADOW traverse the same Risk/Control Engine. PAPER mutates only separate simulated tables; SHADOW mutates no holdings and explicitly records that no order was sent. The strategy package imports neither Schwab nor Neural Engine code; a credential-aware financial service supplies only normalized read models. Missing, stale, or future-dated market timestamps fail closed. No live or broker execution adapter exists.

## AWS production trust boundaries

The AWS foundation exposes only the ALB. ECS tasks have no public IP; AI accepts only API-security-group ingress; RDS and ElastiCache are private and accept only explicitly authorized task groups. NAT provides outbound HTTPS without adding inbound reachability. Cloud Map network discovery does not replace the AI internal token. Financial-provider secrets remain consumable only by Go, and Neural Engine tasks receive no Schwab or database credentials.

GitHub authenticates with repository- and production-environment-restricted OIDC roles, never static AWS access keys. Separate Terraform plan/apply and application deployment roles are distinct from least-privilege ECS execution/task roles. Secrets Manager contains operator-populated values encrypted under KMS; Terraform creates containers and never stores values in source or outputs. RDS credentials are AWS-managed, Redis requires TLS and authentication, and the stable application credential-encryption key is not interchangeable with a KMS key.

Deletion protection, final snapshots, secret recovery windows, protected state/KMS resources, manual apply/deploy approval, and DNS-disabled-by-default settings reduce accidental production loss or takeover. No AWS resource was created as part of repository preparation, and live broker execution remains absent.
