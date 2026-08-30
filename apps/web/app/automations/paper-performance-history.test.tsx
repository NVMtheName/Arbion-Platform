import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { PaperPerformanceHistory } from "./paper-performance-history";

function decision({
  id,
  at,
  cash,
  marketValue,
}: {
  id: string;
  at: string;
  cash: string;
  marketValue?: string;
}) {
  return {
    id,
    source: "AI",
    decision_type: "ABSTAIN",
    created_at: at,
    structured_rationale: {
      execution_mode: "PAPER",
      decision: "ABSTAIN",
      symbol: "NONE",
      side: "NONE",
      input_evidence: {
        provider: "coinbase",
        available_cash_usd: cash,
        positions: marketValue
          ? [
              {
                symbol: "BTC",
                instrument: "CRYPTO",
                quantity: "0.0003177277",
                market_value_usd: marketValue,
                performance_status: "PARTIAL",
              },
            ]
          : [],
        markets: [
          {
            symbol: "BTC",
            asset_class: "CRYPTO",
            mark: "78098.31",
            feed: "rest_ticker",
            quality: "REAL_TIME_SINGLE_VENUE",
            observed_at: at,
          },
        ],
      },
    },
  };
}

describe("PaperPerformanceHistory", () => {
  afterEach(cleanup);

  it("shows exact immutable decision-time performance and provenance", () => {
    render(
      <PaperPerformanceHistory
        startingCash="1000.0000000000"
        currency="USD"
        decisions={[
          decision({
            id: "decision-2",
            at: "2026-08-30T02:28:30Z",
            cash: "975.0000077919",
            marketValue: "24.8139482347",
          }),
          decision({
            id: "decision-1",
            at: "2026-08-29T21:27:00Z",
            cash: "1000.0000000000",
          }),
        ]}
      />,
    );

    expect(
      screen.getByRole("heading", { name: /decision-time performance/i }),
    ).toBeInTheDocument();
    expect(screen.getByText("2 exact marks")).toBeInTheDocument();
    expect(screen.getAllByText("$999.814")).toHaveLength(2);
    expect(screen.getByText("-$0.186")).toBeInTheDocument();
    expect(screen.getByText("-0.0186%")).toBeInTheDocument();
    expect(
      screen.getByRole("img", {
        name: /2 decision-time simulated equity marks/i,
      }),
    ).toBeInTheDocument();
    const table = screen.getByRole("table", {
      name: /immutable decision-time paper performance marks/i,
    });
    expect(within(table).getAllByText(/coinbase · 1 market/i)).toHaveLength(2);
    expect(screen.getByText(/neither a live quote/i)).toBeInTheDocument();
  });

  it("fails closed and reports incomplete historical evidence", () => {
    const invalid = decision({
      id: "decision-invalid",
      at: "2026-08-30T02:28:30Z",
      cash: "975",
      marketValue: "25",
    });
    invalid.structured_rationale.input_evidence.markets[0].mark = "";
    render(
      <PaperPerformanceHistory
        startingCash="1000"
        currency="USD"
        decisions={[invalid]}
      />,
    );
    expect(screen.getByText(/history is collecting/i)).toBeInTheDocument();
    expect(
      screen.getByText(/1 Paper decision was omitted/i),
    ).toBeInTheDocument();
    expect(screen.queryByRole("img")).not.toBeInTheDocument();
  });
});
