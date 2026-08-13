# Neural Engine

## Role

The Neural Engine is Arbion's Python boundary for AI-assisted and quantitative computation. It can eventually support chat, reasoning, research, structured analysis, streaming, tool calling, security analysis, and backtesting. It is advisory and computational, not an authorization or execution authority.

Secure provider connection management is implemented, but inference and external provider verification are not. No prompts are sent to AI providers in this milestone.

## Connection lifecycle

Entitled users can create, rename, replace credentials for, enable, disable, list, and delete their own AI connections. New and replaced credentials enter `pending`; encryption success is not verification. The documented lifecycle vocabulary is `pending`, `active`, `error`, `expired`, `revoked`, and `disabled`. A future verifier may move a pending connection to active or an appropriate failure state after contacting only its registered provider adapter.

Disabling preserves the connection and encrypted credential but makes it ineligible for future Neural Engine use. Enabling returns it to pending eligibility and does not claim validity. Deletion first refuses connections referenced by durable configuration, then removes the vault secret and connection. Ownership-filtered queries return the same not-found result for absent and foreign IDs.

All mutations are checked through centralized `CanUseNeuralEngine` policy. Founder and premium entitlements currently satisfy that capability naturally. **Administrative role does not itself grant Neural Engine product access; entitlement policy controls Neural Engine access.**

The registry contains provider identifiers and safe labels for OpenAI, Anthropic / Claude, and Google Gemini. Connections store only safe metadata and a deliberately masked suffix; plaintext is accepted in a bounded request, passed to the server-side authenticated-encryption vault, cleared from the immediate byte buffer, and never returned. **AI-provider credentials belong to the Arbion user and persist independently of browser sessions.**

Future work will add external credential verification and provider-independent Neural Engine consumption. It must exclude disabled/non-active connections as policy requires, retrieve credentials only inside the authorized server-side boundary, and preserve the rule that Go remains authoritative for entitlement, ownership, and vault access.

## Provider abstraction

OpenAI, Anthropic/Claude, and Google Gemini are the initial candidate providers. Provider-specific SDKs, request formats, authentication, model names, streaming protocols, errors, and tool-call formats must remain inside adapters implementing a common interface.

The common contract should describe capabilities rather than assume every vendor behaves identically. Candidate contract concepts include:

- chat or message input and output;
- reasoning and research requests;
- schema-constrained structured analysis;
- incremental streaming events;
- tool definitions and tool-call requests;
- model capability metadata, token/size limits, usage, and normalized errors; and
- cancellation, deadlines, retry safety, and provider health.

Callers should request a task or capability, not import a vendor SDK or depend on a vendor response type. Routing may later select different providers and models for different tasks based on user choice, capability, policy, availability, latency, privacy, and cost. Fallback must be explicit because changing providers can change data handling and output behavior.

## Credential boundary

Users may eventually supply credentials for supported AI providers. These credentials are a distinct secret class from financial-provider credentials. They must be scoped to the AI adapter that needs them, encrypted or placed in managed secret storage, redacted from telemetry, and never returned to the browser after storage.

The Neural Engine and external AI providers must never receive brokerage credentials, OAuth refresh tokens, or other financial-provider secrets. Prompts and tool results should contain only the minimum authorized financial data required for the task.

## Controlled tool model

Arbion may expose MCP-like internal tools to models, including:

- `get_portfolio`
- `get_position`
- `get_buying_power`
- `get_market_quote`
- `get_recent_orders`
- `get_watchlist`
- `analyze_security`
- `run_backtest`
- `calculate_risk`
- `preview_order`

This is a conceptual catalog, not an implemented API. Arbion owns tool names, versioned input/output schemas, identity context, permissions, validation, timeouts, resource limits, data minimization, and audit records. A model can request a tool call; it cannot grant itself access or define the trusted meaning of its arguments.

Tool output is also untrusted at subsequent boundaries: external data can be stale or malicious, and model interpretation can be wrong. Results should carry provenance and freshness where relevant and must be validated before they influence a financial action.

Direct trade execution must not be offered as an unrestricted AI tool. A future execution workflow would require explicit user intent and approval plus deterministic control-plane checks.

## Control-plane relationship

The Go control plane mediates the Neural Engine's access to platform and connector capabilities:

1. The experience layer sends a request with future authenticated user and tenant context.
2. The control plane authorizes the task and determines the minimum data and tools available.
3. The Neural Engine invokes a selected provider through the common abstraction.
4. Any model-requested tool call returns to an Arbion-controlled dispatcher.
5. The control plane validates identity, permissions, schema, policy, limits, and resource ownership before dispatch.
6. The result is filtered and audited before it is returned to the model.
7. Any proposed financial action is treated as untrusted and enters a separate deterministic validation and approval workflow.

The Neural Engine cannot call financial providers directly, hold financial-provider credentials, bypass approval, or turn natural-language output into execution authority.

## Trust and safety properties

- Model output and tool arguments are untrusted input.
- Prompt text and retrieved content cannot alter platform authorization.
- Tool access is deny-by-default and scoped per request, identity, account, and purpose.
- Sensitive data sent to a provider is minimized and governed by explicit retention/privacy policy.
- Logs capture decisions and correlation metadata without prompts, secrets, or sensitive payloads by default.
- Provider errors, rate limits, and malformed output fail safely; retries must not duplicate side effects.
- Deterministic calculations and financial controls remain outside probabilistic model decisions.

## Future MCP compatibility

Arbion may later expose an MCP-compatible server so external AI clients can use an approved subset of financial tools. Such a server would be an adapter over the same controlled tool registry, not a path around it. It requires a separate design for authentication, delegated authorization, consent, tenant/account scoping, client trust, rate limits, schema/version compatibility, data loss prevention, audit, and revocation. It is not implemented now.

## Deferred decisions

Later design must choose the common interface and event schemas, provider capability discovery, model catalog, routing/fallback policy, user credential lifecycle, prompt and response retention, tool registry ownership, sandboxing, cost and quota policy, evaluation strategy, and MCP exposure model.
