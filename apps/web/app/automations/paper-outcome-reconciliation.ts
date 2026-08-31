import {
  compareExactDecimals,
  exactDecimalSign,
  subtractExactDecimals,
  sumExactMoney,
} from "../exact-money";
import type { PaperPerformance } from "./paper-performance";
import type { PaperRealizedOutcome } from "./paper-portfolio-summary";

export const PAPER_OUTCOME_RECONCILIATION_RESIDUAL_LIMIT = "0.000001";

export type PaperOutcomeReconciliation = {
  status:
    | "RECONCILED_EXACT"
    | "RECONCILED_WITH_DECIMAL_RESIDUAL"
    | "MISMATCH"
    | "UNAVAILABLE";
  realizedProfitLoss?: string;
  unrealizedProfitLoss?: string;
  classifiedProfitLoss?: string;
  totalProfitLoss?: string;
  outcomeResidual?: string;
  simulatedEquity?: string;
  cashPlusExposure?: string;
  equityResidual?: string;
  residualLimit: string;
  valuedAt?: string;
  provider?: string;
  marketFeeds: string[];
  marketQualities: string[];
};

function unavailable(): PaperOutcomeReconciliation {
  return {
    status: "UNAVAILABLE",
    residualLimit: PAPER_OUTCOME_RECONCILIATION_RESIDUAL_LIMIT,
    marketFeeds: [],
    marketQualities: [],
  };
}

function absolute(value: string) {
  return exactDecimalSign(value) === -1 ? value.slice(1) : value;
}

// Reconciles two independently derived, saved evidence paths. Total outcome is
// equity less starting cash. Classified outcome is exact realized fill replay
// plus unrealized P&L from one immutable market snapshot. Any difference is
// retained as an explicit decimal residual and never folded into either value.
export function reconcilePaperOutcome({
  currency,
  cash,
  performance,
  realized,
  realizedContractAvailable,
}: {
  currency: string;
  cash: string;
  performance: PaperPerformance;
  realized?: PaperRealizedOutcome;
  realizedContractAvailable: boolean;
}): PaperOutcomeReconciliation {
  const normalizedCurrency = currency.trim().toUpperCase();
  const performanceAvailable =
    performance.status === "AVAILABLE" || performance.status === "CASH_ONLY";
  if (
    !/^[A-Z]{3}$/.test(normalizedCurrency) ||
    !performanceAvailable ||
    !realizedContractAvailable ||
    !realized?.total_realized_profit_loss ||
    !performance.totalProfitLoss ||
    !performance.simulatedEquity ||
    !performance.investedExposure
  ) {
    return unavailable();
  }

  const unrealized =
    performance.status === "CASH_ONLY"
      ? "0"
      : sumExactMoney(
          performance.positions.map((position) => ({
            amount: position.unrealizedProfitLoss,
            currency: normalizedCurrency,
          })),
        )?.amount;
  if (!unrealized) return unavailable();

  const classified = sumExactMoney([
    {
      amount: realized.total_realized_profit_loss,
      currency: normalizedCurrency,
    },
    { amount: unrealized, currency: normalizedCurrency },
  ])?.amount;
  const cashPlusExposure = sumExactMoney([
    { amount: cash, currency: normalizedCurrency },
    { amount: performance.investedExposure, currency: normalizedCurrency },
  ])?.amount;
  if (!classified || !cashPlusExposure) return unavailable();

  const outcomeResidual = subtractExactDecimals(
    performance.totalProfitLoss,
    classified,
  );
  const equityResidual = subtractExactDecimals(
    performance.simulatedEquity,
    cashPlusExposure,
  );
  if (!outcomeResidual || !equityResidual) return unavailable();

  const equityMatches = compareExactDecimals(equityResidual, "0") === 0;
  const outcomeExact = compareExactDecimals(outcomeResidual, "0") === 0;
  const residualBounded =
    compareExactDecimals(
      absolute(outcomeResidual),
      PAPER_OUTCOME_RECONCILIATION_RESIDUAL_LIMIT,
    ) !== 1;
  const providers = new Set(
    performance.positions.map((position) => position.provider),
  );

  return {
    status:
      !equityMatches || !residualBounded
        ? "MISMATCH"
        : outcomeExact
          ? "RECONCILED_EXACT"
          : "RECONCILED_WITH_DECIMAL_RESIDUAL",
    realizedProfitLoss: realized.total_realized_profit_loss,
    unrealizedProfitLoss: unrealized,
    classifiedProfitLoss: classified,
    totalProfitLoss: performance.totalProfitLoss,
    outcomeResidual,
    simulatedEquity: performance.simulatedEquity,
    cashPlusExposure,
    equityResidual,
    residualLimit: PAPER_OUTCOME_RECONCILIATION_RESIDUAL_LIMIT,
    valuedAt: performance.valuedAt,
    provider: providers.size === 1 ? [...providers][0] : undefined,
    marketFeeds: [
      ...new Set(performance.positions.map((position) => position.feed)),
    ].sort(),
    marketQualities: [
      ...new Set(performance.positions.map((position) => position.quality)),
    ].sort(),
  };
}
