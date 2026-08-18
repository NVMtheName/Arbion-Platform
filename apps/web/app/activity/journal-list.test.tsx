import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { JournalList, type JournalEntry } from "./journal-list";

const entry: JournalEntry = {
  id: "decision-1",
  created_at: "2026-08-18T18:00:00Z",
  strategy_instance_id: "instance-1",
  financial_account_id: "account-1",
  account_display_name: "Schwab Brokerage",
  mandate_id: "mandate-1",
  mandate_version: 2,
  strategy_identifier: "wheel",
  execution_mode: "PAPER",
  strategy_state: "READY_FOR_PUT",
  resulting_state: "SHORT_PUT_OPEN",
  source: "STRATEGY",
  decision_type: "ALLOW_SIMULATED_FILLED",
  structured_rationale: { candidate_count: 4, reason: "candidate_selected" },
  risk_decision: "ALLOW",
  approval_required: false,
  risk_reason_codes: ["ALLOWED"],
  execution_status: "SIMULATED_FILLED",
  symbol: "AAPL",
  instrument: "OPTION",
  side: "SELL_TO_OPEN",
  quantity: "1.0000000000",
  price: "1.2500000000",
  notional: "125.0000000000",
};

describe("Decision Journal", () => {
  it("renders useful decision evidence with an explicit non-live boundary", () => {
    render(<JournalList entries={[entry]} />);

    expect(screen.getByText("Wheel · AAPL")).toBeInTheDocument();
    expect(screen.getByText("Risk Allow")).toBeInTheDocument();
    expect(screen.getByText("Simulated Filled")).toBeInTheDocument();
    expect(screen.getByText("$125.00")).toBeInTheDocument();
    expect(
      screen.getByText(
        "Simulation evidence only — no real broker order was sent.",
      ),
    ).toBeInTheDocument();
  });

  it("guides the owner when there are no recorded decisions", () => {
    render(<JournalList entries={[]} />);

    expect(screen.getByText("Your journal is ready.")).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: "View automations" }),
    ).toHaveAttribute("href", "/automations");
  });
});
