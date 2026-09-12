import Link from "next/link";

import { AppPageHeader } from "../app-page-header";
import {
  MarketCommandSurface,
  type MarketAccount,
} from "./market-command-surface";
import { MarketHealthTimeline } from "./market-health-timeline";
import type { MarketHealthHistory } from "./market-health-contract";
import { MarketSourceGrid, type MarketSource } from "./market-source-grid";
import { MarketWatchlist, type MarketWatchlistData } from "./market-watchlist";

export function MarketsWorkspace({
  sources,
  statusGeneratedAt,
  accounts,
  history,
  watchlist,
}: {
  sources: MarketSource[];
  statusGeneratedAt?: string;
  accounts: MarketAccount[];
  history?: MarketHealthHistory;
  watchlist: MarketWatchlistData;
}) {
  const available = sources.filter(
    (source) => source.enabled && source.healthy,
  ).length;
  return (
    <main className="markets-page command-content-continuity">
      <AppPageHeader contentHeadingId="markets-page-title" />
      <section className="markets-workspace-intro">
        <p className="eyebrow">MARKET INTELLIGENCE</p>
        <h1 id="markets-page-title">Markets</h1>
        <p className="lede">
          Read-only equities, options, crypto and primary-source filings. Every
          observation keeps its source.
        </p>
      </section>

      <nav className="markets-section-links" aria-label="Market sections">
        <a href="#market-watchlist">Watchlist</a>
        <a href="#market-equities">Equities &amp; options</a>
        <a href="#market-crypto">Crypto</a>
        <a href="#market-filings">Insider filings</a>
        <a href="#market-sources">Sources &amp; verification</a>
      </nav>

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
            {available}/{sources.length}
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
      <MarketCommandSurface accounts={accounts} sources={sources} />

      <section
        className="market-section"
        id="market-sources"
        aria-labelledby="market-sources-title"
      >
        <div className="market-section-intro">
          <p className="eyebrow">SOURCE CONTROL</p>
          <h2 id="market-sources-title">Sources &amp; verification</h2>
          <p>
            Connected Schwab authorization covers account-scoped equities and
            options, Coinbase supplies keyless single-venue crypto snapshots,
            and SEC EDGAR remains the primary insider-filing record.
          </p>
        </div>
        {sources.length > 0 ? (
          <>
            <MarketHealthTimeline sources={sources} initialHistory={history} />
            <MarketSourceGrid
              sources={sources}
              statusGeneratedAt={statusGeneratedAt}
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
