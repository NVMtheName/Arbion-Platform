import { describe, expect, it } from "vitest";

import {
  calculatePaperPerformance,
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
});
