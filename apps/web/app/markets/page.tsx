import { cookies } from "next/headers";
import Link from "next/link";
import { redirect } from "next/navigation";

import { AppPageHeader } from "../app-page-header";
import {
  MarketCommandSurface,
  type MarketAccount,
} from "./market-command-surface";
import { MarketHealthTimeline } from "./market-health-timeline";
import {
  safeMarketHealthHistory,
  type MarketHealthHistory,
} from "./market-health-contract";
import { MarketSourceGrid, type MarketSource } from "./market-source-grid";
import {
  emptyMarketWatchlist,
  MarketWatchlist,
  type MarketWatchlistData,
} from "./market-watchlist";

type SourcesResponse = {
  sources: MarketSource[];
  status_generated_at?: string;
  status_semantics?: "PROCESS_LOCAL_TIME_BOUNDED_PROVIDER_VERIFICATION";
  provider_errors_exposed?: false;
  live_execution_available: false;
};

export default async function MarketsPage() {
  const jar = await cookies();
  const api = process.env.API_BASE_URL ?? "http://localhost:8080";
  const [response, accountsResponse, historyResponse, watchlistResponse] =
    await Promise.all([
      fetch(`${api}/api/markets/sources`, {
        headers: { cookie: jar.toString() },
        cache: "no-store",
      }),
      fetch(`${api}/api/accounts`, {
        headers: { cookie: jar.toString() },
        cache: "no-store",
      }),
      fetch(`${api}/api/markets/source-history`, {
        headers: { cookie: jar.toString() },
        cache: "no-store",
      }).catch(() => undefined),
      fetch(`${api}/api/markets/watchlist`, {
        headers: { cookie: jar.toString() },
        cache: "no-store",
      }).catch(() => undefined),
    ]);
  if (response.status === 401) redirect("/login");
  const data: SourcesResponse = response.ok
    ? ((await response.json()) as SourcesResponse)
    : {
        sources: [],
        provider_errors_exposed: false,
        live_execution_available: false,
      };
  const accounts = accountsResponse.ok
    ? ((await accountsResponse.json()) as { accounts: MarketAccount[] })
        .accounts
    : [];
  let history: MarketHealthHistory | undefined;
  if (historyResponse?.ok) {
    try {
      history = safeMarketHealthHistory(await historyResponse.json());
    } catch {
      history = undefined;
    }
  }
  let watchlist: MarketWatchlistData = emptyMarketWatchlist;
  if (watchlistResponse?.ok) {
    try {
      const candidate = (await watchlistResponse.json()) as MarketWatchlistData;
      if (
        Array.isArray(candidate.items) &&
        candidate.items.length <= emptyMarketWatchlist.max_items &&
        candidate.max_items === emptyMarketWatchlist.max_items &&
        candidate.provider_write_available === false &&
        candidate.order_actions_available === false &&
        candidate.live_execution_available === false
      ) {
        watchlist = candidate;
      }
    } catch {
      watchlist = emptyMarketWatchlist;
    }
  }
  const available = data.sources.filter(
    (source) => source.enabled && source.healthy,
  ).length;

  return (
    <main className="markets-page command-content-continuity">
      <AppPageHeader contentHeadingId="markets-page-title" />
      <p className="eyebrow">MARKET INTELLIGENCE</p>
      <h1 id="markets-page-title">See the market. Know the source.</h1>
      <p className="lede">
        Arbion is building one read-only command center for equities, options,
        crypto, connected portfolio context, and primary-source insider filings.
      </p>

      <section className="market-safety" aria-label="Command center safeguards">
        <strong>READ-ONLY · LIVE EXECUTION UNAVAILABLE</strong>
        <span>
          No source is allowed to hide whether its data is consolidated,
          single-venue, indicative, delayed, or reference-only.
        </span>
      </section>

      <section className="market-summary" aria-label="Command center status">
        <article>
          <span>Sources available</span>
          <strong>
            {available}/{data.sources.length}
          </strong>
          <small>Every available source is verified before display.</small>
        </article>
        <article>
          <span>Execution path</span>
          <strong>None</strong>
          <small>No order or trading scope exists in this milestone.</small>
        </article>
        <article>
          <span>Evidence policy</span>
          <strong>Source-stamped</strong>
          <small>
            Feed, quality, venue, and freshness travel with the data.
          </small>
        </article>
      </section>

      <MarketWatchlist initialData={watchlist} />

      <MarketCommandSurface accounts={accounts} sources={data.sources} />

      <section className="market-section">
        <div>
          <p className="eyebrow">SOURCE CONTROL</p>
          <h2>Production-approved boundaries</h2>
          <p>
            Connected Schwab authorization covers account-scoped equities and
            options, Coinbase supplies keyless single-venue crypto snapshots,
            and SEC EDGAR remains the primary insider-filing record.
          </p>
        </div>
        {data.sources.length > 0 ? (
          <>
            <MarketHealthTimeline
              sources={data.sources}
              initialHistory={history}
            />
            <MarketSourceGrid
              sources={data.sources}
              statusGeneratedAt={data.status_generated_at}
            />
          </>
        ) : (
          <p className="unavailable">
            Source metadata is temporarily unavailable. No market value will be
            guessed or synthesized.
          </p>
        )}
      </section>

      <section className="market-policy-grid">
        <article>
          <span className="icon">◎</span>
          <h2>Portfolio truth stays separate</h2>
          <p>
            Connected Schwab accounts remain the authority for balances,
            positions, and their delegated market-data entitlement. Coinbase
            never impersonates a brokerage account.
          </p>
          <Link href="/accounts">Review connected accounts</Link>
        </article>
        <article>
          <span className="icon">⌁</span>
          <h2>Research has boundaries</h2>
          <p>
            yfinance stays a local research aid. OpenInsider may be linked for
            human research, while SEC filings remain Arbion&apos;s evidence.
          </p>
        </article>
      </section>
    </main>
  );
}
