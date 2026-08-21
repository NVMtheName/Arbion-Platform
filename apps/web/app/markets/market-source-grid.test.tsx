import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

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
    capability_status: [
      {
        capability: "CRYPTO_MARKETS",
        enabled: true,
        state: "DEGRADED",
        last_attempt_at: "2026-08-21T17:00:00Z",
        consecutive_failures: 2,
        failure_category: "TIMEOUT",
      },
    ],
    enabled: true,
    healthy: false,
  },
];

describe("MarketSourceGrid", () => {
  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

  it("shows source quality and conservative availability", () => {
    render(
      <MarketSourceGrid
        sources={sources}
        statusGeneratedAt="2026-08-21T17:00:01Z"
      />,
    );

    expect(
      screen.getByRole("heading", {
        name: "Every feed earns its own status.",
      }),
    ).toBeVisible();
    expect(screen.getByRole("heading", { name: "Alpaca IEX" })).toBeVisible();
    expect(screen.getByText("real time single venue")).toBeVisible();
    expect(screen.getAllByText("Not configured").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Degraded").length).toBeGreaterThan(0);
    expect(screen.getByText("aggregated reference")).toBeVisible();
    expect(screen.getByText(/timeout · attempted/)).toBeVisible();
    expect(screen.getByText("2 consecutive provider failures")).toBeVisible();
    expect(screen.getByLabelText("Capability status")).toHaveTextContent(
      "0 verified",
    );
  });

  it("refreshes only conservative source metadata", async () => {
    const fetchMock = vi.spyOn(global, "fetch").mockResolvedValue({
      ok: true,
      json: async () => ({
        sources: [
          {
            ...sources[1],
            healthy: true,
            capability_status: [
              {
                capability: "CRYPTO_MARKETS",
                enabled: true,
                state: "VERIFIED",
                last_attempt_at: "2026-08-21T17:01:00Z",
                last_success_at: "2026-08-21T17:01:00Z",
                consecutive_failures: 0,
              },
            ],
          },
        ],
        status_generated_at: "2026-08-21T17:01:01Z",
        status_semantics: "PROCESS_LOCAL_LAST_PROVIDER_ATTEMPT",
        provider_errors_exposed: false,
        live_execution_available: false,
      }),
    } as Response);

    render(<MarketSourceGrid sources={sources} />);
    fireEvent.click(
      screen.getByRole("button", { name: "Refresh source status" }),
    );

    await waitFor(() => expect(screen.getByText("Verified")).toBeVisible());
    expect(fetchMock).toHaveBeenCalledWith("/api/markets/sources", {
      cache: "no-store",
    });
    expect(screen.getByText(/Last provider success/)).toBeVisible();
  });
});
