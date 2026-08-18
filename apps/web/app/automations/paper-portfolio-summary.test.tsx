import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import {
  PaperPortfolioSummary,
  type PaperPortfolio,
} from "./paper-portfolio-summary";

const portfolio: PaperPortfolio = {
  strategy_instance_id: "instance-1",
  currency: "USD",
  starting_cash: "20000.0000000000",
  cash: "20125.0000000000",
  version: 2,
  updated_at: "2026-01-01T12:00:00Z",
  positions: [
    {
      symbol: "AAPL",
      instrument: "OPTION",
      option_type: "PUT",
      strike: "190.0000000000",
      expiration: "2026-01-31",
      quantity: "-1.0000000000",
      average_price: "1.2500000000",
      is_open: true,
      updated_at: "2026-01-01T12:00:00Z",
    },
  ],
};

describe("PaperPortfolioSummary", () => {
  afterEach(cleanup);

  it("shows isolated simulated cash and exact open contract facts", () => {
    render(
      <PaperPortfolioSummary portfolio={portfolio} executionMode="PAPER" />,
    );
    expect(
      screen.getByText(/\$20,125\.00 simulated cash/i),
    ).toBeInTheDocument();
    expect(screen.getByText(/not Schwab balances/i)).toBeInTheDocument();
    const table = screen.getByRole("table", {
      name: /simulated paper positions/i,
    });
    expect(within(table).getByText("Open")).toBeInTheDocument();
    expect(within(table).getByText("AAPL")).toBeInTheDocument();
    expect(
      within(table).getByText(/PUT · \$190\.00 strike · 2026-01-31/i),
    ).toBeInTheDocument();
    expect(within(table).getByText("-1")).toBeInTheDocument();
  });

  it("does not imply a PAPER ledger exists in SHADOW mode", () => {
    render(<PaperPortfolioSummary executionMode="SHADOW" />);
    expect(screen.getByText(/not used in shadow mode/i)).toBeInTheDocument();
    expect(
      screen.getByText(/does not create simulated cash/i),
    ).toBeInTheDocument();
  });
});
