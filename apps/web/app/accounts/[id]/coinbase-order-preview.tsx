"use client";

import { motion } from "motion/react";
import { type FormEvent, useMemo, useState } from "react";

type Money = { amount: string; currency: string };

type Preview = {
  provider: "coinbase";
  feed: "advanced_trade_order_preview";
  product_id: string;
  base_asset: string;
  quote_currency: "USD";
  side: "BUY" | "SELL";
  order_type: "MARKET_IOC";
  requested_size: Money;
  base_size: string;
  quote_size: string;
  order_total: Money;
  commission_total: Money;
  best_bid?: Money;
  best_ask?: Money;
  estimated_average_filled_price?: Money;
  slippage?: string;
  preview_state: "READY" | "BLOCKED";
  block_reasons: string[];
  warnings: string[];
  provider_trading_authorized: boolean;
  previewed_at: string;
};

type PreviewResponse = {
  preview?: Preview;
  preview_semantics?: "PROVIDER_ESTIMATE_ONLY";
  provider_trading_authorized?: boolean;
  provider_preview_id_exposed?: false;
  order_created?: false;
  submission_available?: false;
  ai_execution_authority?: false;
  live_execution_available?: false;
  error?: { message?: string };
};

function exactMoney(value?: Money) {
  return value ? `${value.amount} ${value.currency}` : "Unavailable";
}

function label(value: string) {
  return value.replaceAll("_", " ").toLowerCase();
}

export function CoinbaseOrderPreview({
  accountID,
  symbols,
  tradingAuthorized,
}: {
  accountID: string;
  symbols: string[];
  tradingAuthorized: boolean;
}) {
  const choices = useMemo(
    () => [...new Set(symbols.map((symbol) => symbol.toUpperCase()))].sort(),
    [symbols],
  );
  const [symbol, setSymbol] = useState(choices[0] ?? "BTC");
  const [side, setSide] = useState<"BUY" | "SELL">("BUY");
  const [size, setSize] = useState("");
  const [preview, setPreview] = useState<Preview>();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setBusy(true);
    setError("");
    setPreview(undefined);
    try {
      const response = await fetch(
        `/api/accounts/${encodeURIComponent(accountID)}/orders/preview`,
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ symbol, side, size }),
        },
      );
      const body = (await response.json()) as PreviewResponse;
      if (!response.ok) {
        setError(
          body.error?.message ??
            "Coinbase could not preview this order safely.",
        );
        return;
      }
      if (
        !body.preview ||
        body.preview.provider !== "coinbase" ||
        body.preview.feed !== "advanced_trade_order_preview" ||
        !["READY", "BLOCKED"].includes(body.preview.preview_state) ||
        body.preview.quote_currency !== "USD" ||
        body.preview.product_id !== `${symbol}-USD` ||
        body.preview.side !== side ||
        body.preview_semantics !== "PROVIDER_ESTIMATE_ONLY" ||
        body.provider_trading_authorized !==
          body.preview.provider_trading_authorized ||
        body.provider_preview_id_exposed !== false ||
        body.order_created !== false ||
        body.submission_available !== false ||
        body.ai_execution_authority !== false ||
        body.live_execution_available !== false
      ) {
        setError("Arbion rejected an unsafe Coinbase preview response.");
        return;
      }
      setPreview(body.preview);
    } catch {
      setError("Coinbase order preview is temporarily unavailable.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <motion.section
      className="coinbase-order-preview"
      aria-labelledby="coinbase-preview-title"
      initial={{ opacity: 0, y: 12 }}
      animate={{ opacity: 1, y: 0 }}
    >
      <header>
        <div>
          <p className="eyebrow">CONTROLLED EXECUTION FOUNDATION</p>
          <h2 id="coinbase-preview-title">Coinbase order preview</h2>
          <p>
            Ask Coinbase to estimate a real spot market order against this
            connected portfolio. Previewing cannot create, cancel, or submit an
            order.
          </p>
        </div>
        <span
          className={`coinbase-trade-grant ${tradingAuthorized ? "granted" : "view-only"}`}
        >
          {tradingAuthorized ? "Trade key granted" : "View-only key"}
        </span>
      </header>

      <form onSubmit={submit}>
        <label>
          Asset
          <input
            aria-label="Coinbase preview asset"
            autoComplete="off"
            maxLength={16}
            onChange={(event) =>
              setSymbol(
                event.target.value.toUpperCase().replace(/[^A-Z0-9]/g, ""),
              )
            }
            required
            spellCheck={false}
            value={symbol}
          />
        </label>
        <label>
          Side
          <select
            aria-label="Coinbase preview side"
            onChange={(event) =>
              setSide(event.target.value === "SELL" ? "SELL" : "BUY")
            }
            value={side}
          >
            <option value="BUY">Buy</option>
            <option value="SELL">Sell</option>
          </select>
        </label>
        <label>
          {side === "BUY" ? "USD amount" : `${symbol || "Asset"} amount`}
          <input
            aria-label="Coinbase preview amount"
            autoComplete="off"
            inputMode="decimal"
            onChange={(event) => setSize(event.target.value)}
            placeholder={side === "BUY" ? "25.00" : "0.001"}
            required
            value={size}
          />
        </label>
        <button disabled={busy || !symbol || !size} type="submit">
          {busy ? "Requesting preview…" : "Preview with Coinbase"}
        </button>
      </form>

      {error && (
        <p className="crypto-command-error" role="alert">
          {error}
        </p>
      )}

      {preview && (
        <div className="coinbase-preview-result" aria-live="polite">
          <div className="coinbase-preview-result-heading">
            <div>
              <span>{preview.product_id}</span>
              <strong>{preview.preview_state}</strong>
            </div>
            <small>
              Coinbase · {new Date(preview.previewed_at).toLocaleString()}
            </small>
          </div>
          <dl>
            <div>
              <dt>Requested</dt>
              <dd>{exactMoney(preview.requested_size)}</dd>
            </div>
            <div>
              <dt>Estimated fill</dt>
              <dd>{exactMoney(preview.estimated_average_filled_price)}</dd>
            </div>
            <div>
              <dt>Order total</dt>
              <dd>{exactMoney(preview.order_total)}</dd>
            </div>
            <div>
              <dt>Commission</dt>
              <dd>{exactMoney(preview.commission_total)}</dd>
            </div>
            <div>
              <dt>Best bid / ask</dt>
              <dd>
                {exactMoney(preview.best_bid)} / {exactMoney(preview.best_ask)}
              </dd>
            </div>
            <div>
              <dt>Provider trade grant</dt>
              <dd>
                {preview.provider_trading_authorized
                  ? "Granted"
                  : "Not granted"}
              </dd>
            </div>
          </dl>
          {preview.block_reasons.length > 0 && (
            <p className="coinbase-preview-blocks">
              Blocked: {preview.block_reasons.map(label).join(", ")}
            </p>
          )}
          {preview.warnings.length > 0 && (
            <p className="coinbase-preview-warnings">
              Coinbase warnings: {preview.warnings.map(label).join(", ")}
            </p>
          )}
          <p className="coinbase-preview-lock">
            <strong>No order was created.</strong> Provider preview IDs remain
            server-private and cannot be used by the browser or an AI model as
            execution authority.
          </p>
        </div>
      )}
    </motion.section>
  );
}
