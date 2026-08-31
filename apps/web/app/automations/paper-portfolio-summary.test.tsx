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
        markets={[
          {
            symbol: "BTC",
            assetClass: "CRYPTO",
            price: "78098.3100000000",
            priceBasis: "MARK",
            changePercent24H: "0.9599000000",
            provider: "coinbase",
            feed: "rest_ticker",
            quality: "REAL_TIME_SINGLE_VENUE",
            observedAt: "2026-08-29T21:26:55Z",
            decisionAt: "2026-08-29T21:27:00Z",
          },
        ]}
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
    expect(
      screen.getByRole("heading", { name: /simulated portfolio valuation/i }),
    ).toBeInTheDocument();
    expect(screen.getByText(/not a live quote/i)).toBeInTheDocument();
    expect(screen.getByText("+0.9599%")).toBeInTheDocument();
  });

  it("shows exact production-like Paper performance from immutable evidence", () => {
    render(
      <PaperPortfolioSummary
        executionMode="PAPER"
        portfolio={{
          strategy_instance_id: "instance-paper",
          currency: "USD",
          starting_cash: "1000.0000000000",
          cash: "975.0000077919",
          version: 2,
          updated_at: "2026-08-29T21:27:00Z",
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
        markets={[
          {
            symbol: "BTC",
            assetClass: "CRYPTO",
            price: "78098.3100000000",
            priceBasis: "MARK",
            changePercent24H: "0.9599000000",
            provider: "coinbase",
            feed: "rest_ticker",
            quality: "REAL_TIME_SINGLE_VENUE",
            observedAt: "2026-08-29T21:26:55Z",
            decisionAt: "2026-08-29T21:27:00Z",
          },
        ]}
      />,
    );

    const performance = screen.getByLabelText(
      /point-in-time paper performance/i,
    );
    expect(within(performance).getByText("$999.814")).toBeInTheDocument();
    expect(within(performance).getByText("-$0.186")).toBeInTheDocument();
    expect(within(performance).getByText("-0.0186%")).toBeInTheDocument();
    expect(
      within(performance).getByText(
        /coinbase · rest_ticker · REAL_TIME_SINGLE_VENUE/i,
      ),
    ).toBeInTheDocument();
  });

  it("separates exact fee-inclusive realized outcomes from marked performance", () => {
    render(
      <PaperPortfolioSummary
        executionMode="PAPER"
        portfolio={{
          ...portfolio,
          positions: [],
          realized_outcome: {
            status: "AVAILABLE",
            calculation_method: "AVERAGE_COST_INCLUDED_FEES",
            historical_coverage: "COMPLETE_FROM_PORTFOLIO_GENESIS",
            total_realized_profit_loss: "-1.2411667721",
            fill_count: 4,
            sell_fill_count: 1,
            first_fill_at: "2026-08-29T21:27:00Z",
            last_fill_at: "2026-08-31T02:35:58Z",
            symbols: [
              {
                symbol: "BTC",
                instrument: "CRYPTO",
                realized_profit_loss: "-1.2411667721",
                buy_fill_count: 3,
                sell_fill_count: 1,
                total_fees: "0.7468873200",
                ending_position_quantity: "0.0006216261",
                ending_average_cost: "79039.7919289767",
              },
            ],
          },
        }}
      />,
    );

    const realized = screen.getByLabelText(/exact paper realized outcome/i);
    expect(within(realized).getAllByText("-$1.2412")).toHaveLength(2);
    expect(within(realized).getByText("4")).toBeInTheDocument();
    expect(within(realized).getByText("1")).toBeInTheDocument();
    const table = within(realized).getByRole("table", {
      name: /exact per-symbol paper realized outcomes/i,
    });
    expect(within(table).getByText("BTC")).toBeInTheDocument();
    expect(
      within(realized).getByText(/not broker-reported/i),
    ).toBeInTheDocument();
  });

  it("attributes exact simulated fees and adverse slippage separately", () => {
    render(
      <PaperPortfolioSummary
        executionMode="PAPER"
        portfolio={{
          ...portfolio,
          positions: [],
          execution_costs: {
            status: "AVAILABLE",
            calculation_method: "SAVED_REFERENCE_VERSUS_SIMULATED_FILL",
            historical_coverage: "COMPLETE_FROM_PORTFOLIO_GENESIS",
            total_fees: "0.7755625000",
            total_adverse_slippage: "0.3875000000",
            total_explicit_cost: "1.1630625000",
            provider_reference_notional: "155.0000000000",
            gross_notional: "155.1125000000",
            all_in_cost_rate_bps: "75.0362903226",
            fill_notional_residual: "0.0000000000",
            maximum_absolute_fill_residual: "0.0000000000",
            residual_bound_per_fill: "0.0000000001",
            fill_count: 2,
            buy_fill_count: 1,
            sell_fill_count: 1,
            first_fill_at: "2026-08-31T04:36:38Z",
            last_fill_at: "2026-08-31T05:36:38Z",
            market_providers: ["coinbase"],
            market_feeds: ["rest_ticker"],
            market_qualities: ["REAL_TIME_SINGLE_VENUE"],
            sides: [
              {
                side: "BUY",
                total_fees: "0.5012500000",
                adverse_slippage: "0.2500000000",
                total_explicit_cost: "0.7512500000",
                provider_reference_notional: "100.0000000000",
                gross_notional: "100.2500000000",
                all_in_cost_rate_bps: "75.1250000000",
                fill_count: 1,
              },
              {
                side: "SELL",
                total_fees: "0.2743125000",
                adverse_slippage: "0.1375000000",
                total_explicit_cost: "0.4118125000",
                provider_reference_notional: "55.0000000000",
                gross_notional: "54.8625000000",
                all_in_cost_rate_bps: "74.8750000000",
                fill_count: 1,
              },
            ],
            symbols: [
              {
                symbol: "BTC",
                instrument: "CRYPTO",
                total_fees: "0.7755625000",
                adverse_slippage: "0.3875000000",
                total_explicit_cost: "1.1630625000",
                provider_reference_notional: "155.0000000000",
                gross_notional: "155.1125000000",
                all_in_cost_rate_bps: "75.0362903226",
                fill_count: 2,
                buy_fill_count: 1,
                sell_fill_count: 1,
              },
            ],
          },
        }}
      />,
    );

    const costs = screen.getByLabelText(/exact paper execution costs/i);
    expect(within(costs).getByText("$0.7756")).toBeInTheDocument();
    expect(within(costs).getByText("$0.3875")).toBeInTheDocument();
    expect(within(costs).getAllByText("$1.1631").length).toBeGreaterThan(1);
    expect(within(costs).getAllByText("75.0363 bps").length).toBeGreaterThan(1);
    expect(within(costs).getAllByText("$155.00").length).toBeGreaterThan(1);
    expect(
      within(costs).getByRole("table", {
        name: /exact buy-versus-sale paper execution costs/i,
      }),
    ).toBeInTheDocument();
    expect(within(costs).getByText(/not broker-reported/i)).toBeInTheDocument();
    expect(
      within(costs).getByText(/coinbase · rest_ticker/i),
    ).toBeInTheDocument();
  });

  it("reconciles exact realized, unrealized, total, and equity evidence", () => {
    render(
      <PaperPortfolioSummary
        executionMode="PAPER"
        portfolio={{
          strategy_instance_id: "instance-paper",
          currency: "USD",
          starting_cash: "1000.0000000000",
          cash: "900.0000000000",
          version: 2,
          updated_at: "2026-08-31T05:36:38Z",
          positions: [
            {
              symbol: "BTC",
              instrument: "CRYPTO",
              quantity: "1.0000000000",
              average_price: "100.0000000000",
              is_open: true,
              updated_at: "2026-08-31T05:36:38Z",
            },
          ],
          realized_outcome: {
            status: "NO_REALIZED_SALES",
            calculation_method: "AVERAGE_COST_INCLUDED_FEES",
            historical_coverage: "COMPLETE_FROM_PORTFOLIO_GENESIS",
            total_realized_profit_loss: "0.0000000000",
            fill_count: 1,
            sell_fill_count: 0,
            first_fill_at: "2026-08-31T04:36:38Z",
            last_fill_at: "2026-08-31T04:36:38Z",
            symbols: [
              {
                symbol: "BTC",
                instrument: "CRYPTO",
                realized_profit_loss: "0.0000000000",
                buy_fill_count: 1,
                sell_fill_count: 0,
                total_fees: "0.5000000000",
                ending_position_quantity: "1.0000000000",
                ending_average_cost: "100.0000000000",
              },
            ],
          },
        }}
        markets={[
          {
            symbol: "BTC",
            assetClass: "CRYPTO",
            price: "105.0000000000",
            priceBasis: "MARK",
            provider: "coinbase",
            feed: "rest_ticker",
            quality: "REAL_TIME_SINGLE_VENUE",
            observedAt: "2026-08-31T05:36:37Z",
            decisionAt: "2026-08-31T05:36:38Z",
          },
        ]}
      />,
    );

    const reconciliation = screen.getByLabelText(
      /exact paper outcome reconciliation/i,
    );
    expect(within(reconciliation).getByText("Exact match")).toBeInTheDocument();
    expect(
      within(reconciliation).getByText(
        /cash \+ marked exposure matches exactly/i,
      ),
    ).toBeInTheDocument();
    expect(
      within(reconciliation).getByText(/never folded into realized/i),
    ).toBeInTheDocument();
    expect(
      within(reconciliation).getByText(/provider coinbase/i),
    ).toBeInTheDocument();
  });
});
