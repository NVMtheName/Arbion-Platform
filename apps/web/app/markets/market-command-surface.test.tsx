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
    expect(
      screen.getByRole("button", { name: "Load crypto market" }),
    ).toBeDisabled();
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
});
