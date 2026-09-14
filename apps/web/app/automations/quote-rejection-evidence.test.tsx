import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { QuoteRejectionEvidence } from "./quote-rejection-evidence";
import type { ScheduleRunRecord } from "./schedule-run-history";

const evidence = {
  schema_version: 1,
  provider: "schwab",
  financial_account_id: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
  requested_symbol: "SPY",
  quote_type: "NFL",
  realtime: false,
  provider_observed_at: "2026-09-14T15:00:00.123456Z",
  evaluated_at: "2026-09-14T15:00:01Z",
  rejection_code: "MARKET_DATA_DELAYED",
  before_model: true,
};
const run: ScheduleRunRecord = {
  id: "run-1",
  mandate_version: 6,
  execution_mode: "SHADOW",
  strategy_state: "AI_MONITORING",
  scheduled_for: "2026-09-14T15:00:00Z",
  started_at: "2026-09-14T15:00:00Z",
  completed_at: "2026-09-14T15:00:03Z",
  next_run_at: "2026-09-14T16:00:00Z",
  status: "FAILED",
  error_code: "MARKET_DATA_DELAYED",
  duplicate_recovered: false,
  reconciliation_review_required: false,
  consecutive_failures: 2,
  quote_rejection: evidence,
};

describe("saved quote rejection evidence", () => {
  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
  });
  it("shows exact safe metadata without fetching or granting real-time status", () => {
    const fetch = vi.fn();
    vi.stubGlobal("fetch", fetch);
    const { container } = render(
      <QuoteRejectionEvidence run={run} financialProvider="schwab" />,
    );
    expect(screen.getByText("Schwab / NFL")).toBeInTheDocument();
    expect(screen.getByText("false")).toBeInTheDocument();
    expect(screen.getByText(evidence.provider_observed_at)).toBeInTheDocument();
    expect(
      screen.getByText(/not proof of a specific delay/),
    ).toBeInTheDocument();
    expect(container.querySelector("details")).toHaveAttribute("open");
    expect(fetch).not.toHaveBeenCalled();
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
  });
  it.each([
    undefined,
    { ...evidence, realtime: true },
    { ...evidence, quote_type: "private-raw-output" },
    { ...evidence, extra: "credential" },
    { ...evidence, evaluated_at: "2026-09-14T16:00:00Z" },
    { ...evidence, provider: "coinbase" },
    { ...evidence, before_model: false },
    { ...evidence, provider_observed_at: "invalid" },
  ])("fails closed on absent or inconsistent evidence", (quote_rejection) => {
    render(
      <QuoteRejectionEvidence
        run={{ ...run, quote_rejection }}
        financialProvider="schwab"
      />,
    );
    expect(screen.getByText(/Quote detail UNAVAILABLE/)).toBeInTheDocument();
    expect(screen.queryByText("private-raw-output")).not.toBeInTheDocument();
  });
  it("keeps an omitted real-time flag separate from false", () => {
    render(
      <QuoteRejectionEvidence
        run={{
          ...run,
          error_code: "MARKET_DATA_REALTIME_UNCONFIRMED",
          quote_rejection: {
            ...evidence,
            realtime: null,
            rejection_code: "MARKET_DATA_REALTIME_UNCONFIRMED",
          },
        }}
        financialProvider="schwab"
      />,
    );
    expect(screen.getByText("UNAVAILABLE (not supplied)")).toBeInTheDocument();
  });
  it("preserves a future provider timestamp as rejected evidence, not a fresh quote", () => {
    render(
      <QuoteRejectionEvidence
        run={{
          ...run,
          error_code: "MARKET_DATA_STALE",
          quote_rejection: {
            ...evidence,
            provider_observed_at: "2026-09-14T16:00:00Z",
            rejection_code: "MARKET_DATA_STALE",
          },
        }}
        financialProvider="schwab"
      />,
    );
    expect(
      screen.getByText(/ahead of the evaluation time/),
    ).toBeInTheDocument();
  });
});
