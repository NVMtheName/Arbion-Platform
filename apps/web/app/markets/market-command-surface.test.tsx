import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { MarketCommandSurface } from "./market-command-surface";
import type { MarketSource } from "./market-source-grid";

const enabledSources: MarketSource[] = [
  {
    id: "alpaca_iex",
    label: "Alpaca IEX",
    role: "MARKET_OBSERVATION",
    feed: "iex",
    quality: "REAL_TIME_SINGLE_VENUE",
    capabilities: ["EQUITY_QUOTE"],
    enabled: true,
    healthy: true,
  },
];

describe("MarketCommandSurface", () => {
  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
  });

  it("keeps unconfigured source controls disabled", () => {
    render(<MarketCommandSurface sources={[]} />);

    expect(screen.getByRole("button", { name: "Load quote" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Refresh now" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Load filings" })).toBeDisabled();
    expect(screen.getAllByText("STANDBY")).toHaveLength(3);
  });

  it("renders a real quote with visible provenance", async () => {
    const request = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({
        quote: {
          symbol: "SPY",
          currency: "USD",
          bid: "100.10",
          ask: "100.20",
          provenance: {
            provider: "alpaca",
            role: "MARKET_OBSERVATION",
            feed: "iex",
            quality: "REAL_TIME_SINGLE_VENUE",
            venue: "IEX",
            provider_timestamp: "2026-08-20T18:00:00Z",
            received_at: "2026-08-20T18:00:01Z",
          },
        },
        live_execution_available: false,
      }),
    });
    vi.stubGlobal("fetch", request);
    render(<MarketCommandSurface sources={enabledSources} />);

    fireEvent.click(screen.getByRole("button", { name: "Load quote" }));

    expect(await screen.findAllByText("$100.10")).toHaveLength(2);
    expect(screen.getAllByText("iex").length).toBeGreaterThan(0);
    expect(screen.getByText("real time single venue")).toBeInTheDocument();
    expect(request).toHaveBeenCalledWith("/api/markets/equities/SPY/quote", {
      cache: "no-store",
    });
  });

  it("keeps Schwab observations scoped to the selected connected account", async () => {
    const request = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({
        quote: {
          symbol: "SPY",
          currency: "USD",
          bid: "100.10",
          ask: "100.20",
          provenance: {
            provider: "schwab",
            role: "BROKER_AUTHORITY",
            feed: "broker_entitled",
            quality: "REAL_TIME_CONSOLIDATED",
            provider_timestamp: "2026-08-21T18:00:00Z",
            received_at: "2026-08-21T18:00:01Z",
          },
        },
      }),
    });
    vi.stubGlobal("fetch", request);
    render(
      <MarketCommandSurface
        accounts={[
          {
            id: "account-1",
            provider: "schwab",
            display_name: "Schwab Brokerage •1234",
            base_currency: "USD",
            status: "active",
          },
        ]}
        sources={[
          {
            id: "schwab_broker_market_data",
            label: "Schwab Market Data",
            role: "BROKER_AUTHORITY",
            feed: "broker_entitled",
            quality: "DELAYED",
            capabilities: ["EQUITY_QUOTE", "OPTION_DATA"],
            enabled: true,
            healthy: true,
          },
        ]}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Load quote" }));

    expect(
      await screen.findByText("real time consolidated"),
    ).toBeInTheDocument();
    expect(request).toHaveBeenCalledWith(
      "/api/accounts/account-1/markets/equities/SPY/quote",
      { cache: "no-store" },
    );
    expect(screen.getByRole("button", { name: "Load chain" })).toBeEnabled();
  });
});
