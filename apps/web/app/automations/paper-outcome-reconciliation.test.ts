import { describe, expect, it } from "vitest";

import type { PaperPerformance } from "./paper-performance";
import { reconcilePaperOutcome } from "./paper-outcome-reconciliation";

const performance: PaperPerformance = {
  status: "AVAILABLE",
  simulatedEquity: "989.5035034864",
  investedExposure: "140.6266176978",
  totalProfitLoss: "-10.4964965136",
  totalReturnPercent: "-1.0496496514",
  valuedAt: "2026-08-31T04:36:27Z",
  positions: [
    {
      key: "CRYPTO:BTC",
      symbol: "BTC",
      assetClass: "CRYPTO",
      price: "77496.48",
      priceBasis: "MARK",
      provider: "coinbase",
      feed: "rest_ticker",
      quality: "REAL_TIME_SINGLE_VENUE",
      observedAt: "2026-08-31T04:36:27Z",
      decisionAt: "2026-08-31T04:36:28Z",
      marketValue: "48.1738346261",
      costBasis: "49.1331976016",
      unrealizedProfitLoss: "-0.9593629755",
      unrealizedProfitLossPercent: "-1.9525759005",
    },
    {
      key: "CRYPTO:ETH",
      symbol: "ETH",
      assetClass: "CRYPTO",
      price: "2413.9",
      priceBasis: "MARK",
      provider: "coinbase",
      feed: "rest_ticker",
      quality: "REAL_TIME_SINGLE_VENUE",
      observedAt: "2026-08-31T04:36:27Z",
      decisionAt: "2026-08-31T04:36:28Z",
      marketValue: "44.9218850683",
      costBasis: "46.4902485135",
      unrealizedProfitLoss: "-1.5683634452",
      unrealizedProfitLossPercent: "-3.3735320747",
    },
    {
      key: "CRYPTO:XRP",
      symbol: "XRP",
      assetClass: "CRYPTO",
      price: "1.3491",
      priceBasis: "MARK",
      provider: "coinbase",
      feed: "rest_ticker",
      quality: "REAL_TIME_SINGLE_VENUE",
      observedAt: "2026-08-31T04:36:27Z",
      decisionAt: "2026-08-31T04:36:28Z",
      marketValue: "47.5308980034",
      costBasis: "49.9999999999",
      unrealizedProfitLoss: "-2.4691019965",
      unrealizedProfitLossPercent: "-4.9382039930",
    },
  ],
};

describe("reconcilePaperOutcome", () => {
  it("keeps a production-like ten-decimal residual explicit", () => {
    const result = reconcilePaperOutcome({
      currency: "USD",
      cash: "848.8768857886",
      performance,
      realizedContractAvailable: true,
      realized: {
        status: "AVAILABLE",
        calculation_method: "AVERAGE_COST_INCLUDED_FEES",
        historical_coverage: "COMPLETE_FROM_PORTFOLIO_GENESIS",
        total_realized_profit_loss: "-5.4996680962",
        fill_count: 9,
        sell_fill_count: 2,
        symbols: [],
      },
    });

    expect(result).toMatchObject({
      status: "RECONCILED_WITH_DECIMAL_RESIDUAL",
      realizedProfitLoss: "-5.4996680962",
      unrealizedProfitLoss: "-4.9968284172",
      classifiedProfitLoss: "-10.4964965134",
      totalProfitLoss: "-10.4964965136",
      outcomeResidual: "-0.0000000002",
      equityResidual: "0",
      provider: "coinbase",
    });
  });

  it("flags a material outcome difference instead of hiding it", () => {
    const result = reconcilePaperOutcome({
      currency: "USD",
      cash: "848.8768857886",
      performance: { ...performance, totalProfitLoss: "-10.50" },
      realizedContractAvailable: true,
      realized: {
        status: "AVAILABLE",
        calculation_method: "AVERAGE_COST_INCLUDED_FEES",
        historical_coverage: "COMPLETE_FROM_PORTFOLIO_GENESIS",
        total_realized_profit_loss: "-5.4996680962",
        fill_count: 9,
        sell_fill_count: 2,
        symbols: [],
      },
    });

    expect(result.status).toBe("MISMATCH");
    expect(result.outcomeResidual).toBe("-0.0035034866");
  });

  it("fails closed when realized evidence is unavailable", () => {
    expect(
      reconcilePaperOutcome({
        currency: "USD",
        cash: "848.8768857886",
        performance,
        realizedContractAvailable: false,
      }).status,
    ).toBe("UNAVAILABLE");
  });
});
