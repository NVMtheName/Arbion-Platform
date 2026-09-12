import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../app-page-header", () => ({
  AppPageHeader: () => <header>Shared navigation</header>,
}));
import { MarketsWorkspace } from "./markets-workspace";
import { emptyMarketWatchlist } from "./market-watchlist";
import type { MarketSource } from "./market-source-grid";

const sources: MarketSource[] = [
  {
    id: "coinbase_exchange",
    label: "Coinbase Exchange",
    role: "MARKET_OBSERVATION",
    feed: "rest_ticker",
    quality: "REAL_TIME_SINGLE_VENUE",
    capabilities: ["CRYPTO_MARKETS"],
    enabled: true,
    healthy: true,
  },
  {
    id: "sec_edgar",
    label: "SEC EDGAR",
    role: "PRIMARY_SOURCE",
    feed: "edgar",
    quality: "REFERENCE_ONLY",
    capabilities: ["INSIDER_FILING"],
    enabled: true,
    healthy: false,
    capability_status: [
      {
        capability: "INSIDER_FILING",
        enabled: true,
        state: "DEGRADED",
        consecutive_failures: 2,
        failure_category: "TIMEOUT",
      },
    ],
  },
  {
    id: "alpaca_iex",
    label: "Alpaca IEX",
    role: "MARKET_OBSERVATION",
    feed: "iex",
    quality: "REAL_TIME_SINGLE_VENUE",
    capabilities: ["EQUITY_QUOTE"],
    enabled: false,
    healthy: false,
  },
];

describe("Markets workspace hierarchy", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.stubGlobal("fetch", vi.fn());
  });
  afterEach(() => {
    cleanup();
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it("links directly to all five existing sections without adding a request", () => {
    render(
      <MarketsWorkspace
        sources={sources}
        accounts={[]}
        watchlist={emptyMarketWatchlist}
      />,
    );
    expect(
      screen.getByRole("heading", { level: 1, name: "Markets" }),
    ).toHaveAttribute("id", "markets-page-title");
    const links = within(
      screen.getByRole("navigation", { name: "Market sections" }),
    ).getAllByRole("link");
    expect(links).toHaveLength(5);
    for (const link of links)
      expect(document.querySelector(link.getAttribute("href")!)).not.toBeNull();
    expect(fetch).not.toHaveBeenCalled();
  });

  it("preserves source counts, exact status meanings and non-execution boundaries", () => {
    render(
      <MarketsWorkspace
        sources={sources}
        accounts={[]}
        watchlist={emptyMarketWatchlist}
      />,
    );
    expect(
      within(
        screen.getByRole("region", { name: "Command center status" }),
      ).getByText("1/3"),
    ).toBeVisible();
    expect(screen.getByText("2 consecutive provider failures")).toBeVisible();
    expect(screen.getAllByText("Not configured").length).toBeGreaterThan(0);
    expect(
      screen.getByText("READ-ONLY · LIVE EXECUTION UNAVAILABLE"),
    ).toBeVisible();
    expect(screen.getByText("OBSERVE ONLY")).toBeVisible();
    expect(screen.getByRole("button", { name: "Load quote" })).toBeDisabled();
    expect(
      screen.queryByRole("button", { name: /buy|sell|trade|order/i }),
    ).not.toBeInTheDocument();
  });

  it("keeps partial watchlist evidence and saved symbols visible", () => {
    render(
      <MarketsWorkspace
        sources={[]}
        accounts={[]}
        watchlist={{
          ...emptyMarketWatchlist,
          market_state: "PARTIAL",
          message: "Some observations are unavailable.",
          unavailable_symbols: ["BTC"],
          items: [
            {
              id: "test-btc",
              asset_class: "CRYPTO",
              symbol: "BTC",
              quote_currency: "USD",
              created_at: "2026-09-12T10:00:00Z",
            },
          ],
        }}
      />,
    );
    expect(screen.getByText("PARTIAL")).toBeVisible();
    expect(screen.getByText("BTC")).toBeVisible();
    expect(screen.getByText("Unavailable")).toBeVisible();
    expect(screen.getByText(/No estimate was substituted/)).toBeVisible();
    expect(screen.queryByText("$0.00")).not.toBeInTheDocument();
    expect(
      screen.getByText(/Source metadata is temporarily unavailable/),
    ).toBeVisible();
  });

  it("keeps the successful empty watchlist separate from source unavailability", () => {
    render(
      <MarketsWorkspace
        sources={[]}
        accounts={[]}
        watchlist={emptyMarketWatchlist}
      />,
    );
    expect(screen.getByText("No saved assets yet")).toBeVisible();
    expect(screen.getByText("EMPTY")).toBeVisible();
    expect(
      screen.getByText(/Source metadata is temporarily unavailable/),
    ).toBeVisible();
    expect(
      screen.getByRole("link", { name: "Review connected accounts" }),
    ).toHaveAttribute("href", "/accounts");
  });
});
