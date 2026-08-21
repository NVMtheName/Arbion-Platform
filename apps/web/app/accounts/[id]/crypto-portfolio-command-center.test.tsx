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

describe("CryptoPortfolioCommandCenter", () => {
  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
  });

  it("shows observed value, coverage gaps, provenance, and the read-only boundary", () => {
    render(
      <CryptoPortfolioCommandCenter
        accountID="coinbase-1"
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
});
