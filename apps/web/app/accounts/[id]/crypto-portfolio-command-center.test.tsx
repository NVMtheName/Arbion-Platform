import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import {
  CryptoPortfolioCommandCenter,
  type CoinbaseOrderHistory,
  type CoinbaseTradeActivity,
  type CoinbaseTradingCostSummary,
  type CryptoCandleSeries,
  type CryptoPortfolioSnapshot,
} from "./crypto-portfolio-command-center";

const observedAt = "2026-08-21T14:30:00Z";
const snapshot: CryptoPortfolioSnapshot = {
  account: {
    id: "coinbase-1",
    display_name: "Coinbase Portfolio",
    provider: "coinbase",
    status: "active",
  },
  balances: {
    cash: { amount: "25", currency: "USD" },
    available_cash: { amount: "20", currency: "USD" },
  },
  observed_value: { amount: "30025", currency: "USD" },
  digital_asset_value: { amount: "30000", currency: "USD" },
  positions: [
    {
      symbol: "BTC",
      quantity: "0.5",
      unit_price: { amount: "60000", currency: "USD" },
      bid: { amount: "59999", currency: "USD" },
      ask: { amount: "60001", currency: "USD" },
      market_value: { amount: "30000", currency: "USD" },
      pricing_status: "PRICED",
      provenance: {
        provider: "coinbase",
        feed: "rest_ticker",
        quality: "REAL_TIME_SINGLE_VENUE",
        venue: "coinbase_exchange",
        provider_timestamp: observedAt,
        received_at: observedAt,
      },
    },
    {
      symbol: "RARE",
      quantity: "10",
      pricing_status: "UNAVAILABLE",
    },
  ],
  priced_positions: 1,
  total_positions: 2,
  pricing_complete: false,
  pricing_state: "PARTIAL",
  pricing_basis: "LAST_TRADE",
  pricing_message:
    "Some assets do not have an approved Coinbase Exchange USD ticker.",
  pricing_as_of: observedAt,
  market_data_cached: false,
};

const activity: CoinbaseTradeActivity = {
  provider: "coinbase",
  feed: "advanced_trade_fills",
  fills: [
    {
      product_id: "BTC-USD",
      base_asset: "BTC",
      quote_currency: "USD",
      side: "BUY",
      price: "60123.123456789",
      size: "0.000000010000",
      size_unit: "BTC",
      commission: { amount: "0.00000001", currency: "USD" },
      trade_time: "2026-08-21T14:29:00Z",
      liquidity: "MAKER",
    },
  ],
  has_more: true,
  retrieved_at: observedAt,
};

const orderHistory: CoinbaseOrderHistory = {
  provider: "coinbase",
  feed: "advanced_trade_orders",
  orders: [
    {
      product_id: "BTC-USD",
      base_asset: "BTC",
      quote_currency: "USD",
      side: "BUY",
      status: "OPEN",
      order_type: "LIMIT",
      time_in_force: "GOOD_UNTIL_CANCELLED",
      completion_percentage: "25.000",
      filled_size: "0.000000010000",
      filled_size_unit: "BTC",
      filled_value: { amount: "0.00060123123456789", currency: "USD" },
      average_filled_price: { amount: "60123.123456789", currency: "USD" },
      total_fees: { amount: "0.00000001", currency: "USD" },
      number_of_fills: 1,
      pending_cancel: false,
      settled: false,
      is_liquidation: false,
      outcome_reason: "NONE",
      created_at: "2026-08-21T14:20:00Z",
      last_fill_at: "2026-08-21T14:29:00Z",
    },
  ],
  has_more: true,
  retrieved_at: observedAt,
};

const tradingCosts: CoinbaseTradingCostSummary = {
  provider: "coinbase",
  feed: "advanced_trade_transaction_summary",
  product_type: "SPOT",
  pricing_tier: "<$10k",
  maker_fee_rate: "0.0020",
  taker_fee_rate: "0.0030",
  advanced_trade_volume: { amount: "1000.123456789", currency: "USD" },
  advanced_trade_fees: { amount: "20.00000001", currency: "USD" },
  total_fees: { amount: "25.00000001", currency: "USD" },
  cost_plus_commission: false,
  retrieved_at: observedAt,
};

const history: CryptoCandleSeries = {
  symbol: "BTC",
  currency: "USD",
  granularity_seconds: 900,
  expected_intervals: 96,
  candles: [
    {
      start: "2026-08-21T14:00:00Z",
      low: "59000",
      high: "60500",
      open: "60000",
      close: "60100",
      volume: "10",
    },
    {
      start: "2026-08-21T14:30:00Z",
      low: "60000",
      high: "61000",
      open: "60200",
      close: "60500",
      volume: "12",
    },
  ],
  provenance: {
    provider: "coinbase",
    feed: "rest_candles",
    quality: "REAL_TIME_SINGLE_VENUE",
    venue: "coinbase_exchange",
    provider_timestamp: observedAt,
    received_at: observedAt,
  },
};

describe("CryptoPortfolioCommandCenter", () => {
  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
  });

  it("shows observed value, source-stamped chart gaps, and the read-only boundary", () => {
    const { container } = render(
      <CryptoPortfolioCommandCenter
        accountID="coinbase-1"
        initialActivity={activity}
        initialHistory={history}
        initialOrderHistory={orderHistory}
        initialSnapshot={snapshot}
        initialTradingCosts={tradingCosts}
      />,
    );

    expect(
      screen.getByRole("heading", { name: "Coinbase Portfolio" }),
    ).toBeInTheDocument();
    expect(screen.getByText("$30,025.00")).toBeInTheDocument();
    expect(screen.getByText("1/2")).toBeInTheDocument();
    expect(screen.getByText("RARE")).toBeInTheDocument();
    expect(screen.getByText("No approved USD product")).toBeInTheDocument();
    expect(screen.getByText("coinbase exchange")).toBeInTheDocument();
    expect(screen.getByText("READ-ONLY BY DESIGN")).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { name: "24h venue movement" }),
    ).toBeInTheDocument();
    expect(screen.getByText("2/96 intervals")).toBeInTheDocument();
    expect(screen.getByText(/not portfolio performance/)).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { name: "Recent external fills" }),
    ).toBeInTheDocument();
    expect(screen.getAllByText("60,123.123456789 USD").length).toBeGreaterThan(
      0,
    );
    expect(screen.getAllByText("0.000000010000 BTC").length).toBeGreaterThan(0);
    expect(screen.getByText("Executed outside Arbion")).toBeInTheDocument();
    expect(
      screen.getByText(/never exposes Coinbase order IDs/),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { name: "Provider-reported order state" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { name: "Current Coinbase fee tier" }),
    ).toBeInTheDocument();
    expect(screen.getByText("<$10k")).toBeInTheDocument();
    expect(screen.getByText("0.20%")).toBeInTheDocument();
    expect(screen.getByText("0.30%")).toBeInTheDocument();
    expect(screen.getByText("1,000.123456789 USD")).toBeInTheDocument();
    expect(screen.getByText("20.00000001 USD")).toBeInTheDocument();
    expect(screen.getAllByText("Unavailable").length).toBeGreaterThan(0);
    expect(
      screen.getByText(/does not infer next-tier progress/),
    ).toBeInTheDocument();
    expect(
      screen.getByText("25.000% · 0.000000010000 BTC"),
    ).toBeInTheDocument();
    expect(screen.getByText("0.00060123123456789 USD")).toBeInTheDocument();
    expect(screen.getByText("None · monitor only")).toBeInTheDocument();
    expect(
      screen.getByText(/Coinbase order IDs, user IDs/),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("img", {
        name: /BTC Coinbase Exchange 24-hour price movement/,
      }),
    ).toBeInTheDocument();
    expect(
      container.querySelectorAll(".crypto-history-chart path"),
    ).toHaveLength(2);
    expect(
      screen.getByText(/cannot place orders, convert assets/),
    ).toBeInTheDocument();
  });

  it("refreshes through only the protected read-only portfolio endpoint", async () => {
    const updated = {
      ...snapshot,
      observed_value: { amount: "31025", currency: "USD" } as const,
    };
    const request = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({
        portfolio: updated,
        live_execution_available: false,
      }),
    });
    vi.stubGlobal("fetch", request);
    render(
      <CryptoPortfolioCommandCenter
        accountID="coinbase-1"
        initialHistory={history}
        initialSnapshot={snapshot}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Refresh portfolio" }));

    await waitFor(() => expect(screen.getByText("$31,025.00")).toBeVisible());
    expect(request).toHaveBeenCalledWith(
      "/api/accounts/coinbase-1/portfolio/crypto",
      { cache: "no-store" },
    );
  });

  it("refreshes history through the connected account read route", async () => {
    const updatedHistory = {
      ...history,
      candles: history.candles.map((candle, index) =>
        index === history.candles.length - 1
          ? { ...candle, close: "60600" }
          : candle,
      ),
    };
    const request = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({
        history: updatedHistory,
        cached: true,
        chart_semantics: "VENUE_PRICE_MOVEMENT",
        live_execution_available: false,
      }),
    });
    vi.stubGlobal("fetch", request);
    render(
      <CryptoPortfolioCommandCenter
        accountID="coinbase-1"
        initialHistory={history}
        initialSnapshot={snapshot}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Refresh history" }));

    await waitFor(() => expect(screen.getByText("$60,600.00")).toBeVisible());
    expect(request).toHaveBeenCalledWith(
      "/api/accounts/coinbase-1/markets/crypto/BTC/candles",
      { cache: "no-store" },
    );
  });

  it("refreshes only normalized external execution evidence", async () => {
    const updatedActivity = {
      ...activity,
      fills: [{ ...activity.fills[0], side: "SELL" as const }],
    };
    const request = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({
        activity: updatedActivity,
        history_semantics: "EXTERNAL_EXECUTION_EVIDENCE",
        live_execution_available: false,
      }),
    });
    vi.stubGlobal("fetch", request);
    render(
      <CryptoPortfolioCommandCenter
        accountID="coinbase-1"
        initialActivity={activity}
        initialHistory={history}
        initialSnapshot={snapshot}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Refresh activity" }));

    await waitFor(() => expect(screen.getByText("SELL")).toBeVisible());
    expect(request).toHaveBeenCalledWith(
      "/api/accounts/coinbase-1/activity/fills",
      { cache: "no-store" },
    );
  });

  it("refreshes provider order state without exposing order actions", async () => {
    const updatedOrders = {
      ...orderHistory,
      orders: [
        {
          ...orderHistory.orders[0],
          status: "FILLED" as const,
          completion_percentage: "100",
          settled: true,
        },
      ],
    };
    const request = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({
        orders: updatedOrders,
        history_semantics: "EXTERNAL_ORDER_STATUS",
        order_actions_available: false,
        live_execution_available: false,
      }),
    });
    vi.stubGlobal("fetch", request);
    render(
      <CryptoPortfolioCommandCenter
        accountID="coinbase-1"
        initialActivity={activity}
        initialHistory={history}
        initialOrderHistory={orderHistory}
        initialSnapshot={snapshot}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Refresh orders" }));

    await waitFor(() => expect(screen.getByText("FILLED")).toBeVisible());
    expect(request).toHaveBeenCalledWith(
      "/api/accounts/coinbase-1/activity/orders",
      { cache: "no-store" },
    );
    expect(screen.queryByRole("button", { name: /cancel/i })).toBeNull();
  });

  it("refreshes only provider fee-tier evidence without requesting a preview", async () => {
    const updatedCosts = {
      ...tradingCosts,
      pricing_tier: "$10k-$50k",
      maker_fee_rate: "0.0015",
    };
    const request = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({
        trading_costs: updatedCosts,
        summary_semantics: "PROVIDER_FEE_TIER_SNAPSHOT",
        order_preview_available: false,
        order_actions_available: false,
        live_execution_available: false,
      }),
    });
    vi.stubGlobal("fetch", request);
    render(
      <CryptoPortfolioCommandCenter
        accountID="coinbase-1"
        initialHistory={history}
        initialSnapshot={snapshot}
        initialTradingCosts={tradingCosts}
      />,
    );

    fireEvent.click(
      screen.getByRole("button", { name: "Refresh cost evidence" }),
    );

    await waitFor(() => expect(screen.getByText("$10k-$50k")).toBeVisible());
    expect(request).toHaveBeenCalledWith(
      "/api/accounts/coinbase-1/activity/trading-costs",
      { cache: "no-store" },
    );
    expect(screen.queryByRole("button", { name: /preview|trade/i })).toBeNull();
  });
});
