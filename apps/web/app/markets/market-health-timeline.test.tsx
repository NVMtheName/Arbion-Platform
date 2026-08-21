import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { MarketHealthTimeline } from "./market-health-timeline";
import {
  safeMarketHealthHistory,
  type MarketHealthHistory,
} from "./market-health-contract";
import type { MarketSource } from "./market-source-grid";

const sources: MarketSource[] = [
  {
    id: "coinbase_exchange",
    label: "Coinbase Public Markets",
    role: "MARKET_OBSERVATION",
    feed: "rest",
    quality: "REAL_TIME_SINGLE_VENUE",
    capabilities: ["CRYPTO_MARKETS"],
    enabled: true,
    healthy: true,
  },
];

const history: MarketHealthHistory = {
  buckets: [
    {
      source_id: "coinbase_exchange",
      capability: "CRYPTO_MARKETS",
      interval_started_at: "2026-08-21T18:00:00Z",
      last_observed_at: "2026-08-21T18:40:00Z",
      completed_attempts: 4,
      successes: 4,
      failures: 0,
      last_state: "VERIFIED",
    },
    {
      source_id: "coinbase_exchange",
      capability: "CRYPTO_MARKETS",
      interval_started_at: "2026-08-21T19:00:00Z",
      last_observed_at: "2026-08-21T19:30:00Z",
      completed_attempts: 3,
      successes: 2,
      failures: 1,
      last_state: "DEGRADED",
      failure_category: "TIMEOUT",
    },
  ],
  window_started_at: "2026-08-20T20:00:00Z",
  window_ended_at: "2026-08-21T20:00:00Z",
  window_hours: 24,
  interval_minutes: 60,
};

function response(value: MarketHealthHistory = history) {
  return {
    ...value,
    history_semantics: "DURABLE_PROVIDER_OUTCOMES_5_MINUTE_STORAGE_HOURLY_VIEW",
    subject_dimensions_exposed: false,
    raw_provider_errors_exposed: false,
    live_execution_available: false,
  };
}

describe("MarketHealthTimeline", () => {
  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

  it("shows durable outcomes without classifying empty hours as failures", () => {
    render(<MarketHealthTimeline sources={sources} initialHistory={history} />);

    expect(
      screen.getByRole("heading", {
        name: "Provider outcomes that survive a restart.",
      }),
    ).toBeVisible();
    expect(screen.getByText("Coinbase Public Markets")).toBeVisible();
    expect(screen.getByLabelText("24-hour health summary")).toHaveTextContent(
      /7 completed outcomes/,
    );
    expect(screen.getByLabelText("24-hour health summary")).toHaveTextContent(
      /86% successful/,
    );
    expect(screen.getByLabelText(/4 successful and 0 failed/)).toBeVisible();
    expect(
      screen.getByLabelText(/2 successful and 1 failed.*timeout/),
    ).toBeVisible();
    expect(
      screen.getAllByLabelText(/no completed provider outcome recorded/),
    ).toHaveLength(22);
    expect(screen.getByText(/Empty hours are neutral/)).toBeVisible();
  });

  it("refreshes only the bounded privacy-safe history contract", async () => {
    const fetchMock = vi.spyOn(global, "fetch").mockResolvedValue({
      ok: true,
      json: async () => response(),
    } as Response);
    render(<MarketHealthTimeline sources={sources} />);
    fireEvent.click(
      screen.getByRole("button", { name: "Refresh 24-hour history" }),
    );

    await waitFor(() =>
      expect(screen.getByText("Coinbase Public Markets")).toBeVisible(),
    );
    expect(fetchMock).toHaveBeenCalledWith("/api/markets/source-history", {
      cache: "no-store",
    });
  });

  it("rejects subject dimensions and unsafe semantic expansion", async () => {
    expect(
      safeMarketHealthHistory({
        ...response(),
        buckets: [{ ...history.buckets[0], user_id: "not-allowed" }],
      }),
    ).toBeUndefined();

    vi.spyOn(global, "fetch").mockResolvedValue({
      ok: true,
      json: async () => ({
        ...response(),
        subject_dimensions_exposed: true,
      }),
    } as Response);
    render(<MarketHealthTimeline sources={sources} initialHistory={history} />);
    fireEvent.click(
      screen.getByRole("button", { name: "Refresh 24-hour history" }),
    );
    await waitFor(() =>
      expect(
        screen.getByText(/Durable source history is temporarily unavailable/),
      ).toBeVisible(),
    );
    expect(screen.getByText("Coinbase Public Markets")).toBeVisible();
  });
});
