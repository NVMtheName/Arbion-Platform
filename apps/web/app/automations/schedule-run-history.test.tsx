import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import {
  ScheduleRunHistory,
  type ScheduleRunRecord,
} from "./schedule-run-history";

const base: ScheduleRunRecord = {
  id: "11111111-1111-4111-8111-111111111111",
  mandate_version: 6,
  execution_mode: "SHADOW",
  strategy_state: "AI_MONITORING",
  scheduled_for: "2026-08-28T01:00:00Z",
  started_at: "2026-08-28T01:00:05Z",
  completed_at: "2026-08-28T01:00:09Z",
  next_run_at: "2026-08-28T02:00:00Z",
  status: "SUCCEEDED",
  ai_decision: "ABSTAIN",
  execution_status: "CANCELED",
  duplicate_recovered: false,
  reconciliation_id: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
  reconciliation_review_required: false,
  consecutive_failures: 0,
};

describe("ScheduleRunHistory", () => {
  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
  });

  it("separates completed, safely skipped, and fail-closed cycles", () => {
    render(
      <ScheduleRunHistory
        instanceId="instance-1"
        financialProvider="coinbase"
        initialRuns={[
          base,
          {
            ...base,
            id: "22222222-2222-4222-8222-222222222222",
            status: "SKIPPED",
            error_code: "OUTSIDE_SESSION",
            ai_decision: undefined,
            execution_status: undefined,
            reconciliation_id: undefined,
          },
          {
            ...base,
            id: "33333333-3333-4333-8333-333333333333",
            status: "FAILED",
            error_code: "AI_PROVIDER_UNAVAILABLE",
            ai_decision: undefined,
            execution_status: undefined,
            reconciliation_review_required: true,
            consecutive_failures: 1,
          },
        ]}
      />,
    );

    expect(
      screen.getByRole("heading", {
        name: "Every scheduled cycle, preserved.",
      }),
    ).toBeInTheDocument();
    expect(screen.getByText("AI abstained safely")).toBeInTheDocument();
    expect(screen.getByText("Skipped safely")).toBeInTheDocument();
    expect(screen.getByText("Failed closed")).toBeInTheDocument();
    expect(screen.getByText("No action needed")).toBeInTheDocument();
    expect(screen.getByText("Automatic retry")).toBeInTheDocument();
    expect(
      screen.getByText(/selected AI provider was unavailable/i),
    ).toBeInTheDocument();
    expect(
      screen.getByText("Portfolio inventory review required"),
    ).toBeInTheDocument();
    expect(screen.getAllByText("No broker order was sent.")).toHaveLength(3);
    expect(
      screen.getByText(/no credentials, provider payloads/i),
    ).toBeInTheDocument();
  });

  it("loads an older owner-scoped page without duplicating records", async () => {
    const older = {
      ...base,
      id: "44444444-4444-4444-8444-444444444444",
      duplicate_recovered: true,
    };
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ runs: [base, older], next_cursor: "" }),
    });
    vi.stubGlobal("fetch", fetchMock);
    render(
      <ScheduleRunHistory
        instanceId="instance-1"
        initialRuns={[base]}
        initialCursor="older-cursor"
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Load earlier runs" }));
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    expect(String(fetchMock.mock.calls[0][0])).toContain(
      "/api/strategy-instances/instance-1/schedule-runs?limit=12&cursor=older-cursor",
    );
    expect(
      await screen.findByText("Recovered prior completed evaluation"),
    ).toBeInTheDocument();
    expect(screen.getAllByText("AI abstained safely")).toHaveLength(1);
    expect(
      screen.queryByRole("button", { name: "Load earlier runs" }),
    ).not.toBeInTheDocument();
  });

  it("makes pre-release history absence explicit", () => {
    render(<ScheduleRunHistory instanceId="instance-1" initialRuns={[]} />);
    expect(
      screen.getByText("No completed scheduler runs recorded yet."),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/records begin with this production release/i),
    ).toBeInTheDocument();
  });

  it("distinguishes operator schema failures from owner actions", () => {
    render(
      <ScheduleRunHistory
        instanceId="instance-1"
        financialProvider="coinbase"
        initialRuns={[
          {
            ...base,
            id: "55555555-5555-4555-8555-555555555555",
            status: "FAILED",
            error_code: "AI_REQUEST_INVALID",
            ai_decision: undefined,
            execution_status: undefined,
            consecutive_failures: 4,
          },
        ]}
      />,
    );
    expect(screen.getByText("Operator correction")).toBeInTheDocument();
    expect(screen.getByText(/strict schema contract/i)).toBeInTheDocument();
    expect(screen.getByText("AI_REQUEST_INVALID")).toBeInTheDocument();
    expect(screen.getByText(/no broker order was sent/i)).toBeInTheDocument();
  });
});
