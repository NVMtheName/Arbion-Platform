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
        request_policy: {
          cache_ttl_ms: 60_000,
          minimum_request_interval_ms: 500,
          verification_window_ms: 300_000,
        },
        request_usage: {
          cache_lookups: 5,
          cache_hits: 2,
          provider_attempts: 3,
          counters_saturated: false,
        },
      },
    ],
    enabled: true,
    healthy: false,
  },
  {
    id: "sec_edgar",
    label: "SEC EDGAR",
    role: "PRIMARY_FILING",
    feed: "submissions",
    quality: "FILING",
    capabilities: ["INSIDER_FILING"],
    capability_status: [
      {
        capability: "INSIDER_FILING",
        enabled: true,
        state: "VERIFICATION_EXPIRED",
        last_attempt_at: "2026-08-21T16:55:00Z",
        last_success_at: "2026-08-21T16:55:00Z",
        consecutive_failures: 0,
        request_policy: {
          cache_ttl_ms: 60_000,
          minimum_request_interval_ms: 150,
          verification_window_ms: 300_000,
        },
        request_usage: {
          cache_lookups: 1,
          cache_hits: 0,
          provider_attempts: 1,
          counters_saturated: false,
        },
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
    expect(screen.getByRole("heading", { name: "SEC EDGAR" })).toBeVisible();
    expect(screen.getAllByText("Verification aged").length).toBeGreaterThan(0);
    expect(screen.getByText(/verification aged after 5 min/)).toBeVisible();
    const requestBudget = screen.getByLabelText(
      "crypto markets request budget",
    );
    expect(requestBudget).toHaveTextContent(/3\s*provider attempts/);
    expect(requestBudget).toHaveTextContent(/2\s*cache saves/);
    expect(requestBudget).toHaveTextContent("40% cache reuse");
    expect(
      screen.getAllByText(/not a provider quota or remaining-credit balance/)
        .length,
    ).toBeGreaterThan(0);
    expect(screen.getByLabelText("Capability status")).toHaveTextContent(
      "0 verified",
    );
    expect(screen.getByLabelText("Capability status")).toHaveTextContent(
      "1 verification aged",
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
                request_policy: {
                  cache_ttl_ms: 30_000,
                  minimum_request_interval_ms: 250,
                  verification_window_ms: 300_000,
                },
                request_usage: {
                  cache_lookups: 8,
                  cache_hits: 6,
                  provider_attempts: 2,
                  counters_saturated: false,
                },
              },
            ],
          },
        ],
        status_generated_at: "2026-08-21T17:01:01Z",
        status_semantics: "PROCESS_LOCAL_TIME_BOUNDED_PROVIDER_VERIFICATION",
        request_usage_semantics: "PROCESS_LOCAL_BOUNDED_AGGREGATES",
        provider_quota_exposed: false,
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
    expect(screen.getAllByText("75% cache reuse")).toHaveLength(2);
  });

  it("keeps existing status when request telemetry semantics are unsafe", async () => {
    vi.spyOn(global, "fetch").mockResolvedValue({
      ok: true,
      json: async () => ({
        sources: [],
        status_generated_at: "2026-08-21T17:01:01Z",
        status_semantics: "PROCESS_LOCAL_TIME_BOUNDED_PROVIDER_VERIFICATION",
        request_usage_semantics: "PROVIDER_QUOTA",
        provider_quota_exposed: true,
        provider_errors_exposed: false,
        live_execution_available: false,
      }),
    } as Response);

    render(<MarketSourceGrid sources={sources} />);
    fireEvent.click(
      screen.getByRole("button", { name: "Refresh source status" }),
    );

    await waitFor(() =>
      expect(
        screen.getByText(/Source verification is temporarily unavailable/),
      ).toBeVisible(),
    );
    expect(screen.getByRole("heading", { name: "Alpaca IEX" })).toBeVisible();
  });
});
