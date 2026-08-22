"use client";

import { motion } from "motion/react";
import { useCallback, useEffect, useMemo, useState } from "react";

import {
  CoinbaseOrderPreview,
  type CoinbaseCapitalPolicy,
} from "./coinbase-order-preview";

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
    capabilities?: Record<string, "SUPPORTED" | "UNSUPPORTED" | "UNKNOWN">;
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

type CryptoBookLevel = {
  price: string;
  size: string;
};

export type CryptoLiquiditySnapshot = {
  symbol: string;
  currency: string;
  product_id: string;
  depth: 10;
  bids: CryptoBookLevel[];
  asks: CryptoBookLevel[];
  last: string;
  mid_market: string;
  spread_bps: string;
  spread_absolute: string;
  provenance: Provenance;
};

type LiquidityResponse = {
  liquidity?: CryptoLiquiditySnapshot;
  cached?: boolean;
  snapshot_semantics?: "SINGLE_VENUE_LIQUIDITY_SNAPSHOT";
  order_book_streaming: false;
  order_actions_available: false;
  live_execution_available: false;
  error?: { message?: string };
};

type CryptoPublicTrade = {
  price: string;
  size: string;
  time: string;
  side: "BUY" | "SELL";
};

export type CryptoPublicTradeTape = {
  symbol: string;
  currency: string;
  product_id: string;
  limit: 25;
  trades: CryptoPublicTrade[];
  best_bid: string;
  best_ask: string;
  provenance: Provenance;
};

type MarketTradesResponse = {
  market_trades?: CryptoPublicTradeTape;
  cached?: boolean;
  snapshot_semantics?: "PUBLIC_VENUE_TRADE_TAPE";
  trade_streaming: false;
  order_flow_inference: false;
  order_actions_available: false;
  live_execution_available: false;
  error?: { message?: string };
};

type SourceReceipt = {
  provider: string;
  feed: string;
  quality: string;
  venue?: string;
  received_at: string;
};

export type CryptoVenueStats = {
  symbol: string;
  currency: string;
  product_id: string;
  open: string;
  high: string;
  low: string;
  last: string;
  volume_24h: string;
  volume_30day: string;
  volume_unit: string;
  receipt: SourceReceipt;
};

type VenueStatsResponse = {
  venue_stats?: CryptoVenueStats;
  cached?: boolean;
  summary_semantics?: "ROLLING_SINGLE_VENUE_STATS";
  provider_event_time_available: false;
  timestamp_semantics?: "ARBION_RECEIPT_TIME";
  performance_claim: false;
  order_actions_available: false;
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

export type CoinbaseTradingCostSummary = {
  provider: "coinbase";
  feed: "advanced_trade_transaction_summary";
  product_type: "SPOT";
  pricing_tier: string;
  maker_fee_rate: string;
  taker_fee_rate: string;
  advanced_trade_volume: Money;
  advanced_trade_fees: Money;
  total_fees: Money;
  cost_plus_commission: boolean;
  retrieved_at: string;
};

type TradingCostResponse = {
  trading_costs?: CoinbaseTradingCostSummary;
  summary_semantics?: "PROVIDER_FEE_TIER_SNAPSHOT";
  order_preview_available: true;
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

function feeRate(value: string) {
  const parsed = Number(value);
  if (!Number.isFinite(parsed)) return value;
  return `${new Intl.NumberFormat("en-US", {
    minimumFractionDigits: 2,
    maximumFractionDigits: 6,
  }).format(parsed * 100)}%`;
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

function clockTime(value: string) {
  const parsed = new Date(value);
  if (Number.isNaN(parsed.valueOf())) return value;
  return new Intl.DateTimeFormat("en-US", {
    hour: "numeric",
    minute: "2-digit",
    second: "2-digit",
    fractionalSecondDigits: 3,
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

function bookLevelWidth(level: CryptoBookLevel, levels: CryptoBookLevel[]) {
  const current = Number(level.size);
  const maximum = Math.max(...levels.map((item) => Number(item.size) || 0));
  if (!Number.isFinite(current) || maximum <= 0) return 0;
  return Math.max(4, Math.min(100, (current / maximum) * 100));
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
  initialLiquidity,
  initialLiquidityCached = false,
  initialMarketTrades,
  initialMarketTradesCached = false,
  initialVenueStats,
  initialVenueStatsCached = false,
  initialActivity,
  initialOrderHistory,
  initialTradingCosts,
  capitalPolicies = [],
}: {
  accountID: string;
  initialSnapshot: CryptoPortfolioSnapshot;
  initialHistory?: CryptoCandleSeries;
  initialHistoryCached?: boolean;
  initialLiquidity?: CryptoLiquiditySnapshot;
  initialLiquidityCached?: boolean;
  initialMarketTrades?: CryptoPublicTradeTape;
  initialMarketTradesCached?: boolean;
  initialVenueStats?: CryptoVenueStats;
  initialVenueStatsCached?: boolean;
  initialActivity?: CoinbaseTradeActivity;
  initialOrderHistory?: CoinbaseOrderHistory;
  initialTradingCosts?: CoinbaseTradingCostSummary;
  capitalPolicies?: CoinbaseCapitalPolicy[];
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
  const [liquidities, setLiquidities] = useState<
    Record<string, CryptoLiquiditySnapshot>
  >(initialLiquidity ? { [initialLiquidity.symbol]: initialLiquidity } : {});
  const [liquidityCached, setLiquidityCached] = useState<
    Record<string, boolean>
  >(
    initialLiquidity
      ? { [initialLiquidity.symbol]: initialLiquidityCached }
      : {},
  );
  const [liquidityLoading, setLiquidityLoading] = useState(false);
  const [liquidityError, setLiquidityError] = useState("");
  const [marketTradeTapes, setMarketTradeTapes] = useState<
    Record<string, CryptoPublicTradeTape>
  >(
    initialMarketTrades
      ? { [initialMarketTrades.symbol]: initialMarketTrades }
      : {},
  );
  const [marketTradesCached, setMarketTradesCached] = useState<
    Record<string, boolean>
  >(
    initialMarketTrades
      ? { [initialMarketTrades.symbol]: initialMarketTradesCached }
      : {},
  );
  const [marketTradesLoading, setMarketTradesLoading] = useState(false);
  const [marketTradesError, setMarketTradesError] = useState("");
  const [venueStatsBySymbol, setVenueStatsBySymbol] = useState<
    Record<string, CryptoVenueStats>
  >(initialVenueStats ? { [initialVenueStats.symbol]: initialVenueStats } : {});
  const [venueStatsCached, setVenueStatsCached] = useState<
    Record<string, boolean>
  >(
    initialVenueStats
      ? { [initialVenueStats.symbol]: initialVenueStatsCached }
      : {},
  );
  const [venueStatsLoading, setVenueStatsLoading] = useState(false);
  const [venueStatsError, setVenueStatsError] = useState("");
  const [activity, setActivity] = useState(initialActivity);
  const [activityLoading, setActivityLoading] = useState(false);
  const [activityError, setActivityError] = useState("");
  const [orderHistory, setOrderHistory] = useState(initialOrderHistory);
  const [ordersLoading, setOrdersLoading] = useState(false);
  const [ordersError, setOrdersError] = useState("");
  const [tradingCosts, setTradingCosts] = useState(initialTradingCosts);
  const [costsLoading, setCostsLoading] = useState(false);
  const [costsError, setCostsError] = useState("");

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
  const selectedLiquidity = liquidities[selectedSymbol];
  const selectedMarketTrades = marketTradeTapes[selectedSymbol];
  const selectedVenueStats = venueStatsBySymbol[selectedSymbol];
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

  const loadLiquidity = useCallback(
    async (symbol: string, force = false) => {
      if (!symbol || (!force && liquidities[symbol])) return;
      setLiquidityLoading(true);
      setLiquidityError("");
      try {
        const response = await fetch(
          `/api/accounts/${encodeURIComponent(accountID)}/markets/crypto/${encodeURIComponent(symbol)}/liquidity`,
          { cache: "no-store" },
        );
        const body = (await response.json()) as LiquidityResponse;
        if (
          !response.ok ||
          !body.liquidity ||
          body.snapshot_semantics !== "SINGLE_VENUE_LIQUIDITY_SNAPSHOT" ||
          body.order_book_streaming !== false ||
          body.order_actions_available !== false ||
          body.live_execution_available !== false ||
          body.liquidity.symbol !== symbol ||
          body.liquidity.currency !== "USD" ||
          body.liquidity.product_id !== `${symbol}-USD` ||
          body.liquidity.depth !== 10 ||
          body.liquidity.bids.length > 10 ||
          body.liquidity.asks.length > 10
        ) {
          setLiquidityError(
            body.error?.message ??
              "Coinbase liquidity is temporarily unavailable. No depth or executable price was estimated.",
          );
          return;
        }
        setLiquidities((current) => ({
          ...current,
          [symbol]: body.liquidity as CryptoLiquiditySnapshot,
        }));
        setLiquidityCached((current) => ({
          ...current,
          [symbol]: Boolean(body.cached),
        }));
      } catch {
        setLiquidityError(
          "Coinbase liquidity is temporarily unavailable. No depth or executable price was estimated.",
        );
      } finally {
        setLiquidityLoading(false);
      }
    },
    [accountID, liquidities],
  );

  const loadMarketTrades = useCallback(
    async (symbol: string, force = false) => {
      if (!symbol || (!force && marketTradeTapes[symbol])) return;
      setMarketTradesLoading(true);
      setMarketTradesError("");
      try {
        const response = await fetch(
          `/api/accounts/${encodeURIComponent(accountID)}/markets/crypto/${encodeURIComponent(symbol)}/trades`,
          { cache: "no-store" },
        );
        const body = (await response.json()) as MarketTradesResponse;
        if (
          !response.ok ||
          !body.market_trades ||
          body.snapshot_semantics !== "PUBLIC_VENUE_TRADE_TAPE" ||
          body.trade_streaming !== false ||
          body.order_flow_inference !== false ||
          body.order_actions_available !== false ||
          body.live_execution_available !== false ||
          body.market_trades.symbol !== symbol ||
          body.market_trades.currency !== "USD" ||
          body.market_trades.product_id !== `${symbol}-USD` ||
          body.market_trades.limit !== 25 ||
          body.market_trades.trades.length > 25
        ) {
          setMarketTradesError(
            body.error?.message ??
              "Coinbase public market trades are temporarily unavailable. No trade flow was inferred.",
          );
          return;
        }
        setMarketTradeTapes((current) => ({
          ...current,
          [symbol]: body.market_trades as CryptoPublicTradeTape,
        }));
        setMarketTradesCached((current) => ({
          ...current,
          [symbol]: Boolean(body.cached),
        }));
      } catch {
        setMarketTradesError(
          "Coinbase public market trades are temporarily unavailable. No trade flow was inferred.",
        );
      } finally {
        setMarketTradesLoading(false);
      }
    },
    [accountID, marketTradeTapes],
  );

  const loadVenueStats = useCallback(
    async (symbol: string, force = false) => {
      if (!symbol || (!force && venueStatsBySymbol[symbol])) return;
      setVenueStatsLoading(true);
      setVenueStatsError("");
      try {
        const response = await fetch(
          `/api/accounts/${encodeURIComponent(accountID)}/markets/crypto/${encodeURIComponent(symbol)}/stats`,
          { cache: "no-store" },
        );
        const body = (await response.json()) as VenueStatsResponse;
        if (
          !response.ok ||
          !body.venue_stats ||
          body.summary_semantics !== "ROLLING_SINGLE_VENUE_STATS" ||
          body.provider_event_time_available !== false ||
          body.timestamp_semantics !== "ARBION_RECEIPT_TIME" ||
          body.performance_claim !== false ||
          body.order_actions_available !== false ||
          body.live_execution_available !== false ||
          body.venue_stats.symbol !== symbol ||
          body.venue_stats.currency !== "USD" ||
          body.venue_stats.product_id !== `${symbol}-USD` ||
          body.venue_stats.volume_unit !== symbol
        ) {
          setVenueStatsError(
            body.error?.message ??
              "Coinbase rolling venue statistics are temporarily unavailable. No window values were estimated.",
          );
          return;
        }
        setVenueStatsBySymbol((current) => ({
          ...current,
          [symbol]: body.venue_stats as CryptoVenueStats,
        }));
        setVenueStatsCached((current) => ({
          ...current,
          [symbol]: Boolean(body.cached),
        }));
      } catch {
        setVenueStatsError(
          "Coinbase rolling venue statistics are temporarily unavailable. No window values were estimated.",
        );
      } finally {
        setVenueStatsLoading(false);
      }
    },
    [accountID, venueStatsBySymbol],
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

  const refreshTradingCosts = useCallback(async () => {
    setCostsLoading(true);
    setCostsError("");
    try {
      const response = await fetch(
        `/api/accounts/${encodeURIComponent(accountID)}/activity/trading-costs`,
        { cache: "no-store" },
      );
      const body = (await response.json()) as TradingCostResponse;
      if (
        !response.ok ||
        !body.trading_costs ||
        body.summary_semantics !== "PROVIDER_FEE_TIER_SNAPSHOT" ||
        body.order_preview_available !== true ||
        body.order_actions_available !== false ||
        body.live_execution_available !== false ||
        body.trading_costs.provider !== "coinbase" ||
        body.trading_costs.feed !== "advanced_trade_transaction_summary" ||
        body.trading_costs.product_type !== "SPOT" ||
        body.trading_costs.advanced_trade_volume.currency !== "USD" ||
        body.trading_costs.advanced_trade_fees.currency !== "USD" ||
        body.trading_costs.total_fees.currency !== "USD"
      ) {
        setCostsError(
          body.error?.message ??
            "Coinbase trading-cost evidence is temporarily unavailable. No fee tier or cost estimate was inferred.",
        );
        return;
      }
      setTradingCosts(body.trading_costs);
    } catch {
      setCostsError(
        "Coinbase trading-cost evidence is temporarily unavailable. No fee tier or cost estimate was inferred.",
      );
    } finally {
      setCostsLoading(false);
    }
  }, [accountID]);

  const liveOrderCount =
    orderHistory?.orders.filter((order) =>
      ["PENDING", "OPEN", "QUEUED", "CANCEL_QUEUED", "EDIT_QUEUED"].includes(
        order.status,
      ),
    ).length ?? 0;
  const providerTradingAuthorized =
    snapshot.account.capabilities?.provider_trade_authorization === "SUPPORTED";

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
            connected trading command surface.
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
                  setLiquidityError("");
                  setMarketTradesError("");
                  setVenueStatsError("");
                  void loadHistory(position.symbol);
                  void loadLiquidity(position.symbol);
                  void loadMarketTrades(position.symbol);
                  void loadVenueStats(position.symbol);
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
        className="crypto-venue-stats-panel"
        aria-labelledby="crypto-venue-stats-title"
        initial={{ opacity: 0, y: 18 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ delay: 0.225 }}
      >
        <header>
          <div>
            <p className="eyebrow">ROLLING VENUE WINDOW</p>
            <h2 id="crypto-venue-stats-title">Coinbase range &amp; volume</h2>
            <p>
              Exact public Coinbase Exchange window values for the selected held
              asset—not portfolio return, performance, liquidity, or an
              executable market.
            </p>
          </div>
          <div className="crypto-venue-stats-actions">
            <span>
              {selectedSymbol ? `${selectedSymbol} / USD` : "No priced asset"}
            </span>
            <button
              type="button"
              disabled={!selectedSymbol || venueStatsLoading}
              onClick={() => void loadVenueStats(selectedSymbol, true)}
            >
              {venueStatsLoading ? "Refreshing…" : "Refresh venue window"}
            </button>
          </div>
        </header>

        {venueStatsError ? (
          <p className="crypto-venue-stats-unavailable" role="alert">
            {venueStatsError}
          </p>
        ) : !selectedVenueStats ? (
          <p className="crypto-venue-stats-unavailable">
            {venueStatsLoading
              ? "Loading provider-reported rolling values…"
              : "No Coinbase rolling venue statistics are available for this connected asset."}
          </p>
        ) : (
          <>
            <div className="crypto-venue-stats-grid">
              <article>
                <span>Window open</span>
                <strong>
                  {price(selectedVenueStats.open, selectedVenueStats.currency)}
                </strong>
                <small>{exactDecimal(selectedVenueStats.open)} USD</small>
              </article>
              <article>
                <span>Window high</span>
                <strong>
                  {price(selectedVenueStats.high, selectedVenueStats.currency)}
                </strong>
                <small>{exactDecimal(selectedVenueStats.high)} USD</small>
              </article>
              <article>
                <span>Window low</span>
                <strong>
                  {price(selectedVenueStats.low, selectedVenueStats.currency)}
                </strong>
                <small>{exactDecimal(selectedVenueStats.low)} USD</small>
              </article>
              <article>
                <span>Latest venue value</span>
                <strong>
                  {price(selectedVenueStats.last, selectedVenueStats.currency)}
                </strong>
                <small>{exactDecimal(selectedVenueStats.last)} USD</small>
              </article>
              <article>
                <span>24h base volume</span>
                <strong>{quantity(selectedVenueStats.volume_24h)}</strong>
                <small>
                  {exactDecimal(selectedVenueStats.volume_24h)}{" "}
                  {selectedVenueStats.volume_unit}
                </small>
              </article>
              <article>
                <span>30d base volume</span>
                <strong>{quantity(selectedVenueStats.volume_30day)}</strong>
                <small>
                  {exactDecimal(selectedVenueStats.volume_30day)}{" "}
                  {selectedVenueStats.volume_unit}
                </small>
              </article>
            </div>
            <div className="crypto-venue-stats-receipt">
              <span>Coinbase Exchange · public product stats</span>
              <span>
                Received by Arbion{" "}
                {timestamp(selectedVenueStats.receipt.received_at)}
              </span>
              <span>
                {venueStatsCached[selectedSymbol]
                  ? "30-second display cache"
                  : "Provider response"}
              </span>
            </div>
          </>
        )}
        <footer>
          Coinbase publishes these rolling values without an event timestamp.
          Arbion shows its receipt time only and does not relabel it as provider
          observation time. No change percentage, return, or trading signal is
          inferred here.
        </footer>
      </motion.section>

      <motion.section
        className="crypto-liquidity-panel"
        aria-labelledby="crypto-liquidity-title"
        initial={{ opacity: 0, y: 18 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ delay: 0.23 }}
      >
        <header>
          <div>
            <p className="eyebrow">COINBASE LIQUIDITY MAP</p>
            <h2 id="crypto-liquidity-title">Top-of-book depth</h2>
            <p>
              A ten-level, single-venue REST snapshot for the selected connected
              asset—not a streaming book, executable quote, or order preview.
            </p>
          </div>
          <div className="crypto-liquidity-actions">
            <span>
              {selectedSymbol ? `${selectedSymbol} / USD` : "No priced asset"}
            </span>
            <button
              type="button"
              disabled={!selectedSymbol || liquidityLoading}
              onClick={() => void loadLiquidity(selectedSymbol, true)}
            >
              {liquidityLoading ? "Refreshing…" : "Refresh liquidity"}
            </button>
          </div>
        </header>

        {liquidityError ? (
          <p className="crypto-liquidity-unavailable" role="alert">
            {liquidityError}
          </p>
        ) : !selectedLiquidity ? (
          <p className="crypto-liquidity-unavailable">
            {liquidityLoading
              ? "Loading provider-reported levels…"
              : "No Coinbase liquidity snapshot is available for this connected asset."}
          </p>
        ) : (
          <>
            <div className="crypto-liquidity-summary">
              <article>
                <span>Mid-market</span>
                <strong>
                  {price(
                    selectedLiquidity.mid_market,
                    selectedLiquidity.currency,
                  )}
                </strong>
                <small>{exactDecimal(selectedLiquidity.mid_market)} USD</small>
              </article>
              <article>
                <span>Venue spread</span>
                <strong>
                  {exactDecimal(selectedLiquidity.spread_bps)} bps
                </strong>
                <small>
                  {exactDecimal(selectedLiquidity.spread_absolute)} USD absolute
                </small>
              </article>
              <article>
                <span>Last trade</span>
                <strong>
                  {price(selectedLiquidity.last, selectedLiquidity.currency)}
                </strong>
                <small>{exactDecimal(selectedLiquidity.last)} USD</small>
              </article>
              <article>
                <span>Observed</span>
                <strong>
                  {timestamp(selectedLiquidity.provenance.provider_timestamp)}
                </strong>
                <small>
                  {liquidityCached[selectedSymbol]
                    ? "One-second display cache"
                    : "Provider response"}
                </small>
              </article>
            </div>
            <div
              className="crypto-book"
              aria-label={`${selectedLiquidity.product_id} Coinbase liquidity levels`}
            >
              <section className="crypto-book-side bid">
                <header>
                  <div>
                    <span>Bids</span>
                    <small>Buy-side interest</small>
                  </div>
                  <div aria-hidden="true">
                    <span>Price</span>
                    <span>Size {selectedLiquidity.symbol}</span>
                  </div>
                </header>
                <ol>
                  {selectedLiquidity.bids.map((level, index) => (
                    <li key={`bid-${level.price}-${index}`}>
                      <i
                        style={{
                          width: `${bookLevelWidth(level, selectedLiquidity.bids)}%`,
                        }}
                      />
                      <span>{exactDecimal(level.price)}</span>
                      <strong>{exactDecimal(level.size)}</strong>
                    </li>
                  ))}
                </ol>
              </section>
              <section className="crypto-book-side ask">
                <header>
                  <div>
                    <span>Asks</span>
                    <small>Sell-side interest</small>
                  </div>
                  <div aria-hidden="true">
                    <span>Price</span>
                    <span>Size {selectedLiquidity.symbol}</span>
                  </div>
                </header>
                <ol>
                  {selectedLiquidity.asks.map((level, index) => (
                    <li key={`ask-${level.price}-${index}`}>
                      <i
                        style={{
                          width: `${bookLevelWidth(level, selectedLiquidity.asks)}%`,
                        }}
                      />
                      <span>{exactDecimal(level.price)}</span>
                      <strong>{exactDecimal(level.size)}</strong>
                    </li>
                  ))}
                </ol>
              </section>
            </div>
          </>
        )}
        <footer>
          Coinbase Advanced Trade public REST · ten levels per side · manual
          refresh. Size is provider-reported base-asset quantity at one price
          level. Arbion does not stream, aggregate venues, estimate slippage, or
          expose order actions from this panel.
        </footer>
      </motion.section>

      <motion.section
        className="crypto-market-tape"
        aria-labelledby="crypto-market-tape-title"
        initial={{ opacity: 0, y: 18 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ delay: 0.235 }}
      >
        <header>
          <div>
            <p className="eyebrow">PUBLIC TIME &amp; SALES</p>
            <h2 id="crypto-market-tape-title">Latest Coinbase venue ticks</h2>
            <p>
              The newest 25 public trades for the selected held asset. Side is
              reported by Coinbase; Arbion does not infer aggressor flow,
              sentiment, execution quality, cost basis, or P&amp;L.
            </p>
          </div>
          <div className="crypto-market-tape-actions">
            <span>
              {selectedSymbol ? `${selectedSymbol} / USD` : "No priced asset"}
            </span>
            <button
              type="button"
              disabled={!selectedSymbol || marketTradesLoading}
              onClick={() => void loadMarketTrades(selectedSymbol, true)}
            >
              {marketTradesLoading ? "Refreshing…" : "Refresh public trades"}
            </button>
          </div>
        </header>

        {marketTradesError ? (
          <p className="crypto-market-tape-unavailable" role="alert">
            {marketTradesError}
          </p>
        ) : !selectedMarketTrades ? (
          <p className="crypto-market-tape-unavailable">
            {marketTradesLoading
              ? "Loading provider-reported public trades…"
              : "No Coinbase public trade tape is available for this connected asset."}
          </p>
        ) : (
          <>
            <div className="crypto-market-tape-summary">
              <div>
                <span>Best bid</span>
                <strong>
                  {price(
                    selectedMarketTrades.best_bid,
                    selectedMarketTrades.currency,
                  )}
                </strong>
                <small>{exactDecimal(selectedMarketTrades.best_bid)} USD</small>
              </div>
              <div>
                <span>Best ask</span>
                <strong>
                  {price(
                    selectedMarketTrades.best_ask,
                    selectedMarketTrades.currency,
                  )}
                </strong>
                <small>{exactDecimal(selectedMarketTrades.best_ask)} USD</small>
              </div>
              <div>
                <span>Ticks shown</span>
                <strong>
                  {selectedMarketTrades.trades.length}/
                  {selectedMarketTrades.limit}
                </strong>
                <small>Newest provider snapshot</small>
              </div>
              <div>
                <span>Observed</span>
                <strong>
                  {timestamp(
                    selectedMarketTrades.provenance.provider_timestamp,
                  )}
                </strong>
                <small>
                  {marketTradesCached[selectedSymbol]
                    ? "One-second display cache"
                    : "Provider response"}
                </small>
              </div>
            </div>
            <div
              className="crypto-market-tape-table"
              role="region"
              aria-label={`${selectedMarketTrades.product_id} public Coinbase time and sales`}
              tabIndex={0}
            >
              <table>
                <thead>
                  <tr>
                    <th>Provider time</th>
                    <th>Reported side</th>
                    <th>Price USD</th>
                    <th>Size {selectedMarketTrades.symbol}</th>
                  </tr>
                </thead>
                <tbody>
                  {selectedMarketTrades.trades.map((trade, index) => (
                    <tr key={`${trade.time}-${index}`}>
                      <td>{clockTime(trade.time)}</td>
                      <td>
                        <span
                          className={`crypto-public-trade-side ${trade.side.toLowerCase()}`}
                        >
                          {trade.side}
                        </span>
                      </td>
                      <td>{exactDecimal(trade.price)}</td>
                      <td>{exactDecimal(trade.size)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </>
        )}
        <footer>
          Coinbase Advanced Trade public REST · 25 newest ticks · manual
          refresh. Trade identifiers, per-tick bid/ask copies, cursors, and
          arbitrary time ranges are discarded. This panel has no streaming or
          order path.
        </footer>
      </motion.section>

      <motion.section
        className="crypto-cost-panel"
        aria-labelledby="crypto-cost-title"
        initial={{ opacity: 0, y: 18 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ delay: 0.23 }}
      >
        <header>
          <div>
            <p className="eyebrow">TRADING COST INTELLIGENCE</p>
            <h2 id="crypto-cost-title">Current Coinbase fee tier</h2>
            <p>
              Provider-reported spot volume, fees, and maker/taker rates. This
              snapshot is not an order preview, quote, tax record, cost basis,
              or performance calculation.
            </p>
          </div>
          <button
            type="button"
            disabled={costsLoading}
            onClick={() => void refreshTradingCosts()}
          >
            {costsLoading ? "Refreshing…" : "Refresh cost evidence"}
          </button>
        </header>

        {costsError ? (
          <p className="crypto-cost-unavailable" role="alert">
            {costsError}
          </p>
        ) : !tradingCosts ? (
          <p className="crypto-cost-unavailable">
            Coinbase fee-tier evidence is not available yet. Refresh to try the
            protected private account connection.
          </p>
        ) : (
          <div className="crypto-cost-workspace">
            <article className="crypto-cost-tier">
              <span>Current pricing tier</span>
              <strong>{tradingCosts.pricing_tier}</strong>
              <small>Spot · Coinbase Advanced Trade</small>
              <dl>
                <div>
                  <dt>Pricing model</dt>
                  <dd>
                    {tradingCosts.cost_plus_commission
                      ? "Cost plus commission"
                      : "Standard tier"}
                  </dd>
                </div>
                <div>
                  <dt>Retrieved</dt>
                  <dd>{timestamp(tradingCosts.retrieved_at)}</dd>
                </div>
                <div>
                  <dt>Arbion order preview</dt>
                  <dd>Unavailable</dd>
                </div>
              </dl>
            </article>
            <div className="crypto-cost-metrics">
              <article>
                <span>Maker fee rate</span>
                <strong>{feeRate(tradingCosts.maker_fee_rate)}</strong>
                <small>
                  Exact provider rate{" "}
                  {exactDecimal(tradingCosts.maker_fee_rate)}
                </small>
              </article>
              <article>
                <span>Taker fee rate</span>
                <strong>{feeRate(tradingCosts.taker_fee_rate)}</strong>
                <small>
                  Exact provider rate{" "}
                  {exactDecimal(tradingCosts.taker_fee_rate)}
                </small>
              </article>
              <article>
                <span>Advanced Trade volume</span>
                <strong>{money(tradingCosts.advanced_trade_volume)}</strong>
                <small>{exactMoney(tradingCosts.advanced_trade_volume)}</small>
              </article>
              <article>
                <span>Advanced Trade fees</span>
                <strong>{money(tradingCosts.advanced_trade_fees)}</strong>
                <small>{exactMoney(tradingCosts.advanced_trade_fees)}</small>
              </article>
              <article className="crypto-cost-total">
                <span>Provider total fees</span>
                <strong>{money(tradingCosts.total_fees)}</strong>
                <small>{exactMoney(tradingCosts.total_fees)}</small>
              </article>
            </div>
          </div>
        )}
        <footer>
          Coinbase defines the applicable tier and volume basis. Arbion does not
          infer next-tier progress, forecast fees, or submit a preview or order
          from this evidence.
        </footer>
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
            protected private account connection.
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
            protected private account connection.
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

      <CoinbaseOrderPreview
        accountID={accountID}
        capitalPolicies={capitalPolicies}
        symbols={snapshot.positions.map((position) => position.symbol)}
        tradingAuthorized={providerTradingAuthorized}
      />

      <p className="crypto-safety-band">
        <strong>CONNECTED · EXECUTION LOCKED</strong>
        <span>
          Arbion can sync your private Coinbase account and request real order
          previews. It cannot submit, cancel, convert, deposit, withdraw, or
          transfer assets until the execution-control gate is implemented.
        </span>
      </p>
    </section>
  );
}
