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

type CryptoCandle = {
  start: string;
  low: string;
  high: string;
  open: string;
  close: string;
  volume: string;
};

export type CryptoCandleSeries = {
  symbol: string;
  currency: string;
  granularity_seconds: 900;
  expected_intervals: 96;
  candles: CryptoCandle[];
  provenance: Provenance;
};

type HistoryResponse = {
  history?: CryptoCandleSeries;
  cached?: boolean;
  chart_semantics?: "VENUE_PRICE_MOVEMENT";
  live_execution_available: false;
  error?: { message?: string };
};

export type CoinbaseTradeFill = {
  product_id: string;
  base_asset: string;
  quote_currency: string;
  side: "BUY" | "SELL";
  price: string;
  size: string;
  size_unit: string;
  commission: Money;
  trade_time: string;
  liquidity: "MAKER" | "TAKER" | "UNKNOWN";
};

export type CoinbaseTradeActivity = {
  provider: "coinbase";
  feed: "advanced_trade_fills";
  fills: CoinbaseTradeFill[];
  has_more: boolean;
  retrieved_at: string;
};

type ActivityResponse = {
  activity?: CoinbaseTradeActivity;
  history_semantics?: "EXTERNAL_EXECUTION_EVIDENCE";
  live_execution_available: false;
  error?: { message?: string };
};

export type CoinbaseOrderObservation = {
  product_id: string;
  base_asset: string;
  quote_currency: string;
  side: "BUY" | "SELL";
  status:
    | "PENDING"
    | "OPEN"
    | "FILLED"
    | "CANCELLED"
    | "EXPIRED"
    | "FAILED"
    | "QUEUED"
    | "CANCEL_QUEUED"
    | "EDIT_QUEUED"
    | "UNKNOWN";
  order_type: string;
  time_in_force: string;
  completion_percentage: string;
  filled_size: string;
  filled_size_unit: string;
  filled_value: Money;
  average_filled_price?: Money;
  total_fees: Money;
  number_of_fills: number;
  pending_cancel: boolean;
  settled: boolean;
  is_liquidation: boolean;
  outcome_reason: string;
  created_at: string;
  last_fill_at?: string;
};

export type CoinbaseOrderHistory = {
  provider: "coinbase";
  feed: "advanced_trade_orders";
  orders: CoinbaseOrderObservation[];
  has_more: boolean;
  retrieved_at: string;
};

type OrderHistoryResponse = {
  orders?: CoinbaseOrderHistory;
  history_semantics?: "EXTERNAL_ORDER_STATUS";
  order_actions_available: false;
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

function exactDecimal(value: string) {
  const match = value.match(/^(-?)(\d+)(?:\.(\d+))?$/);
  if (!match) return value;
  const grouped = match[2].replace(/\B(?=(\d{3})+(?!\d))/g, ",");
  return `${match[1]}${grouped}${match[3] === undefined ? "" : `.${match[3]}`}`;
}

function exactMoney(value: Money) {
  return `${exactDecimal(value.amount)} ${value.currency}`;
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

function price(value?: string, currency = "USD") {
  if (!value) return "—";
  const parsed = Number(value);
  if (!Number.isFinite(parsed)) return value;
  return new Intl.NumberFormat("en-US", {
    style: "currency",
    currency,
    maximumFractionDigits: Math.abs(parsed) < 1 ? 6 : 2,
  }).format(parsed);
}

function historyChart(series?: CryptoCandleSeries) {
  if (!series || series.candles.length === 0) return null;
  const candles = series.candles
    .map((candle) => ({
      ...candle,
      time: new Date(candle.start).valueOf(),
      lowValue: Number(candle.low),
      highValue: Number(candle.high),
      closeValue: Number(candle.close),
    }))
    .filter(
      (candle) =>
        Number.isFinite(candle.time) &&
        Number.isFinite(candle.lowValue) &&
        Number.isFinite(candle.highValue) &&
        Number.isFinite(candle.closeValue),
    )
    .sort((left, right) => left.time - right.time);
  if (candles.length === 0) return null;
  const width = 960;
  const height = 300;
  const paddingX = 14;
  const paddingY = 18;
  const granularityMs = series.granularity_seconds * 1000;
  const receivedAt = new Date(series.provenance.received_at).valueOf();
  const end = Number.isFinite(receivedAt)
    ? receivedAt
    : candles[candles.length - 1].time;
  const start = end - series.expected_intervals * granularityMs;
  const minimum = Math.min(...candles.map((candle) => candle.lowValue));
  const maximum = Math.max(...candles.map((candle) => candle.highValue));
  const spread = maximum - minimum || Math.max(Math.abs(maximum) * 0.01, 1);
  const x = (value: number) =>
    paddingX +
    ((value - start) / Math.max(end - start, granularityMs)) *
      (width - paddingX * 2);
  const y = (value: number) =>
    paddingY + ((maximum - value) / spread) * (height - paddingY * 2);
  const segments: string[] = [];
  let current = "";
  candles.forEach((candle, index) => {
    const prior = candles[index - 1];
    const command =
      !prior || candle.time - prior.time > granularityMs * 1.5 ? "M" : "L";
    if (command === "M" && current) {
      segments.push(current);
      current = "";
    }
    current += `${command}${x(candle.time).toFixed(2)},${y(candle.closeValue).toFixed(2)} `;
  });
  if (current) segments.push(current);
  const first = candles[0];
  const last = candles[candles.length - 1];
  const change = first.closeValue
    ? ((last.closeValue - first.closeValue) / first.closeValue) * 100
    : null;
  return { width, height, segments, minimum, maximum, first, last, change };
}

export function CryptoPortfolioCommandCenter({
  accountID,
  initialSnapshot,
  initialHistory,
  initialHistoryCached = false,
  initialActivity,
  initialOrderHistory,
}: {
  accountID: string;
  initialSnapshot: CryptoPortfolioSnapshot;
  initialHistory?: CryptoCandleSeries;
  initialHistoryCached?: boolean;
  initialActivity?: CoinbaseTradeActivity;
  initialOrderHistory?: CoinbaseOrderHistory;
}) {
  const [snapshot, setSnapshot] = useState(initialSnapshot);
  const [refreshing, setRefreshing] = useState(false);
  const [refreshError, setRefreshError] = useState("");

  const initialHistorySymbol = initialHistory?.symbol ?? "";
  const [selectedSymbol, setSelectedSymbol] = useState(
    initialHistorySymbol ||
      initialSnapshot.positions.find((position) => position.market_value)
        ?.symbol ||
      "",
  );
  const [histories, setHistories] = useState<
    Record<string, CryptoCandleSeries>
  >(initialHistory ? { [initialHistory.symbol]: initialHistory } : {});
  const [historyCached, setHistoryCached] = useState<Record<string, boolean>>(
    initialHistory ? { [initialHistory.symbol]: initialHistoryCached } : {},
  );
  const [historyLoading, setHistoryLoading] = useState(false);
  const [historyError, setHistoryError] = useState("");
  const [activity, setActivity] = useState(initialActivity);
  const [activityLoading, setActivityLoading] = useState(false);
  const [activityError, setActivityError] = useState("");
  const [orderHistory, setOrderHistory] = useState(initialOrderHistory);
  const [ordersLoading, setOrdersLoading] = useState(false);
  const [ordersError, setOrdersError] = useState("");

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
      setSelectedSymbol((current) => {
        if (
          body.portfolio?.positions.some(
            (position) => position.symbol === current && position.market_value,
          )
        ) {
          return current;
        }
        return (
          body.portfolio?.positions.find((position) => position.market_value)
            ?.symbol ?? ""
        );
      });
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
  const selectedHistory = histories[selectedSymbol];
  const chart = useMemo(() => historyChart(selectedHistory), [selectedHistory]);

  const loadHistory = useCallback(
    async (symbol: string, force = false) => {
      if (!symbol || (!force && histories[symbol])) return;
      setHistoryLoading(true);
      setHistoryError("");
      try {
        const response = await fetch(
          `/api/accounts/${encodeURIComponent(accountID)}/markets/crypto/${encodeURIComponent(symbol)}/candles`,
          { cache: "no-store" },
        );
        const body = (await response.json()) as HistoryResponse;
        if (
          !response.ok ||
          !body.history ||
          body.chart_semantics !== "VENUE_PRICE_MOVEMENT" ||
          body.live_execution_available !== false ||
          body.history.symbol !== symbol ||
          body.history.currency !== "USD" ||
          body.history.granularity_seconds !== 900 ||
          body.history.expected_intervals !== 96
        ) {
          setHistoryError(
            body.error?.message ??
              "Coinbase venue history is temporarily unavailable. No intervals were estimated.",
          );
          return;
        }
        setHistories((current) => ({
          ...current,
          [symbol]: body.history as CryptoCandleSeries,
        }));
        setHistoryCached((current) => ({
          ...current,
          [symbol]: Boolean(body.cached),
        }));
      } catch {
        setHistoryError(
          "Coinbase venue history is temporarily unavailable. No intervals were estimated.",
        );
      } finally {
        setHistoryLoading(false);
      }
    },
    [accountID, histories],
  );

  const refreshActivity = useCallback(async () => {
    setActivityLoading(true);
    setActivityError("");
    try {
      const response = await fetch(
        `/api/accounts/${encodeURIComponent(accountID)}/activity/fills`,
        { cache: "no-store" },
      );
      const body = (await response.json()) as ActivityResponse;
      if (
        !response.ok ||
        !body.activity ||
        body.history_semantics !== "EXTERNAL_EXECUTION_EVIDENCE" ||
        body.live_execution_available !== false ||
        body.activity.provider !== "coinbase" ||
        body.activity.feed !== "advanced_trade_fills" ||
        body.activity.fills.length > 50
      ) {
        setActivityError(
          body.error?.message ??
            "Coinbase execution history is temporarily unavailable. No activity was inferred or substituted.",
        );
        return;
      }
      setActivity(body.activity);
    } catch {
      setActivityError(
        "Coinbase execution history is temporarily unavailable. No activity was inferred or substituted.",
      );
    } finally {
      setActivityLoading(false);
    }
  }, [accountID]);

  const refreshOrderHistory = useCallback(async () => {
    setOrdersLoading(true);
    setOrdersError("");
    try {
      const response = await fetch(
        `/api/accounts/${encodeURIComponent(accountID)}/activity/orders`,
        { cache: "no-store" },
      );
      const body = (await response.json()) as OrderHistoryResponse;
      if (
        !response.ok ||
        !body.orders ||
        body.history_semantics !== "EXTERNAL_ORDER_STATUS" ||
        body.order_actions_available !== false ||
        body.live_execution_available !== false ||
        body.orders.provider !== "coinbase" ||
        body.orders.feed !== "advanced_trade_orders" ||
        body.orders.orders.length > 50
      ) {
        setOrdersError(
          body.error?.message ??
            "Coinbase order status is temporarily unavailable. No state was inferred or substituted.",
        );
        return;
      }
      setOrderHistory(body.orders);
    } catch {
      setOrdersError(
        "Coinbase order status is temporarily unavailable. No state was inferred or substituted.",
      );
    } finally {
      setOrdersLoading(false);
    }
  }, [accountID]);

  const liveOrderCount =
    orderHistory?.orders.filter((order) =>
      ["PENDING", "OPEN", "QUEUED", "CANCEL_QUEUED", "EDIT_QUEUED"].includes(
        order.status,
      ),
    ).length ?? 0;

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

      <motion.section
        className="crypto-history-panel"
        aria-labelledby="crypto-history-title"
        initial={{ opacity: 0, y: 18 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ delay: 0.22 }}
      >
        <header>
          <div>
            <p className="eyebrow">CONNECTED ASSET HISTORY</p>
            <h2 id="crypto-history-title">24h venue movement</h2>
            <p>
              Coinbase Exchange price observations—not portfolio performance,
              return, cost basis, or P&amp;L.
            </p>
          </div>
          <div className="crypto-history-actions">
            <span>
              {selectedSymbol ? `${selectedSymbol} / USD` : "No priced asset"}
            </span>
            <button
              type="button"
              disabled={!selectedSymbol || historyLoading}
              onClick={() => void loadHistory(selectedSymbol, true)}
            >
              {historyLoading ? "Loading…" : "Refresh history"}
            </button>
          </div>
        </header>

        {pricedPositions.length > 0 && (
          <div className="crypto-history-symbols" aria-label="Chart asset">
            {pricedPositions.map((position) => (
              <button
                key={position.symbol}
                className={
                  selectedSymbol === position.symbol ? "selected" : undefined
                }
                type="button"
                aria-pressed={selectedSymbol === position.symbol}
                onClick={() => {
                  setSelectedSymbol(position.symbol);
                  setHistoryError("");
                  void loadHistory(position.symbol);
                }}
              >
                {position.symbol}
              </button>
            ))}
          </div>
        )}

        {historyError ? (
          <p className="crypto-history-unavailable" role="alert">
            {historyError}
          </p>
        ) : !chart || !selectedHistory ? (
          <p className="crypto-history-unavailable">
            {historyLoading
              ? "Loading provider-reported intervals…"
              : "No provider-reported candle history is available for this connected asset."}
          </p>
        ) : (
          <div className="crypto-history-workspace">
            <div className="crypto-history-chart">
              <div className="crypto-history-summary">
                <div>
                  <span>Latest close</span>
                  <strong>
                    {price(
                      selectedHistory.candles[
                        selectedHistory.candles.length - 1
                      ]?.close,
                      selectedHistory.currency,
                    )}
                  </strong>
                </div>
                <div>
                  <span>Observed movement</span>
                  <strong
                    className={
                      chart.change !== null && chart.change < 0
                        ? "negative"
                        : "positive"
                    }
                  >
                    {chart.change === null
                      ? "—"
                      : `${chart.change >= 0 ? "+" : ""}${chart.change.toFixed(2)}%`}
                  </strong>
                </div>
                <div>
                  <span>Venue range</span>
                  <strong>
                    {price(String(chart.minimum), selectedHistory.currency)} —{" "}
                    {price(String(chart.maximum), selectedHistory.currency)}
                  </strong>
                </div>
              </div>
              <svg
                viewBox={`0 0 ${chart.width} ${chart.height}`}
                role="img"
                aria-label={`${selectedSymbol} Coinbase Exchange 24-hour price movement with provider gaps preserved`}
                preserveAspectRatio="none"
              >
                <defs>
                  <linearGradient
                    id="arbion-history-line"
                    x1="0"
                    y1="0"
                    x2="1"
                    y2="0"
                  >
                    <stop offset="0" stopColor="#5ee0a0" />
                    <stop offset="1" stopColor="#57cce3" />
                  </linearGradient>
                </defs>
                <g className="crypto-history-grid" aria-hidden="true">
                  <line x1="0" y1="75" x2="960" y2="75" />
                  <line x1="0" y1="150" x2="960" y2="150" />
                  <line x1="0" y1="225" x2="960" y2="225" />
                </g>
                {chart.segments.map((path, index) => (
                  <motion.path
                    key={`${selectedSymbol}-${index}`}
                    d={path}
                    fill="none"
                    stroke="url(#arbion-history-line)"
                    strokeWidth="3"
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    initial={{ pathLength: 0, opacity: 0 }}
                    animate={{ pathLength: 1, opacity: 1 }}
                    transition={{ duration: 0.7, ease: "easeOut" }}
                  />
                ))}
              </svg>
              <div className="crypto-history-axis" aria-hidden="true">
                <span>24 hours ago</span>
                <span>Latest provider interval</span>
              </div>
            </div>
            <aside className="crypto-history-evidence">
              <p className="eyebrow">SERIES EVIDENCE</p>
              <dl>
                <div>
                  <dt>Coverage</dt>
                  <dd>
                    {selectedHistory.candles.length}/
                    {selectedHistory.expected_intervals} intervals
                  </dd>
                </div>
                <div>
                  <dt>Interval</dt>
                  <dd>15 minutes</dd>
                </div>
                <div>
                  <dt>Venue</dt>
                  <dd>Coinbase Exchange</dd>
                </div>
                <div>
                  <dt>Feed</dt>
                  <dd>REST candles · single venue</dd>
                </div>
                <div>
                  <dt>Latest interval</dt>
                  <dd>
                    {timestamp(selectedHistory.provenance.provider_timestamp)}
                  </dd>
                </div>
                <div>
                  <dt>Delivery</dt>
                  <dd>
                    {historyCached[selectedSymbol]
                      ? "Bounded display cache"
                      : "Provider response"}
                  </dd>
                </div>
              </dl>
              <p>
                Coinbase may omit intervals with no ticks. Arbion preserves
                those gaps and does not interpolate missing prices.
              </p>
            </aside>
          </div>
        )}
      </motion.section>

      <motion.section
        className="crypto-order-monitor"
        aria-labelledby="crypto-order-title"
        initial={{ opacity: 0, y: 18 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ delay: 0.24 }}
      >
        <header>
          <div>
            <p className="eyebrow">CONNECTED ORDER MONITOR</p>
            <h2 id="crypto-order-title">Provider-reported order state</h2>
            <p>
              Open and historical spot orders created outside Arbion. Monitor
              status and fill progress here; create, edit, and cancel actions
              remain unavailable.
            </p>
          </div>
          <button
            type="button"
            disabled={ordersLoading}
            onClick={() => void refreshOrderHistory()}
          >
            {ordersLoading ? "Refreshing…" : "Refresh orders"}
          </button>
        </header>

        {orderHistory && (
          <div
            className="crypto-order-summary"
            aria-label="Order monitor evidence"
          >
            <div>
              <span>Working state</span>
              <strong>{liveOrderCount} open or queued</strong>
            </div>
            <div>
              <span>Orders shown</span>
              <strong>{orderHistory.orders.length}/50 maximum</strong>
            </div>
            <div>
              <span>History window</span>
              <strong>
                {orderHistory.has_more
                  ? "More remains at Coinbase"
                  : "First page complete"}
              </strong>
            </div>
            <div>
              <span>Arbion actions</span>
              <strong>None · monitor only</strong>
            </div>
          </div>
        )}

        {ordersError ? (
          <p className="crypto-order-unavailable" role="alert">
            {ordersError}
          </p>
        ) : !orderHistory ? (
          <p className="crypto-order-unavailable">
            Coinbase order status is not available yet. Refresh to try the
            protected View-only connection.
          </p>
        ) : orderHistory.orders.length === 0 ? (
          <p className="crypto-order-unavailable">
            Coinbase reported no recent Advanced Trade spot orders for this
            portfolio.
          </p>
        ) : (
          <div
            className="crypto-order-table"
            role="region"
            aria-label="Provider-reported Coinbase order status"
            tabIndex={0}
          >
            <table>
              <thead>
                <tr>
                  <th>Created</th>
                  <th>Market</th>
                  <th>State</th>
                  <th>Instruction</th>
                  <th>Fill progress</th>
                  <th>Filled value</th>
                  <th>Average / Fees</th>
                </tr>
              </thead>
              <tbody>
                {orderHistory.orders.map((order, index) => {
                  const completion = Number(order.completion_percentage);
                  return (
                    <tr
                      key={`${order.created_at}-${order.product_id}-${index}`}
                    >
                      <td>{timestamp(order.created_at)}</td>
                      <td>
                        <strong>{order.product_id}</strong>
                        <small>
                          {order.is_liquidation
                            ? "Provider liquidation"
                            : "Created outside Arbion"}
                        </small>
                      </td>
                      <td>
                        <span
                          className={`crypto-order-state ${order.status.toLowerCase()}`}
                        >
                          {order.status.replaceAll("_", " ")}
                        </span>
                        <small>
                          {order.pending_cancel
                            ? "Cancellation pending at Coinbase"
                            : order.outcome_reason === "NONE"
                              ? order.settled
                                ? "Settled"
                                : "Provider state"
                              : order.outcome_reason
                                  .replaceAll("_", " ")
                                  .toLowerCase()}
                        </small>
                      </td>
                      <td>
                        <strong>
                          {order.side} · {order.order_type.replaceAll("_", " ")}
                        </strong>
                        <small>
                          {order.time_in_force
                            .replaceAll("_", " ")
                            .toLowerCase()}
                        </small>
                      </td>
                      <td>
                        <div className="crypto-order-progress">
                          <span>
                            <i
                              style={{
                                width: `${Number.isFinite(completion) ? Math.min(100, Math.max(0, completion)) : 0}%`,
                              }}
                            />
                          </span>
                          <small>
                            {exactDecimal(order.completion_percentage)}% ·{" "}
                            {exactDecimal(order.filled_size)}{" "}
                            {order.filled_size_unit}
                          </small>
                        </div>
                      </td>
                      <td>{exactMoney(order.filled_value)}</td>
                      <td>
                        <strong>
                          {order.average_filled_price
                            ? exactMoney(order.average_filled_price)
                            : "No provider average"}
                        </strong>
                        <small>
                          {exactMoney(order.total_fees)} fees ·{" "}
                          {order.number_of_fills} fills
                        </small>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
        <footer>
          Arbion receives normalized order state only. Coinbase order IDs, user
          IDs, portfolio IDs, messages, and pagination tokens never reach this
          page.
        </footer>
      </motion.section>

      <motion.section
        className="crypto-activity-panel"
        aria-labelledby="crypto-activity-title"
        initial={{ opacity: 0, y: 18 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ delay: 0.25 }}
      >
        <header>
          <div>
            <p className="eyebrow">COINBASE EXECUTION EVIDENCE</p>
            <h2 id="crypto-activity-title">Recent external fills</h2>
            <p>
              Historical trades reported by Coinbase. Arbion did not place these
              orders, and this is not cost basis, performance, or P&amp;L.
            </p>
          </div>
          <button
            type="button"
            disabled={activityLoading}
            onClick={() => void refreshActivity()}
          >
            {activityLoading ? "Refreshing…" : "Refresh activity"}
          </button>
        </header>

        {activity && (
          <div
            className="crypto-activity-summary"
            aria-label="Activity evidence"
          >
            <div>
              <span>Latest execution</span>
              <strong>{timestamp(activity.fills[0]?.trade_time)}</strong>
            </div>
            <div>
              <span>Entries shown</span>
              <strong>{activity.fills.length}/50 maximum</strong>
            </div>
            <div>
              <span>History window</span>
              <strong>
                {activity.has_more
                  ? "More remains at Coinbase"
                  : "First page complete"}
              </strong>
            </div>
            <div>
              <span>Source permission</span>
              <strong>Advanced Trade · View</strong>
            </div>
          </div>
        )}

        {activityError ? (
          <p className="crypto-activity-unavailable" role="alert">
            {activityError}
          </p>
        ) : !activity ? (
          <p className="crypto-activity-unavailable">
            Coinbase execution evidence is not available yet. Refresh to try the
            protected view-only connection.
          </p>
        ) : activity.fills.length === 0 ? (
          <p className="crypto-activity-unavailable">
            Coinbase reported no recent spot fills for this connected portfolio.
          </p>
        ) : (
          <div
            className="crypto-activity-table"
            role="region"
            aria-label="Recent Coinbase execution evidence"
            tabIndex={0}
          >
            <table>
              <thead>
                <tr>
                  <th>Executed</th>
                  <th>Market</th>
                  <th>Side</th>
                  <th>Provider price</th>
                  <th>Provider size</th>
                  <th>Commission</th>
                  <th>Liquidity</th>
                </tr>
              </thead>
              <tbody>
                {activity.fills.map((fill, index) => (
                  <tr key={`${fill.trade_time}-${fill.product_id}-${index}`}>
                    <td>{timestamp(fill.trade_time)}</td>
                    <td>
                      <strong>{fill.product_id}</strong>
                      <small>Executed outside Arbion</small>
                    </td>
                    <td>
                      <span
                        className={`crypto-activity-side ${fill.side.toLowerCase()}`}
                      >
                        {fill.side}
                      </span>
                    </td>
                    <td>
                      {exactDecimal(fill.price)} {fill.quote_currency}
                    </td>
                    <td>
                      {exactDecimal(fill.size)} {fill.size_unit}
                    </td>
                    <td>{exactMoney(fill.commission)}</td>
                    <td>{fill.liquidity.toLowerCase()}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
        <footer>
          Arbion requests only the first 50 spot fills and never exposes
          Coinbase order IDs, trade IDs, entry IDs, or pagination tokens.
        </footer>
      </motion.section>

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
