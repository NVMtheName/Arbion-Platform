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
    expect(
      screen.getByText(/not connected-account balances/i),
    ).toBeInTheDocument();
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

  it("shows immutable AI Paper fill provenance without implying a broker order", () => {
    const cryptoPortfolio: PaperPortfolio = {
      ...portfolio,
      positions: [
        {
          symbol: "BTC",
          instrument: "CRYPTO",
          quantity: "0.0100000000",
          average_price: "100250.0000000000",
          is_open: true,
          updated_at: "2026-08-28T19:00:00Z",
        },
      ],
    };
    render(
      <PaperPortfolioSummary
        portfolio={cryptoPortfolio}
        executionMode="PAPER"
        fills={[
          {
            id: "fill-1",
            symbol: "BTC",
            instrument: "CRYPTO",
            side: "BUY",
            quantity: "0.0100000000",
            reference_price: "100000.0000000000",
            fill_price: "100250.0000000000",
            gross_notional: "1002.5000000000",
            fee: "5.0125000000",
            resulting_cash: "8992.4875000000",
            resulting_position_quantity: "0.0100000000",
            pricing_basis: "ASK_PLUS_SLIPPAGE",
            market_provider: "coinbase",
            market_feed: "advanced_trade",
            market_quality: "BROKER_REALTIME",
            market_observed_at: "2026-08-28T18:59:59Z",
            simulated_at: "2026-08-28T19:00:00Z",
            simulation_only: true,
          },
        ]}
      />,
    );
    expect(screen.getByText("Crypto spot")).toBeInTheDocument();
    const table = screen.getByRole("table", {
      name: /immutable ai paper simulated fills/i,
    });
    expect(within(table).getByText("BUY BTC")).toBeInTheDocument();
    expect(
      within(table).getByText(/coinbase · advanced_trade · BROKER_REALTIME/i),
    ).toBeInTheDocument();
    expect(screen.getByText(/not a broker order/i)).toBeInTheDocument();
  });
});
