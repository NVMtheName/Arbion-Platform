import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { AIDecisionJournal } from "./ai-decision-journal";

describe("AI Decision Journal", () => {
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
              decision: "ABSTAIN",
              symbol: "NONE",
              proposed_notional: "0",
              confidence: "HIGH",
              thesis: "No cautious trade is supported.",
              risk_flags: ["Buying power is unavailable."],
              limitations: ["No cost basis supplied."],
              market_observed_at: "2026-08-25T20:11:39Z",
            },
          },
        ]}
      />,
    );

    expect(screen.getByText("Abstain · No action")).toBeInTheDocument();
    expect(screen.getByText("High")).toBeInTheDocument();
    expect(screen.getByText("Safe abstention")).toBeInTheDocument();
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
      />,
    );

    expect(
      screen.getByRole("heading", { name: "Hypothetical trade evidence" }),
    ).toBeInTheDocument();
    expect(screen.getByText("0.6993006993 XRP")).toBeInTheDocument();
    expect(screen.getByText("$1.43")).toBeInTheDocument();
    expect(screen.getByText("Would Have Submitted")).toBeInTheDocument();
    expect(screen.getByText("Allow")).toBeInTheDocument();
  });
});
