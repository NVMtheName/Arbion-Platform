import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { StrategyRuntimeEvidenceLedger } from "./strategy-runtime-evidence-ledger";

const currentTransition = {
  id: "11111111-1111-4111-8111-111111111111",
  previous_state: "AI_MONITORING",
  new_state: "AI_MONITORING",
  state_version: 4,
  trigger: "SCHEDULED_EVALUATION",
  occurred_at: "2026-08-28T06:00:00Z",
};

const currentExecution = {
  id: "22222222-2222-4222-8222-222222222222",
  mandate_version: 6,
  mode: "SHADOW",
  status: "WOULD_HAVE_SUBMITTED",
  symbol: "XRP",
  instrument: "CRYPTO_SPOT",
  side: "SELL",
  quantity: "0.7000000000",
  price: "1.4300000000",
  notional: "1.0010000000",
  created_at: "2026-08-28T06:00:00Z",
};

describe("StrategyRuntimeEvidenceLedger", () => {
  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
  });

  it("renders minimized non-live runtime evidence with an explicit boundary", () => {
    render(
      <StrategyRuntimeEvidenceLedger
        strategyInstanceId="instance-1"
        initialTransitions={[currentTransition]}
        initialExecutions={[currentExecution]}
        loadedDecisionCount={12}
      />,
    );

    const ledger = screen.getByRole("region", {
      name: "Strategy runtime evidence ledger",
    });
    expect(
      within(ledger).getAllByText("1", { selector: "strong" }),
    ).toHaveLength(2);
    expect(within(ledger).getByText("12")).toBeInTheDocument();
    expect(
      within(ledger).getByText("AI Monitoring → AI Monitoring"),
    ).toBeInTheDocument();
    expect(
      within(ledger).getByText("Scheduled Evaluation"),
    ).toBeInTheDocument();
    expect(
      within(ledger).getByText("Would Have Submitted"),
    ).toBeInTheDocument();
    expect(within(ledger).getByText("Sell 0.7 XRP")).toBeInTheDocument();
    expect(
      within(ledger).getByText("SHADOW hypothetical only — no broker order."),
    ).toBeInTheDocument();
    expect(
      within(ledger).getByText(/cannot authorize, submit, replace, cancel/i),
    ).toBeInTheDocument();
  });

  it("loads and deduplicates older state and execution pages independently", async () => {
    const olderTransition = {
      ...currentTransition,
      id: "33333333-3333-4333-8333-333333333333",
      state_version: 3,
      previous_state: "AI_MONITORING",
      new_state: "PAUSED",
      trigger: "PAUSED",
      occurred_at: "2026-08-28T05:00:00Z",
    };
    const olderExecution = {
      ...currentExecution,
      id: "44444444-4444-4444-8444-444444444444",
      mode: "PAPER",
      status: "SIMULATED_FILLED",
      symbol: "SPY",
      instrument: "EQUITY",
      side: "BUY",
      quantity: "1.0000000000",
      price: "650.0000000000",
      notional: "650.0000000000",
      created_at: "2026-08-28T05:00:00Z",
    };
    const fetchMock = vi.fn(async (url: string) => {
      if (url.includes("/history?")) {
        return {
          ok: true,
          json: async () => ({
            transitions: [currentTransition, olderTransition],
            next_cursor: "",
            history_semantics: "IMMUTABLE_OWNER_STRATEGY_STATE_HISTORY",
          }),
        };
      }
      return {
        ok: true,
        json: async () => ({
          executions: [currentExecution, olderExecution],
          next_cursor: "",
          history_semantics: "IMMUTABLE_OWNER_NONLIVE_EXECUTION_HISTORY",
        }),
      };
    });
    vi.stubGlobal("fetch", fetchMock);
    render(
      <StrategyRuntimeEvidenceLedger
        strategyInstanceId="instance/one"
        initialTransitions={[currentTransition]}
        initialExecutions={[currentExecution]}
        initialTransitionCursor="state cursor"
        initialExecutionCursor="execution cursor"
      />,
    );

    fireEvent.click(
      screen.getByRole("button", { name: "Load earlier state changes" }),
    );
    fireEvent.click(
      screen.getByRole("button", { name: "Load earlier non-live results" }),
    );
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2));
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/strategy-instances/instance%2Fone/history?limit=16&cursor=state%20cursor",
      { cache: "no-store" },
    );
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/strategy-instances/instance%2Fone/executions?limit=16&cursor=execution%20cursor",
      { cache: "no-store" },
    );
    expect(screen.getByText("AI Monitoring → Paused")).toBeInTheDocument();
    expect(screen.getByText("Buy 1 SPY")).toBeInTheDocument();
    expect(
      screen.getByText("PAPER simulation only — no broker fill."),
    ).toBeInTheDocument();
    expect(screen.getAllByText("Would Have Submitted")).toHaveLength(1);
    expect(
      screen.queryByRole("button", { name: "Load earlier state changes" }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", {
        name: "Load earlier non-live results",
      }),
    ).not.toBeInTheDocument();
  });

  it("keeps unavailable projections distinct from honest empty ledgers", () => {
    render(
      <StrategyRuntimeEvidenceLedger
        strategyInstanceId="instance-1"
        transitionHistoryAvailable={false}
        executionHistoryAvailable={false}
      />,
    );

    expect(
      screen.getByText(/state history is temporarily unavailable/i),
    ).toBeInTheDocument();
    expect(
      screen.getByText(
        /non-live execution history is temporarily unavailable/i,
      ),
    ).toBeInTheDocument();
    expect(
      screen.queryByText(/no state transitions are recorded/i),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByText(/no paper or shadow execution evidence/i),
    ).not.toBeInTheDocument();
  });
});
