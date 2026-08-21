"use client";

import { motion } from "motion/react";
import { useCallback, useEffect, useMemo, useState } from "react";

type Money = { amount: string; currency: string };
type Provenance = {
  provider: string;
  feed: string;
  quality: string;
  venue?: string;
  provider_timestamp: string;
  received_at: string;
};
type PortfolioPosition = {
  symbol: string;
  quantity: string;
  unit_price?: Money;
  bid?: Money;
  ask?: Money;
  market_value?: Money;
  pricing_status: "PRICED" | "UNAVAILABLE";
  provenance?: Provenance;
};

export type CryptoPortfolioSnapshot = {
  account: {
    id: string;
    display_name: string;
    provider: string;
    status: string;
  };
  balances: {
    cash?: Money;
    available_cash?: Money;
  };
  observed_value?: Money;
  digital_asset_value?: Money;
  positions: PortfolioPosition[];
  priced_positions: number;
  total_positions: number;
  pricing_complete: boolean;
  pricing_state: "READY" | "PARTIAL" | "UNAVAILABLE";
  pricing_basis: "LAST_TRADE";
  pricing_message: string;
  pricing_as_of?: string;
  market_data_cached: boolean;
};

type PortfolioResponse = {
  portfolio?: CryptoPortfolioSnapshot;
  live_execution_available: false;
  error?: { message?: string };
};

function money(value?: Money, compact = false) {
  if (!value) return "—";
  const parsed = Number(value.amount);
  if (!Number.isFinite(parsed)) return value.amount;
  return new Intl.NumberFormat("en-US", {
    style: "currency",
    currency: value.currency,
    notation: compact ? "compact" : "standard",
    maximumFractionDigits: compact ? 2 : Math.abs(parsed) < 1 ? 6 : 2,
  }).format(parsed);
}

function quantity(value: string) {
  const parsed = Number(value);
  if (!Number.isFinite(parsed)) return value;
  return new Intl.NumberFormat("en-US", {
    maximumFractionDigits: 8,
  }).format(parsed);
}

function timestamp(value?: string) {
  if (!value) return "No market observation";
  const parsed = new Date(value);
  if (Number.isNaN(parsed.valueOf())) return value;
  return new Intl.DateTimeFormat("en-US", {
    month: "short",
    day: "numeric",
    hour: "numeric",
    minute: "2-digit",
    second: "2-digit",
    timeZoneName: "short",
  }).format(parsed);
}

function observedNumber(value?: Money) {
  const parsed = Number(value?.amount ?? "0");
  return Number.isFinite(parsed) && parsed > 0 ? parsed : 0;
}

export function CryptoPortfolioCommandCenter({
  accountID,
  initialSnapshot,
}: {
  accountID: string;
  initialSnapshot: CryptoPortfolioSnapshot;
}) {
  const [snapshot, setSnapshot] = useState(initialSnapshot);
  const [refreshing, setRefreshing] = useState(false);
  const [refreshError, setRefreshError] = useState("");

  const refresh = useCallback(async () => {
    setRefreshing(true);
    setRefreshError("");
    try {
      const response = await fetch(
        `/api/accounts/${encodeURIComponent(accountID)}/portfolio/crypto`,
        { cache: "no-store" },
      );
      const body = (await response.json()) as PortfolioResponse;
      if (!response.ok || !body.portfolio) {
        setRefreshError(
          body.error?.message ??
            "Portfolio refresh is temporarily unavailable. Existing observations remain labeled by time.",
        );
        return;
      }
      setSnapshot(body.portfolio);
    } catch {
      setRefreshError(
        "Portfolio refresh is temporarily unavailable. Existing observations remain labeled by time.",
      );
    } finally {
      setRefreshing(false);
    }
  }, [accountID]);

  useEffect(() => {
    const interval = window.setInterval(() => void refresh(), 30_000);
    return () => window.clearInterval(interval);
  }, [refresh]);

  const pricedPositions = useMemo(
    () => snapshot.positions.filter((position) => position.market_value),
    [snapshot.positions],
  );
  const digitalAssetValue = observedNumber(snapshot.digital_asset_value);

  return (
    <section className="crypto-command" aria-labelledby="crypto-command-title">
      <motion.header
        className="crypto-command-hero"
        initial={{ opacity: 0, y: 16 }}
        animate={{ opacity: 1, y: 0 }}
      >
        <div>
          <p className="eyebrow">ARBION PORTFOLIO INTELLIGENCE</p>
          <h1 id="crypto-command-title">{snapshot.account.display_name}</h1>
          <p>
            Coinbase holdings meet venue-stamped market observations in one
            read-only portfolio view.
          </p>
        </div>
        <div className="crypto-command-actions">
          <span
            className={`crypto-pricing-state ${snapshot.pricing_state.toLowerCase()}`}
          >
            <i /> {snapshot.pricing_state}
          </span>
          <button
            disabled={refreshing}
            onClick={() => void refresh()}
            type="button"
          >
            {refreshing ? "Refreshing…" : "Refresh portfolio"}
          </button>
          <small>Auto-refreshes every 30 seconds</small>
        </div>
      </motion.header>

      {refreshError && (
        <p className="crypto-command-error" role="alert">
          {refreshError}
        </p>
      )}

      <section className="crypto-value-rail" aria-label="Portfolio summary">
        <motion.article initial={{ opacity: 0 }} animate={{ opacity: 1 }}>
          <span>Observed portfolio</span>
          <strong>{money(snapshot.observed_value)}</strong>
          <small>USD cash + priced assets</small>
        </motion.article>
        <motion.article
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          transition={{ delay: 0.04 }}
        >
          <span>Digital assets</span>
          <strong>{money(snapshot.digital_asset_value)}</strong>
          <small>Coinbase Exchange last trade</small>
        </motion.article>
        <motion.article
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          transition={{ delay: 0.08 }}
        >
          <span>Cash reserve</span>
          <strong>{money(snapshot.balances.cash)}</strong>
          <small>{money(snapshot.balances.available_cash)} available</small>
        </motion.article>
        <motion.article
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          transition={{ delay: 0.12 }}
        >
          <span>Pricing coverage</span>
          <strong>
            {snapshot.priced_positions}/{snapshot.total_positions}
          </strong>
          <small>
            {snapshot.pricing_complete ? "Complete" : "Partial—no estimates"}
          </small>
        </motion.article>
      </section>

      <section className="crypto-command-grid">
        <motion.article
          className="crypto-allocation-panel"
          initial={{ opacity: 0, y: 18 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ delay: 0.12 }}
        >
          <header>
            <div>
              <p className="eyebrow">PRICED ALLOCATION</p>
              <h2>Concentration map</h2>
            </div>
            <span>{pricedPositions.length} observed</span>
          </header>
          {pricedPositions.length === 0 ? (
            <p className="crypto-empty">
              No approved USD market observations are available yet.
            </p>
          ) : (
            <div className="crypto-allocation-list">
              {pricedPositions.map((position, index) => {
                const value = observedNumber(position.market_value);
                const percent =
                  digitalAssetValue > 0 ? (value / digitalAssetValue) * 100 : 0;
                return (
                  <motion.div
                    key={position.symbol}
                    initial={{ opacity: 0, x: -10 }}
                    animate={{ opacity: 1, x: 0 }}
                    transition={{ delay: 0.16 + index * 0.035 }}
                  >
                    <div>
                      <strong>{position.symbol}</strong>
                      <span>{percent.toFixed(percent < 1 ? 2 : 1)}%</span>
                      <small>{money(position.market_value, true)}</small>
                    </div>
                    <span className="crypto-allocation-track">
                      <i
                        style={{
                          width: `${Math.min(100, Math.max(0, percent))}%`,
                        }}
                      />
                    </span>
                  </motion.div>
                );
              })}
            </div>
          )}
          <footer>
            Allocation uses only positions with an approved USD observation.
            Unpriced assets are excluded and remain visible below.
          </footer>
        </motion.article>

        <motion.article
          className="crypto-evidence-panel"
          initial={{ opacity: 0, y: 18 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ delay: 0.18 }}
        >
          <p className="eyebrow">EVIDENCE CONTROL</p>
          <h2>What this value means</h2>
          <dl>
            <div>
              <dt>Pricing basis</dt>
              <dd>Last trade</dd>
            </div>
            <div>
              <dt>Venue</dt>
              <dd>Coinbase Exchange</dd>
            </div>
            <div>
              <dt>Oldest observation</dt>
              <dd>{timestamp(snapshot.pricing_as_of)}</dd>
            </div>
            <div>
              <dt>Delivery</dt>
              <dd>
                {snapshot.market_data_cached
                  ? "Bounded display cache"
                  : "Provider response"}
              </dd>
            </div>
            <div>
              <dt>Execution path</dt>
              <dd>None</dd>
            </div>
          </dl>
          <p>{snapshot.pricing_message}</p>
        </motion.article>
      </section>

      <section
        className="crypto-position-ledger"
        aria-labelledby="crypto-position-title"
      >
        <header>
          <div>
            <p className="eyebrow">CONNECTED HOLDINGS</p>
            <h2 id="crypto-position-title">Position ledger</h2>
          </div>
          <span>{snapshot.total_positions} assets</span>
        </header>
        {snapshot.positions.length === 0 ? (
          <p className="crypto-empty">
            Coinbase reported no non-zero digital-asset positions.
          </p>
        ) : (
          <div className="crypto-position-table" role="region" tabIndex={0}>
            <table>
              <thead>
                <tr>
                  <th>Asset</th>
                  <th>Quantity</th>
                  <th>Last trade</th>
                  <th>Observed value</th>
                  <th>Bid / Ask</th>
                  <th>Evidence</th>
                </tr>
              </thead>
              <tbody>
                {snapshot.positions.map((position) => (
                  <tr key={position.symbol}>
                    <td>
                      <strong>{position.symbol}</strong>
                      <small>{position.pricing_status}</small>
                    </td>
                    <td>{quantity(position.quantity)}</td>
                    <td>{money(position.unit_price)}</td>
                    <td>{money(position.market_value)}</td>
                    <td>
                      {position.bid || position.ask
                        ? `${money(position.bid)} / ${money(position.ask)}`
                        : "—"}
                    </td>
                    <td>
                      {position.provenance ? (
                        <>
                          <strong>
                            {position.provenance.venue?.replaceAll("_", " ")}
                          </strong>
                          <small>
                            {position.provenance.quality
                              .replaceAll("_", " ")
                              .toLowerCase()}{" "}
                            ·{" "}
                            {timestamp(position.provenance.provider_timestamp)}
                          </small>
                        </>
                      ) : (
                        <small>No approved USD product</small>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>

      <p className="crypto-safety-band">
        <strong>READ-ONLY BY DESIGN</strong>
        <span>
          Arbion can view balances and market evidence here. It cannot place
          orders, convert assets, deposit, withdraw, or transfer funds.
        </span>
      </p>
    </section>
  );
}
