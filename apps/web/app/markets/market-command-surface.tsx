"use client";

import { motion } from "motion/react";
import Link from "next/link";
import { FormEvent, useState } from "react";

import type { MarketSource } from "./market-source-grid";

type Provenance = {
  provider: string;
  provider_request_id?: string;
  role: string;
  feed: string;
  quality: string;
  venue?: string;
  provider_timestamp: string;
  received_at: string;
};

type QuoteObservation = {
  symbol: string;
  currency: string;
  bid?: string;
  ask?: string;
  mark?: string;
  last?: string;
  provenance: Provenance;
};

type CryptoMarketObservation = {
  id: string;
  symbol: string;
  name: string;
  currency: string;
  current_price: string;
  market_cap?: string;
  market_cap_rank?: number;
  volume_24h?: string;
  change_percent_24h?: string;
  provenance: Provenance;
};

type InsiderFilingObservation = {
  issuer_cik: string;
  accession_number: string;
  form: string;
  is_amendment: boolean;
  filed_at: string;
  report_date?: string;
  source_url: string;
  provenance: Provenance;
};

type APIError = { error?: { code?: string; message?: string } };

function readable(value: string) {
  return value.replaceAll("_", " ").toLowerCase();
}

function timestamp(value: string) {
  const parsed = new Date(value);
  if (Number.isNaN(parsed.valueOf())) return value;
  return new Intl.DateTimeFormat("en-US", {
    dateStyle: "medium",
    timeStyle: "short",
    timeZone: "UTC",
  }).format(parsed);
}

function money(value: string | undefined, currency = "USD") {
  if (value === undefined) return "—";
  const parsed = Number(value);
  if (!Number.isFinite(parsed)) return value;
  return new Intl.NumberFormat("en-US", {
    style: "currency",
    currency,
    maximumFractionDigits: parsed < 1 ? 6 : 2,
  }).format(parsed);
}

function compact(value: string | undefined, currency = "USD") {
  if (value === undefined) return "—";
  const parsed = Number(value);
  if (!Number.isFinite(parsed)) return value;
  return new Intl.NumberFormat("en-US", {
    style: "currency",
    currency,
    notation: "compact",
    maximumFractionDigits: 1,
  }).format(parsed);
}

function providerError(body: APIError) {
  if (body.error?.code === "MARKET_SOURCE_UNAVAILABLE") {
    return "This source is not configured yet.";
  }
  if (body.error?.code === "INVALID_MARKET_QUERY") {
    return "Check the identifier and try again.";
  }
  return "The provider is temporarily unavailable. No value was substituted.";
}

function ProvenanceLine({ provenance }: { provenance: Provenance }) {
  return (
    <footer className="live-provenance">
      <span>{provenance.provider}</span>
      <span>{readable(provenance.feed)}</span>
      {provenance.venue && <span>{readable(provenance.venue)}</span>}
      <span>{readable(provenance.quality)}</span>
      <time dateTime={provenance.provider_timestamp}>
        {timestamp(provenance.provider_timestamp)} UTC
      </time>
    </footer>
  );
}

export function MarketCommandSurface({ sources }: { sources: MarketSource[] }) {
  const equityEnabled = sources.some(
    (source) => source.enabled && source.capabilities.includes("EQUITY_QUOTE"),
  );
  const cryptoEnabled = sources.some(
    (source) =>
      source.enabled && source.capabilities.includes("CRYPTO_MARKETS"),
  );
  const filingsEnabled = sources.some(
    (source) =>
      source.enabled && source.capabilities.includes("INSIDER_FILING"),
  );

  const [symbol, setSymbol] = useState("SPY");
  const [quote, setQuote] = useState<QuoteObservation | null>(null);
  const [quoteError, setQuoteError] = useState("");
  const [quoteLoading, setQuoteLoading] = useState(false);
  const [crypto, setCrypto] = useState<CryptoMarketObservation[]>([]);
  const [cryptoError, setCryptoError] = useState("");
  const [cryptoLoading, setCryptoLoading] = useState(false);
  const [cik, setCIK] = useState("0000320193");
  const [filings, setFilings] = useState<InsiderFilingObservation[]>([]);
  const [filingError, setFilingError] = useState("");
  const [filingLoading, setFilingLoading] = useState(false);

  async function loadQuote(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const normalized = symbol.trim().toUpperCase();
    if (!normalized || !equityEnabled) return;
    setQuoteLoading(true);
    setQuoteError("");
    setQuote(null);
    try {
      const response = await fetch(
        `/api/markets/equities/${encodeURIComponent(normalized)}/quote`,
        { cache: "no-store" },
      );
      const body = (await response.json()) as APIError & {
        quote?: QuoteObservation;
      };
      if (!response.ok || !body.quote) {
        setQuoteError(providerError(body));
        return;
      }
      setQuote(body.quote);
    } catch {
      setQuoteError("The provider is temporarily unavailable.");
    } finally {
      setQuoteLoading(false);
    }
  }

  async function loadCrypto() {
    if (!cryptoEnabled) return;
    setCryptoLoading(true);
    setCryptoError("");
    setCrypto([]);
    try {
      const response = await fetch("/api/markets/crypto?currency=usd&limit=8", {
        cache: "no-store",
      });
      const body = (await response.json()) as APIError & {
        markets?: CryptoMarketObservation[];
      };
      if (!response.ok || !body.markets) {
        setCryptoError(providerError(body));
        return;
      }
      setCrypto(body.markets);
    } catch {
      setCryptoError("The provider is temporarily unavailable.");
    } finally {
      setCryptoLoading(false);
    }
  }

  async function loadFilings(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const normalized = cik.trim();
    if (!normalized || !filingsEnabled) return;
    setFilingLoading(true);
    setFilingError("");
    setFilings([]);
    try {
      const response = await fetch(
        `/api/markets/insiders/${encodeURIComponent(normalized)}?limit=10`,
        { cache: "no-store" },
      );
      const body = (await response.json()) as APIError & {
        filings?: InsiderFilingObservation[];
      };
      if (!response.ok || !body.filings) {
        setFilingError(providerError(body));
        return;
      }
      setFilings(body.filings);
    } catch {
      setFilingError("SEC EDGAR is temporarily unavailable.");
    } finally {
      setFilingLoading(false);
    }
  }

  return (
    <section
      className="live-market-surface"
      aria-labelledby="live-markets-title"
    >
      <header>
        <div>
          <p className="eyebrow">LIVE READ-ONLY SURFACE</p>
          <h2 id="live-markets-title">Pull the signal. Keep the source.</h2>
        </div>
        <p>
          Values load only from configured providers. Every observation carries
          feed quality, venue, and provider time; failures stay failures.
        </p>
      </header>

      <div className="live-market-grid">
        <motion.article
          className="live-market-panel equity"
          initial={{ opacity: 0, y: 18 }}
          whileInView={{ opacity: 1, y: 0 }}
          viewport={{ once: true, amount: 0.2 }}
        >
          <div className="live-panel-heading">
            <span>01 · EQUITIES</span>
            <strong>{equityEnabled ? "ALPACA" : "STANDBY"}</strong>
          </div>
          <h3>Latest quote</h3>
          <p>
            Independent bid/ask context—not a broker balance or order price.
          </p>
          <form className="market-query" onSubmit={loadQuote}>
            <label htmlFor="equity-symbol">Ticker symbol</label>
            <div>
              <input
                id="equity-symbol"
                maxLength={15}
                onChange={(event) => setSymbol(event.target.value)}
                pattern="[A-Za-z][A-Za-z0-9.\-]{0,14}"
                value={symbol}
                disabled={!equityEnabled}
              />
              <button disabled={!equityEnabled || quoteLoading} type="submit">
                {quoteLoading ? "Loading…" : "Load quote"}
              </button>
            </div>
          </form>
          {!equityEnabled && (
            <p className="live-source-note">
              Add Alpaca data credentials to activate.
            </p>
          )}
          {quoteError && (
            <p className="live-market-error" role="alert">
              {quoteError}
            </p>
          )}
          {quote && (
            <motion.div
              className="live-quote"
              initial={{ opacity: 0 }}
              animate={{ opacity: 1 }}
            >
              <div>
                <span>{quote.symbol}</span>
                <strong>
                  {money(quote.mark ?? quote.last ?? quote.bid, quote.currency)}
                </strong>
              </div>
              <dl>
                <div>
                  <dt>Bid</dt>
                  <dd>{money(quote.bid, quote.currency)}</dd>
                </div>
                <div>
                  <dt>Ask</dt>
                  <dd>{money(quote.ask, quote.currency)}</dd>
                </div>
              </dl>
              <ProvenanceLine provenance={quote.provenance} />
            </motion.div>
          )}
        </motion.article>

        <motion.article
          className="live-market-panel crypto"
          initial={{ opacity: 0, y: 18 }}
          whileInView={{ opacity: 1, y: 0 }}
          viewport={{ once: true, amount: 0.2 }}
          transition={{ delay: 0.06 }}
        >
          <div className="live-panel-heading">
            <span>02 · CRYPTO</span>
            <strong>{cryptoEnabled ? "COINGECKO" : "STANDBY"}</strong>
          </div>
          <h3>Market breadth</h3>
          <p>Aggregated reference context, never an executable venue quote.</p>
          <button
            className="market-load-button"
            disabled={!cryptoEnabled || cryptoLoading}
            onClick={loadCrypto}
            type="button"
          >
            {cryptoLoading ? "Loading…" : "Load crypto market"}
          </button>
          {!cryptoEnabled && (
            <p className="live-source-note">
              Add a CoinGecko keyed plan to activate.
            </p>
          )}
          {cryptoError && (
            <p className="live-market-error" role="alert">
              {cryptoError}
            </p>
          )}
          {crypto.length > 0 && (
            <div className="crypto-market-list">
              {crypto.map((asset) => (
                <article key={asset.id}>
                  <div>
                    <strong>{asset.symbol}</strong>
                    <span>{asset.name}</span>
                  </div>
                  <div>
                    <strong>
                      {money(asset.current_price, asset.currency)}
                    </strong>
                    <span
                      className={
                        Number(asset.change_percent_24h ?? 0) < 0
                          ? "negative"
                          : "positive"
                      }
                    >
                      {asset.change_percent_24h ?? "—"}%
                    </span>
                  </div>
                  <small>
                    {compact(asset.market_cap, asset.currency)} market cap
                  </small>
                </article>
              ))}
              <ProvenanceLine provenance={crypto[0].provenance} />
            </div>
          )}
        </motion.article>

        <motion.article
          className="live-market-panel filings"
          initial={{ opacity: 0, y: 18 }}
          whileInView={{ opacity: 1, y: 0 }}
          viewport={{ once: true, amount: 0.2 }}
          transition={{ delay: 0.12 }}
        >
          <div className="live-panel-heading">
            <span>03 · INSIDER FILINGS</span>
            <strong>{filingsEnabled ? "SEC EDGAR" : "STANDBY"}</strong>
          </div>
          <h3>Primary-source evidence</h3>
          <p>Forms 3, 4, and 5 from EDGAR remain the authoritative record.</p>
          <form className="market-query" onSubmit={loadFilings}>
            <label htmlFor="issuer-cik">Issuer CIK</label>
            <div>
              <input
                id="issuer-cik"
                inputMode="numeric"
                maxLength={10}
                onChange={(event) => setCIK(event.target.value)}
                pattern="[0-9]{1,10}"
                value={cik}
                disabled={!filingsEnabled}
              />
              <button disabled={!filingsEnabled || filingLoading} type="submit">
                {filingLoading ? "Loading…" : "Load filings"}
              </button>
            </div>
          </form>
          {!filingsEnabled && (
            <p className="live-source-note">
              Set the SEC contact identity to activate.
            </p>
          )}
          {filingError && (
            <p className="live-market-error" role="alert">
              {filingError}
            </p>
          )}
          {filings.length > 0 && (
            <div className="filing-list">
              {filings.map((filing) => (
                <article key={filing.accession_number}>
                  <span>FORM {filing.form}</span>
                  <div>
                    <strong>{filing.accession_number}</strong>
                    <time dateTime={filing.filed_at}>
                      {timestamp(filing.filed_at)} UTC
                    </time>
                  </div>
                  <Link
                    href={filing.source_url}
                    rel="noreferrer"
                    target="_blank"
                  >
                    Open SEC filing ↗
                  </Link>
                </article>
              ))}
              <ProvenanceLine provenance={filings[0].provenance} />
            </div>
          )}
          <Link
            className="openinsider-research-link"
            href="https://www.openinsider.com/"
            rel="noreferrer"
            target="_blank"
          >
            Optional human research: OpenInsider ↗
          </Link>
        </motion.article>
      </div>
    </section>
  );
}
