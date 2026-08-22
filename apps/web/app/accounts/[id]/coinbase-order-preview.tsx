"use client";

import { motion } from "motion/react";
import { type FormEvent, useMemo, useState } from "react";

type Money = { amount: string; currency: string };

type ProductRules = {
  provider: "coinbase";
  feed: "advanced_trade_product";
  product_id: string;
  product_type: "SPOT";
  base_asset: string;
  quote_currency: "USD";
  base_increment: string;
  quote_increment: string;
  base_min_size: string;
  base_max_size: string;
  quote_min_size: string;
  quote_max_size: string;
  status: string;
  market_ioc_enabled: boolean;
  block_reasons: string[];
  observed_at: string;
};

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
  product_rules: ProductRules;
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

type OrderIntent = {
  id: string;
  financial_account_id: string;
  source: "UI" | "AI" | "HYBRID";
  provider: "coinbase";
  product_id: string;
  base_asset: string;
  quote_currency: "USD";
  side: "BUY" | "SELL";
  order_type: "MARKET_IOC";
  requested_size: Money;
  status: "REVIEW_REQUIRED" | "BLOCKED" | "USER_APPROVED_NONEXECUTABLE";
  version: number;
  preview: {
    preview_state: "READY" | "BLOCKED";
    order_total: Money;
    commission_total: Money;
    provider_trading_authorized: boolean;
    block_reasons: string[];
    warnings: string[];
    previewed_at: string;
    expires_at: string;
    product_rules: ProductRules;
  };
  review_scope: "PROPOSAL_REVIEW_ONLY";
  submission_available: false;
  risk_approval_available: false;
  ai_execution_authority: false;
  live_execution_available: false;
};

type OrderIntentResponse = {
  order_intent?: OrderIntent;
  approval_scope?: "PROPOSAL_REVIEW_ONLY";
  provider_order_created?: false;
  submission_available?: false;
  risk_approval_available?: false;
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

function canonicalDecimal(value: string) {
  const [whole, fraction] = value.split(".");
  const trimmed = fraction?.replace(/0+$/, "");
  return trimmed ? `${whole}.${trimmed}` : whole;
}

const productBlockReasons = new Set([
  "PRODUCT_AUCTION_MODE",
  "PRODUCT_CANCEL_ONLY",
  "PRODUCT_DISABLED",
  "PRODUCT_LIMIT_ONLY",
  "PRODUCT_POST_ONLY",
  "SIZE_ABOVE_MAXIMUM",
  "SIZE_BELOW_MINIMUM",
  "SIZE_INCREMENT_MISMATCH",
]);

function safePositiveDecimal(value: string) {
  return (
    /^(0|[1-9][0-9]{0,19})(\.[0-9]{1,18})?$/.test(value) &&
    !/^0(?:\.0+)?$/.test(value)
  );
}

function safeProductRules(
  rules: ProductRules | undefined,
  symbol: string,
  previewedAt: string,
) {
  const reasons = rules?.block_reasons;
  return Boolean(
    rules &&
      rules.provider === "coinbase" &&
      rules.feed === "advanced_trade_product" &&
      rules.product_id === `${symbol}-USD` &&
      rules.product_type === "SPOT" &&
      rules.base_asset === symbol &&
      rules.quote_currency === "USD" &&
      [
        rules.base_increment,
        rules.quote_increment,
        rules.base_min_size,
        rules.base_max_size,
        rules.quote_min_size,
        rules.quote_max_size,
      ].every(safePositiveDecimal) &&
      /^[A-Z][A-Z0-9_-]{0,31}$/.test(rules.status) &&
      Array.isArray(reasons) &&
      reasons.length <= 20 &&
      new Set(reasons).size === reasons.length &&
      reasons.every((reason) => productBlockReasons.has(reason)) &&
      (rules.status === "ONLINE" || reasons.includes("PRODUCT_DISABLED")) &&
      rules.market_ioc_enabled === (reasons.length === 0) &&
      rules.observed_at === previewedAt &&
      !Number.isNaN(Date.parse(rules.observed_at)),
  );
}

function safeIntentEnvelope(
  body: OrderIntentResponse,
  accountID: string,
  symbol: string,
  side: "BUY" | "SELL",
  size: string,
) {
  const intent = body.order_intent;
  const requestedCurrency = side === "BUY" ? "USD" : symbol;
  return Boolean(
    intent &&
      intent.id &&
      intent.financial_account_id === accountID &&
      intent.source === "UI" &&
      intent.provider === "coinbase" &&
      intent.product_id === `${symbol}-USD` &&
      intent.base_asset === symbol &&
      intent.quote_currency === "USD" &&
      intent.side === side &&
      intent.order_type === "MARKET_IOC" &&
      safeProductRules(
        intent.preview.product_rules,
        symbol,
        intent.preview.previewed_at,
      ) &&
      intent.requested_size.currency === requestedCurrency &&
      canonicalDecimal(intent.requested_size.amount) ===
        canonicalDecimal(size) &&
      ["REVIEW_REQUIRED", "BLOCKED", "USER_APPROVED_NONEXECUTABLE"].includes(
        intent.status,
      ) &&
      Number.isSafeInteger(intent.version) &&
      intent.version > 0 &&
      intent.review_scope === "PROPOSAL_REVIEW_ONLY" &&
      intent.submission_available === false &&
      intent.risk_approval_available === false &&
      intent.ai_execution_authority === false &&
      intent.live_execution_available === false &&
      body.approval_scope === "PROPOSAL_REVIEW_ONLY" &&
      body.provider_order_created === false &&
      body.submission_available === false &&
      body.risk_approval_available === false &&
      body.ai_execution_authority === false &&
      body.live_execution_available === false,
  );
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
  const [intent, setIntent] = useState<OrderIntent>();
  const [intentBusy, setIntentBusy] = useState(false);
  const [intentError, setIntentError] = useState("");
  const [mfaCode, setMFACode] = useState("");

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setBusy(true);
    setError("");
    setPreview(undefined);
    setIntent(undefined);
    setIntentError("");
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
        !safeProductRules(
          body.preview.product_rules,
          symbol,
          body.preview.previewed_at,
        ) ||
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

  async function createIntent() {
    if (!preview || !size) return;
    setIntentBusy(true);
    setIntentError("");
    try {
      const response = await fetch(
        `/api/accounts/${encodeURIComponent(accountID)}/order-intents`,
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            symbol,
            side,
            size,
            idempotency_key: crypto.randomUUID(),
          }),
        },
      );
      const body = (await response.json()) as OrderIntentResponse;
      if (!response.ok) {
        setIntentError(
          body.error?.message ?? "Arbion could not save this order proposal.",
        );
        return;
      }
      if (!safeIntentEnvelope(body, accountID, symbol, side, size)) {
        setIntentError("Arbion rejected an unsafe order-intent response.");
        return;
      }
      setIntent(body.order_intent);
    } catch {
      setIntentError("The durable order-proposal service is unavailable.");
    } finally {
      setIntentBusy(false);
    }
  }

  async function reviewIntent() {
    if (!intent || intent.status !== "REVIEW_REQUIRED" || !mfaCode) return;
    setIntentBusy(true);
    setIntentError("");
    try {
      const response = await fetch(
        `/api/order-intents/${encodeURIComponent(intent.id)}/review`,
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            expected_version: intent.version,
            mfa_code: mfaCode,
          }),
        },
      );
      setMFACode("");
      const body = (await response.json()) as OrderIntentResponse;
      if (!response.ok) {
        setIntentError(
          body.error?.message ?? "Arbion could not review this proposal.",
        );
        return;
      }
      const reviewed = body.order_intent;
      if (
        !safeIntentEnvelope(body, accountID, symbol, side, size) ||
        !reviewed ||
        reviewed.id !== intent.id ||
        reviewed.status !== "USER_APPROVED_NONEXECUTABLE" ||
        reviewed.version !== intent.version + 1
      ) {
        setIntentError("Arbion rejected an unsafe proposal-review response.");
        return;
      }
      setIntent(reviewed);
    } catch {
      setMFACode("");
      setIntentError("The proposal-review service is unavailable.");
    } finally {
      setIntentBusy(false);
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
            <div>
              <dt>Product control</dt>
              <dd>
                {preview.product_rules.market_ioc_enabled
                  ? `Market IOC enabled · ${side === "BUY" ? preview.product_rules.quote_increment : preview.product_rules.base_increment} increment`
                  : "Market IOC blocked"}
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
          <div className="coinbase-intent-entry">
            <div>
              <strong>Build a durable proposal</strong>
              <p>
                Save this idea with immutable preview evidence and a replay-safe
                request key. This still cannot create a Coinbase order.
              </p>
            </div>
            <button
              disabled={intentBusy || Boolean(intent)}
              onClick={() => void createIntent()}
              type="button"
            >
              {intentBusy
                ? "Saving proposal…"
                : intent
                  ? "Proposal saved"
                  : "Save reviewable proposal"}
            </button>
          </div>
        </div>
      )}

      {intentError && (
        <p className="crypto-command-error" role="alert">
          {intentError}
        </p>
      )}

      {intent && (
        <div className="coinbase-intent-result" aria-live="polite">
          <header>
            <div>
              <span>DURABLE ORDER INTENT</span>
              <strong>{label(intent.status)}</strong>
            </div>
            <small>Version {intent.version} · proposal review only</small>
          </header>
          <p>
            Bound to {intent.product_id}, {intent.side.toLowerCase()},{" "}
            {exactMoney(intent.requested_size)}. Coinbase re-quoted the proposal
            when it was saved: estimated total{" "}
            {exactMoney(intent.preview.order_total)} plus{" "}
            {exactMoney(intent.preview.commission_total)} commission. This saved
            evidence includes Coinbase product status{" "}
            {intent.preview.product_rules.status} and expires at{" "}
            {new Date(intent.preview.expires_at).toLocaleTimeString()}.
          </p>
          {intent.status === "REVIEW_REQUIRED" && (
            <div className="coinbase-intent-review">
              <label>
                Fresh authenticator code
                <input
                  aria-label="Order proposal authenticator code"
                  autoComplete="one-time-code"
                  inputMode="numeric"
                  maxLength={6}
                  onChange={(event) =>
                    setMFACode(event.target.value.replace(/\D/g, ""))
                  }
                  pattern="[0-9]{6}"
                  value={mfaCode}
                />
              </label>
              <button
                disabled={intentBusy || mfaCode.length !== 6}
                onClick={() => void reviewIntent()}
                type="button"
              >
                {intentBusy ? "Verifying…" : "Review proposal with MFA"}
              </button>
            </div>
          )}
          {intent.status === "BLOCKED" && (
            <p className="coinbase-preview-blocks">
              This proposal is recorded but blocked. Confirm Coinbase Trade is
              enabled, Transfer is disabled, and request a fresh preview.
            </p>
          )}
          <p className="coinbase-intent-boundary">
            <strong>No execution approval exists.</strong> User review is an
            immutable acknowledgment of this proposal only. Risk approval, order
            submission, and AI execution authority remain unavailable.
          </p>
        </div>
      )}
    </motion.section>
  );
}
