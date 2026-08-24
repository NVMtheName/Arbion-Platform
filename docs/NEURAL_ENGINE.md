# Neural Engine

## Role

The Neural Engine is Arbion's Python boundary for AI-assisted and quantitative computation. It can eventually support chat, reasoning, research, structured analysis, streaming, tool calling, security analysis, and backtesting. It is advisory and computational, not an authorization or execution authority.

Ask Arbion is one input surface, not an AI-owned trading path. It interprets natural language into a structured Arbion Intent that joins the same Go domain commands used by the UI. Conversation state is not authoritative for mandates, strategies, approvals, orders, positions, or execution.

Secure provider connection management, credential verification, model discovery, default provider/model preferences, a bounded read-only Arbion Insight flow, and one bounded proposal-only Coinbase research flow are implemented. General chat, automatic intent routing, streaming, model tools, arbitrary financial-data retrieval, execution approval, and execution remain unimplemented.

## Implemented provider connectivity

`NeuralProvider` is the provider-independent Python contract. It defines credential verification, model listing, capability metadata, credential types, and future method boundaries for generation, structured output, streaming, and tool support. OpenAI, Anthropic/Claude, and Gemini adapters own their authentication headers, trusted fixed API destinations, response parsing, and error mapping. Arbion uses direct `httpx` calls rather than vendor SDKs for this narrow milestone: each official models API is a small, stable HTTP surface, direct calls avoid three large runtime dependency graphs, and transports remain straightforward to mock.

The adapters declare capabilities independently. Capability metadata describes the external provider/API surface—not Arbion authorization—and does not expose trading tools. Credential metadata supports evolving types (`api_key`, `authorization_key`, and future OAuth or managed identity forms); the current UI accepts the documented API-key/authorization-key form for each initial provider.

Verification uses the smallest safe officially documented model-list request: OpenAI `GET /v1/models` with bearer authorization, Anthropic `GET /v1/models?limit=1` with `x-api-key` and `anthropic-version`, and Gemini `GET /v1beta/models?pageSize=1` with `x-goog-api-key`. Full model discovery uses those same official list APIs and returns normalized IDs, display names, providers, and only capabilities that can be safely stated. There is no hard-coded global model catalog.

Implementation references the current official [OpenAI API specification](https://github.com/openai/openai-openapi), [Anthropic Models API](https://platform.claude.com/docs/en/api/models/list), [Anthropic API overview](https://platform.claude.com/docs/en/api/overview), [Gemini Models API](https://ai.google.dev/api/models), and [Gemini API-key guidance](https://ai.google.dev/gemini-api/docs/api-key). These references must be rechecked before changing an adapter because authentication and catalogs evolve.

Normalized failures are `AUTHENTICATION_FAILED`, `RATE_LIMITED`, `PROVIDER_UNAVAILABLE`, `TIMEOUT`, `INVALID_REQUEST`, `UNSUPPORTED`, and `INTERNAL_ERROR`. Provider bodies stay inside the adapter boundary and the browser receives a sanitized message. Normalized response metadata reserves optional provider, model, input/output usage, request ID, latency, and provider-supplied usage metadata fields; absent fields are never fabricated.

## Implemented read-only insight

`POST /api/neural/insight` is the first bounded analysis surface. Go authenticates the user, checks Neural Engine entitlement, requires an active owned connection and saved model preference, limits input to 2,000 bytes, and enforces a 12-credit hourly budget through Redis. Fast requests consume one credit, Core consumes three, and Deep consumes six, so higher-cost profiles reduce the number of requests available in the same window. Credit consumption is atomic: an over-budget request is rejected without consuming the user's remaining credits. Rate-limiter failure denies the request. The stored AI credential is decrypted only for the call and cleared from the immediate byte buffer afterward.

The browser chooses one explicit analysis profile; it cannot submit a model ID. Go and Python independently bind each profile to one exact OpenAI route: Fast uses `gpt-5.6-luna` with low reasoning and a 700-token cap, Core uses `gpt-5.6-terra` with medium reasoning and a 900-token cap, and Deep uses `gpt-5.6-sol` with high reasoning and a 1,200-token cap. Fast is the backward-compatible default when the profile is omitted. Python rejects unknown profiles before contacting a provider, and Go rejects any response whose provider/model/profile metadata does not match the selected route.

The internal service sends OpenAI a Responses API request containing only the user's text, the fixed route, fixed Arbion instructions, and a one-way per-user safety identifier. `store` is false, reasoning effort and verbosity are explicit, output is capped, no tools are supplied, and a strict JSON schema requires a summary, key points, risk flags, limitations, and a current-data flag. Unsupported providers fail closed rather than silently switching providers. The browser receives only that normalized structure.

The implementation follows OpenAI's official [Responses API guidance](https://developers.openai.com/api/docs/guides/migrate-to-responses), [Structured Outputs guidance](https://developers.openai.com/api/docs/guides/structured-outputs), and [GPT-5.6 model guidance](https://developers.openai.com/api/docs/guides/latest-model). These evolving provider details must be rechecked before changing the adapter.

The feature deliberately has no access to accounts, positions, portfolio data, broker credentials, quotes, news, or current market data. It cannot preview, place, approve, or authorize a trade. Audit records include safe operational metadata—provider, profile, model, charged credits, input byte count, token usage, outcome, latency, and provider request ID—but exclude prompt text, model output, credentials, and private chain-of-thought.

The default Neural Engine preference is a durable user setting pointing to one active, owned AI connection and a model returned for that credential. Arbion Insight uses the selected connection while its request profile chooses the fixed GPT-5.6 route; it never mutates the saved preference. Future Automation Mandates may choose another connection/model under a separately approved policy. Logging out affects only the browser session and preserves both connections and this preference.

## Implemented proposal-only trade research

`POST /api/accounts/{id}/order-intents/ai-proposals` is a narrow authenticated, CSRF-protected Coinbase surface. The owner fixes one canonical asset, BUY/SELL side, exact maximum size, capital policy, and a 500-byte objective. Go independently refreshes normalized available USD cash and the target asset's total and tradable quantities; account IDs, provider account IDs, broker credentials, provider preview identifiers, and unrelated holdings are excluded from the Neural Engine request. The active saved OpenAI connection is reused, and the request is fixed to the Core/Terra route under a separate weighted Redis budget.

Python sends a tool-free OpenAI Responses API request with `store: false`, explicit reasoning/output limits, a one-way safety identifier, fixed Arbion instructions that treat the objective and facts as untrusted data, and a strict JSON schema. The model may return only `PROPOSE` with a positive exact size no greater than the owner's maximum or `ABSTAIN` with size zero, plus bounded confidence, thesis, risk flags, limitations, and route metadata. Python validates those invariants before returning; Go validates them again, rejects a SELL above the provider-reported tradable quantity, and rejects any provider/model/profile mismatch. Prompt/objective and model text are excluded from audit metadata.

An abstention creates no Coinbase preview and no Order Intent, but records a metadata-only audit event. An actionable proposal enters the existing `SourceAI` Order Intent method, which obtains a fresh Coinbase product/price preview, refreshes cash and holdings again, applies capital reservations and the deterministic Risk/Control Engine, and persists either a review-required or blocked non-executing record. The model cannot choose another account, asset, side, policy, or maximum; cannot see financial credentials; cannot receive a provider preview ID; cannot approve its own proposal; and has no broker-write interface. Every response and persisted intent explicitly keeps submission, risk approval, AI execution authority, and live execution unavailable.

## Connection lifecycle

Entitled users can create, rename, replace credentials for, verify, enable, disable, list, and delete their own AI connections. New and replaced credentials enter `pending`; encryption success is not verification. The documented lifecycle vocabulary is `pending`, `active`, `error`, `expired`, `revoked`, and `disabled`. Verification moves a pending connection to active or error after contacting only its registered provider adapter; failed verification preserves the stored credential so it can be replaced.

Disabling preserves the connection and encrypted credential but makes it ineligible for future Neural Engine use. Enabling returns it to pending eligibility and does not claim validity. Deletion first refuses connections referenced by durable configuration, then removes the vault secret and connection. Ownership-filtered queries return the same not-found result for absent and foreign IDs.

All mutations are checked through centralized `CanUseNeuralEngine` policy. Founder and premium entitlements currently satisfy that capability naturally. **Administrative role does not itself grant Neural Engine product access; entitlement policy controls Neural Engine access.**

The registry contains provider identifiers and safe labels for OpenAI, Anthropic / Claude, and Google Gemini. Connections store only safe metadata and a deliberately masked suffix; plaintext is accepted in a bounded request, passed to the server-side authenticated-encryption vault, cleared from the immediate byte buffer, and never returned. **AI-provider credentials belong to the Arbion user and persist independently of browser sessions.**

Verification and discovery exclude disabled connections; default selection additionally requires an active connection. Go remains authoritative for entitlement, ownership, state, preferences, and vault access.

The secret flow is browser → authenticated Go API → ownership and entitlement checks → vault retrieval/decryption → authenticated internal Python request → one fixed provider adapter → external provider. Python uses the credential only in request memory and does not persist or log it. Credentials are never placed in URLs, query strings, environment variables, browser responses, provider errors, traces, or audit metadata. Request and response bodies, deadlines, and upstream responses are bounded, and authentication failures are not retried.

The internal service token prevents the Python endpoints from becoming a public browser proxy, and Compose exposes the service only to the internal network. Production must replace the development token/cleartext container network with strong workload identity, encrypted service-to-service transport, token rotation, restrictive network policy, and redacted observability.

**AI provider verification does not grant trading authority.**

**AI provider credentials never cross into financial-provider connectors.**

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

Callers should request a task or capability, not import a vendor SDK or depend on a vendor response type. Arbion Insight now supports explicit user-selected OpenAI profiles; automatic task classification, cross-provider routing, fallback, and escalation remain deferred. Any fallback must be explicit because changing providers can change data handling and output behavior.

## Credential boundary

Users may eventually supply credentials for supported AI providers. These credentials are a distinct secret class from financial-provider credentials. They must be scoped to the AI adapter that needs them, encrypted or placed in managed secret storage, redacted from telemetry, and never returned to the browser after storage.

The Neural Engine and external AI providers must never receive brokerage credentials, OAuth refresh tokens, or other financial-provider secrets. Prompts and tool results should contain only the minimum authorized financial data required for the task.

## Controlled tool model

Arbion may eventually expose controlled internal tools to models, including:

- `get_accounts`
- `get_balances`
- `get_buying_power`
- `get_positions`
- `get_orders`
- `get_automations`
- `get_capital_buckets`
- `get_market_quote`
- `get_strategy_state`
- `get_decision_journal`
- `preview_order`
- `analyze_security`
- `run_backtest`
- `calculate_risk`

This remains a conceptual tool catalog, not an implemented general-purpose model API. The proposal-only Coinbase flow above is a fixed Go-orchestrated structured request, supplies no OpenAI tools, and cannot invoke these catalog entries. Arbion owns tool names, versioned input/output schemas, identity context, permissions, validation, timeouts, resource limits, data minimization, and audit records. A future model may request a tool call; it cannot grant itself access or define the trusted meaning of its arguments.

Tool output is also untrusted at subsequent boundaries: external data can be stale or malicious, and model interpretation can be wrong. Results should carry provenance and freshness where relevant and must be validated before they influence a financial action.

Tools are permissioned and scoped to the authenticated user, account, and purpose. Direct trade execution must not be offered as an unrestricted AI tool. A future execution workflow requires a structured intent plus authorization, active mandate validation where applicable, deterministic risk/control, approval policy, and the shared execution boundary.

## Multi-model operation

A future request may use a primary model, verifier, and secondary analyst (for example Claude, OpenAI, and Gemini), or a consensus mode in which models produce separate schema-constrained proposals. Arbion may compare, score, or aggregate those proposals using an explicit policy. Agreement is evidence, not authority: consensus cannot create permission, change a strategy transition, clear a breaker, or bypass deterministic validation.

The Decision Journal records provider/model identity, proposal references, selected structured factors, and concise rationale where appropriate. It does not store private model chain-of-thought. Explanations must be reconstructed from authoritative mandates, strategy state, market references, policy decisions, orders, and broker reconciliation rather than generated from conversational memory. See [Automation Engine](AUTOMATION_ENGINE.md#decision-journal-and-explainability).

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

AI assistance in `AI_AUTONOMOUS` or `HYBRID` automation is bounded by the active immutable mandate. In a Hybrid strategy, AI may choose among allowed decision variables but cannot define or skip deterministic state transitions. The [Risk and Control Engine](RISK_CONTROL_ENGINE.md) remains authoritative.

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

Later design must choose the common interface and event schemas, provider capability discovery, dynamic model catalog, automatic and cross-provider routing/fallback policy, user credential lifecycle, prompt and response retention, tool registry ownership, sandboxing, longer-term cost and quota policy, evaluation strategy, and MCP exposure model.
