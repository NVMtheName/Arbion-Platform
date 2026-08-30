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

  it("shows exact current Paper cash and exposure headroom", () => {
    render(
      <AutonomyGuardrailSummary
        executionMode="PAPER"
        riskPolicy={{
          minimum_cash_reserve: "200",
          max_capital_deployed: "800",
          max_single_position_amount: "300",
        }}
        strategyPolicy={{ max_proposal_notional: "100" }}
        paperPortfolio={{
          strategy_instance_id: "paper-1",
          currency: "USD",
          starting_cash: "1000.0000000000",
          cash: "975.0000077919",
          version: 2,
          updated_at: "2026-08-30T05:29:22Z",
          positions: [
            {
              symbol: "BTC",
              instrument: "CRYPTO",
              quantity: "0.0003177277",
              average_price: "78683.7037126445",
              is_open: true,
              updated_at: "2026-08-29T21:27:00Z",
            },
          ],
        }}
        paperMarkets={[
          {
            symbol: "BTC",
            assetClass: "CRYPTO",
            price: "78143.3200000000",
            priceBasis: "MARK",
            changePercent24H: "0.5",
            provider: "coinbase",
            feed: "rest_ticker",
            quality: "REAL_TIME_SINGLE_VENUE",
            observedAt: "2026-08-30T05:29:21Z",
            decisionAt: "2026-08-30T05:29:22Z",
          },
        ]}
      />,
    );

    const envelope = screen.getByLabelText(/current paper capital headroom/i);
    expect(envelope).toHaveTextContent("WITHIN LIMITS");
    expect(envelope).toHaveTextContent("$975.00");
    expect(envelope).toHaveTextContent(
      "$775.00 floor headroom · $200.00 reserve",
    );
    expect(envelope).toHaveTextContent("BTC · $24.8283");
    expect(envelope).toHaveTextContent("$100.00");
    expect(envelope).toHaveTextContent(/not broker balances or live quotes/i);
  });

  it("fails closed when exact Paper usage breaches a deterministic limit", () => {
    render(
      <AutonomyGuardrailSummary
        executionMode="PAPER"
        riskPolicy={{
          minimum_cash_reserve: "200",
          max_capital_deployed: "800",
          max_single_position_amount: "20",
        }}
        strategyPolicy={{ max_proposal_notional: "100" }}
        paperPortfolio={{
          strategy_instance_id: "paper-1",
          currency: "USD",
          starting_cash: "1000",
          cash: "975",
          version: 2,
          updated_at: "2026-08-30T05:29:22Z",
          positions: [
            {
              symbol: "BTC",
              instrument: "CRYPTO",
              quantity: "0.0003177277",
              average_price: "78683.7037126445",
              is_open: true,
              updated_at: "2026-08-29T21:27:00Z",
            },
          ],
        }}
        paperMarkets={[
          {
            symbol: "BTC",
            assetClass: "CRYPTO",
            price: "78143.32",
            priceBasis: "MARK",
            provider: "coinbase",
            feed: "rest_ticker",
            quality: "REAL_TIME_SINGLE_VENUE",
            observedAt: "2026-08-30T05:29:21Z",
            decisionAt: "2026-08-30T05:29:22Z",
          },
        ]}
      />,
    );

    expect(
      screen.getByLabelText(/current paper capital headroom/i),
    ).toHaveTextContent("LIMIT BREACH");
  });
});
