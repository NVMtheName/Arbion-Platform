"use client";

import { motion } from "motion/react";
import { FormEvent, useCallback, useEffect, useState } from "react";

type Provenance = {
  provider: string;
  feed: string;
  quality: string;
  venue?: string;
  provider_timestamp: string;
  received_at: string;
};

type CryptoObservation = {
  symbol: string;
  name: string;
  currency: string;
  current_price: string;
  bid?: string;
  ask?: string;
  provenance: Provenance;
};

export type MarketWatchlistItem = {
  id: string;
  asset_class: "CRYPTO";
  symbol: string;
  quote_currency: "USD";
  created_at: string;
  observation?: CryptoObservation;
};

export type MarketWatchlistData = {
  items: MarketWatchlistItem[];
  market_state: "EMPTY" | "READY" | "PARTIAL" | "UNAVAILABLE";
  message: string;
  unavailable_symbols: string[];
  cached: boolean;
  generated_at?: string;
  max_items: number;
  provider_errors_exposed: false;
  provider_write_available: false;
  order_actions_available: false;
  live_execution_available: false;
};

export const emptyMarketWatchlist: MarketWatchlistData = {
  items: [],
  market_state: "EMPTY",
  message: "Add a crypto asset to begin a durable, read-only venue watchlist.",
  unavailable_symbols: [],
  cached: false,
  max_items: 12,
  provider_errors_exposed: false,
  provider_write_available: false,
  order_actions_available: false,
  live_execution_available: false,
};

type APIError = { error?: { code?: string; message?: string } };

function money(value: string | undefined) {
  if (value === undefined) return "Unavailable";
  const parsed = Number(value);
  if (!Number.isFinite(parsed)) return "Unavailable";
  return new Intl.NumberFormat("en-US", {
    style: "currency",
    currency: "USD",
    maximumFractionDigits: parsed < 1 ? 6 : 2,
  }).format(parsed);
}

function observedAt(value: string) {
  const parsed = new Date(value);
  if (Number.isNaN(parsed.valueOf())) return "time unavailable";
  return new Intl.DateTimeFormat("en-US", {
    hour: "numeric",
    minute: "2-digit",
    second: "2-digit",
    timeZone: "UTC",
    timeZoneName: "short",
  }).format(parsed);
}

function mutationError(body: APIError) {
  switch (body.error?.code) {
    case "WATCHLIST_ITEM_EXISTS":
      return "That asset is already tracked.";
    case "WATCHLIST_LIMIT_REACHED":
      return "The watchlist has reached its 12-asset safety limit.";
    case "INVALID_WATCHLIST_ITEM":
      return "Use a Coinbase crypto symbol such as BTC or ETH.";
    default:
      return "The watchlist could not be changed right now.";
  }
}

export function MarketWatchlist({
  initialData = emptyMarketWatchlist,
}: {
  initialData?: MarketWatchlistData;
}) {
  const [data, setData] = useState(initialData);
  const [symbol, setSymbol] = useState("");
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [deletingID, setDeletingID] = useState("");
  const [error, setError] = useState("");

  const refresh = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const response = await fetch("/api/markets/watchlist", {
        cache: "no-store",
      });
      const body = (await response.json()) as MarketWatchlistData & APIError;
      if (!response.ok || !Array.isArray(body.items)) {
        setError("The saved watchlist is temporarily unavailable.");
        return;
      }
      setData(body);
    } catch {
      setError("The saved watchlist is temporarily unavailable.");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    const refreshTimer = window.setInterval(() => void refresh(), 30_000);
    return () => window.clearInterval(refreshTimer);
  }, [refresh]);

  async function addItem(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const canonical = symbol.trim().toUpperCase();
    if (!/^(?=.*[A-Z])[A-Z0-9]{1,12}$/.test(canonical)) {
      setError("Use a Coinbase crypto symbol such as BTC or ETH.");
      return;
    }
    setSaving(true);
    setError("");
    try {
      const response = await fetch("/api/markets/watchlist", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ symbol: canonical }),
      });
      const body =
        response.status === 204 ? {} : ((await response.json()) as APIError);
      if (!response.ok) {
        setError(mutationError(body));
        return;
      }
      setSymbol("");
      await refresh();
    } catch {
      setError("The watchlist could not be changed right now.");
    } finally {
      setSaving(false);
    }
  }

  async function deleteItem(item: MarketWatchlistItem) {
    setDeletingID(item.id);
    setError("");
    try {
      const response = await fetch(
        `/api/markets/watchlist/${encodeURIComponent(item.id)}`,
        { method: "DELETE" },
      );
      if (!response.ok) {
        const body = (await response.json()) as APIError;
        setError(mutationError(body));
        return;
      }
      await refresh();
    } catch {
      setError("The watchlist could not be changed right now.");
    } finally {
      setDeletingID("");
    }
  }

  return (
    <section className="market-watchlist" aria-labelledby="watchlist-title">
      <header className="market-watchlist-header">
        <div>
          <p className="eyebrow">DURABLE VENUE WATCHLIST</p>
          <h2 id="watchlist-title">Your market radar.</h2>
          <p>
            Save up to {data.max_items} crypto assets. Prices are current,
            keyless Coinbase Exchange observations—not portfolio positions or
            executable quotes.
          </p>
        </div>
        <div className="market-watchlist-actions">
          <span
            className={`market-watchlist-state ${data.market_state.toLowerCase()}`}
          >
            {data.market_state}
          </span>
          <button
            disabled={loading}
            onClick={() => void refresh()}
            type="button"
          >
            {loading ? "Refreshing…" : "Refresh venue"}
          </button>
        </div>
      </header>

      <div className="market-watchlist-rail" aria-label="Watchlist status">
        <article>
          <span>Tracked</span>
          <strong>{data.items.length}</strong>
          <small>{data.max_items} maximum</small>
        </article>
        <article>
          <span>Provider writes</span>
          <strong>None</strong>
          <small>Preference changes stay inside Arbion</small>
        </article>
        <article>
          <span>Pricing scope</span>
          <strong>USD venue</strong>
          <small>Single-venue observations only</small>
        </article>
      </div>

      <form className="market-watchlist-form" onSubmit={addItem}>
        <label htmlFor="watchlist-symbol">Add a crypto asset</label>
        <div>
          <input
            autoComplete="off"
            disabled={saving || data.items.length >= data.max_items}
            id="watchlist-symbol"
            maxLength={12}
            onChange={(event) => setSymbol(event.target.value.toUpperCase())}
            placeholder="BTC"
            value={symbol}
          />
          <button
            disabled={saving || data.items.length >= data.max_items}
            type="submit"
          >
            {saving ? "Adding…" : "Add to radar"}
          </button>
        </div>
      </form>

      <p className="market-watchlist-message">{data.message}</p>
      {error && (
        <p className="market-watchlist-error" role="alert">
          {error}
        </p>
      )}

      {data.items.length === 0 ? (
        <div className="market-watchlist-empty">
          <span aria-hidden="true">◎</span>
          <strong>No saved assets yet</strong>
          <p>Add BTC, ETH, or another supported Coinbase USD asset.</p>
        </div>
      ) : (
        <div className="market-watchlist-grid">
          {data.items.map((item, index) => (
            <motion.article
              initial={{ opacity: 0, y: 12 }}
              animate={{ opacity: 1, y: 0 }}
              transition={{ delay: Math.min(index * 0.035, 0.2) }}
              key={item.id}
            >
              <header>
                <div>
                  <strong>{item.symbol}</strong>
                  <span>{item.observation?.name ?? "Saved asset"}</span>
                </div>
                <button
                  aria-label={`Remove ${item.symbol}`}
                  disabled={deletingID === item.id}
                  onClick={() => void deleteItem(item)}
                  type="button"
                >
                  {deletingID === item.id ? "Removing…" : "Remove"}
                </button>
              </header>
              <div className="market-watchlist-price">
                <span>Last trade</span>
                <strong>{money(item.observation?.current_price)}</strong>
              </div>
              {item.observation ? (
                <>
                  <dl>
                    <div>
                      <dt>Bid</dt>
                      <dd>{money(item.observation.bid)}</dd>
                    </div>
                    <div>
                      <dt>Ask</dt>
                      <dd>{money(item.observation.ask)}</dd>
                    </div>
                  </dl>
                  <footer>
                    <span>
                      {item.observation.provenance.venue ?? "coinbase_exchange"}
                    </span>
                    <span>
                      {item.observation.provenance.quality.replaceAll("_", " ")}
                    </span>
                    <time
                      dateTime={item.observation.provenance.provider_timestamp}
                    >
                      {observedAt(
                        item.observation.provenance.provider_timestamp,
                      )}
                    </time>
                  </footer>
                </>
              ) : (
                <p className="market-watchlist-unavailable">
                  No approved USD observation is available. No estimate was
                  substituted.
                </p>
              )}
            </motion.article>
          ))}
        </div>
      )}

      <footer className="market-watchlist-safety">
        <strong>OBSERVE ONLY</strong>
        <span>
          No order preview, placement, edit, cancellation, transfer, or live
          execution path exists here.
        </span>
      </footer>
    </section>
  );
}
