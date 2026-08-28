import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { AIShadowDecisionWorkspace } from "./ai-shadow-decision-workspace";

function decision(
  id: string,
  createdAt: string,
  symbol: string,
  executionRecordID?: string,
) {
  const proposed = symbol !== "NONE";
  return {
    ID: id,
    Source: "AI",
    DecisionType: proposed ? "PROPOSE" : "ABSTAIN",
    CreatedAt: createdAt,
    ExecutionRecordID: executionRecordID,
    RiskDecision: proposed ? "ALLOW" : undefined,
    ExecutionStatus: proposed ? "WOULD_HAVE_SUBMITTED" : undefined,
    StructuredRationale: {
      ai_provider: "openai",
      model_id: "gpt-5.6-sol",
      profile: "deep",
      decision: proposed ? "PROPOSE" : "ABSTAIN",
      symbol,
      side: proposed ? "SELL" : undefined,
      proposed_notional: proposed ? "1" : "0",
      confidence: "MEDIUM",
      thesis: proposed
        ? `A bounded ${symbol} proposal.`
        : "The evidence supports waiting.",
      input_evidence: {
        provider: "coinbase",
        available_cash_usd: "10",
        buying_power_usd: "10",
        observed_at: createdAt,
        positions: [],
        markets: [],
        recent_decisions: [],
      },
    },
  };
}

describe("AIShadowDecisionWorkspace", () => {
  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
  });

  it("loads earlier immutable decisions with only matched, deduplicated outcomes", async () => {
    const newest = decision(
      "11111111-1111-4111-8111-111111111111",
      "2026-08-28T06:00:00Z",
      "BTC",
      "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
    );
    const older = decision(
      "22222222-2222-4222-8222-222222222222",
      "2026-08-28T05:00:00Z",
      "XRP",
      "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
    );
    const newestOutcome = {
      ID: "33333333-3333-4333-8333-333333333333",
      ExecutionRecordID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
      Horizon: "ONE_HOUR",
      DirectionalChangeUSD: "0.01",
      DirectionalChangePercent: "1",
      PricingBasis: "BID_TO_CLOSE",
    };
    const olderOutcome = {
      ID: "44444444-4444-4444-8444-444444444444",
      ExecutionRecordID: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
      Horizon: "ONE_HOUR",
      DirectionalChangeUSD: "-0.01",
      DirectionalChangePercent: "-1",
      PricingBasis: "ASK_TO_CLOSE",
    };
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({
        decisions: [newest, older],
        outcomes: [newestOutcome, olderOutcome],
        next_cursor: "",
        decision_history_semantics: "IMMUTABLE_OWNER_STRATEGY_DECISION_HISTORY",
      }),
    });
    vi.stubGlobal("fetch", fetchMock);
    render(
      <AIShadowDecisionWorkspace
        strategyInstanceId="instance/one"
        initialDecisions={[newest]}
        initialOutcomes={[newestOutcome]}
        initialCursor="older cursor"
      />,
    );

    expect(
      screen.getByText("1 immutable decisions loaded"),
    ).toBeInTheDocument();
    fireEvent.click(
      screen.getByRole("button", { name: "Load earlier decisions" }),
    );
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/strategy-instances/instance%2Fone/decisions?limit=24&cursor=older%20cursor",
      { cache: "no-store" },
    );
    expect(
      screen.getByText("2 immutable decisions loaded"),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Load earlier decisions" }),
    ).not.toBeInTheDocument();

    const replay = screen.getByRole("region", {
      name: "AI Decision Replay Lab",
    });
    expect(
      within(replay).getByRole("button", { name: /All 2/i }),
    ).toBeInTheDocument();
    fireEvent.click(
      within(replay).getByRole("button", { name: /XRP SHADOW PROPOSAL/i }),
    );
    expect(within(replay).getByText("Propose · XRP")).toBeInTheDocument();
    expect(within(replay).getByText("1 immutable mark")).toBeInTheDocument();
    expect(within(replay).getByText(/Ask To Close/)).toBeInTheDocument();
  });

  it("does not infer an empty journal when durable history is unavailable", () => {
    render(
      <AIShadowDecisionWorkspace
        strategyInstanceId="instance-1"
        historyAvailable={false}
      />,
    );

    expect(
      screen.getByRole("heading", {
        name: "Immutable decision evidence is temporarily unavailable.",
      }),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/will not infer an empty history/i),
    ).toBeInTheDocument();
    expect(
      screen.queryByText("No completed AI decisions yet."),
    ).not.toBeInTheDocument();
  });

  it("fails visibly when an earlier page does not satisfy the history contract", async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ decisions: [], outcomes: [] }),
    });
    vi.stubGlobal("fetch", fetchMock);
    render(
      <AIShadowDecisionWorkspace
        strategyInstanceId="instance-1"
        initialCursor="older-cursor"
      />,
    );

    fireEvent.click(
      screen.getByRole("button", { name: "Load earlier decisions" }),
    );
    expect(await screen.findByRole("status")).toHaveTextContent(
      "Earlier immutable decision evidence could not be loaded.",
    );
  });
});
