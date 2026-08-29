import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { AutonomyGuardrailSummary } from "./autonomy-guardrail-summary";

describe("autonomy guardrail summary", () => {
  afterEach(cleanup);

  it("shows the immutable server-enforced limits in owner language", () => {
    render(
      <AutonomyGuardrailSummary
        executionMode="SHADOW"
        riskPolicy={{
          max_trades_per_day: 4,
          minimum_cash_reserve: "25.0000000000",
          max_capital_deployed: "500",
          max_single_position_amount: "100",
          max_single_position_percentage: "30.0000",
        }}
      />,
    );
    expect(screen.getByText("SERVER ENFORCED")).toBeInTheDocument();
    expect(screen.getByText("4")).toBeInTheDocument();
    expect(screen.getByText("$25")).toBeInTheDocument();
    expect(screen.getByText("$500")).toBeInTheDocument();
    expect(screen.getByText("$100")).toBeInTheDocument();
    expect(screen.getByText("30% concentration ceiling")).toBeInTheDocument();
    expect(
      screen.getByText(/does not infer broker profit and loss/i),
    ).toBeInTheDocument();
  });

  it("labels older mandates without inventing missing limits", () => {
    render(<AutonomyGuardrailSummary executionMode="SHADOW" riskPolicy={{}} />);
    expect(screen.getByText("Legacy mandate")).toBeInTheDocument();
    expect(screen.getAllByText("Bucket policy")).toHaveLength(2);
    expect(screen.getByText("No extra cap")).toBeInTheDocument();
  });

  it("describes Paper guardrails as an isolated simulation", () => {
    render(<AutonomyGuardrailSummary executionMode="PAPER" riskPolicy={{}} />);
    expect(
      screen.getByText(/simulated result or immutable abstention/i),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/isolated simulated cash and position ledger/i),
    ).toBeInTheDocument();
  });
});
