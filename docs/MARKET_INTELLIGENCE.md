# Market Intelligence and Command Center

## Status

This document defines the read-only market-intelligence architecture for Arbion. It does not enable live trading, add an order endpoint, request a broker trading scope, or place provider credentials in the browser or Neural Engine.

The implemented foundation includes normalized observations and source-selection policy, fixture-tested read-only clients for Alpaca equity quotes, CoinGecko crypto markets, and SEC ownership-filing discovery, an authenticated no-store source catalog, and the branded `/markets` shell. All external sources remain runtime-disabled: deployment configuration, provider credentials, health checks, user-facing data routes, caching, durable watchlists/filing records, and production polling are later milestones. The UI displays no placeholder market values.

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

| Source | Approved role | Initial use | Production restriction |
| --- | --- | --- | --- |
| Charles Schwab | Connected-account authority | Account inventory, balances, positions, and the existing read-only quote/option flow | Remains the authority for Schwab account facts; no broker-write operation |
| Alpaca Market Data | Independent equity and option observations | Latest quotes, snapshots, bars, and later option observations | Use data credentials only. The free equity feed is IEX-only and the free option feed is indicative; neither may be labeled consolidated |
| CoinGecko | Crypto reference and breadth | Prices, market capitalization, volume, trending assets, global statistics, and asset history | Use an authenticated keyed plan for production. WebSocket data is supplementary while the provider marks it beta and outside its SLA |
| SEC EDGAR | Primary issuer and insider filing source | Filing discovery plus Forms 3, 4, and 5 ownership data | Identify Arbion in the user agent, cache efficiently, preserve accession/source links, and stay below the SEC fair-access ceiling |
| yfinance | Developer research aid only | Optional local exploration and test-fixture comparison | Never used for a production screen, alert, strategy input, risk input, or fallback; the project states that Yahoo data is intended for personal use |
| OpenInsider | Optional human research link only | Deep-link from an SEC-derived filing when useful | No automated production dependency without a documented supported API and completed license/availability review; never the authoritative filing record |

Sources:

- [Alpaca Market Data API](https://docs.alpaca.markets/us/docs/about-market-data-api)
- [Alpaca market-data feed differences](https://docs.alpaca.markets/us/docs/market-data-faq)
- [CoinGecko Pro authentication](https://docs.coingecko.com/reference/authentication)
- [CoinGecko keyless API limits](https://docs.coingecko.com/docs/keyless-public-api)
- [CoinGecko WebSocket status](https://docs.coingecko.com/websocket)
- [SEC EDGAR data APIs](https://www.sec.gov/search-filings/edgar-application-programming-interfaces)
- [SEC developer resources and fair access](https://www.sec.gov/about/developer-resources)
- [yfinance project notice](https://github.com/ranaroussi/yfinance#readme)

Provider terms, entitlement, pricing, feeds, and redistribution rules must be revalidated before every production enablement. A technically accessible endpoint does not grant Arbion a right to store or redistribute its data.

## Normalized observation contract

Independent market data must not be forced through a connected brokerage account or reuse a broker token. A new provider-neutral market-data module belongs in the Go control plane and should expose narrow read capabilities such as equity quotes, bars, option observations, crypto markets, and filings.

Every normalized observation carries at least:

- provider and provider correlation identifier when available;
- source role and exact feed name;
- asset class, canonical instrument identifier, and display symbol;
- exact decimal values and currency, never binary floating point for financial amounts;
- provider event time and Arbion receipt time;
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
  Alpaca adapter  CoinGecko adapter  SEC adapter
   equities/options    crypto        filings
```

Provider adapters own URL construction, authentication headers, pagination, provider rate limits, response-schema validation, and safe error translation. They may make read-only requests only. Transport handlers do not know provider wire formats and cannot choose an arbitrary upstream URL.

Application-level data keys are managed deployment secrets and are distinct from each user's encrypted broker authorization. Secrets are injected only into the adapter that needs them, never returned after storage, never sent to Python, and never placed in query strings when the provider supports authentication headers.

## Reliability, storage, and cost

PostgreSQL remains durable truth for watchlists, source policy, SEC filing metadata, user-visible alerts, and immutable evidence that affected a strategy or decision. Redis may hold short-lived quote and overview caches, rate-limit state, and refresh coordination. Redis loss may degrade availability but cannot change source policy, silently downgrade a feed, or make stale data current.

Raw market-data payloads and long-term price history are stored only when the provider's agreement permits it. Otherwise Arbion retains the minimum normalized evidence required for its own decision journal and fetches display data under bounded caches. SEC records retain accession numbers and source URLs so the filing can be independently verified.

Each adapter must implement:

- explicit deadlines and response-size bounds;
- bounded retry with jitter for safe idempotent reads only;
- provider-aware rate limiting and a per-provider request budget;
- circuit state and health telemetry without secret values;
- schema and decimal validation before normalization;
- no-store browser responses for user-specific or licensed data; and
- metrics for latency, error class, cache age, feed quality, and quota consumption.

For the initial founder deployment, use Alpaca Basic and CoinGecko's authenticated development tier to validate the experience. Alpaca Basic must show `IEX` for equities and `INDICATIVE` for options. CoinGecko's keyless endpoint is acceptable only for local manual prototyping, not scheduled or production traffic. A production gate decides whether consolidated Alpaca SIP/OPRA access or a paid CoinGecko plan is justified by actual usage.

## Branded command-center experience

The `/markets` experience uses Arbion's existing wordmark, colors, spacing, cards, and accessibility patterns. It should include:

- a global market strip with market session, feed, and freshness badges;
- a founder watchlist with equity and crypto rows;
- equity detail with quote, bars, option summary, and SEC insider activity;
- a crypto board with price, market cap, volume, and trend context;
- connected-portfolio exposure sourced from Schwab, clearly separated from public market data;
- provider health and degraded-data notices; and
- links from insider events to the original SEC filing.

The page must avoid presenting a single-venue or aggregate reference price as a guaranteed executable price. Any future "Ask Arbion" action receives a bounded normalized snapshot through a Go-owned tool and returns sourced analysis, not a trade command.

## Delivery milestones

### 1. Market-data foundation

- Add provider-neutral observation, feed-quality, freshness, and capability models in Go.
- Add deterministic validation and source-selection policy with tests.
- Add provider health and source metadata endpoints backed by disabled-by-default configuration.
- Document configuration without adding real credentials.

Exit gate: malformed and stale data fail closed; source quality cannot be omitted; no order or trading scope exists.

### 2. Equity and option observations

- Add an Alpaca data-only adapter for equity snapshots and bars.
- Add option data only after its indicative-versus-OPRA behavior is represented in the contract and UI.
- Exercise the adapter against provider fixtures and a manually enabled development key.

Exit gate: IEX and indicative data are visibly labeled and cannot satisfy a consolidated-feed policy.

### 3. Crypto reference data

- Add a CoinGecko keyed adapter for global overview, markets, and asset history.
- Add request-credit accounting, cache policy, and identifier mapping by CoinGecko ID and contract address.

Exit gate: no keyless production polling; no secret in a URL, log, browser bundle, or Neural Engine request.

### 4. Primary-source insider intelligence

- Ingest SEC submissions and ownership XML for Forms 3, 4, and 5.
- Preserve issuer/reporting-owner identifiers, transaction codes, direct/indirect ownership, accession number, filing time, and source link.
- Display amendments and derivative transactions without flattening them into misleading buy/sell labels.

Exit gate: all displayed events link to SEC evidence and the importer complies with fair-access policy.

### 5. Arbion command center

- Deliver the branded `/markets` overview, watchlist, equity detail, crypto board, source health, and freshness states.
- Keep broker account truth visually distinct from public/reference data.
- Add responsive, loading, empty, degraded, stale, and provider-outage states with browser tests.

Exit gate: the product remains useful when a provider is unavailable and never hides the resulting loss of coverage or quality.

### 6. Controlled intelligence

- Define read-only Go tools that return bounded, source-stamped snapshots to Arbion Insight.
- Add prompt-injection defenses, output schemas, budgets, audit metadata, and explicit educational labels.

Exit gate: Python has no financial credentials, AI cannot choose an upstream URL, and no analysis result can authorize or submit a trade.

## Explicitly deferred

- Alpaca brokerage OAuth or account connection;
- order preview, submission, replacement, cancellation, or reconciliation;
- automated strategies using a new source before feed-quality safety review;
- social/sentiment scraping;
- automated OpenInsider collection;
- yfinance in production; and
- redistribution or resale of provider data without contractual approval.
