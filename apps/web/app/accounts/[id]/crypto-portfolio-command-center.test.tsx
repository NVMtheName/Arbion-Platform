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
        initialHistory={history}
        initialSnapshot={snapshot}
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
});
