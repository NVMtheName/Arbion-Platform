import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { MarketSourceGrid, type MarketSource } from "./market-source-grid";

const sources: MarketSource[] = [
  {
    id: "alpaca_iex",
    label: "Alpaca IEX",
    role: "MARKET_OBSERVATION",
    feed: "iex",
    quality: "REAL_TIME_SINGLE_VENUE",
    capabilities: ["EQUITY_QUOTE", "EQUITY_BARS"],
    enabled: false,
    healthy: false,
  },
  {
    id: "coingecko_rest",
    label: "CoinGecko",
    role: "REFERENCE_DATA",
    feed: "rest",
    quality: "AGGREGATED_REFERENCE",
    capabilities: ["CRYPTO_MARKETS"],
    enabled: true,
    healthy: false,
  },
];

describe("MarketSourceGrid", () => {
  it("shows source quality and conservative availability", () => {
    render(<MarketSourceGrid sources={sources} />);

    expect(screen.getByRole("heading", { name: "Alpaca IEX" })).toBeVisible();
    expect(screen.getByText("real time single venue")).toBeVisible();
    expect(screen.getByText("Not configured")).toBeVisible();
    expect(screen.getByText("Degraded")).toBeVisible();
    expect(screen.getByText("aggregated reference")).toBeVisible();
  });
});
