"use client";

import { motion } from "motion/react";
import Link from "next/link";
import { FormEvent, useCallback, useEffect, useState } from "react";

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
  bid?: string;
  ask?: string;
  market_cap?: string;
  market_cap_rank?: number;
  volume_24h?: string;
  volume_24h_unit?: string;
  change_percent_24h?: string;
  provenance: Provenance;
};

export type MarketAccount = {
  id: string;
  provider: string;
  display_name: string;
  base_currency: string;
  status: string;
};

type OptionContractObservation = {
  symbol: string;
  underlying: string;
  put_call: "PUT" | "CALL";
  expiration: string;
  strike: string;
  bid?: string;
  ask?: string;
  mark?: string;
  delta?: string;
  implied_volatility?: string;
  open_interest?: number;
  volume?: number;
};

type OptionChainObservation = {
  symbol: string;
  underlying_price?: string;
  contracts: OptionContractObservation[];
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

function compactNumber(value: string | undefined) {
  if (value === undefined) return "—";
  const parsed = Number(value);
  if (!Number.isFinite(parsed)) return value;
  return new Intl.NumberFormat("en-US", {
    notation: "compact",
    maximumFractionDigits: 2,
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

export function MarketCommandSurface({
  sources,
  accounts = [],
}: {
  sources: MarketSource[];
  accounts?: MarketAccount[];
}) {
  const independentEquityEnabled = sources.some(
    (source) =>
      source.id !== "schwab_broker_market_data" &&
      source.enabled &&
      source.capabilities.includes("EQUITY_QUOTE"),
  );
  const schwabSourceEnabled = sources.some(
    (source) => source.id === "schwab_broker_market_data" && source.enabled,
  );
  const schwabAccounts = accounts.filter(
    (account) => account.provider === "schwab" && account.status === "active",
  );
  const brokerEquityEnabled = schwabSourceEnabled && schwabAccounts.length > 0;
  const equityEnabled = brokerEquityEnabled || independentEquityEnabled;
  const cryptoEnabled = sources.some(
    (source) =>
      source.enabled && source.capabilities.includes("CRYPTO_MARKETS"),
  );
  const filingsEnabled = sources.some(
    (source) =>
      source.enabled && source.capabilities.includes("INSIDER_FILING"),
  );

  const [symbol, setSymbol] = useState("SPY");
  const [accountID, setAccountID] = useState(schwabAccounts[0]?.id ?? "");
  const [quote, setQuote] = useState<QuoteObservation | null>(null);
  const [quoteError, setQuoteError] = useState("");
  const [quoteLoading, setQuoteLoading] = useState(false);
  const [contractType, setContractType] = useState<"PUT" | "CALL">("PUT");
  const [optionChain, setOptionChain] = useState<OptionChainObservation | null>(
    null,
  );
  const [optionError, setOptionError] = useState("");
  const [optionLoading, setOptionLoading] = useState(false);
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
      const endpoint = brokerEquityEnabled
        ? `/api/accounts/${encodeURIComponent(accountID)}/markets/equities/${encodeURIComponent(normalized)}/quote`
        : `/api/markets/equities/${encodeURIComponent(normalized)}/quote`;
      const response = await fetch(endpoint, { cache: "no-store" });
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

  async function loadOptions() {
    const normalized = symbol.trim().toUpperCase();
    if (!normalized || !brokerEquityEnabled || !accountID) return;
    setOptionLoading(true);
    setOptionError("");
    setOptionChain(null);
    try {
      const query = new URLSearchParams({
        symbol: normalized,
        contract_type: contractType,
        strike_count: "12",
      });
      const response = await fetch(
        `/api/accounts/${encodeURIComponent(accountID)}/markets/options?${query.toString()}`,
        { cache: "no-store" },
      );
      const body = (await response.json()) as APIError & {
        chain?: OptionChainObservation;
      };
      if (!response.ok || !body.chain) {
        setOptionError(providerError(body));
        return;
      }
      setOptionChain(body.chain);
    } catch {
      setOptionError("Schwab option data is temporarily unavailable.");
    } finally {
      setOptionLoading(false);
    }
  }

  const loadCrypto = useCallback(async () => {
    if (!cryptoEnabled) return;
    setCryptoLoading(true);
    setCryptoError("");
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
  }, [cryptoEnabled]);

  useEffect(() => {
    if (!cryptoEnabled) return;
    let cancelled = false;
    let refresh: number | undefined;
    const run = async () => {
      await loadCrypto();
      if (!cancelled) refresh = window.setTimeout(() => void run(), 5000);
    };
    const initial = window.setTimeout(() => void run(), 0);
    return () => {
      cancelled = true;
      window.clearTimeout(initial);
      if (refresh !== undefined) window.clearTimeout(refresh);
    };
  }, [cryptoEnabled, loadCrypto]);

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
            <strong>
              {brokerEquityEnabled
                ? "SCHWAB"
                : independentEquityEnabled
                  ? "ALPACA"
                  : "STANDBY"}
            </strong>
          </div>
          <h3>Equity and options desk</h3>
          <p>
            Broker-entitled observations stay read-only and account-scoped. Feed
            quality is taken from Schwab&apos;s response.
          </p>
          {brokerEquityEnabled && (
            <label className="broker-account-select" htmlFor="market-account">
              <span>Market-data authorization</span>
              <select
                id="market-account"
                onChange={(event) => setAccountID(event.target.value)}
                value={accountID}
              >
                {schwabAccounts.map((account) => (
                  <option key={account.id} value={account.id}>
                    {account.display_name}
                  </option>
                ))}
              </select>
            </label>
          )}
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
              Connect a Schwab account to activate equity and option data.
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
          {brokerEquityEnabled && (
            <section className="option-chain-desk" aria-label="Option chain">
              <header>
                <div>
                  <span>OPTION CHAIN</span>
                  <strong>{symbol.trim().toUpperCase() || "SPY"}</strong>
                </div>
                <div className="option-chain-controls">
                  <label htmlFor="contract-type">Contract</label>
                  <select
                    id="contract-type"
                    onChange={(event) =>
                      setContractType(event.target.value as "PUT" | "CALL")
                    }
                    value={contractType}
                  >
                    <option value="PUT">Puts</option>
                    <option value="CALL">Calls</option>
                  </select>
                  <button
                    disabled={optionLoading}
                    onClick={loadOptions}
                    type="button"
                  >
                    {optionLoading ? "Loading…" : "Load chain"}
                  </button>
                </div>
              </header>
              {optionError && (
                <p className="live-market-error" role="alert">
                  {optionError}
                </p>
              )}
              {optionChain && (
                <div className="option-chain-result">
                  <div className="option-chain-summary">
                    <span>Underlying</span>
                    <strong>
                      {money(optionChain.underlying_price, "USD")}
                    </strong>
                    <small>{optionChain.contracts.length} contracts</small>
                  </div>
                  <p
                    className="command-data-scroll-hint"
                    id="option-chain-scroll-hint"
                  >
                    Swipe or scroll horizontally to review every contract field.
                  </p>
                  <div
                    aria-describedby="option-chain-scroll-hint"
                    aria-label="Option chain contracts"
                    className="option-chain-table command-data-scroll"
                    role="region"
                    tabIndex={0}
                  >
                    <table>
                      <thead>
                        <tr>
                          <th>Expiry</th>
                          <th>Strike</th>
                          <th>Bid</th>
                          <th>Ask</th>
                          <th>Delta</th>
                          <th>OI</th>
                        </tr>
                      </thead>
                      <tbody>
                        {optionChain.contracts.map((contract) => (
                          <tr key={contract.symbol}>
                            <td>{contract.expiration}</td>
                            <td>{money(contract.strike, "USD")}</td>
                            <td>{money(contract.bid, "USD")}</td>
                            <td>{money(contract.ask, "USD")}</td>
                            <td>{contract.delta ?? "—"}</td>
                            <td>{contract.open_interest ?? "—"}</td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                  <ProvenanceLine provenance={optionChain.provenance} />
                </div>
              )}
            </section>
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
            <strong>{cryptoEnabled ? "COINBASE LIVE" : "STANDBY"}</strong>
          </div>
          <h3>Live venue board</h3>
          <p>
            Keyless Coinbase Exchange snapshots refresh every five seconds. They
            describe one venue and never represent an executable Arbion order.
          </p>
          <button
            className="market-load-button"
            disabled={!cryptoEnabled || cryptoLoading}
            onClick={loadCrypto}
            type="button"
          >
            {cryptoLoading ? "Refreshing…" : "Refresh now"}
          </button>
          {!cryptoEnabled && (
            <p className="live-source-note">
              Coinbase public market data is temporarily unavailable.
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
                    {asset.change_percent_24h && (
                      <span
                        className={
                          Number(asset.change_percent_24h) < 0
                            ? "negative"
                            : "positive"
                        }
                      >
                        {asset.change_percent_24h}%
                      </span>
                    )}
                  </div>
                  <small>
                    {asset.market_cap
                      ? `${compact(asset.market_cap, asset.currency)} market cap`
                      : `${compactNumber(asset.volume_24h)} ${asset.volume_24h_unit ?? asset.symbol} 24h volume · ${money(asset.bid, asset.currency)} bid · ${money(asset.ask, asset.currency)} ask`}
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
