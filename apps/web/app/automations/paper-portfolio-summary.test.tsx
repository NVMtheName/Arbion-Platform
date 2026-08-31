import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import {
  PaperPortfolioSummary,
  type PaperGuardrailEvidence,
  type PaperGuardrailProposalFact,
  type PaperPortfolio,
} from "./paper-portfolio-summary";

const paperCheckCodes = [
  "AUTHORIZATION_DENIED",
  "CIRCUIT_BREAKER_ACTIVE",
  "MANDATE_NOT_READY",
  "CAPITAL_POLICY_REQUIRED",
  "AUTONOMY_DENIED",
  "STALE_ACCOUNT_DATA",
  "SYMBOL_NOT_ALLOWED",
  "OPTIONS_NOT_ALLOWED",
  "MARGIN_NOT_ALLOWED",
  "INSUFFICIENT_POSITION",
  "CAPITAL_LIMIT_EXCEEDED",
  "POSITION_LIMIT_EXCEEDED",
  "DAILY_LOSS_LIMIT_EXCEEDED",
  "REPEAT_ACTION_COOLDOWN_ACTIVE",
];

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
          starting_cash: "1000.0000000000",
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
          starting_cash: "1000.0000000000",
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
            timeline_sample_count: 2,
            timeline_capped: false,
            timeline: [
              {
                sequence: 1,
                fill_id: "fill-buy",
                execution_record_id: "execution-buy",
                proposed_action_id: "action-buy",
                risk_evaluation_id: "risk-buy",
                symbol: "BTC",
                instrument: "CRYPTO",
                side: "BUY",
                fee: "0.5012500000",
                adverse_slippage: "0.2500000000",
                explicit_cost: "0.7512500000",
                provider_reference_notional: "100.0000000000",
                gross_notional: "100.2500000000",
                fill_notional_residual: "0.0000000000",
                cumulative_fees: "0.5012500000",
                cumulative_adverse_slippage: "0.2500000000",
                cumulative_explicit_cost: "0.7512500000",
                cumulative_provider_reference_notional: "100.0000000000",
                cumulative_gross_notional: "100.2500000000",
                cumulative_all_in_cost_rate_bps: "75.1250000000",
                cumulative_rate_change: "FIRST",
                symbol_sequence: 1,
                same_side_streak: 1,
                side_transition: "FIRST",
                market_provider: "coinbase",
                market_feed: "rest_ticker",
                market_quality: "REAL_TIME_SINGLE_VENUE",
                market_observed_at: "2026-08-31T04:36:37Z",
                simulated_at: "2026-08-31T04:36:38Z",
              },
              {
                sequence: 2,
                fill_id: "fill-sell",
                execution_record_id: "execution-sell",
                proposed_action_id: "action-sell",
                risk_evaluation_id: "risk-sell",
                symbol: "BTC",
                instrument: "CRYPTO",
                side: "SELL",
                fee: "0.2743125000",
                adverse_slippage: "0.1375000000",
                explicit_cost: "0.4118125000",
                provider_reference_notional: "55.0000000000",
                gross_notional: "54.8625000000",
                fill_notional_residual: "0.0000000000",
                cumulative_fees: "0.7755625000",
                cumulative_adverse_slippage: "0.3875000000",
                cumulative_explicit_cost: "1.1630625000",
                cumulative_provider_reference_notional: "155.0000000000",
                cumulative_gross_notional: "155.1125000000",
                cumulative_all_in_cost_rate_bps: "75.0362903226",
                cumulative_rate_change: "FELL",
                symbol_sequence: 2,
                same_side_streak: 1,
                side_transition: "BUY_TO_SELL",
                opposite_side_elapsed_seconds: "3600.0000000000",
                market_provider: "coinbase",
                market_feed: "rest_ticker",
                market_quality: "REAL_TIME_SINGLE_VENUE",
                market_observed_at: "2026-08-31T05:36:37Z",
                simulated_at: "2026-08-31T05:36:38Z",
              },
            ],
            trade_sequence: {
              status: "AVAILABLE",
              calculation_method: "COMPLETE_IMMUTABLE_FILL_SEQUENCE",
              historical_coverage: "COMPLETE_FROM_PORTFOLIO_GENESIS",
              starting_cash: "1000.0000000000",
              provider_reference_turnover_to_starting_cash_bps:
                "1550.0000000000",
              explicit_cost_to_starting_cash_bps: "11.6306250000",
              fill_count: 2,
              same_side_transition_count: 0,
              opposite_side_transition_count: 1,
              buy_to_sell_reversal_count: 1,
              sell_to_buy_reversal_count: 0,
              symbols: [
                {
                  symbol: "BTC",
                  instrument: "CRYPTO",
                  fill_count: 2,
                  buy_fill_count: 1,
                  sell_fill_count: 1,
                  same_side_transition_count: 0,
                  opposite_side_transition_count: 1,
                  buy_to_sell_reversal_count: 1,
                  sell_to_buy_reversal_count: 0,
                  longest_same_side_streak: 1,
                  first_side: "BUY",
                  last_side: "SELL",
                  first_fill_at: "2026-08-31T04:36:38Z",
                  last_fill_at: "2026-08-31T05:36:38Z",
                },
              ],
            },
          },
          activity_cadence: {
            status: "AVAILABLE",
            calculation_method: "IMMUTABLE_SCHEDULE_AND_SIMULATION_CHRONOLOGY",
            as_of: "2026-08-31T06:10:00Z",
            schedule_interval_minutes: 60,
            twenty_four_hours: {
              status: "AVAILABLE",
              horizon_hours: 24,
              window_started_at: "2026-08-30T06:10:00Z",
              window_ended_at: "2026-08-31T06:10:00Z",
              scheduled_cycle_count: 24,
              succeeded_cycle_count: 24,
              failed_cycle_count: 0,
              safe_wait_cycle_count: 0,
              abstention_count: 21,
              deterministic_deny_count: 1,
              simulated_fill_count: 2,
              other_succeeded_count: 0,
            },
            seven_days: {
              status: "UNAVAILABLE",
              horizon_hours: 168,
              scheduled_cycle_count: 48,
              succeeded_cycle_count: 48,
              failed_cycle_count: 0,
              safe_wait_cycle_count: 0,
              abstention_count: 45,
              deterministic_deny_count: 1,
              simulated_fill_count: 2,
              other_succeeded_count: 0,
            },
            disposition_funnel: {
              status: "AVAILABLE",
              calculation_method:
                "IMMUTABLE_PAPER_EVALUATION_DISPOSITION_FUNNEL",
              twenty_four_hours: {
                status: "AVAILABLE",
                horizon_hours: 24,
                window_started_at: "2026-08-30T06:10:00Z",
                window_ended_at: "2026-08-31T06:10:00Z",
                scheduled_cycle_count: 24,
                completed_cycle_count: 24,
                succeeded_evaluation_count: 24,
                failed_cycle_count: 0,
                safe_wait_cycle_count: 0,
                decision_count: 24,
                abstention_count: 21,
                proposal_count: 3,
                deterministic_deny_count: 1,
                simulated_fill_count: 2,
                other_proposal_outcome_count: 0,
                completion_rate_percent: "100.0000000000",
                succeeded_evaluation_rate_percent: "100.0000000000",
                decision_rate_percent: "100.0000000000",
                abstention_rate_percent: "87.5000000000",
                proposal_rate_percent: "12.5000000000",
                deterministic_deny_rate_percent: "33.3333333333",
                simulated_fill_rate_percent: "66.6666666667",
                other_proposal_outcome_rate_percent: "0.0000000000",
              },
              seven_days: {
                status: "UNAVAILABLE",
                horizon_hours: 168,
                scheduled_cycle_count: 0,
                completed_cycle_count: 0,
                succeeded_evaluation_count: 0,
                failed_cycle_count: 0,
                safe_wait_cycle_count: 0,
                decision_count: 0,
                abstention_count: 0,
                proposal_count: 0,
                deterministic_deny_count: 0,
                simulated_fill_count: 0,
                other_proposal_outcome_count: 0,
              },
            },
            fill_timing: {
              status: "AVAILABLE",
              historical_coverage: "COMPLETE_FROM_PORTFOLIO_GENESIS",
              fill_count: 2,
              first_fill_at: "2026-08-31T04:36:38Z",
              last_fill_at: "2026-08-31T05:36:38Z",
              minimum_inter_fill_seconds: "3600.0000000000",
              median_inter_fill_seconds: "3600.0000000000",
              maximum_inter_fill_seconds: "3600.0000000000",
              symbols: [
                {
                  status: "AVAILABLE",
                  symbol: "BTC",
                  instrument: "CRYPTO",
                  fill_count: 2,
                  first_fill_at: "2026-08-31T04:36:38Z",
                  last_fill_at: "2026-08-31T05:36:38Z",
                  minimum_inter_fill_seconds: "3600.0000000000",
                  median_inter_fill_seconds: "3600.0000000000",
                  maximum_inter_fill_seconds: "3600.0000000000",
                },
              ],
            },
            longest_no_fill_interval: {
              status: "AVAILABLE",
              cycle_count: 8,
              interval_seconds: "25800.0000000000",
              scheduled_started_at: "2026-08-30T18:00:00Z",
              completed_ended_at: "2026-08-31T01:10:00Z",
            },
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
    const timeline = within(costs).getByLabelText(
      /immutable paper cost and turnover timeline/i,
    );
    expect(within(timeline).getAllByText(/fell vs prior/i)).toHaveLength(2);
    expect(
      within(timeline).getByRole("link", { name: /fill #2/i }),
    ).toHaveAttribute("href", "#paper-fill-fill-sell");
    const sequence = within(costs).getByLabelText(
      /immutable paper trade sequence and churn evidence/i,
    );
    expect(within(sequence).getByText("1,550.00 bps")).toBeInTheDocument();
    expect(within(sequence).getAllByText(/buy → sale/i).length).toBeGreaterThan(
      0,
    );
    expect(within(timeline).getByText(/3,600.00 sec/i)).toBeInTheDocument();
    expect(within(costs).getByText(/not broker-reported/i)).toBeInTheDocument();
    expect(
      within(costs).getAllByText(/coinbase · rest_ticker/i).length,
    ).toBeGreaterThan(1);
    const cadence = screen.getByLabelText(/exact paper activity cadence/i);
    expect(
      within(cadence).getByText("24 cycles · 2 fills / 24h"),
    ).toBeInTheDocument();
    expect(
      within(cadence).getByText(/seven-day evidence/i),
    ).toBeInTheDocument();
    expect(
      within(cadence).getByText(/21 abstain · 3 propose/i),
    ).toBeInTheDocument();
    expect(within(cadence).getByText(/66.67% filled/i)).toBeInTheDocument();
    expect(
      within(cadence).getByText(/do not establish conversion quality/i),
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

  it("shows exact immutable Paper guardrail attribution without implying live readiness", () => {
    render(
      <PaperPortfolioSummary
        executionMode="PAPER"
        portfolio={{
          ...portfolio,
          positions: [],
          guardrail_evidence: {
            status: "AVAILABLE",
            calculation_method:
              "IMMUTABLE_PAPER_PROPOSAL_RISK_AND_SIMULATION_ATTRIBUTION",
            as_of: "2026-08-31T15:00:00Z",
            twenty_four_hours: {
              status: "AVAILABLE",
              coverage_status: "COMPLETE",
              horizon_hours: 24,
              window_started_at: "2026-08-30T15:00:00Z",
              window_ended_at: "2026-08-31T15:00:00Z",
              proposal_count: 2,
              allow_count: 1,
              deny_count: 1,
              simulated_fill_count: 1,
              minimum_proposed_notional: "50.0000000000",
              median_proposed_notional: "75.0000000000",
              maximum_proposed_notional: "100.0000000000",
              denial_reason_codes: [
                { code: "INSUFFICIENT_POSITION", count: 1 },
              ],
              failed_check_codes: [{ code: "INSUFFICIENT_POSITION", count: 1 }],
              expected_check_codes: [
                "AUTHORIZATION_DENIED",
                "INSUFFICIENT_POSITION",
              ],
              check_results: [
                {
                  code: "AUTHORIZATION_DENIED",
                  evaluation_count: 2,
                  pass_count: 2,
                  fail_count: 0,
                  warn_count: 0,
                },
                {
                  code: "INSUFFICIENT_POSITION",
                  evaluation_count: 2,
                  pass_count: 1,
                  fail_count: 1,
                  warn_count: 0,
                },
              ],
              fully_evaluated_count: 1,
              fail_closed_prefix_count: 1,
              check_set_drift_count: 0,
              symbols: [
                {
                  symbol: "BTC",
                  instrument: "CRYPTO",
                  proposal_count: 1,
                  allow_count: 1,
                  deny_count: 0,
                  simulated_fill_count: 1,
                  proposed_notional: "100.0000000000",
                },
                {
                  symbol: "ETH",
                  instrument: "CRYPTO",
                  proposal_count: 1,
                  allow_count: 0,
                  deny_count: 1,
                  simulated_fill_count: 0,
                  proposed_notional: "50.0000000000",
                },
              ],
              proposals: [
                {
                  decision_journal_entry_id: "decision-denied",
                  proposed_action_id: "action-denied",
                  risk_evaluation_id: "risk-denied",
                  execution_record_id: "execution-denied",
                  created_at: "2026-08-31T13:00:00Z",
                  symbol: "ETH",
                  instrument: "CRYPTO",
                  side: "SELL",
                  proposed_quantity: "1.0000000000",
                  proposed_notional: "50.0000000000",
                  risk_decision: "DENY",
                  execution_status: "RISK_DENIED",
                  denial_reason_codes: ["INSUFFICIENT_POSITION"],
                  failed_check_codes: ["INSUFFICIENT_POSITION"],
                  checks: [
                    { code: "AUTHORIZATION_DENIED", result: "PASS" },
                    { code: "INSUFFICIENT_POSITION", result: "FAIL" },
                  ],
                  coverage_status: "FAIL_CLOSED_PREFIX",
                  terminal_check_stage: "INSUFFICIENT_POSITION",
                  financial_provider: "coinbase",
                  market_feed: "rest_ticker",
                  market_quality: "REAL_TIME_SINGLE_VENUE",
                  market_observed_at: "2026-08-31T12:59:59Z",
                },
                {
                  decision_journal_entry_id: "decision-allowed",
                  proposed_action_id: "action-allowed",
                  risk_evaluation_id: "risk-allowed",
                  execution_record_id: "execution-allowed",
                  created_at: "2026-08-31T14:00:00Z",
                  symbol: "BTC",
                  instrument: "CRYPTO",
                  side: "BUY",
                  proposed_quantity: "0.0010000000",
                  proposed_notional: "100.0000000000",
                  risk_decision: "ALLOW",
                  execution_status: "SIMULATED_FILLED",
                  denial_reason_codes: [],
                  failed_check_codes: [],
                  checks: [
                    { code: "AUTHORIZATION_DENIED", result: "PASS" },
                    { code: "INSUFFICIENT_POSITION", result: "PASS" },
                  ],
                  coverage_status: "FULL_EVALUATION",
                  terminal_check_stage: "ALL_REQUIRED_CHECKS",
                  financial_provider: "coinbase",
                  market_feed: "rest_ticker",
                  market_quality: "REAL_TIME_SINGLE_VENUE",
                  market_observed_at: "2026-08-31T13:59:59Z",
                },
              ],
            },
            seven_days: {
              status: "UNAVAILABLE",
              coverage_status: "UNAVAILABLE",
              horizon_hours: 168,
              proposal_count: 0,
              allow_count: 0,
              deny_count: 0,
              simulated_fill_count: 0,
              denial_reason_codes: [],
              failed_check_codes: [],
              expected_check_codes: [],
              check_results: [],
              fully_evaluated_count: 0,
              fail_closed_prefix_count: 0,
              check_set_drift_count: 0,
              symbols: [],
              proposals: [],
            },
            coverage_change: {
              status: "UNAVAILABLE",
              baseline_horizon_hours: 168,
              current_horizon_hours: 24,
              baseline_proposal_count: 0,
              current_proposal_count: 0,
              proposal_count_delta: 0,
              financial_providers: [],
              coverage_metrics: [],
              check_changes: [],
              symbol_changes: [],
            },
            denial_eligibility: {
              status: "AVAILABLE",
              calculation_method:
                "IMMUTABLE_PAPER_DETERMINISTIC_DENIAL_AND_LATER_ELIGIBILITY",
              horizon_hours: 24,
              window_started_at: "2026-08-30T15:00:00Z",
              window_ended_at: "2026-08-31T15:00:00Z",
              denial_count: 1,
              later_allowed_count: 0,
              later_denied_count: 0,
              no_later_comparable_proposal_count: 1,
              financial_providers: ["coinbase"],
              first_denial_at: "2026-08-31T13:00:00Z",
              latest_denial_at: "2026-08-31T13:00:00Z",
              denials: [
                {
                  decision_journal_entry_id: "decision-denied",
                  proposed_action_id: "action-denied",
                  risk_evaluation_id: "risk-denied",
                  execution_record_id: "execution-denied",
                  created_at: "2026-08-31T13:00:00Z",
                  symbol: "ETH",
                  instrument: "CRYPTO",
                  side: "SELL",
                  proposed_quantity: "1.0000000000",
                  proposed_notional: "50.0000000000",
                  denial_reason_codes: ["INSUFFICIENT_POSITION"],
                  failed_check_codes: ["INSUFFICIENT_POSITION"],
                  terminal_check_stage: "INSUFFICIENT_POSITION",
                  financial_provider: "coinbase",
                  market_feed: "rest_ticker",
                  market_quality: "REAL_TIME_SINGLE_VENUE",
                  market_observed_at: "2026-08-31T12:59:59Z",
                  later_disposition: "NO_LATER_COMPARABLE_PROPOSAL",
                  changed_risk_results: [],
                },
              ],
            },
          },
        }}
      />,
    );

    const guardrails = screen.getByLabelText(
      /exact paper guardrail disposition/i,
    );
    expect(
      within(guardrails).getByText("1 allowed · 1 denied / 24h"),
    ).toBeInTheDocument();
    expect(
      within(guardrails).getByText("$75.00 exact median"),
    ).toBeInTheDocument();
    expect(
      within(guardrails).getByText(/INSUFFICIENT_POSITION \(1\)/),
    ).toBeInTheDocument();
    expect(within(guardrails).getByText("Complete")).toBeInTheDocument();
    expect(
      within(guardrails).getByRole("table", {
        name: /deterministic check coverage/i,
      }),
    ).toBeInTheDocument();
    expect(
      within(guardrails).getByText(/2 required checks completed/i),
    ).toBeInTheDocument();
    expect(within(guardrails).getByText(/decision-denied/)).toBeInTheDocument();
    expect(
      within(guardrails).getByRole("table", {
        name: /deterministic denial and later eligibility/i,
      }),
    ).toBeInTheDocument();
    expect(
      within(guardrails).getByText(/none in this saved window/i),
    ).toBeInTheDocument();
    expect(
      within(guardrails).getByText(/does not infer model intent/i),
    ).toBeInTheDocument();
  });

  it("compares complete immutable 24-hour and seven-day guardrail coverage", () => {
    const currentProposal: PaperGuardrailProposalFact = {
      decision_journal_entry_id: "decision-current",
      proposed_action_id: "action-current",
      risk_evaluation_id: "risk-current",
      execution_record_id: "execution-current",
      created_at: "2026-08-31T14:00:00Z",
      symbol: "BTC",
      instrument: "CRYPTO",
      side: "BUY",
      proposed_quantity: "0.0010000000",
      proposed_notional: "100.0000000000",
      risk_decision: "ALLOW",
      execution_status: "SIMULATED_FILLED",
      denial_reason_codes: [],
      failed_check_codes: [],
      checks: paperCheckCodes.map((code) => ({ code, result: "PASS" })),
      coverage_status: "FULL_EVALUATION",
      terminal_check_stage: "ALL_REQUIRED_CHECKS",
      financial_provider: "coinbase",
      market_feed: "rest_ticker",
      market_quality: "REAL_TIME_SINGLE_VENUE",
      market_observed_at: "2026-08-31T13:59:59Z",
    };
    const olderProposal: PaperGuardrailProposalFact = {
      ...currentProposal,
      decision_journal_entry_id: "decision-older",
      proposed_action_id: "action-older",
      risk_evaluation_id: "risk-older",
      execution_record_id: "execution-older",
      created_at: "2026-08-29T14:00:00Z",
      symbol: "ETH",
      proposed_notional: "50.0000000000",
      market_observed_at: "2026-08-29T13:59:59Z",
    };
    const evidence: PaperGuardrailEvidence = {
      status: "AVAILABLE",
      calculation_method:
        "IMMUTABLE_PAPER_PROPOSAL_RISK_AND_SIMULATION_ATTRIBUTION",
      as_of: "2026-08-31T15:00:00Z",
      twenty_four_hours: {
        status: "AVAILABLE",
        coverage_status: "COMPLETE",
        horizon_hours: 24,
        window_started_at: "2026-08-30T15:00:00Z",
        window_ended_at: "2026-08-31T15:00:00Z",
        proposal_count: 1,
        allow_count: 1,
        deny_count: 0,
        simulated_fill_count: 1,
        minimum_proposed_notional: "100.0000000000",
        median_proposed_notional: "100.0000000000",
        maximum_proposed_notional: "100.0000000000",
        denial_reason_codes: [],
        failed_check_codes: [],
        expected_check_codes: paperCheckCodes,
        check_results: paperCheckCodes.map((code) => ({
          code,
          evaluation_count: 1,
          pass_count: 1,
          fail_count: 0,
          warn_count: 0,
        })),
        fully_evaluated_count: 1,
        fail_closed_prefix_count: 0,
        check_set_drift_count: 0,
        symbols: [
          {
            symbol: "BTC",
            instrument: "CRYPTO",
            proposal_count: 1,
            allow_count: 1,
            deny_count: 0,
            simulated_fill_count: 1,
            proposed_notional: "100.0000000000",
          },
        ],
        proposals: [currentProposal],
      },
      seven_days: {
        status: "AVAILABLE",
        coverage_status: "COMPLETE",
        horizon_hours: 168,
        window_started_at: "2026-08-24T15:00:00Z",
        window_ended_at: "2026-08-31T15:00:00Z",
        proposal_count: 2,
        allow_count: 2,
        deny_count: 0,
        simulated_fill_count: 2,
        minimum_proposed_notional: "50.0000000000",
        median_proposed_notional: "75.0000000000",
        maximum_proposed_notional: "100.0000000000",
        denial_reason_codes: [],
        failed_check_codes: [],
        expected_check_codes: paperCheckCodes,
        check_results: paperCheckCodes.map((code) => ({
          code,
          evaluation_count: 2,
          pass_count: 2,
          fail_count: 0,
          warn_count: 0,
        })),
        fully_evaluated_count: 2,
        fail_closed_prefix_count: 0,
        check_set_drift_count: 0,
        symbols: [
          {
            symbol: "BTC",
            instrument: "CRYPTO",
            proposal_count: 1,
            allow_count: 1,
            deny_count: 0,
            simulated_fill_count: 1,
            proposed_notional: "100.0000000000",
          },
          {
            symbol: "ETH",
            instrument: "CRYPTO",
            proposal_count: 1,
            allow_count: 1,
            deny_count: 0,
            simulated_fill_count: 1,
            proposed_notional: "50.0000000000",
          },
        ],
        proposals: [olderProposal, currentProposal],
      },
      coverage_change: {
        status: "AVAILABLE",
        calculation_method:
          "IMMUTABLE_24_HOUR_AND_SEVEN_DAY_GUARDRAIL_COVERAGE_COMPARISON",
        baseline_horizon_hours: 168,
        current_horizon_hours: 24,
        baseline_window_started_at: "2026-08-24T15:00:00Z",
        baseline_window_ended_at: "2026-08-31T15:00:00Z",
        current_window_started_at: "2026-08-30T15:00:00Z",
        current_window_ended_at: "2026-08-31T15:00:00Z",
        baseline_proposal_count: 2,
        current_proposal_count: 1,
        proposal_count_delta: -1,
        financial_providers: ["coinbase"],
        first_evidence_at: "2026-08-29T14:00:00Z",
        latest_evidence_at: "2026-08-31T14:00:00Z",
        first_market_observed_at: "2026-08-29T13:59:59Z",
        latest_market_observed_at: "2026-08-31T13:59:59Z",
        coverage_metrics: [
          {
            metric: "FULL_EVALUATION",
            baseline_count: 2,
            current_count: 1,
            count_delta: -1,
            baseline_share_percent: "100.0000000000",
            current_share_percent: "100.0000000000",
            share_change: "UNCHANGED",
          },
          {
            metric: "FAIL_CLOSED_PREFIX",
            baseline_count: 0,
            current_count: 0,
            count_delta: 0,
            baseline_share_percent: "0.0000000000",
            current_share_percent: "0.0000000000",
            share_change: "UNCHANGED",
          },
          {
            metric: "CHECK_SET_DRIFT",
            baseline_count: 0,
            current_count: 0,
            count_delta: 0,
            baseline_share_percent: "0.0000000000",
            current_share_percent: "0.0000000000",
            share_change: "UNCHANGED",
          },
        ],
        check_changes: paperCheckCodes.map((code) => ({
          code,
          baseline_evaluation_count: 2,
          current_evaluation_count: 1,
          evaluation_count_delta: -1,
          baseline_pass_count: 2,
          current_pass_count: 1,
          pass_count_delta: -1,
          baseline_fail_count: 0,
          current_fail_count: 0,
          fail_count_delta: 0,
          baseline_warn_count: 0,
          current_warn_count: 0,
          warn_count_delta: 0,
          baseline_evaluation_percent: "100.0000000000",
          current_evaluation_percent: "100.0000000000",
          evaluation_share_change: "UNCHANGED",
        })),
        symbol_changes: [
          {
            symbol: "BTC",
            instrument: "CRYPTO",
            baseline_proposal_count: 1,
            current_proposal_count: 1,
            proposal_count_delta: 0,
            baseline_proposal_percent: "50.0000000000",
            current_proposal_percent: "100.0000000000",
            proposal_share_change: "INCREASED",
            baseline_allow_count: 1,
            current_allow_count: 1,
            baseline_deny_count: 0,
            current_deny_count: 0,
            baseline_simulated_fill_count: 1,
            current_simulated_fill_count: 1,
            baseline_proposed_notional: "100.0000000000",
            current_proposed_notional: "100.0000000000",
            proposed_notional_delta: "0.0000000000",
          },
          {
            symbol: "ETH",
            instrument: "CRYPTO",
            baseline_proposal_count: 1,
            current_proposal_count: 0,
            proposal_count_delta: -1,
            baseline_proposal_percent: "50.0000000000",
            current_proposal_percent: "0.0000000000",
            proposal_share_change: "DECREASED",
            baseline_allow_count: 1,
            current_allow_count: 0,
            baseline_deny_count: 0,
            current_deny_count: 0,
            baseline_simulated_fill_count: 1,
            current_simulated_fill_count: 0,
            baseline_proposed_notional: "50.0000000000",
            current_proposed_notional: "0.0000000000",
            proposed_notional_delta: "-50.0000000000",
          },
        ],
      },
      denial_eligibility: {
        status: "AVAILABLE",
        calculation_method:
          "IMMUTABLE_PAPER_DETERMINISTIC_DENIAL_AND_LATER_ELIGIBILITY",
        horizon_hours: 168,
        window_started_at: "2026-08-24T15:00:00Z",
        window_ended_at: "2026-08-31T15:00:00Z",
        denial_count: 0,
        later_allowed_count: 0,
        later_denied_count: 0,
        no_later_comparable_proposal_count: 0,
        financial_providers: [],
        denials: [],
      },
    };
    render(
      <PaperPortfolioSummary
        executionMode="PAPER"
        portfolio={{
          ...portfolio,
          positions: [],
          guardrail_evidence: evidence,
        }}
      />,
    );
    const comparison = screen.getByRole("table", {
      name: /guardrail coverage window changes/i,
    });
    expect(within(comparison).getByText("FULL EVALUATION")).toBeInTheDocument();
    expect(
      screen.getByRole("table", { name: /guardrail stage coverage changes/i }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("table", { name: /guardrail symbol coverage changes/i }),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/none in the complete saved seven-day window/i),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/share changes compare coverage proportions/i),
    ).toBeInTheDocument();
  });
});
