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
                    performance_status: "UNAVAILABLE",
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
                    liquidity_status: "AVAILABLE",
                    spread_bps: "0.0590000000",
                    bid_depth_usd: "12500.5000000000",
                    ask_depth_usd: "11890.2500000000",
                    bid_levels: 10,
                    ask_levels: 10,
                    liquidity_feed: "advanced_trade_public_book",
                    liquidity_quality: "REAL_TIME_SINGLE_VENUE",
                    liquidity_observed_at: "2026-08-25T20:11:40Z",
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
      within(evidence!).getByText("Position performance unavailable"),
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
      within(evidence!).getByText(
        "Spread 0.059 bps · $12500.5 bid / $11890.25 ask · 10/10 levels",
      ),
    ).toBeInTheDocument();
    expect(
      within(evidence!).getByText(
        /Advanced Trade Public Book · Aug 25, 2026, 8:11 PM UTC · Real Time Single Venue/,
      ),
    ).toBeInTheDocument();
    expect(
      screen.getByText(
        "No broker order was sent. Arbion has no live execution path for this engine.",
      ),
    ).toBeInTheDocument();
  });

  it("shows provider-normalized Schwab position performance", () => {
    render(
      <AIDecisionJournal
        decisions={[
          {
            ID: "decision-performance",
            Source: "AI",
            DecisionType: "ABSTAIN",
            CreatedAt: "2026-08-26T15:00:00Z",
            StructuredRationale: {
              decision: "ABSTAIN",
              symbol: "NONE",
              proposed_notional: "0",
              thesis: "The bounded evidence supports waiting.",
              input_evidence: {
                provider: "schwab",
                available_cash_usd: "100",
                buying_power_usd: "100",
                observed_at: "2026-08-26T15:00:00Z",
                markets: [],
                recent_decisions: [],
                positions: [
                  {
                    symbol: "SPY",
                    quantity: "2",
                    available_quantity: "2",
                    market_value_usd: "1050",
                    performance_status: "AVAILABLE",
                    average_price_usd: "500",
                    current_price_usd: "525",
                    day_profit_loss_usd: "4",
                    day_profit_loss_percent: "0.3824091778",
                    open_profit_loss_usd: "50",
                    open_profit_loss_percent: "5",
                    price_basis: "PROVIDER_POSITION_MARKET_VALUE_PER_UNIT",
                  },
                ],
              },
            },
          },
        ]}
      />,
    );

    const evidence = screen
      .getByText(
        "Evidence considered · 1 allowlisted holding · 0 market snapshots",
      )
      .closest("details");
    expect(evidence).not.toBeNull();
    expect(
      within(evidence!).getByText("Avg purchase $500 · Current $525"),
    ).toBeInTheDocument();
    expect(
      within(evidence!).getByText("Day +$4 · +0.3824091778%"),
    ).toBeInTheDocument();
    expect(within(evidence!).getByText("Open +$50 · +5%")).toBeInTheDocument();
  });

  it("distinguishes bounded SEC event coverage from filing contents", () => {
    render(
      <AIDecisionJournal
        decisions={[
          {
            ID: "decision-sec",
            Source: "AI",
            DecisionType: "ABSTAIN",
            CreatedAt: "2026-08-25T20:11:42Z",
            StructuredRationale: {
              decision: "ABSTAIN",
              symbol: "NONE",
              proposed_notional: "0",
              thesis: "No cautious equity action is supported.",
              input_evidence: {
                provider: "schwab",
                available_cash_usd: "100",
                buying_power_usd: "100",
                observed_at: "2026-08-25T20:11:42Z",
                positions: [],
                recent_decisions: [],
                markets: [
                  {
                    symbol: "AAPL",
                    mark: "200",
                    feed: "schwab_market_data",
                    quality: "BROKER_REALTIME",
                    observed_at: "2026-08-25T20:11:39Z",
                    history_status: "UNAVAILABLE",
                    liquidity_status: "UNAVAILABLE",
                  },
                ],
                market_event_coverage: [
                  {
                    symbol: "AAPL",
                    status: "AVAILABLE",
                    lookback_days: 30,
                    event_count: 1,
                    resolver_feed: "company_tickers",
                    resolver_received_at: "2026-08-25T20:11:40Z",
                  },
                ],
                market_events: [
                  {
                    symbol: "AAPL",
                    event_type: "SEC_OWNERSHIP_FILING",
                    form: "4",
                    evidence_id: "0000320193-26-000001",
                    feed: "submissions",
                    occurred_at: "2026-08-24T18:00:00Z",
                  },
                ],
              },
            },
          },
        ]}
      />,
    );

    const evidence = screen
      .getByText(
        "Evidence considered · 0 allowlisted holdings · 1 market snapshot · 1 SEC event check · 1 filing event",
      )
      .closest("details");
    expect(evidence).not.toBeNull();
    expect(
      within(evidence!).getByRole("heading", {
        name: "Primary event coverage",
      }),
    ).toBeInTheDocument();
    expect(
      within(evidence!).getByText(
        "1 ownership filing event in the bounded window",
      ),
    ).toBeInTheDocument();
    expect(
      within(evidence!).getByText(
        "30-day window · Company Tickers · Aug 25, 2026, 8:11 PM UTC",
      ),
    ).toBeInTheDocument();
    expect(within(evidence!).getByText("Form 4 · AAPL")).toBeInTheDocument();
    expect(
      within(evidence!).getByText("0000320193-26-000001 · Submissions"),
    ).toBeInTheDocument();
    expect(
      within(evidence!).getByText(/filing identity only/),
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
