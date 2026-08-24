# Market Intelligence and Command Center

## Status

This document defines the observational and non-executing market-intelligence architecture for Arbion. It does not enable live trading, submit an order, or place provider credentials in the browser or Neural Engine. Coinbase connection credentials may include the provider's optional Trade grant so the account is ready for a separately approved execution architecture, but the current application exposes no order-creation operation.

The implemented vertical slice includes normalized observations and source-selection policy; fixture-tested read-only clients for Alpaca equity quotes, CoinGecko crypto markets, keyless Coinbase venue tickers, bounded candles, ten-level Advanced Trade public books, 25-tick public trade tapes, and exact rolling product statistics, delegated Schwab quotes/options, Coinbase connected-account reads and non-executing spot-order previews with authenticated product precision/trading controls, and SEC ownership-filing discovery; strict runtime configuration; startup source probes; bounded display caches; authenticated no-store observation routes; runtime source health; a durable owner-scoped crypto watchlist; durable non-executing Coinbase proposals; and branded market and connected-portfolio surfaces. The UI displays no placeholder market values and provider failure never triggers a substitute value. Equity bars, ownership-transaction parsing, and Redis-backed coordination remain later milestones.

Current authenticated read routes are `GET /api/markets/equities/{symbol}/quote`, `GET /api/markets/crypto`, `GET /api/markets/watchlist`, `GET /api/markets/insiders/{cik}`, `GET /api/accounts/{id}/markets/equities/{symbol}/quote`, `GET /api/accounts/{id}/markets/options`, `GET /api/accounts/{id}/portfolio/crypto`, `GET /api/accounts/{id}/markets/crypto/{symbol}/candles`, `GET /api/accounts/{id}/markets/crypto/{symbol}/liquidity`, `GET /api/accounts/{id}/markets/crypto/{symbol}/trades`, `GET /api/accounts/{id}/markets/crypto/{symbol}/stats`, and `GET /api/accounts/{id}/order-intents`. The CSRF-protected `POST /api/accounts/{id}/orders/preview` route asks Coinbase to calculate a market-IoC spot-order preview, then discards the provider preview identifier and returns only bounded cost, fill, warning, and blocking evidence. Separate CSRF-protected create/review routes bind a proposal to an explicit account capital policy, refresh real connected cash and holdings, apply the deterministic Risk/Control Engine and applicable circuit breakers, durably record the normalized evidence, and permit only a TOTP-backed `PROPOSAL_REVIEW_ONLY` decision after ALLOW. They cannot contact an order-creation endpoint. Every route declares `live_execution_available: false`; credentials remain server-only. Account-scoped routes authorize ownership before using the current user's encrypted broker connection or exposing history, liquidity, public ticks, rolling venue statistics, a preview, or a non-executing proposal for a connected holding. yfinance is not a runtime fallback, and OpenInsider is exposed only as an optional human research link.

The authenticated `GET /api/markets/sources` route exposes process-local verification independently for every configured capability. A successful Coinbase ticker call cannot mark candles, liquidity, public trades, or rolling statistics verified. Each capability reports `NOT_CONFIGURED`, `AWAITING_OBSERVATION`, `VERIFIED`, `VERIFICATION_EXPIRED`, or `DEGRADED`, plus last-attempt/last-success times, consecutive failures, and a bounded failure category. A successful provider observation remains current for three cache lifetimes, with a five-minute minimum and one-hour maximum; after that it becomes `VERIFICATION_EXPIRED` until a fresh provider success. Expiry preserves the prior success and is not classified as a provider failure. Configured capabilities also report their Arbion cache lifetime, minimum provider-request interval, verification window, and capped aggregate counts of valid cache lookups, cache hits, and provider attempts. These counters carry no user, account, or instrument dimension and are explicitly not a provider quota or remaining-credit balance. Raw provider errors are never returned. Cache hits do not create a new provider success, status and usage counters reset with the API process, and runtime verification is not a quote, observation-freshness guarantee, consolidated-market claim, or execution guarantee. The `/markets` monitor refreshes this no-store metadata every 30 seconds and on demand.

The authenticated `GET /api/markets/source-history` route adds a separate durable operational view. Completed provider results are atomically aggregated into five-minute PostgreSQL buckets, retained for 30 days, and returned only as a fixed 24-hour hourly timeline. Each bucket contains source, capability, success/failure counts, latest safe state/category, and timestamps. It cannot contain a user, account, symbol, instrument, request, credential, provider-request ID, URL, response body, or raw error. Canceled requests and instrument-specific unavailability do not become provider failures. Empty hours mean no completed outcome was recorded, not downtime. History persistence is best-effort and bounded to 250 milliseconds, so a database problem cannot change or fail the provider result; the history endpoint itself fails closed with a sanitized unavailable response.

The authenticated watchlist routes persist only an owning user, canonical crypto symbol, fixed USD quote currency, and creation time. `POST /api/markets/watchlist` and `DELETE /api/markets/watchlist/{id}` are CSRF-protected Arbion preference changes; they never call a provider write API. A per-owner database transaction and advisory lock enforce uniqueness and a hard 12-item ceiling under concurrent requests. `GET /api/markets/watchlist` joins those saved preferences only to the independent keyless Coinbase observation adapter. When Coinbase is unavailable or lacks a USD product, the saved asset remains visible with an explicit gap and no substituted value. The response exposes no provider errors and declares provider writes, order actions, and live execution unavailable.

The first delivery target is a branded, authenticated `/markets` command center for one founder and a small number of test users. Cost can remain low while the product is being validated, but every observation must disclose whether it is consolidated, single-venue, indicative, delayed, or filing-derived. A cheap feed may reduce coverage; it must never make lower-quality data look authoritative.

## Product boundary

The command center brings four read-only views together:

- portfolio and account truth from connected broker accounts;
- equity and option market observations;
- crypto market breadth and asset detail; and
- issuer and insider research derived from primary filings.

These views inform a user. They do not authorize a trade. Arbion may later create bounded analysis tools over normalized snapshots, but AI remains unable to access provider credentials or bypass authorization, risk, approval, audit, and execution controls. Live execution requires a separate architecture and security approval.

## Source hierarchy

Arbion assigns each integration a declared role instead of treating providers as interchangeable.

| Source             | Approved role                              | Initial use                                                                                       | Production restriction                                                                                                                                           |
| ------------------ | ------------------------------------------ | ------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Charles Schwab     | Connected-account authority                | Account inventory, balances, positions, and the existing read-only quote/option flow              | Remains the authority for Schwab account facts; no broker-write operation                                                                                        |
| Alpaca Market Data | Independent equity and option observations | Latest quotes, snapshots, bars, and later option observations                                     | Use data credentials only. The free equity feed is IEX-only and the free option feed is indicative; neither may be labeled consolidated                          |
| CoinGecko          | Crypto reference and breadth               | Prices, market capitalization, volume, trending assets, global statistics, and asset history      | Use an authenticated keyed plan for production. WebSocket data is supplementary while the provider marks it beta and outside its SLA                             |
| Coinbase App       | Connected-account authority                | Permissioned portfolio inventory, USD cash, exact crypto holdings, and non-executing spot preview | Per-user encrypted key requires View, may include Trade, and must exclude Transfer; Trade is recorded as provider authorization but no provider write is exposed |
| Coinbase venues    | Keyless single-venue crypto observation    | Current trade, bid, ask, 24-hour volume/candles, and bounded Advanced Trade public book depth     | Public data only; label it single-venue and non-executable, use short bounded caching, and revalidate redistribution terms before broader release                |
| SEC EDGAR          | Primary issuer and insider filing source   | Filing discovery plus Forms 3, 4, and 5 ownership data                                            | Identify Arbion in the user agent, cache efficiently, preserve accession/source links, and stay below the SEC fair-access ceiling                                |
| yfinance           | Developer research aid only                | Optional local exploration and test-fixture comparison                                            | Never used for a production screen, alert, strategy input, risk input, or fallback; the project states that Yahoo data is intended for personal use              |
| OpenInsider        | Optional human research link only          | Deep-link from an SEC-derived filing when useful                                                  | No automated production dependency without a documented supported API and completed license/availability review; never the authoritative filing record           |

Sources:

- [Alpaca Market Data API](https://docs.alpaca.markets/us/docs/about-market-data-api)
- [Alpaca market-data feed differences](https://docs.alpaca.markets/us/docs/market-data-faq)
- [CoinGecko Pro authentication](https://docs.coingecko.com/reference/authentication)
- [CoinGecko keyless API limits](https://docs.coingecko.com/docs/keyless-public-api)
- [CoinGecko WebSocket status](https://docs.coingecko.com/websocket)
- [Coinbase public ticker](https://docs.cdp.coinbase.com/api-reference/exchange-api/rest-api/products/get-product-ticker)
- [Coinbase App API-key authentication](https://docs.cdp.coinbase.com/coinbase-app/authentication-authorization/api-key-authentication)
- [Coinbase Advanced Trade preview order](https://docs.cdp.coinbase.com/api-reference/advanced-trade-api/rest-api/orders/preview-orders)
- [Coinbase Advanced Trade create order](https://docs.cdp.coinbase.com/api-reference/advanced-trade-api/rest-api/orders/create-order)
- [Coinbase public product book](https://docs.cdp.coinbase.com/api-reference/advanced-trade-api/rest-api/public/get-public-product-book)
- [Coinbase public market trades](https://docs.cdp.coinbase.com/api-reference/advanced-trade-api/rest-api/public/get-public-market-trades)
- [Coinbase public product stats](https://docs.cdp.coinbase.com/api-reference/exchange-api/rest-api/products/get-product-stats)
- [Coinbase public real-time WebSocket](https://docs.cdp.coinbase.com/coinbase-app/advanced-trade-apis/websocket/websocket-overview)
- [SEC EDGAR data APIs](https://www.sec.gov/search-filings/edgar-application-programming-interfaces)
- [SEC developer resources and fair access](https://www.sec.gov/about/developer-resources)
- [yfinance project notice](https://github.com/ranaroussi/yfinance#readme)

Provider terms, entitlement, pricing, feeds, and redistribution rules must be revalidated before every production enablement. A technically accessible endpoint does not grant Arbion a right to store or redistribute its data.

## Normalized observation contract

Independent market data must not be forced through a connected brokerage account or reuse a broker token. Delegated Schwab observations are a separate, explicitly broker-authorized source: every request is account-scoped and ownership-checked, and its provenance role is `BROKER_AUTHORITY`. The provider-neutral market-data module remains in the Go control plane and exposes narrow read capabilities such as equity quotes, bars, option observations, crypto markets, bounded crypto liquidity, and filings.

Every normalized observation carries at least:

- provider and provider correlation identifier when available;
- source role and exact feed name;
- asset class, canonical instrument identifier, and display symbol;
- exact decimal values and currency, never binary floating point for financial amounts;
- provider event time and Arbion receipt time when the upstream contract supplies both; otherwise an explicit receipt-only record that cannot be relabeled as provider event time;
- quality classification: `REAL_TIME_CONSOLIDATED`, `REAL_TIME_SINGLE_VENUE`, `INDICATIVE`, `DELAYED`, `AGGREGATED_REFERENCE`, `END_OF_DAY`, or `FILING`;
- freshness deadline and current stale state;
- venue or aggregation scope when applicable; and
- license-aware storage and display policy.

Provider timestamp means when the source says the event occurred. Receipt timestamp means when Arbion accepted the response. The two are not interchangeable. Missing, future-dated, malformed, or stale observations fail closed anywhere that requires current data.

Fallback is explicit. Arbion may select another approved source only when the caller's policy permits that source role and quality. A fallback must change the provider/feed badges and resulting evidence; it must never silently substitute IEX, indicative, delayed, or aggregate reference data for consolidated executable-market data.

## Control-plane design

The implementation remains inside the Go modular monolith:

```text
Authenticated UI / future bounded AI tool
                    |
                    v
       market-intelligence use cases
     authorization, policy, freshness,
      source selection, audit, budgets
                    |
                    v
        provider-neutral capabilities
         /            |             \
        v             v              v
 Schwab/Alpaca     Coinbase/CoinGecko     SEC
 equities/options       crypto           filings
```

Provider adapters own URL construction, authentication headers, pagination, provider rate limits, response-schema validation, and safe error translation. They may make read-only requests only. Transport handlers do not know provider wire formats and cannot choose an arbitrary upstream URL.

Application-level data keys are managed deployment secrets and are distinct from each user's encrypted broker authorization. Secrets are injected only into the adapter that needs them, never returned after storage, never sent to Python, and never placed in query strings when the provider supports authentication headers.

## Reliability, storage, and cost

PostgreSQL remains durable truth for watchlists, source policy, SEC filing metadata, user-visible alerts, and immutable evidence that affected a strategy or decision. Redis may hold short-lived quote and overview caches, rate-limit state, and refresh coordination. Redis loss may degrade availability but cannot change source policy, silently downgrade a feed, or make stale data current.

Raw market-data payloads and long-term price history are stored only when the provider's agreement permits it. Otherwise Arbion retains the minimum normalized evidence required for its own decision journal and fetches display data under bounded caches. SEC records retain accession numbers and source URLs so the filing can be independently verified.

Each adapter must implement:

- explicit deadlines and response-size bounds;
- bounded retry with jitter for safe idempotent reads only;
- provider-aware rate limiting and a separately verified provider-quota budget;
- circuit state and health telemetry without secret values;
- schema and decimal validation before normalization;
- no-store browser responses for user-specific or licensed data; and
- metrics for latency, error class, cache age, feed quality, and quota consumption.

The current founder deployment implements in-process, per-capability last-attempt and last-success telemetry with safe failure categories. Time-bounded verification prevents an old success from remaining green indefinitely while keeping neutral aging distinct from an observed upstream failure. The monitor also reports capped cache-lookup, cache-hit, and provider-attempt counters beside the configured cache lifetime, minimum request interval, and verification window. A separate 24-hour branded timeline displays privacy-safe durable provider outcomes across API restarts without turning missing buckets into outages. This makes Arbion's request protection, cache reuse, and recent provider behavior visible without claiming knowledge of an upstream quota. Latency percentiles, provider-published quota accounting, alert thresholds, and cross-host aggregation remain later operational work.

For the initial founder deployment, use the founder's delegated Schwab market-data entitlement for account-scoped equities/options and Coinbase Exchange public tickers for a bounded crypto venue board. Coinbase values must show `REAL_TIME_SINGLE_VENUE`; Schwab responses must preserve the provider's real-time/delayed entitlement flag and use `INDICATIVE` when that flag is absent. A production gate decides whether independent consolidated SIP/OPRA access, aggregated crypto breadth, or paid historical data is justified by actual usage.

## Branded command-center experience

The `/markets` experience uses Arbion's existing wordmark, colors, spacing, cards, and accessibility patterns. It should include:

- a global market strip with market session, feed, and freshness badges;
- a founder watchlist with equity and crypto rows;
- equity detail with quote, bars, option summary, and SEC insider activity;
- a crypto board with price, market cap, volume, and trend context;
- connected-portfolio exposure sourced from Schwab or Coinbase, clearly separated from public market data;
- provider health and degraded-data notices; and
- links from insider events to the original SEC filing.

The page must avoid presenting a single-venue or aggregate reference price as a guaranteed executable price. Any future "Ask Arbion" action receives a bounded normalized snapshot through a Go-owned tool and returns sourced analysis, not a trade command.

## Delivery milestones

### 1. Market-data foundation

- Implemented: provider-neutral observation, feed-quality, freshness, and capability models in Go.
- Implemented: deterministic validation and source-selection policy with tests.
- Implemented: runtime health and source metadata backed by disabled-by-default configuration.
- Implemented: independent per-capability verification timestamps and safe failure categories; raw provider diagnostics are excluded.
- Implemented: cache-policy-derived verification windows with a neutral aged state, five-minute minimum, and one-hour maximum.
- Implemented: 30-day bounded PostgreSQL health buckets and a privacy-safe 24-hour hourly history API.
- Implemented: documented optional configuration without real credentials.

Exit gate: malformed and stale data fail closed; source quality cannot be omitted; no order or trading scope exists.

### 2. Equity and option observations

- Implemented: an Alpaca data-only adapter and authenticated latest-equity-quote route with bounded caching.
- Implemented: account-owned Schwab quote and standard option-chain routes with explicit entitlement provenance.
- Next: equity bars and richer snapshots.
- Add option data only after its indicative-versus-OPRA behavior is represented in the contract and UI.
- Exercise the adapter against provider fixtures and a manually enabled development key.

Exit gate: IEX and indicative data are visibly labeled and cannot satisfy a consolidated-feed policy.

### 3. Crypto reference data

- Implemented: a CoinGecko keyed adapter and authenticated top-markets route.
- Implemented: a keyless Coinbase Exchange ticker adapter for bounded single-venue production snapshots.
- Implemented: owner-scoped Coinbase Portfolio Breakdown observations that retain exact total, available-to-trade, and staked-or-otherwise-unavailable quantities, price up to 32 holdings against approved USD tickers, preserve per-position venue/time evidence, and expose partial coverage without estimates.
- Implemented: owner-scoped Coinbase Exchange 24-hour asset history using exactly 96 requested fifteen-minute intervals, exact decimal validation, one-minute caching, explicit single-venue provenance, and provider gaps that remain unfilled.
- Implemented: owner-scoped Coinbase Exchange rolling product statistics with exact open/high/low/last, 24-hour base volume, 30-day base volume, a 30-second cache, and explicit Arbion-receipt-time semantics because the provider response has no event timestamp.
- Implemented: a durable owner-scoped 12-asset crypto watchlist priced only from keyless Coinbase USD venue observations, with CSRF-protected preference changes and explicit partial/outage states.
- Implemented: a private Coinbase account connection that requires View, permits Trade, rejects Transfer, keeps the ECDSA key server-side, and projects the verified permission boundary without granting Arbion or AI execution authority.
- Implemented: an owner-scoped, CSRF-protected Coinbase spot market-IoC preview that accepts one exact USD buy size or base-asset sell size, reads authenticated product increments/bounds/trading state, blocks invalid or restricted sizes, returns normalized provider cost/fill evidence, discards the provider preview identifier, and cannot create an order.
- Implemented: a bounded display-cache policy with per-capability cache/pacing configuration, capped process-local usage counters, and responsive cache-efficiency visibility.
- Next: add Redis coordination and provider-published quota accounting only where an approved provider contract supplies reliable quota facts.
- Next: global overview and identifier mapping by CoinGecko ID and contract address.

Exit gate: keyless traffic is bounded and visibly single-venue; no secret enters a URL, log, browser bundle, or Neural Engine request.

### 4. Primary-source insider intelligence

- Implemented: SEC submissions discovery for Forms 3, 4, and 5 with primary-source links and fair-access pacing.
- Next: parse ownership XML transactions without reducing nuanced codes to generic buy/sell claims.
- Preserve issuer/reporting-owner identifiers, transaction codes, direct/indirect ownership, accession number, filing time, and source link.
- Display amendments and derivative transactions without flattening them into misleading buy/sell labels.

Exit gate: all displayed events link to SEC evidence and the importer complies with fair-access policy.

### 5. Arbion command center

- Implemented: the branded `/markets` overview plus a motion-enhanced Coinbase portfolio command surface with observed value, cash, coverage, priced allocation, source-stamped 24-hour connected-asset charts, a position ledger, and evidence controls.
- Implemented: responsive per-capability source status with current, aged, degraded, process-local, durable-history, and non-executable semantics.
- Implemented: responsive 24-hour health strips that distinguish success, mixed outcomes, failure, and neutral no-observation intervals.
- Implemented: a responsive motion-enhanced durable crypto radar with exact source evidence, empty/partial/outage states, and no provider action controls.
- Implemented: a branded connected-account preview panel that distinguishes Coinbase Trade permission from Arbion execution authority and exposes no submit, replace, cancel, or transfer control.
- Next: dedicated equity detail and bounded health alert thresholds.
- Keep broker account truth visually distinct from public/reference data.
- Add responsive, loading, empty, degraded, stale, and provider-outage states with browser tests.

Exit gate: the product remains useful when a provider is unavailable and never hides the resulting loss of coverage or quality.

### 6. Controlled intelligence

- Define read-only Go tools that return bounded, source-stamped snapshots to Arbion Insight.
- Add prompt-injection defenses, output schemas, budgets, audit metadata, and explicit educational labels.

Exit gate: Python has no financial credentials, AI cannot choose an upstream URL, and no analysis result can authorize or submit a trade.

## Explicitly deferred

- Alpaca brokerage OAuth or account connection;
- order submission, replacement, cancellation, execution reconciliation, or provider-write automation;
- automated strategies using a new source before feed-quality safety review;
- social/sentiment scraping;
- automated OpenInsider collection;
- yfinance in production; and
- redistribution or resale of provider data without contractual approval.
