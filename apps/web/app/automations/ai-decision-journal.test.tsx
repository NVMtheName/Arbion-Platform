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
});
