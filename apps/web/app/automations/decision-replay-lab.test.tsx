import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { DecisionReplayLab } from "./decision-replay-lab";

const inputEvidence = {
  provider: "coinbase",
  available_cash_usd: "125.0000000000",
  buying_power_usd: "125.0000000000",
  observed_at: "2026-08-27T16:20:00Z",
  positions: [
    {
      symbol: "XRP",
      quantity: "10",
      available_quantity: "5",
    },
  ],
  markets: [
    {
      symbol: "XRP",
      mark: "1.4300000000",
      change_percent_1h: "0.2500000000",
      change_percent_6h: "-1.5000000000",
      change_percent_24h: "2.7500000000",
      feed: "advanced_trade_public_market",
      quality: "real_time_single_venue",
      observed_at: "2026-08-27T16:19:58Z",
    },
  ],
  recent_decisions: [
    {
      decision: "ABSTAIN",
      occurred_at: "2026-08-27T15:20:00Z",
    },
  ],
};

const decisions = [
  {
    id: "decision-proposal",
    source: "AI",
    created_at: "2026-08-27T16:20:00Z",
    risk_evaluation_id: "risk-1",
    execution_record_id: "execution-1",
    risk_decision: "ALLOW",
    execution_status: "WOULD_HAVE_SUBMITTED",
    symbol: "XRP",
    side: "SELL",
    quantity: "0.6993006993",
    structured_rationale: {
      ai_provider: "openai",
      model_id: "gpt-5.6-sol",
      profile: "deep",
      decision: "PROPOSE",
      symbol: "XRP",
      side: "SELL",
      proposed_notional: "1",
      confidence: "MEDIUM",
      thesis: "Trim exposure inside the bounded policy.",
      input_evidence: inputEvidence,
    },
  },
  {
    id: "decision-abstain",
    source: "AI",
    created_at: "2026-08-27T15:20:00Z",
    structured_rationale: {
      ai_provider: "openai",
      model_id: "gpt-5.6-sol",
      profile: "deep",
      decision: "ABSTAIN",
      symbol: "NONE",
      proposed_notional: "0",
      confidence: "HIGH",
      thesis: "The evidence supports waiting.",
      input_evidence: inputEvidence,
    },
  },
  {
    id: "decision-hold",
    source: "AI",
    created_at: "2026-08-27T14:20:00Z",
    risk_evaluation_id: "risk-2",
    execution_record_id: "execution-2",
    risk_decision: "DENY",
    risk_reason_codes: ["REPEAT_ACTION_COOLDOWN_ACTIVE"],
    execution_status: "RISK_DENIED",
    symbol: "XRP",
    side: "SELL",
    structured_rationale: {
      ai_provider: "openai",
      model_id: "gpt-5.6-sol",
      profile: "deep",
      decision: "PROPOSE",
      symbol: "XRP",
      proposed_notional: "1",
      confidence: "LOW",
      thesis: "The earlier action remains inside its cooldown.",
      input_evidence: inputEvidence,
    },
  },
];

describe("Decision Replay Lab", () => {
  afterEach(cleanup);

  it("reconstructs a proposal across model, risk, shadow, and outcome evidence", () => {
    render(
      <DecisionReplayLab
        decisions={decisions}
        outcomes={[
          {
            id: "outcome-1h",
            execution_record_id: "execution-1",
            horizon: "ONE_HOUR",
            directional_change_usd: "0.0100000000",
            directional_change_percent: "1.0000000000",
            pricing_basis: "BID_TO_CLOSE",
          },
          {
            id: "outcome-24h",
            execution_record_id: "execution-1",
            horizon: "TWENTY_FOUR_HOURS",
            directional_change_usd: "-0.0200000000",
            directional_change_percent: "-2.0000000000",
            pricing_basis: "MARK_FALLBACK",
          },
        ]}
      />,
    );

    const lab = screen.getByRole("region", { name: "AI Decision Replay Lab" });
    expect(within(lab).getByText("Propose · XRP")).toBeInTheDocument();
    expect(
      within(lab).getByText("OpenAI · gpt-5.6-sol · Deep"),
    ).toBeInTheDocument();
    expect(within(lab).getByText("Would Have Submitted")).toBeInTheDocument();
    expect(within(lab).getByText("2 immutable marks")).toBeInTheDocument();
    expect(within(lab).getByText(/\$0\.01/)).toBeInTheDocument();
    expect(within(lab).getByText(/Bid To Close/)).toBeInTheDocument();
    expect(within(lab).getByText(/-\$0\.02/)).toBeInTheDocument();
    expect(within(lab).getByText(/Mark Fallback/)).toBeInTheDocument();
    expect(within(lab).getByText("$1.43")).toBeInTheDocument();
    expect(
      within(lab).getByText(
        "Advanced Trade Public Market · Real Time Single Venue",
      ),
    ).toBeInTheDocument();
    expect(
      within(lab).getByText("NO BROKER ORDER WAS SENT"),
    ).toBeInTheDocument();
  });

  it("filters and replays abstentions and deterministic control holds", () => {
    render(<DecisionReplayLab decisions={decisions} />);

    fireEvent.click(screen.getByRole("button", { name: /Abstentions 1/ }));
    expect(screen.getByText("Abstain · No action")).toBeInTheDocument();
    expect(
      screen.getByText("Not reached after abstention"),
    ).toBeInTheDocument();
    expect(screen.getByText("No hypothetical action")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /Control Holds 1/ }));
    expect(screen.getAllByText("CONTROL HOLD")).toHaveLength(2);
    expect(
      screen.getByText("Repeat Action Cooldown Active"),
    ).toBeInTheDocument();
    expect(screen.getByText("Risk Denied")).toBeInTheDocument();
  });

  it("keeps an honest empty state until a successful cycle exists", () => {
    render(<DecisionReplayLab decisions={[]} />);
    expect(
      screen.getByText("No completed AI decisions yet."),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/first successful Shadow cycle/),
    ).toBeInTheDocument();
  });
});
