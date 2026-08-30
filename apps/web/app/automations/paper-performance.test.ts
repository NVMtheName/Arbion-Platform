import { describe, expect, it } from "vitest";

import {
  calculatePaperPerformance,
  extractPaperPerformanceHistory,
  extractLatestPaperMarketSnapshots,
} from "./paper-performance";

const marketDecision = {
  source: "AI",
  created_at: "2026-08-29T21:27:00Z",
  structured_rationale: {
    execution_mode: "PAPER",
    input_evidence: {
      provider: "coinbase",
      markets: [
        {
          symbol: "BTC",
          asset_class: "CRYPTO",
          mark: "78098.3100000000",
          last: "78098.3100000000",
          change_percent_24h: "0.9599000000",
          feed: "rest_ticker",
          quality: "REAL_TIME_SINGLE_VENUE",
          observed_at: "2026-08-29T21:26:55Z",
        },
      ],
    },
  },
};

describe("Paper performance", () => {
  it("selects the newest complete Paper decision without mixing market sets", () => {
    const snapshots = extractLatestPaperMarketSnapshots([
      {
        ...marketDecision,
        created_at: "2026-08-29T20:27:00Z",
        structured_rationale: {
          ...marketDecision.structured_rationale,
          input_evidence: {
            ...marketDecision.structured_rationale.input_evidence,
            markets: [
              {
                ...marketDecision.structured_rationale.input_evidence
                  .markets[0],
                mark: "77000",
              },
            ],
          },
        },
      },
      marketDecision,
    ]);

    expect(snapshots).toEqual([
      expect.objectContaining({
        symbol: "BTC",
        assetClass: "CRYPTO",
        price: "78098.3100000000",
        priceBasis: "MARK",
        provider: "coinbase",
        observedAt: "2026-08-29T21:26:55Z",
      }),
    ]);
  });

  it("computes exact simulated equity and return from one immutable snapshot", () => {
    const markets = extractLatestPaperMarketSnapshots([marketDecision]);
    const performance = calculatePaperPerformance(
      {
        starting_cash: "1000.0000000000",
        cash: "975.0000077919",
        positions: [
          {
            symbol: "BTC",
            instrument: "CRYPTO",
            quantity: "0.0003177277",
            average_price: "78683.7037126445",
            is_open: true,
          },
        ],
      },
      markets,
    );

    expect(performance).toEqual({
      status: "AVAILABLE",
      simulatedEquity: "999.814004202087",
      investedExposure: "24.813996410187",
      totalProfitLoss: "-0.185995797913",
      totalReturnPercent: "-0.0186",
      valuedAt: "2026-08-29T21:26:55Z",
      positions: [
        expect.objectContaining({
          marketValue: "24.813996410187",
          costBasis: "24.99999220809999790265",
          unrealizedProfitLoss: "-0.18599579791299790265",
          unrealizedProfitLossPercent: "-0.744",
        }),
      ],
    });
  });

  it("fails closed instead of inferring a missing current price", () => {
    const performance = calculatePaperPerformance(
      {
        starting_cash: "1000",
        cash: "975",
        positions: [
          {
            symbol: "ETH",
            instrument: "CRYPTO",
            quantity: "0.01",
            average_price: "2500",
            is_open: true,
          },
        ],
      },
      extractLatestPaperMarketSnapshots([marketDecision]),
    );

    expect(performance).toEqual({ status: "UNAVAILABLE", positions: [] });
  });

  it("derives exact chronological history only from complete Paper decisions", () => {
    const history = extractPaperPerformanceHistory(
      [
        {
          id: "later",
          source: "AI",
          created_at: "2026-08-30T02:28:30Z",
          structured_rationale: {
            execution_mode: "PAPER",
            decision: "ABSTAIN",
            input_evidence: {
              provider: "coinbase",
              available_cash_usd: "975.0000077919",
              positions: [
                {
                  symbol: "BTC",
                  instrument: "CRYPTO",
                  quantity: "0.0003177277",
                  market_value_usd: "24.8139482347",
                  performance_status: "PARTIAL",
                },
              ],
              markets: [
                {
                  symbol: "BTC",
                  mark: "78098.31",
                  feed: "rest_ticker",
                  quality: "REAL_TIME_SINGLE_VENUE",
                  observed_at: "2026-08-30T02:28:29Z",
                },
              ],
            },
          },
        },
        {
          id: "earlier",
          source: "AI",
          created_at: "2026-08-29T21:27:00Z",
          structured_rationale: {
            execution_mode: "PAPER",
            decision: "PROPOSE",
            symbol: "BTC",
            side: "BUY",
            input_evidence: {
              provider: "coinbase",
              available_cash_usd: "1000",
              positions: [],
              markets: [
                {
                  symbol: "BTC",
                  mark: "78097",
                  feed: "rest_ticker",
                  quality: "REAL_TIME_SINGLE_VENUE",
                  observed_at: "2026-08-29T21:26:59Z",
                },
              ],
            },
          },
        },
        {
          id: "incomplete",
          source: "AI",
          created_at: "2026-08-30T03:28:30Z",
          structured_rationale: {
            execution_mode: "PAPER",
            decision: "ABSTAIN",
            input_evidence: { provider: "coinbase" },
          },
        },
      ],
      "1000.0000000000",
    );

    expect(history.unavailableDecisionCount).toBe(1);
    expect(history.points.map((point) => point.decisionId)).toEqual([
      "earlier",
      "later",
    ]);
    expect(history.points[1]).toMatchObject({
      simulatedEquity: "999.8139560266",
      investedExposure: "24.8139482347",
      totalProfitLoss: "-0.1860439734",
      totalReturnPercent: "-0.0186",
      provider: "coinbase",
      marketCount: 1,
    });
  });
});
