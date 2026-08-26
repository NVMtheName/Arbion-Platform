import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { AIDecisionJournal } from "./ai-decision-journal";

describe("AI Decision Journal", () => {
  afterEach(cleanup);

  it("shows the latest AI rationale and explicit shadow boundary", () => {
    render(
      <AIDecisionJournal
        decisions={[
          {
            ID: "decision-1",
            Source: "AI",
            DecisionType: "ABSTAIN",
            CreatedAt: "2026-08-25T20:11:42Z",
            StructuredRationale: {
              ai_provider: "openai",
              model_id: "gpt-5.6-sol",
              profile: "deep",
              decision: "ABSTAIN",
              symbol: "NONE",
              proposed_notional: "0",
              confidence: "HIGH",
              thesis: "No cautious trade is supported.",
              risk_flags: ["Buying power is unavailable."],
              limitations: ["No cost basis supplied."],
              market_observed_at: "2026-08-25T20:11:39Z",
              input_evidence: {
                provider: "coinbase",
                available_cash_usd: "0.0000000000",
                buying_power_usd: "0.0000000000",
                observed_at: "2026-08-25T20:11:42Z",
                recent_decisions: [
                  {
                    decision: "PROPOSE",
                    symbol: "XRP",
                    side: "SELL",
                    disposition: "WOULD_HAVE_SUBMITTED",
                    occurred_at: "2026-08-25T19:41:42Z",
                  },
                ],
                positions: [
                  {
                    symbol: "XRP",
                    quantity: "7.5000000000",
                    available_quantity: "0.5000000000",
                    market_value_usd: "10.7250000000",
                  },
                ],
                markets: [
                  {
                    symbol: "XRP",
                    mark: "1.4300000000",
                    change_percent_1h: "0.2400000000",
                    change_percent_6h: "-1.1000000000",
                    change_percent_24h: "-3.6200000000",
                    feed: "rest_ticker",
                    quality: "REAL_TIME_SINGLE_VENUE",
                    observed_at: "2026-08-25T20:11:39Z",
                    history_status: "COMPLETE",
                    history_granularity_seconds: 900,
                    history_contiguous_intervals: 96,
                    history_expected_intervals: 96,
                    history_feed: "rest_candles",
                    history_quality: "REAL_TIME_SINGLE_VENUE",
                    history_observed_at: "2026-08-25T20:00:00Z",
                  },
                ],
              },
            },
          },
        ]}
      />,
    );

    expect(screen.getByText("Abstain · No action")).toBeInTheDocument();
    expect(screen.getByText("High")).toBeInTheDocument();
    expect(
      screen.getByText("OpenAI · gpt-5.6-sol · Deep profile"),
    ).toBeInTheDocument();
    expect(screen.getByText("Safe abstention")).toBeInTheDocument();
    const evidence = screen
      .getByText(
        "Evidence considered · 1 allowlisted holding · 1 market snapshot · 1 prior decision",
      )
      .closest("details");
    expect(evidence).not.toBeNull();
    expect(within(evidence!).getAllByText("$0")).toHaveLength(2);
    expect(
      within(evidence!).getByText("7.5 held · 0.5 available"),
    ).toBeInTheDocument();
    expect(
      within(evidence!).getByRole("heading", {
        name: "Recent decision context",
      }),
    ).toBeInTheDocument();
    expect(within(evidence!).getByText("Propose XRP")).toBeInTheDocument();
    expect(
      within(evidence!).getByText("Sell · Would Have Submitted"),
    ).toBeInTheDocument();
    expect(
      within(evidence!).getByText("$10.725 observed value"),
    ).toBeInTheDocument();
    expect(
      within(evidence!).getByText("1h +0.24% · 6h -1.1% · 24h -3.62%"),
    ).toBeInTheDocument();
    expect(
      within(evidence!).getByText(
        "Complete history · 96/96 exact 15m candles · Real Time Single Venue",
      ),
    ).toBeInTheDocument();
    expect(
      within(evidence!).getByText(/Rest Ticker · Aug 25, 2026, 8:11 PM UTC/),
    ).toBeInTheDocument();
    expect(
      within(evidence!).getByText(/Rest Candles · Aug 25, 2026, 8:00 PM UTC/),
    ).toBeInTheDocument();
    expect(
      screen.getByText(
        "No broker order was sent. Arbion has no live execution path for this engine.",
      ),
    ).toBeInTheDocument();
  });

  it("explains when no model decision completed", () => {
    render(<AIDecisionJournal decisions={[]} />);

    expect(
      screen.getByText(/No AI decision has completed yet/),
    ).toBeInTheDocument();
  });

  it("shows the exact Claude and Gemini route without inferring it from the model", () => {
    const { rerender } = render(
      <AIDecisionJournal
        decisions={[
          {
            ID: "decision-claude",
            Source: "AI",
            DecisionType: "ABSTAIN",
            CreatedAt: "2026-08-26T02:00:00Z",
            StructuredRationale: {
              ai_provider: "anthropic",
              model_id: "claude-sonnet-5",
              profile: "core",
              decision: "ABSTAIN",
            },
          },
        ]}
      />,
    );
    expect(
      screen.getByText("Claude · claude-sonnet-5 · Core profile"),
    ).toBeInTheDocument();

    rerender(
      <AIDecisionJournal
        decisions={[
          {
            ID: "decision-gemini",
            Source: "AI",
            DecisionType: "ABSTAIN",
            CreatedAt: "2026-08-26T02:01:00Z",
            StructuredRationale: {
              ai_provider: "gemini",
              model_id: "gemini-3.7-flash",
              profile: "deep",
              decision: "ABSTAIN",
            },
          },
        ]}
      />,
    );
    expect(
      screen.getByText("Gemini · gemini-3.7-flash · Deep profile"),
    ).toBeInTheDocument();
  });

  it("shows when deterministic controls hold a repeated shadow proposal", () => {
    render(
      <AIDecisionJournal
        decisions={[
          {
            ID: "decision-repeat",
            Source: "AI",
            DecisionType: "DENY_RISK_DENIED",
            CreatedAt: "2026-08-26T00:40:00Z",
            RiskEvaluationID: "risk-repeat",
            ExecutionRecordID: "execution-repeat",
            RiskDecision: "DENY",
            RiskReasonCodes: ["REPEAT_ACTION_COOLDOWN_ACTIVE"],
            ExecutionStatus: "RISK_DENIED",
            Symbol: "XRP",
            Side: "SELL",
            Quantity: "0.6993",
            Price: "1.43",
            Notional: "1",
            StructuredRationale: {
              decision: "PROPOSE",
              symbol: "XRP",
              side: "SELL",
              proposed_notional: "1",
              confidence: "MEDIUM",
              thesis: "The signal still favors reducing exposure.",
            },
          },
        ]}
      />,
    );

    expect(screen.getByText("Held by controls")).toBeInTheDocument();
    expect(
      screen.getByText("Repeat Action Cooldown Active"),
    ).toBeInTheDocument();
    expect(screen.getByText("Held by controls · XRP")).toBeInTheDocument();
    expect(screen.getByText("Control denial")).toBeInTheDocument();
    expect(
      screen.queryByRole("region", { name: "Hypothetical trade evidence" }),
    ).not.toBeInTheDocument();
    expect(
      screen.getByText(
        "No broker order was sent. Arbion has no live execution path for this engine.",
      ),
    ).toBeInTheDocument();
  });

  it("shows immutable risk and hypothetical trade evidence for a proposal", () => {
    render(
      <AIDecisionJournal
        decisions={[
          {
            ID: "decision-2",
            Source: "AI",
            DecisionType: "ALLOW_WOULD_HAVE_SUBMITTED",
            CreatedAt: "2026-08-25T21:12:04Z",
            RiskEvaluationID: "risk-1",
            ExecutionRecordID: "execution-1",
            RiskDecision: "ALLOW",
            ExecutionStatus: "WOULD_HAVE_SUBMITTED",
            Symbol: "XRP",
            Side: "SELL",
            Quantity: "0.6993006993",
            Price: "1.4300000000",
            Notional: "1.0000000000",
            StructuredRationale: {
              decision: "PROPOSE",
              symbol: "XRP",
              side: "SELL",
              proposed_notional: "1",
              confidence: "LOW",
              thesis: "Cautiously trim the weakest performer.",
              market_observed_at: "2026-08-25T21:12:01Z",
            },
          },
        ]}
        outcomes={[
          {
            id: "outcome-1",
            execution_record_id: "execution-1",
            horizon: "ONE_HOUR",
            observed_price: "1.4400000000",
            directional_change_usd: "-0.0100000000",
            directional_change_percent: "-1.0000000000",
            pricing_basis: "ASK_TO_CLOSE",
            market_observed_at: "2026-08-25T22:12:01Z",
            elapsed_seconds: 3600,
          },
        ]}
      />,
    );

    expect(
      screen.getByRole("heading", { name: "Hypothetical trade evidence" }),
    ).toBeInTheDocument();
    expect(screen.getByText("0.6993006993 XRP")).toBeInTheDocument();
    expect(screen.getByText("$1.43")).toBeInTheDocument();
    expect(screen.getByText("Would Have Submitted")).toBeInTheDocument();
    expect(screen.getByText("Allow")).toBeInTheDocument();
    expect(screen.getByText("1-hour mark")).toBeInTheDocument();
    expect(screen.getByText("-$0.01 (-1%)")).toBeInTheDocument();
    expect(screen.getByText("Observed 1h after proposal")).toBeInTheDocument();
    expect(
      screen.getByText(/not a fill, realized return/i),
    ).toBeInTheDocument();
  });
});
