import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { hydrateRoot } from "react-dom/client";
import { renderToString } from "react-dom/server";

import {
  CryptoPortfolioCommandCenter,
  type CoinbaseOrderHistory,
  type CoinbaseTradeActivity,
  type CoinbaseTradingCostSummary,
  type CryptoCandleSeries,
  type CryptoLiquiditySnapshot,
  type CryptoPublicTradeTape,
  type CryptoPortfolioSnapshot,
  type CryptoVenueStats,
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
      available_quantity: "0.3",
      unavailable_to_trade_quantity: "0.2",
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
      available_quantity: "0",
      unavailable_to_trade_quantity: "10",
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

const liquidity: CryptoLiquiditySnapshot = {
  symbol: "BTC",
  currency: "USD",
  product_id: "BTC-USD",
  depth: 10,
  bids: [
    { price: "60122.900000000001", size: "0.12500000" },
    { price: "60122.80", size: "0.5" },
  ],
  asks: [
    { price: "60123.20", size: "0.25000000" },
    { price: "60123.30", size: "0.75" },
  ],
  last: "60123.10",
  mid_market: "60123.05",
  spread_bps: "0.049897",
  spread_absolute: "0.30",
  provenance: {
    provider: "coinbase",
    feed: "advanced_trade_public_product_book",
    quality: "REAL_TIME_SINGLE_VENUE",
    venue: "coinbase_advanced_trade",
    provider_timestamp: observedAt,
    received_at: observedAt,
  },
};

const marketTrades: CryptoPublicTradeTape = {
  symbol: "BTC",
  currency: "USD",
  product_id: "BTC-USD",
  limit: 25,
  trades: [
    {
      price: "60123.123456789",
      size: "0.00000001",
      time: observedAt,
      side: "BUY",
    },
    {
      price: "60122.90",
      size: "0.12500000",
      time: "2026-08-21T14:29:59Z",
      side: "SELL",
    },
  ],
  best_bid: "60122.90",
  best_ask: "60123.20",
  provenance: {
    provider: "coinbase",
    feed: "advanced_trade_public_market_trades",
    quality: "REAL_TIME_SINGLE_VENUE",
    venue: "coinbase_advanced_trade",
    provider_timestamp: observedAt,
    received_at: observedAt,
  },
};

const venueStats: CryptoVenueStats = {
  symbol: "BTC",
  currency: "USD",
  product_id: "BTC-USD",
  open: "59000.00000000",
  high: "61000.00000000",
  low: "58000.00000000",
  last: "60123.123456789",
  volume_24h: "19734.31498542",
  volume_30day: "189836.08275489",
  volume_unit: "BTC",
  receipt: {
    provider: "coinbase",
    feed: "exchange_public_product_stats",
    quality: "REAL_TIME_SINGLE_VENUE",
    venue: "coinbase_exchange",
    received_at: observedAt,
  },
};

describe("CryptoPortfolioCommandCenter", () => {
  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
  });

  it("shows observed value, source-stamped chart gaps, and the execution lock", () => {
    const { container } = render(
      <CryptoPortfolioCommandCenter
        accountID="coinbase-1"
        initialActivity={activity}
        initialHistory={history}
        initialLiquidity={liquidity}
        initialMarketTrades={marketTrades}
        initialVenueStats={venueStats}
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
    expect(
      screen.getByRole("columnheader", { name: "Available to trade" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("columnheader", { name: "Staked / unavailable" }),
    ).toBeInTheDocument();
    expect(screen.getByText("0.2")).toBeInTheDocument();
    expect(
      screen.getByText(
        /Total holdings include Coinbase App wallets and vaults/,
      ),
    ).toBeInTheDocument();
    expect(screen.getByText("No approved USD product")).toBeInTheDocument();
    expect(screen.getByText("coinbase exchange")).toBeInTheDocument();
    expect(
      screen.getByText("CONNECTED · EXECUTION LOCKED"),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { name: "24h venue movement" }),
    ).toBeInTheDocument();
    expect(screen.getByText("2/96 intervals")).toBeInTheDocument();
    expect(screen.getByText(/not portfolio performance/)).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { name: "Top-of-book depth" }),
    ).toBeInTheDocument();
    expect(screen.getByText("60,122.900000000001")).toBeInTheDocument();
    expect(screen.getByText("0.049897 bps")).toBeInTheDocument();
    expect(
      screen.getByText(/does not stream, aggregate venues/),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { name: "Latest Coinbase venue ticks" }),
    ).toBeInTheDocument();
    expect(screen.getByText("2/25")).toBeInTheDocument();
    expect(
      screen.getByText(/does not infer aggressor flow/),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { name: "Coinbase range & volume" }),
    ).toBeInTheDocument();
    expect(screen.getByText("189,836.08275489 BTC")).toBeInTheDocument();
    expect(
      screen.getByText(/does not relabel it as provider observation time/),
    ).toBeInTheDocument();
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
      screen.getByText(/cannot submit, cancel, convert/),
    ).toBeInTheDocument();
  });

  it("includes USDC in value and allocation without treating its redemption reference as a venue market", () => {
    const usdcSnapshot: CryptoPortfolioSnapshot = {
      ...snapshot,
      observed_value: { amount: "47565.0263083", currency: "USD" },
      digital_asset_value: { amount: "47540.0263083", currency: "USD" },
      positions: [
        {
          symbol: "USDC",
          quantity: "17540.0263083",
          available_quantity: "5.3263083",
          unavailable_to_trade_quantity: "17534.7",
          unit_price: { amount: "1", currency: "USD" },
          market_value: { amount: "17540.0263083", currency: "USD" },
          pricing_status: "PRICED",
          valuation_basis: "COINBASE_USDC_USD_REDEMPTION",
        },
        snapshot.positions[0],
      ],
      priced_positions: 2,
      total_positions: 2,
      pricing_complete: true,
      pricing_state: "READY",
      pricing_basis: "LAST_TRADE_AND_USDC_USD_REDEMPTION",
      pricing_message:
        "Coinbase Exchange last trades are combined with Coinbase's 1:1 USDC-to-USD redemption reference.",
    };

    render(
      <CryptoPortfolioCommandCenter
        accountID="coinbase-1"
        initialHistory={history}
        initialSnapshot={usdcSnapshot}
      />,
    );

    expect(screen.getByText("$47,565.03")).toBeInTheDocument();
    expect(screen.getByText("17,540.0263083")).toBeInTheDocument();
    expect(screen.getByText("17,534.7")).toBeInTheDocument();
    expect(
      screen.getByText("Coinbase USDC · 1:1 USD redemption reference"),
    ).toBeInTheDocument();
    expect(screen.getByText("Last trade + USDC 1:1")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "BTC" })).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "USDC" }),
    ).not.toBeInTheDocument();
  });

  it("renders canonical UTC evidence before switching to the browser timezone", () => {
    const serverHTML = renderToString(
      <CryptoPortfolioCommandCenter
        accountID="coinbase-1"
        initialHistory={history}
        initialSnapshot={snapshot}
      />,
    );

    expect(serverHTML).toContain("Aug 21, 2:30:00 PM UTC");
    expect(serverHTML).not.toContain("Aug 21, 10:30:00 AM EDT");
  });

  it("hydrates server-rendered portfolio evidence without a recoverable mismatch", async () => {
    const view = (
      <CryptoPortfolioCommandCenter
        accountID="coinbase-1"
        initialHistory={history}
        initialSnapshot={snapshot}
      />
    );
    const host = document.createElement("div");
    host.innerHTML = renderToString(view);
    document.body.append(host);
    const onRecoverableError = vi.fn();

    const root = hydrateRoot(host, view, { onRecoverableError });
    await act(async () => undefined);

    expect(onRecoverableError).not.toHaveBeenCalled();
    await act(async () => root.unmount());
    host.remove();
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

  it("refreshes a bounded keyless liquidity snapshot without exposing order actions", async () => {
    const updatedLiquidity = {
      ...liquidity,
      spread_bps: "0.059000",
      bids: [{ price: "60122.85", size: "0.75" }],
    };
    const request = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({
        liquidity: updatedLiquidity,
        cached: false,
        snapshot_semantics: "SINGLE_VENUE_LIQUIDITY_SNAPSHOT",
        order_book_streaming: false,
        order_actions_available: false,
        live_execution_available: false,
      }),
    });
    vi.stubGlobal("fetch", request);
    render(
      <CryptoPortfolioCommandCenter
        accountID="coinbase-1"
        initialHistory={history}
        initialLiquidity={liquidity}
        initialSnapshot={snapshot}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Refresh liquidity" }));

    await waitFor(() => expect(screen.getByText("0.059000 bps")).toBeVisible());
    expect(request).toHaveBeenCalledWith(
      "/api/accounts/coinbase-1/markets/crypto/BTC/liquidity",
      { cache: "no-store" },
    );
    expect(
      screen.queryByRole("button", {
        name: /^(buy|sell|trade|place order|cancel order)$/i,
      }),
    ).toBeNull();
  });

  it("refreshes a bounded public trade tape without requesting IDs or flow inference", async () => {
    const updatedTrades = {
      ...marketTrades,
      trades: [{ ...marketTrades.trades[0], side: "SELL" as const }],
    };
    const request = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({
        market_trades: updatedTrades,
        cached: false,
        snapshot_semantics: "PUBLIC_VENUE_TRADE_TAPE",
        trade_streaming: false,
        order_flow_inference: false,
        order_actions_available: false,
        live_execution_available: false,
      }),
    });
    vi.stubGlobal("fetch", request);
    render(
      <CryptoPortfolioCommandCenter
        accountID="coinbase-1"
        initialHistory={history}
        initialMarketTrades={marketTrades}
        initialSnapshot={snapshot}
      />,
    );

    fireEvent.click(
      screen.getByRole("button", { name: "Refresh public trades" }),
    );

    await waitFor(() => expect(screen.getByText("1/25")).toBeVisible());
    expect(request).toHaveBeenCalledWith(
      "/api/accounts/coinbase-1/markets/crypto/BTC/trades",
      { cache: "no-store" },
    );
    expect(screen.queryByText(/trade_id|sentiment score/i)).toBeNull();
  });

  it("refreshes exact rolling venue stats without inventing event time or performance", async () => {
    const updatedStats = {
      ...venueStats,
      last: "60200.123456789",
      volume_24h: "20000.00000001",
    };
    const request = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({
        venue_stats: updatedStats,
        cached: true,
        summary_semantics: "ROLLING_SINGLE_VENUE_STATS",
        provider_event_time_available: false,
        timestamp_semantics: "ARBION_RECEIPT_TIME",
        performance_claim: false,
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
        initialVenueStats={venueStats}
      />,
    );

    fireEvent.click(
      screen.getByRole("button", { name: "Refresh venue window" }),
    );

    await waitFor(() =>
      expect(screen.getByText("60,200.123456789 USD")).toBeVisible(),
    );
    expect(request).toHaveBeenCalledWith(
      "/api/accounts/coinbase-1/markets/crypto/BTC/stats",
      { cache: "no-store" },
    );
    expect(
      screen.queryByText(/provider event time|performance score/i),
    ).toBeNull();
    expect(
      screen.queryByRole("button", {
        name: /^(buy|sell|trade|place order|cancel order)$/i,
      }),
    ).toBeNull();
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
        order_preview_available: true,
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
    expect(
      screen.queryByRole("button", { name: /preview order|place order/i }),
    ).toBeNull();
  });
});
