# Financial Connectors

## Production Schwab callback

Schwab must register exactly `https://www.arbion.ai/api/connections/financial/schwab/callback`. Caddy routes that unchanged `/api/*` path to Go. `SCHWAB_CLIENT_ID` and `SCHWAB_CLIENT_SECRET` remain host-managed secrets; production fails closed for partial configuration or a different callback. Production verification is manual and read-only: account discovery, balances, positions, quotes, option chains, and duplicate-safe sync. It never places an order.

## Role and scope

The financial connector layer translates Arbion's provider-independent financial operations into external provider APIs. Schwab delegated reads and a founder-phase Coinbase account/holdings connection are implemented as described below; E\*TRADE, Alpaca brokerage, Interactive Brokers, and other providers remain candidates.

Connectors belong in the Go modular monolith because Go owns the control plane and financial integration boundary. A connector is an adapter, not a source of authorization or risk policy.

## Provider abstraction

Platform modules should depend on narrow capability interfaces rather than provider SDKs or wire formats. Provider-specific authentication, endpoints, identifiers, pagination, rate limits, errors, asset conventions, and order semantics stay behind adapters.

Connected-account reads and independent market-data subscriptions have different identity and credential lifecycles. The existing Schwab market-data capability may use the user's delegated broker authorization. Coinbase portfolio credentials are used only for account inventory, holdings, bounded external activity, and provider fee-tier evidence; the public Coinbase Exchange ticker remains a separate keyless observation source. Alpaca, CoinGecko, SEC EDGAR, and another independent source instead belong behind the provider-neutral market-intelligence boundary described in [Market Intelligence and Command Center](MARKET_INTELLIGENCE.md); they must not fabricate a financial account or reuse a user's account credential.

Conceptual capabilities may eventually include:

- accounts and account metadata;
- balances and buying power;
- positions;
- orders and order status;
- quotes and market data;
- transactions;
- order preview;
- order placement; and
- order cancellation.

Interfaces should be capability-based because providers do not offer identical products or semantics. Adapters must report unsupported capabilities explicitly rather than emulate unsafe behavior. Normalized models must preserve provider identifiers, timestamps, precision, currency, asset class, provenance, freshness, and raw status needed for reconciliation.

Order placement and cancellation are listed only to reserve the abstraction boundary. Live or automated trading remains prohibited until an explicit future architecture and security review approves its authorization, risk, approval, audit, idempotency, reconciliation, and failure behavior.

## Control-plane relationship

All connector calls originate from an authorized Go use case. Before a sensitive financial operation reaches an adapter, the control plane must deterministically evaluate applicable identity, tenant/account ownership, permission, risk, account status, buying power, order validity, approval, and execution policy. The connector must not accept an AI assertion as evidence that these checks passed.

Read operations are also permissioned and audited as appropriate. Connector responses are external input and require schema validation, normalization, provenance, and safe failure handling.

The Python Neural Engine does not call connectors directly. AI-requested reads or previews use controlled tools mediated by Go. AI-generated order proposals remain untrusted input.

## Credential isolation

Financial-provider credentials form a separate trust domain from AI-provider credentials. They must be available only to the minimum Go connector components that require them. They must never be included in AI prompts, tool payloads, browser responses, analytics, error messages, or logs, and must never be committed to Git.

After storage, the platform returns only non-secret connection metadata such as provider name, connection state, scopes, and safe account labels. Production credentials will require managed secret storage or envelope encryption, access auditing, rotation, revocation, and strict network and process access policies.

## Implemented Coinbase account connection

The founder-phase Coinbase connector uses a user-created Coinbase App secret API key because Coinbase currently reserves third-party OAuth client creation for approved partners and directs single-owner server integrations to API keys. Arbion accepts only the full CDP key name plus an ECDSA P-256 private key, generates a unique ES256 request-bound JWT with a two-minute lifetime for every provider call, and sends it only from Go to `https://api.coinbase.com`. The private key is stored only through the authenticated financial Vault class and is never returned, logged, committed, or sent to Next.js or Python.

Connection verification calls Coinbase's key-permission endpoint and requires `can_view=true`, `can_trade=false`, and `can_transfer=false`. A key with trading or asset-transfer authority is rejected instead of merely hiding that authority in the UI. Operators should additionally restrict the key to the intended Coinbase portfolio and the production host IP. Coinbase requires the ECDSA signature option for Coinbase App APIs; Ed25519 keys are rejected by the adapter.

The connector paginates the authenticated Advanced Trade account inventory with bounded pages and normalizes it as one `Coinbase Portfolio` financial account. Individual nonzero asset wallets become exact-decimal holdings under that portfolio; the provider wallet UUIDs and portfolio ID remain server-only. USD available/held balances are represented without inventing a cross-asset portfolio value or cost basis. Sync is duplicate-safe through the existing `(provider_connection_id, provider_account_id)` inventory key. Disable, enable, sync, and disconnect share the existing owner-scoped lifecycle; disconnect deletes Arbion's encrypted key but does not claim to delete the key in Coinbase, so the owner should revoke it in the Coinbase portal when appropriate.

The authenticated `GET /api/accounts/{id}/portfolio/crypto` read model combines those current holdings with a separately keyless Coinbase Exchange `GET /products/{product_id}/ticker` observation for at most 32 unique USD products per refresh. Exact-decimal multiplication produces an informational observed value based on the ticker's last trade; bid, ask, venue, feed quality, and provider time remain attached to every priced position. A missing USD product or market outage leaves the holding visible and unpriced, reports partial or unavailable coverage, and never substitutes a stablecoin peg, cross rate, cached database estimate, or cost basis. The response is owner-scoped, `no-store`, and always declares that live execution is unavailable.

The owner-scoped `GET /api/accounts/{id}/markets/crypto/{symbol}/candles` route is limited to assets currently present in that Coinbase account. It requests exactly 96 fifteen-minute Coinbase Exchange candle intervals for a 24-hour venue view, retains exact provider decimals and source timestamps, caches a normalized series for one minute, and returns `chart_semantics: VENUE_PRICE_MOVEMENT`. Coinbase may omit intervals in which there were no ticks; Arbion sorts and bounds the response but never inserts or interpolates a missing bucket. The chart is therefore market evidence for one venue and one asset, not portfolio performance, return, cost basis, or P&L.

The separate keyless `marketintelligence.CryptoLiquidityProvider` calls Coinbase Advanced Trade's public `GET /api/v3/brokerage/market/product_book` endpoint for only a currently held USD asset. The owner-scoped `GET /api/accounts/{id}/markets/crypto/{symbol}/liquidity` route fixes depth to ten bid and ten ask levels, keeps exact provider price/size and summary decimals plus source time, sorts both sides, rejects books older than 30 seconds or crossed books, and caches the normalized snapshot for one second. The response declares `SINGLE_VENUE_LIQUIDITY_SNAPSHOT`, `order_book_streaming=false`, `order_actions_available=false`, and `live_execution_available=false`. It is a manual-refresh REST observation, not a full or streaming book, consolidated market, executable quote, order preview, or slippage estimate.

The keyless `marketintelligence.CryptoTradeProvider` calls the public `GET /api/v3/brokerage/market/products/{product_id}/ticker` endpoint for the newest 25 ticks of a currently held USD asset. The owner-scoped `GET /api/accounts/{id}/markets/crypto/{symbol}/trades` projection preserves exact price, size, provider time, and Coinbase-reported BUY/SELL side plus current best bid/ask, while discarding trade IDs, per-tick bid/ask copies, exchange labels, cursors, and arbitrary time-range input. It uses the same 30-second freshness and one-second cache bounds and declares `trade_streaming=false`, `order_flow_inference=false`, `order_actions_available=false`, and `live_execution_available=false`.

The keyless `marketintelligence.CryptoVenueStatsProvider` calls Coinbase Exchange public `GET /products/{product_id}/stats` for only a currently held USD asset. The owner-scoped `GET /api/accounts/{id}/markets/crypto/{symbol}/stats` route preserves exact rolling open, high, low, last, 24-hour base volume, and 30-day base volume, validates their range and window relationships, and stores only a 30-second process cache. Coinbase supplies no event timestamp in this contract, so Arbion returns a separate source receipt and declares `provider_event_time_available=false` and `timestamp_semantics=ARBION_RECEIPT_TIME`; receipt time is never presented as Coinbase observation time. The route is no-store, performance-free, action-free, and execution-free.

The optional `financial.TradeHistoryProvider` extension is separate from the base account connector and contains only a historical fill read. Coinbase implements it with the Advanced Trade View-authorized `GET /api/v3/brokerage/orders/historical/fills` endpoint after re-verifying View and the absence of Transfer; an optional Trade grant does not change this read boundary. The owner-scoped `GET /api/accounts/{id}/activity/fills` projection fixes the request to the first 50 spot fills, preserves exact price, size, commission, side, liquidity, and execution time, and labels the result `EXTERNAL_EXECUTION_EVIDENCE`. Provider entry, trade, order, user, and portfolio identifiers plus pagination cursors never enter the response. A Coinbase proof-token challenge fails closed because Arbion does not implement that stronger-authentication flow. This projection is not an order interface, transaction-history archive, tax lot, cost-basis, performance, or P&L model.

The separate `financial.OrderHistoryProvider` extension contains one bounded order-status read and no preview or mutation method. Coinbase implements it with the View-authorized `GET /api/v3/brokerage/orders/historical/batch` endpoint, fixed to the first 50 Advanced Trade spot orders. The owner-scoped `GET /api/accounts/{id}/activity/orders` projection preserves normalized status, side/type/time-in-force, exact fill percentage/size/value/average/fees, fill count, settlement/cancel-pending/liquidation flags, and provider times. It omits order, client-order, user, portfolio, originating/attached-order identifiers, provider messages, configurations, edit history, and cursors. The UI labels every record provider-reported and external to Arbion, exposes no order action, and does not call Coinbase's View-authorized preview endpoint.

The `financial.TradingCostProvider` extension performs only Coinbase's View-authorized `GET /api/v3/brokerage/transaction_summary?product_type=SPOT` read. The owner-scoped `GET /api/accounts/{id}/activity/trading-costs` projection retains the current pricing-tier label, exact maker/taker rates, exact Advanced Trade volume/fees, exact provider total fees, the cost-plus-commission flag, and retrieval time. It discards balances, margin/tax fields, Coinbase Pro fields, volume-breakdown internals, and tier-range internals. The UI labels the result a provider fee-tier snapshot and does not infer a reporting window, next-tier progress, fee forecast, order-specific quote, tax record, cost basis, or performance.

This adapter implements no order mutation, trade, convert, address, transfer, deposit, withdrawal, transaction-ledger, or tax endpoint. The transaction-summary read is fee-tier evidence only. A later execution milestone must use a separate write-capability interface and permission set rather than adding a write method to this read boundary.

## OAuth and token lifecycle

Use OAuth or provider-supported delegated token authorization instead of collecting reusable usernames and passwords wherever possible. A future implementation must define:

- authorization-code flow and PKCE where applicable;
- exact requested scopes and incremental consent;
- state and redirect validation;
- encrypted access/refresh-token storage;
- refresh concurrency and token rotation;
- expiry, revocation, disconnect, and reauthorization behavior;
- webhook authenticity and replay protection; and
- redaction of headers, URLs, payloads, and provider errors.

OAuth does not remove the need for Arbion authorization: possession of a provider token must not by itself authorize a platform action.

## Reliability and audit expectations

Future adapters should use explicit deadlines, bounded retries with jitter, rate-limit handling, circuit protection where justified, and idempotency controls for side effects. The platform must distinguish accepted, rejected, pending, unknown, and reconciled outcomes. Audit events should record actor, policy decision, account, requested operation, provider correlation identifiers, and outcome without recording secrets.

## Deferred decisions

Later design must choose the initial providers and capabilities, canonical account/asset/order models, decimal and currency rules, market-data sources and licensing, polling/webhook strategy, OAuth scope sets, secret storage, reconciliation and idempotency schemes, sandbox certification, error taxonomy, provider outage behavior, and the approval model for any live execution.

## Implemented read-only financial foundation (Schwab)

The Go `financial.BrokerProvider` boundary contains only verification, authorization refresh or static-key re-verification, account discovery, account and balance reads, position reads, capability reads, and disconnect. Separate optional `financial.MarketDataProvider`, `financial.TradeHistoryProvider`, `financial.OrderHistoryProvider`, and `financial.TradingCostProvider` boundaries contain only market observations, historical execution/order-state evidence, and provider fee-tier evidence. A distinct `financial.OrderPreviewProvider` contains one non-executing preview operation; it has no placement, replacement, cancellation, or other trading method. The centralized registry marks `schwab` and `coinbase` implemented and `etrade` planned; auth type is provider metadata, with Schwab using OAuth 2 authorization code and Coinbase using a JWT/key-pair credential.

Schwab numeric fields remain exact through `json.Number` and rational validation. Integer-valued market-data fields may use decimal notation such as `100.0`; Arbion accepts them only when they are mathematically whole, nonnegative, and fit the host integer range. Fractional, negative, malformed, or overflowing integer fields fail closed before strategy evaluation. Option expiration values may be an ISO calendar date or a fully validated RFC 3339 timestamp; Arbion preserves the declared calendar date in its provider-independent model and rejects partial or malformed timestamps. Quote responses and option-chain contracts retain their distinct provider field names; required option `bid` and `delta` values are never fabricated when absent.

Schwab authorization is an OAuth 2 authorization-code redirect. Arbion generates a 256-bit URL-safe state, binds it to the initiating authenticated and `CanConnectFinancialAccounts`-entitled user, stores it ephemerally with a short expiry, and consumes it exactly once before exchanging a code. The callback never accepts a caller-selected redirect. Cancellation, invalid/expired/reused state, provider denial, token exchange failure, and success are distinct outcomes. Authorization codes, access tokens, refresh tokens, client secrets, full account numbers, and raw provider bodies are excluded from logs, audits, metadata, and browser responses.

Platform/client credentials (`SCHWAB_CLIENT_ID`, `SCHWAB_CLIENT_SECRET`, and the exact registered redirect URI) are deployment configuration, never per-user data. User access/refresh tokens and expiry metadata form a structured payload encrypted as credential class `financial` in the existing Vault. On-demand server-side refresh replaces that payload atomically; callers must serialize refresh per connection to prevent token-rotation races. A terminal refresh denial moves the connection to `expired` or `revoked`; transient failures do not erase usable encrypted material. Browser logout only destroys the browser session and has no provider-connection side effect.

Account discovery first obtains Schwab's opaque account hash identifiers, then upserts every accessible account by `(provider_connection_id, provider_account_id)`. Full brokerage account numbers are not required: APIs and UI expose labels such as `Schwab Brokerage ••••4821`, while opaque API identifiers remain server-side. Repeated synchronization updates inventory and does not duplicate it. One connection may own many accounts.

Balances and positions use JSON decimal strings at transport boundaries and are never converted to binary floating point. Missing Schwab fields remain absent rather than being synthesized. Instrument types are preserved instead of assuming equity. Capability values are `SUPPORTED`, `UNSUPPORTED`, or `UNKNOWN`; unknown is the default unless Schwab supplies a reliable fact (for example, an account type explicitly reported as margin).

Quotes and option chains also preserve exact decimal strings, provider timestamps, and Schwab's reported real-time/delayed entitlement state. The adapter performs only authenticated GET requests against the configurable Market Data base URL, requests quote/reference fields and single-leg chains, and normalizes only standard non-mini 100-share contracts. The `/markets` UI reaches these observations only through account-owned authenticated routes. Missing or malformed required data fails closed; the manual strategy service additionally rejects stale, missing, or implausibly future-dated market observations.

**Buying power is information, not trading permission.**

**Broker-reported buying power does not grant Arbion authority to deploy that capital.**

### Official documentation and external app setup

Schwab-specific deployment must be revalidated against the current, authenticated [Charles Schwab Developer Portal](https://developer.schwab.com/) before release. The portal was consulted for the Trader API product boundary; its documentation is access-controlled and the repository therefore keeps authorization, token, and Trader base URLs configurable rather than treating source constants as an immutable specification. An operator must create and obtain approval for a Schwab developer application, enable the Trader API read access offered to that app, register the exact HTTPS callback URI, obtain the app key/client ID and secret, accept Schwab agreements, and complete any provider-required account linking/certification. Do not request execution access for this milestone. Confirm the approved app's current endpoints, required consent/scopes, token and refresh lifetimes/rotation, account-hash behavior, response schemas, limits, and disconnect/revocation procedure in the portal before production deployment.

The current account flow uses the Trader API account-number mapping and account reads. Market evaluation uses the separately configurable Market Data base URL with `GET /{symbol}/quotes` and `GET /chains`; the adapter defaults shown in `.env.example` remain explicit deploy-time configuration. Schwab does not currently expose a verified safe revocation call in this implementation, so disconnect deletes Arbion's Vault material and retires the local connection; the user/operator must also revoke application access through Schwab when required.

## Implemented Schwab read-only lifecycle

PR #10 established the provider registry, `BrokerProvider` contract, exact-decimal normalized account/balance/position models, Schwab HTTP adapter, OAuth-state interface, financial-account schema, provider catalog endpoint, Vault credential class, and placeholder settings card. This milestone completes the user lifecycle without introducing another connector architecture.

An entitled, authenticated user starts authorization from **Settings → Connections**. Go creates a 256-bit opaque state in Redis with a ten-minute TTL and sends the browser to Schwab. The fixed callback consumes state exactly once, exchanges the code only in Go, stores the structured token set in the financial Vault class, upserts the durable Schwab `provider_connections` record, discovers every account/hash, and safely redirects to Arbion. Codes and tokens never enter Next.js.

Before reads, Go loads the encrypted credential and refreshes near-expiry authorization under a PostgreSQL advisory lock, coordinating all API instances. A terminal refresh failure marks the connection expired and preserves inventory. Sync upserts accounts by connection plus Schwab account hash and marks missing accounts unavailable. Disable retains authorization and inventory but blocks reads; enable validates authorization; disconnect deletes Vault material and retires the connection and accounts. Schwab does not publish a supported Trader API token-revocation endpoint in the accessible product documentation, so Arbion does not invent one; users may separately remove application access in Schwab security settings.

Required Schwab Developer Portal setup: create and obtain approval for a Trader API Individual application, configure the callback to exactly match `SCHWAB_REDIRECT_URI`, enable the portal's Market Data Production product, and provide `SCHWAB_CLIENT_ID` and `SCHWAB_CLIENT_SECRET` as deployment secrets. Arbion uses the authorization-code endpoints at `https://api.schwabapi.com/v1/oauth/authorize` and `/v1/oauth/token`, Trader API account-number/hash and account endpoints, and the read-only Market Data endpoints at `https://api.schwabapi.com/marketdata/v1`. The application uses only portal-approved read access; no order operation exists. Provider approval, redirect propagation, token lifetime/refresh rules, scopes, and applicable rate limits remain controlled by Schwab and must be rechecked in the authenticated portal before production launch.
