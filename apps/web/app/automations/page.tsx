import Link from "next/link";
import { cookies } from "next/headers";
import { redirect } from "next/navigation";

import { AppPageHeader } from "../app-page-header";
import {
  compareExactDecimals,
  divideExactDecimals,
  multiplyExactDecimals,
  subtractExactDecimals,
  sumExactMoney,
} from "../exact-money";
import {
  capitalReservationMatchesPolicy,
  paperCapitalReservationMatchesPolicy,
} from "./capital-authority";
import { asList } from "./response";
import {
  calculatePaperPerformance,
  extractLatestPaperMarketSnapshots,
} from "./paper-performance";
import { reconcilePaperOutcome } from "./paper-outcome-reconciliation";
import type {
  PaperExecutionCosts,
  PaperExecutionCheckpoint,
  PaperExecutionSideCost,
  PaperExecutionSymbolCost,
  PaperPortfolio,
  PaperPosition,
  PaperRealizedOutcome,
  PaperRealizedSymbolOutcome,
  PaperTradeSequenceEvidence,
  PaperTradeSequenceSymbol,
} from "./paper-portfolio-summary";
import {
  projectPinnedAIRuntime,
  scheduleMatchesPinnedAIRuntime,
} from "./runtime-contract";
import {
  reconciliationFreshWithinTwentyFourHours,
  scheduledRunTimingStatus,
  StrategyFleet,
  type StrategyFleetDecisionInputCoverageSnapshot,
  type StrategyFleetItem,
  type StrategyFleetOutcomeHistorySnapshot,
} from "./strategy-fleet";

type RecordValue = Record<string, unknown>;

type DecisionWindow = {
  decisions?: RecordValue[] | null;
  decision_history_semantics?: string;
  model_rerun?: boolean;
  financial_provider_called?: boolean;
  broker_action_available?: boolean;
  live_execution_available?: boolean;
};

type ShadowOutcomeWindow = {
  outcomes?: RecordValue[] | null;
  performance_semantics?: string;
  fees_and_slippage_included?: boolean;
  live_execution_available?: boolean;
};

type ScheduleRunWindow = {
  runs?: RecordValue[] | null;
  history_semantics?: string;
  broker_action_available?: boolean;
  live_execution_available?: boolean;
};

type ReconciliationEnvelope = {
  reconciliation?: RecordValue;
  autonomy_enforcement_active?: boolean;
  live_execution_available?: boolean;
};

type PaperPortfolioEnvelope = {
  paper_portfolio?: RecordValue;
  realized_outcome_semantics?: string;
  realized_outcome_includes_fees?: boolean;
  execution_cost_semantics?: string;
  execution_costs_broker_reported?: boolean;
  broker_action_available?: boolean;
  live_execution_available?: boolean;
};

function normalizedPaperTradeSequence(
  value: unknown,
  fillCount: number,
  providerReferenceNotional?: string,
  totalExplicitCost?: string,
): PaperTradeSequenceEvidence | undefined {
  const sequence = record(value);
  const status = text(sequence, "status", "Status");
  const rawSymbols = sequence?.symbols ?? sequence?.Symbols;
  const sequenceFillCount = nonnegativeInteger(
    sequence?.fill_count ?? sequence?.FillCount,
  );
  const sameCount = nonnegativeInteger(
    sequence?.same_side_transition_count ?? sequence?.SameSideTransitionCount,
  );
  const oppositeCount = nonnegativeInteger(
    sequence?.opposite_side_transition_count ??
      sequence?.OppositeSideTransitionCount,
  );
  const buyToSellCount = nonnegativeInteger(
    sequence?.buy_to_sell_reversal_count ?? sequence?.BuyToSellReversalCount,
  );
  const sellToBuyCount = nonnegativeInteger(
    sequence?.sell_to_buy_reversal_count ?? sequence?.SellToBuyReversalCount,
  );
  if (
    !["AVAILABLE", "NO_FILLS", "UNAVAILABLE"].includes(status ?? "") ||
    sequenceFillCount !== fillCount ||
    sameCount === undefined ||
    oppositeCount === undefined ||
    buyToSellCount === undefined ||
    sellToBuyCount === undefined ||
    buyToSellCount + sellToBuyCount !== oppositeCount ||
    !Array.isArray(rawSymbols)
  )
    return;
  if (status === "UNAVAILABLE") {
    if (rawSymbols.length !== 0) return;
    return {
      status: "UNAVAILABLE",
      fill_count: fillCount,
      same_side_transition_count: sameCount,
      opposite_side_transition_count: oppositeCount,
      buy_to_sell_reversal_count: buyToSellCount,
      sell_to_buy_reversal_count: sellToBuyCount,
      symbols: [],
    };
  }
  const calculationMethod = text(
    sequence,
    "calculation_method",
    "CalculationMethod",
  );
  const historicalCoverage = text(
    sequence,
    "historical_coverage",
    "HistoricalCoverage",
  );
  const startingCash = exactDecimal(
    sequence?.starting_cash ?? sequence?.StartingCash,
  );
  const turnoverRate = exactDecimal(
    sequence?.provider_reference_turnover_to_starting_cash_bps ??
      sequence?.ProviderReferenceTurnoverToStartingCashBPS,
  );
  const explicitCostRate = exactDecimal(
    sequence?.explicit_cost_to_starting_cash_bps ??
      sequence?.ExplicitCostToStartingCashBPS,
  );
  if (
    calculationMethod !== "COMPLETE_IMMUTABLE_FILL_SEQUENCE" ||
    historicalCoverage !== "COMPLETE_FROM_PORTFOLIO_GENESIS" ||
    !startingCash ||
    !turnoverRate ||
    !explicitCostRate ||
    (compareExactDecimals(startingCash, "0") ?? 0) <= 0 ||
    !providerReferenceNotional ||
    !totalExplicitCost ||
    compareExactDecimals(
      divideExactDecimals(
        multiplyExactDecimals(providerReferenceNotional, "10000") ?? "invalid",
        startingCash,
        10,
      ) ?? "invalid",
      turnoverRate,
    ) !== 0 ||
    compareExactDecimals(
      divideExactDecimals(
        multiplyExactDecimals(totalExplicitCost, "10000") ?? "invalid",
        startingCash,
        10,
      ) ?? "invalid",
      explicitCostRate,
    ) !== 0
  )
    return;
  const symbols: PaperTradeSequenceSymbol[] = [];
  const seen = new Set<string>();
  for (const rawSymbol of rawSymbols) {
    const symbolRecord = record(rawSymbol);
    const symbol = text(symbolRecord, "symbol", "Symbol");
    const instrument = text(symbolRecord, "instrument", "Instrument");
    const symbolFillCount = nonnegativeInteger(
      symbolRecord?.fill_count ?? symbolRecord?.FillCount,
    );
    const buyCount = nonnegativeInteger(
      symbolRecord?.buy_fill_count ?? symbolRecord?.BuyFillCount,
    );
    const sellCount = nonnegativeInteger(
      symbolRecord?.sell_fill_count ?? symbolRecord?.SellFillCount,
    );
    const symbolSameCount = nonnegativeInteger(
      symbolRecord?.same_side_transition_count ??
        symbolRecord?.SameSideTransitionCount,
    );
    const symbolOppositeCount = nonnegativeInteger(
      symbolRecord?.opposite_side_transition_count ??
        symbolRecord?.OppositeSideTransitionCount,
    );
    const symbolBuyToSell = nonnegativeInteger(
      symbolRecord?.buy_to_sell_reversal_count ??
        symbolRecord?.BuyToSellReversalCount,
    );
    const symbolSellToBuy = nonnegativeInteger(
      symbolRecord?.sell_to_buy_reversal_count ??
        symbolRecord?.SellToBuyReversalCount,
    );
    const longestStreak = nonnegativeInteger(
      symbolRecord?.longest_same_side_streak ??
        symbolRecord?.LongestSameSideStreak,
    );
    const firstSide = text(symbolRecord, "first_side", "FirstSide");
    const lastSide = text(symbolRecord, "last_side", "LastSide");
    const firstFillAt = text(symbolRecord, "first_fill_at", "FirstFillAt");
    const lastFillAt = text(symbolRecord, "last_fill_at", "LastFillAt");
    const key = `${instrument}:${symbol}`;
    if (
      !symbol ||
      !["EQUITY", "CRYPTO"].includes(instrument ?? "") ||
      symbolFillCount === undefined ||
      symbolFillCount < 1 ||
      buyCount === undefined ||
      sellCount === undefined ||
      buyCount + sellCount !== symbolFillCount ||
      symbolSameCount === undefined ||
      symbolOppositeCount === undefined ||
      symbolSameCount + symbolOppositeCount !== symbolFillCount - 1 ||
      symbolBuyToSell === undefined ||
      symbolSellToBuy === undefined ||
      symbolBuyToSell + symbolSellToBuy !== symbolOppositeCount ||
      longestStreak === undefined ||
      longestStreak < 1 ||
      longestStreak > symbolFillCount ||
      !["BUY", "SELL"].includes(firstSide ?? "") ||
      !["BUY", "SELL"].includes(lastSide ?? "") ||
      !firstFillAt ||
      !lastFillAt ||
      Number.isNaN(Date.parse(firstFillAt)) ||
      Number.isNaN(Date.parse(lastFillAt)) ||
      Date.parse(firstFillAt) > Date.parse(lastFillAt) ||
      seen.has(key)
    )
      return;
    seen.add(key);
    symbols.push({
      symbol,
      instrument: instrument as PaperTradeSequenceSymbol["instrument"],
      fill_count: symbolFillCount,
      buy_fill_count: buyCount,
      sell_fill_count: sellCount,
      same_side_transition_count: symbolSameCount,
      opposite_side_transition_count: symbolOppositeCount,
      buy_to_sell_reversal_count: symbolBuyToSell,
      sell_to_buy_reversal_count: symbolSellToBuy,
      longest_same_side_streak: longestStreak,
      first_side: firstSide as PaperTradeSequenceSymbol["first_side"],
      last_side: lastSide as PaperTradeSequenceSymbol["last_side"],
      first_fill_at: firstFillAt,
      last_fill_at: lastFillAt,
    });
  }
  if (
    symbols.reduce((total, symbol) => total + symbol.fill_count, 0) !==
      fillCount ||
    symbols.reduce(
      (total, symbol) => total + symbol.same_side_transition_count,
      0,
    ) !== sameCount ||
    symbols.reduce(
      (total, symbol) => total + symbol.opposite_side_transition_count,
      0,
    ) !== oppositeCount ||
    symbols.reduce(
      (total, symbol) => total + symbol.buy_to_sell_reversal_count,
      0,
    ) !== buyToSellCount ||
    symbols.reduce(
      (total, symbol) => total + symbol.sell_to_buy_reversal_count,
      0,
    ) !== sellToBuyCount ||
    (fillCount === 0 ? symbols.length !== 0 : symbols.length === 0) ||
    (status === "AVAILABLE" ? fillCount === 0 : fillCount !== 0)
  )
    return;
  return {
    status: status as PaperTradeSequenceEvidence["status"],
    calculation_method: calculationMethod,
    historical_coverage: historicalCoverage,
    starting_cash: startingCash,
    provider_reference_turnover_to_starting_cash_bps: turnoverRate,
    explicit_cost_to_starting_cash_bps: explicitCostRate,
    fill_count: fillCount,
    same_side_transition_count: sameCount,
    opposite_side_transition_count: oppositeCount,
    buy_to_sell_reversal_count: buyToSellCount,
    sell_to_buy_reversal_count: sellToBuyCount,
    symbols,
  };
}

function normalizedPaperExecutionCosts(
  value: unknown,
): PaperExecutionCosts | undefined {
  const costs = record(value);
  const status = text(costs, "status", "Status");
  const fillCount = nonnegativeInteger(costs?.fill_count ?? costs?.FillCount);
  const buyFillCount = nonnegativeInteger(
    costs?.buy_fill_count ?? costs?.BuyFillCount,
  );
  const sellFillCount = nonnegativeInteger(
    costs?.sell_fill_count ?? costs?.SellFillCount,
  );
  const rawSides = costs?.sides ?? costs?.Sides;
  const rawSymbols = costs?.symbols ?? costs?.Symbols;
  const rawTimeline = costs?.timeline ?? costs?.Timeline;
  const rawTradeSequence = costs?.trade_sequence ?? costs?.TradeSequence;
  const timelineSampleCount = nonnegativeInteger(
    costs?.timeline_sample_count ?? costs?.TimelineSampleCount,
  );
  const timelineCappedValue = costs?.timeline_capped ?? costs?.TimelineCapped;
  const marketProviders = stringList(
    costs?.market_providers ?? costs?.MarketProviders,
  );
  const marketFeeds = stringList(costs?.market_feeds ?? costs?.MarketFeeds);
  const marketQualities = stringList(
    costs?.market_qualities ?? costs?.MarketQualities,
  );
  if (
    !["AVAILABLE", "NO_FILLS", "UNAVAILABLE"].includes(status ?? "") ||
    fillCount === undefined ||
    buyFillCount === undefined ||
    sellFillCount === undefined ||
    buyFillCount + sellFillCount !== fillCount ||
    !Array.isArray(rawSides) ||
    !Array.isArray(rawSymbols) ||
    !Array.isArray(rawTimeline) ||
    timelineSampleCount === undefined ||
    typeof timelineCappedValue !== "boolean"
  )
    return;
  if (status === "UNAVAILABLE") {
    const tradeSequence = normalizedPaperTradeSequence(
      rawTradeSequence,
      fillCount,
    );
    if (
      rawSides.length !== 0 ||
      rawSymbols.length !== 0 ||
      rawTimeline.length !== 0 ||
      timelineSampleCount !== 0 ||
      timelineCappedValue ||
      tradeSequence?.status !== "UNAVAILABLE"
    )
      return;
    return {
      status: "UNAVAILABLE",
      fill_count: fillCount,
      buy_fill_count: buyFillCount,
      sell_fill_count: sellFillCount,
      market_providers: marketProviders,
      market_feeds: marketFeeds,
      market_qualities: marketQualities,
      sides: [],
      symbols: [],
      timeline_sample_count: 0,
      timeline_capped: false,
      timeline: [],
      trade_sequence: tradeSequence,
    };
  }
  const calculationMethod = text(
    costs,
    "calculation_method",
    "CalculationMethod",
  );
  const historicalCoverage = text(
    costs,
    "historical_coverage",
    "HistoricalCoverage",
  );
  const totalFees = exactDecimal(costs?.total_fees ?? costs?.TotalFees);
  const totalAdverseSlippage = exactDecimal(
    costs?.total_adverse_slippage ?? costs?.TotalAdverseSlippage,
  );
  const totalExplicitCost = exactDecimal(
    costs?.total_explicit_cost ?? costs?.TotalExplicitCost,
  );
  const providerReferenceNotional = exactDecimal(
    costs?.provider_reference_notional ?? costs?.ProviderReferenceNotional,
  );
  const grossNotional = exactDecimal(
    costs?.gross_notional ?? costs?.GrossNotional,
  );
  const allInCostRateBPS = exactDecimal(
    costs?.all_in_cost_rate_bps ?? costs?.AllInCostRateBPS,
  );
  const fillNotionalResidual = exactDecimal(
    costs?.fill_notional_residual ?? costs?.FillNotionalResidual,
  );
  const maximumAbsoluteFillResidual = exactDecimal(
    costs?.maximum_absolute_fill_residual ?? costs?.MaximumAbsoluteFillResidual,
  );
  const residualBoundPerFill = exactDecimal(
    costs?.residual_bound_per_fill ?? costs?.ResidualBoundPerFill,
  );
  const tradeSequence = normalizedPaperTradeSequence(
    rawTradeSequence,
    fillCount,
    providerReferenceNotional,
    totalExplicitCost,
  );
  const expectedExplicitCost =
    totalFees && totalAdverseSlippage
      ? sumUSD([totalFees, totalAdverseSlippage])
      : undefined;
  const expectedRate =
    totalExplicitCost && providerReferenceNotional
      ? divideExactDecimals(
          multiplyExactDecimals(totalExplicitCost, "10000") ?? "invalid",
          providerReferenceNotional,
          10,
        )
      : undefined;
  const firstFillAt = text(costs, "first_fill_at", "FirstFillAt");
  const lastFillAt = text(costs, "last_fill_at", "LastFillAt");
  if (
    calculationMethod !== "SAVED_REFERENCE_VERSUS_SIMULATED_FILL" ||
    historicalCoverage !== "COMPLETE_FROM_PORTFOLIO_GENESIS" ||
    !totalFees ||
    !totalAdverseSlippage ||
    !totalExplicitCost ||
    !providerReferenceNotional ||
    !grossNotional ||
    !allInCostRateBPS ||
    !fillNotionalResidual ||
    !maximumAbsoluteFillResidual ||
    !residualBoundPerFill ||
    !tradeSequence ||
    (compareExactDecimals(totalFees, "0") ?? -1) < 0 ||
    (compareExactDecimals(totalAdverseSlippage, "0") ?? -1) < 0 ||
    (compareExactDecimals(totalExplicitCost, "0") ?? -1) < 0 ||
    (compareExactDecimals(providerReferenceNotional, "0") ?? -1) < 0 ||
    (compareExactDecimals(grossNotional, "0") ?? -1) < 0 ||
    (compareExactDecimals(allInCostRateBPS, "0") ?? -1) < 0 ||
    (compareExactDecimals(maximumAbsoluteFillResidual, "0") ?? -1) < 0 ||
    (compareExactDecimals(residualBoundPerFill, "0") ?? 1) <= 0 ||
    compareExactDecimals(maximumAbsoluteFillResidual, residualBoundPerFill) ===
      1 ||
    compareExactDecimals(
      fillNotionalResidual,
      multiplyExactDecimals(residualBoundPerFill, String(-fillCount)) ??
        "invalid",
    ) === -1 ||
    compareExactDecimals(
      fillNotionalResidual,
      multiplyExactDecimals(residualBoundPerFill, String(fillCount)) ??
        "invalid",
    ) === 1 ||
    compareExactDecimals(
      expectedExplicitCost ?? "invalid",
      totalExplicitCost,
    ) !== 0 ||
    (fillCount === 0
      ? compareExactDecimals(allInCostRateBPS, "0") !== 0 ||
        compareExactDecimals(providerReferenceNotional, "0") !== 0
      : compareExactDecimals(expectedRate ?? "invalid", allInCostRateBPS) !==
        0) ||
    (status === "NO_FILLS" && fillCount !== 0) ||
    (status === "AVAILABLE" && fillCount === 0) ||
    (fillCount > 0 &&
      (!firstFillAt ||
        !lastFillAt ||
        Number.isNaN(Date.parse(firstFillAt)) ||
        Number.isNaN(Date.parse(lastFillAt))))
  )
    return;
  const sides: PaperExecutionSideCost[] = [];
  const seenSides = new Set<string>();
  for (const rawSide of rawSides) {
    const sideCost = record(rawSide);
    const side = text(sideCost, "side", "Side");
    const sideFees = exactDecimal(sideCost?.total_fees ?? sideCost?.TotalFees);
    const sideSlippage = exactDecimal(
      sideCost?.adverse_slippage ?? sideCost?.AdverseSlippage,
    );
    const sideExplicit = exactDecimal(
      sideCost?.total_explicit_cost ?? sideCost?.TotalExplicitCost,
    );
    const sideReference = exactDecimal(
      sideCost?.provider_reference_notional ??
        sideCost?.ProviderReferenceNotional,
    );
    const sideGross = exactDecimal(
      sideCost?.gross_notional ?? sideCost?.GrossNotional,
    );
    const sideRate = exactDecimal(
      sideCost?.all_in_cost_rate_bps ?? sideCost?.AllInCostRateBPS,
    );
    const sideFillCount = nonnegativeInteger(
      sideCost?.fill_count ?? sideCost?.FillCount,
    );
    const expectedSideExplicit =
      sideFees && sideSlippage ? sumUSD([sideFees, sideSlippage]) : undefined;
    const expectedSideRate =
      sideExplicit && sideReference
        ? divideExactDecimals(
            multiplyExactDecimals(sideExplicit, "10000") ?? "invalid",
            sideReference,
            10,
          )
        : undefined;
    if (
      !side ||
      !["BUY", "SELL"].includes(side) ||
      !sideFees ||
      !sideSlippage ||
      !sideExplicit ||
      !sideReference ||
      !sideGross ||
      !sideRate ||
      sideFillCount === undefined ||
      sideFillCount < 1 ||
      (compareExactDecimals(sideFees, "0") ?? -1) < 0 ||
      (compareExactDecimals(sideSlippage, "0") ?? -1) < 0 ||
      (compareExactDecimals(sideReference, "0") ?? 0) <= 0 ||
      (compareExactDecimals(sideGross, "0") ?? 0) <= 0 ||
      compareExactDecimals(expectedSideExplicit ?? "invalid", sideExplicit) !==
        0 ||
      compareExactDecimals(expectedSideRate ?? "invalid", sideRate) !== 0 ||
      seenSides.has(side)
    )
      return;
    seenSides.add(side);
    sides.push({
      side: side as PaperExecutionSideCost["side"],
      total_fees: sideFees,
      adverse_slippage: sideSlippage,
      total_explicit_cost: sideExplicit,
      provider_reference_notional: sideReference,
      gross_notional: sideGross,
      all_in_cost_rate_bps: sideRate,
      fill_count: sideFillCount,
    });
  }
  const symbols: PaperExecutionSymbolCost[] = [];
  const seenSymbols = new Set<string>();
  for (const rawSymbol of rawSymbols) {
    const symbolCost = record(rawSymbol);
    const symbol = text(symbolCost, "symbol", "Symbol");
    const instrument = text(symbolCost, "instrument", "Instrument");
    const symbolFees = exactDecimal(
      symbolCost?.total_fees ?? symbolCost?.TotalFees,
    );
    const adverseSlippage = exactDecimal(
      symbolCost?.adverse_slippage ?? symbolCost?.AdverseSlippage,
    );
    const symbolExplicit = exactDecimal(
      symbolCost?.total_explicit_cost ?? symbolCost?.TotalExplicitCost,
    );
    const symbolReference = exactDecimal(
      symbolCost?.provider_reference_notional ??
        symbolCost?.ProviderReferenceNotional,
    );
    const symbolGross = exactDecimal(
      symbolCost?.gross_notional ?? symbolCost?.GrossNotional,
    );
    const symbolRate = exactDecimal(
      symbolCost?.all_in_cost_rate_bps ?? symbolCost?.AllInCostRateBPS,
    );
    const symbolFillCount = nonnegativeInteger(
      symbolCost?.fill_count ?? symbolCost?.FillCount,
    );
    const symbolBuyCount = nonnegativeInteger(
      symbolCost?.buy_fill_count ?? symbolCost?.BuyFillCount,
    );
    const symbolSellCount = nonnegativeInteger(
      symbolCost?.sell_fill_count ?? symbolCost?.SellFillCount,
    );
    const key = `${instrument}:${symbol}`;
    const expectedSymbolExplicit =
      symbolFees && adverseSlippage
        ? sumUSD([symbolFees, adverseSlippage])
        : undefined;
    const expectedSymbolRate =
      symbolExplicit && symbolReference
        ? divideExactDecimals(
            multiplyExactDecimals(symbolExplicit, "10000") ?? "invalid",
            symbolReference,
            10,
          )
        : undefined;
    if (
      !symbol ||
      !["EQUITY", "CRYPTO"].includes(instrument ?? "") ||
      !symbolFees ||
      !adverseSlippage ||
      !symbolExplicit ||
      !symbolReference ||
      !symbolGross ||
      !symbolRate ||
      symbolFillCount === undefined ||
      symbolBuyCount === undefined ||
      symbolSellCount === undefined ||
      symbolBuyCount + symbolSellCount !== symbolFillCount ||
      (compareExactDecimals(symbolFees, "0") ?? -1) < 0 ||
      (compareExactDecimals(adverseSlippage, "0") ?? -1) < 0 ||
      (compareExactDecimals(symbolReference, "0") ?? 0) <= 0 ||
      (compareExactDecimals(symbolGross, "0") ?? -1) < 0 ||
      compareExactDecimals(
        expectedSymbolExplicit ?? "invalid",
        symbolExplicit,
      ) !== 0 ||
      compareExactDecimals(expectedSymbolRate ?? "invalid", symbolRate) !== 0 ||
      seenSymbols.has(key)
    )
      return;
    seenSymbols.add(key);
    symbols.push({
      symbol,
      instrument: instrument as PaperExecutionSymbolCost["instrument"],
      total_fees: symbolFees,
      adverse_slippage: adverseSlippage,
      total_explicit_cost: symbolExplicit,
      provider_reference_notional: symbolReference,
      gross_notional: symbolGross,
      all_in_cost_rate_bps: symbolRate,
      fill_count: symbolFillCount,
      buy_fill_count: symbolBuyCount,
      sell_fill_count: symbolSellCount,
    });
  }
  const timeline: PaperExecutionCheckpoint[] = [];
  const seenTimelineIDs = new Set<string>();
  for (const rawCheckpoint of rawTimeline) {
    const checkpoint = record(rawCheckpoint);
    const sequence = nonnegativeInteger(
      checkpoint?.sequence ?? checkpoint?.Sequence,
    );
    const fillID = text(checkpoint, "fill_id", "FillID");
    const executionRecordID = text(
      checkpoint,
      "execution_record_id",
      "ExecutionRecordID",
    );
    const proposedActionID = text(
      checkpoint,
      "proposed_action_id",
      "ProposedActionID",
    );
    const riskEvaluationID = text(
      checkpoint,
      "risk_evaluation_id",
      "RiskEvaluationID",
    );
    const symbol = text(checkpoint, "symbol", "Symbol");
    const instrument = text(checkpoint, "instrument", "Instrument");
    const side = text(checkpoint, "side", "Side");
    const fee = exactDecimal(checkpoint?.fee ?? checkpoint?.Fee);
    const adverseSlippage = exactDecimal(
      checkpoint?.adverse_slippage ?? checkpoint?.AdverseSlippage,
    );
    const explicitCost = exactDecimal(
      checkpoint?.explicit_cost ?? checkpoint?.ExplicitCost,
    );
    const referenceNotional = exactDecimal(
      checkpoint?.provider_reference_notional ??
        checkpoint?.ProviderReferenceNotional,
    );
    const checkpointGross = exactDecimal(
      checkpoint?.gross_notional ?? checkpoint?.GrossNotional,
    );
    const checkpointResidual = exactDecimal(
      checkpoint?.fill_notional_residual ?? checkpoint?.FillNotionalResidual,
    );
    const cumulativeFees = exactDecimal(
      checkpoint?.cumulative_fees ?? checkpoint?.CumulativeFees,
    );
    const cumulativeSlippage = exactDecimal(
      checkpoint?.cumulative_adverse_slippage ??
        checkpoint?.CumulativeAdverseSlippage,
    );
    const cumulativeExplicit = exactDecimal(
      checkpoint?.cumulative_explicit_cost ??
        checkpoint?.CumulativeExplicitCost,
    );
    const cumulativeReference = exactDecimal(
      checkpoint?.cumulative_provider_reference_notional ??
        checkpoint?.CumulativeProviderReferenceNotional,
    );
    const cumulativeGross = exactDecimal(
      checkpoint?.cumulative_gross_notional ??
        checkpoint?.CumulativeGrossNotional,
    );
    const cumulativeRate = exactDecimal(
      checkpoint?.cumulative_all_in_cost_rate_bps ??
        checkpoint?.CumulativeAllInCostRateBPS,
    );
    const rateChange = text(
      checkpoint,
      "cumulative_rate_change",
      "CumulativeRateChange",
    );
    const symbolSequence = nonnegativeInteger(
      checkpoint?.symbol_sequence ?? checkpoint?.SymbolSequence,
    );
    const sameSideStreak = nonnegativeInteger(
      checkpoint?.same_side_streak ?? checkpoint?.SameSideStreak,
    );
    const sideTransition = text(
      checkpoint,
      "side_transition",
      "SideTransition",
    );
    const oppositeSideElapsedSeconds = exactDecimal(
      checkpoint?.opposite_side_elapsed_seconds ??
        checkpoint?.OppositeSideElapsedSeconds,
    );
    const marketProvider = text(
      checkpoint,
      "market_provider",
      "MarketProvider",
    );
    const marketFeed = text(checkpoint, "market_feed", "MarketFeed");
    const marketQuality = text(checkpoint, "market_quality", "MarketQuality");
    const marketObservedAt = text(
      checkpoint,
      "market_observed_at",
      "MarketObservedAt",
    );
    const simulatedAt = text(checkpoint, "simulated_at", "SimulatedAt");
    const expectedExplicit =
      fee && adverseSlippage ? sumUSD([fee, adverseSlippage]) : undefined;
    const expectedCumulativeExplicit =
      cumulativeFees && cumulativeSlippage
        ? sumUSD([cumulativeFees, cumulativeSlippage])
        : undefined;
    const expectedCumulativeRate =
      cumulativeExplicit && cumulativeReference
        ? divideExactDecimals(
            multiplyExactDecimals(cumulativeExplicit, "10000") ?? "invalid",
            cumulativeReference,
            10,
          )
        : undefined;
    if (
      sequence === undefined ||
      sequence < 1 ||
      !fillID ||
      !executionRecordID ||
      !proposedActionID ||
      !riskEvaluationID ||
      !symbol ||
      !["EQUITY", "CRYPTO"].includes(instrument ?? "") ||
      !["BUY", "SELL"].includes(side ?? "") ||
      !fee ||
      !adverseSlippage ||
      !explicitCost ||
      !referenceNotional ||
      !checkpointGross ||
      !checkpointResidual ||
      !cumulativeFees ||
      !cumulativeSlippage ||
      !cumulativeExplicit ||
      !cumulativeReference ||
      !cumulativeGross ||
      !cumulativeRate ||
      !["FIRST", "ROSE", "FELL", "HELD"].includes(rateChange ?? "") ||
      symbolSequence === undefined ||
      symbolSequence < 1 ||
      sameSideStreak === undefined ||
      sameSideStreak < 1 ||
      sameSideStreak > symbolSequence ||
      !["FIRST", "SAME_SIDE", "BUY_TO_SELL", "SELL_TO_BUY"].includes(
        sideTransition ?? "",
      ) ||
      (symbolSequence === 1
        ? sideTransition !== "FIRST"
        : sideTransition === "FIRST") ||
      (["BUY_TO_SELL", "SELL_TO_BUY"].includes(sideTransition ?? "")
        ? !oppositeSideElapsedSeconds ||
          (compareExactDecimals(oppositeSideElapsedSeconds, "0") ?? 0) < 0
        : oppositeSideElapsedSeconds !== undefined) ||
      !marketProvider ||
      !marketFeed ||
      !marketQuality ||
      !marketObservedAt ||
      !simulatedAt ||
      Number.isNaN(Date.parse(marketObservedAt)) ||
      Number.isNaN(Date.parse(simulatedAt)) ||
      Date.parse(marketObservedAt) < Date.parse(simulatedAt) - 120_000 ||
      Date.parse(marketObservedAt) > Date.parse(simulatedAt) + 5_000 ||
      (compareExactDecimals(fee, "0") ?? -1) < 0 ||
      (compareExactDecimals(adverseSlippage, "0") ?? -1) < 0 ||
      (compareExactDecimals(referenceNotional, "0") ?? 0) <= 0 ||
      (compareExactDecimals(checkpointGross, "0") ?? 0) <= 0 ||
      compareExactDecimals(expectedExplicit ?? "invalid", explicitCost) !== 0 ||
      compareExactDecimals(
        expectedCumulativeExplicit ?? "invalid",
        cumulativeExplicit,
      ) !== 0 ||
      compareExactDecimals(
        expectedCumulativeRate ?? "invalid",
        cumulativeRate,
      ) !== 0 ||
      compareExactDecimals(
        checkpointResidual,
        multiplyExactDecimals(residualBoundPerFill, "-1") ?? "invalid",
      ) === -1 ||
      compareExactDecimals(checkpointResidual, residualBoundPerFill) === 1 ||
      seenTimelineIDs.has(fillID)
    )
      return;
    const prior = timeline.at(-1);
    if (
      (prior &&
        (sequence !== prior.sequence + 1 ||
          Date.parse(simulatedAt) < Date.parse(prior.simulated_at) ||
          compareExactDecimals(
            subtractExactDecimals(cumulativeFees, prior.cumulative_fees) ??
              "invalid",
            fee,
          ) !== 0 ||
          compareExactDecimals(
            subtractExactDecimals(
              cumulativeSlippage,
              prior.cumulative_adverse_slippage,
            ) ?? "invalid",
            adverseSlippage,
          ) !== 0 ||
          compareExactDecimals(
            subtractExactDecimals(
              cumulativeReference,
              prior.cumulative_provider_reference_notional,
            ) ?? "invalid",
            referenceNotional,
          ) !== 0 ||
          compareExactDecimals(
            subtractExactDecimals(
              cumulativeGross,
              prior.cumulative_gross_notional,
            ) ?? "invalid",
            checkpointGross,
          ) !== 0 ||
          rateChange !==
            (compareExactDecimals(
              cumulativeRate,
              prior.cumulative_all_in_cost_rate_bps,
            ) === 1
              ? "ROSE"
              : compareExactDecimals(
                    cumulativeRate,
                    prior.cumulative_all_in_cost_rate_bps,
                  ) === -1
                ? "FELL"
                : "HELD"))) ||
      (!prior && sequence === 1 && rateChange !== "FIRST") ||
      (!prior && sequence > 1 && rateChange === "FIRST")
    )
      return;
    seenTimelineIDs.add(fillID);
    timeline.push({
      sequence,
      fill_id: fillID,
      execution_record_id: executionRecordID,
      proposed_action_id: proposedActionID,
      risk_evaluation_id: riskEvaluationID,
      symbol,
      instrument: instrument as PaperExecutionCheckpoint["instrument"],
      side: side as PaperExecutionCheckpoint["side"],
      fee,
      adverse_slippage: adverseSlippage,
      explicit_cost: explicitCost,
      provider_reference_notional: referenceNotional,
      gross_notional: checkpointGross,
      fill_notional_residual: checkpointResidual,
      cumulative_fees: cumulativeFees,
      cumulative_adverse_slippage: cumulativeSlippage,
      cumulative_explicit_cost: cumulativeExplicit,
      cumulative_provider_reference_notional: cumulativeReference,
      cumulative_gross_notional: cumulativeGross,
      cumulative_all_in_cost_rate_bps: cumulativeRate,
      cumulative_rate_change:
        rateChange as PaperExecutionCheckpoint["cumulative_rate_change"],
      symbol_sequence: symbolSequence,
      same_side_streak: sameSideStreak,
      side_transition:
        sideTransition as PaperExecutionCheckpoint["side_transition"],
      opposite_side_elapsed_seconds: oppositeSideElapsedSeconds,
      market_provider: marketProvider,
      market_feed: marketFeed,
      market_quality: marketQuality,
      market_observed_at: marketObservedAt,
      simulated_at: simulatedAt,
    });
  }
  const latestTimeline = timeline.at(-1);
  if (
    timelineSampleCount !== fillCount ||
    timeline.length !== Math.min(fillCount, 12) ||
    timelineCappedValue !== fillCount > 12 ||
    (timeline.length > 0 &&
      (!latestTimeline ||
        latestTimeline.sequence !== fillCount ||
        compareExactDecimals(latestTimeline.cumulative_fees, totalFees) !== 0 ||
        compareExactDecimals(
          latestTimeline.cumulative_adverse_slippage,
          totalAdverseSlippage,
        ) !== 0 ||
        compareExactDecimals(
          latestTimeline.cumulative_explicit_cost,
          totalExplicitCost,
        ) !== 0 ||
        compareExactDecimals(
          latestTimeline.cumulative_provider_reference_notional,
          providerReferenceNotional,
        ) !== 0 ||
        compareExactDecimals(
          latestTimeline.cumulative_gross_notional,
          grossNotional,
        ) !== 0 ||
        compareExactDecimals(
          latestTimeline.cumulative_all_in_cost_rate_bps,
          allInCostRateBPS,
        ) !== 0)) ||
    timeline.some((checkpoint) => {
      const sequenceSymbol = tradeSequence.symbols.find(
        (symbol) =>
          symbol.symbol === checkpoint.symbol &&
          symbol.instrument === checkpoint.instrument,
      );
      return (
        !sequenceSymbol ||
        checkpoint.symbol_sequence > sequenceSymbol.fill_count ||
        checkpoint.same_side_streak > sequenceSymbol.longest_same_side_streak ||
        (checkpoint.side_transition === "BUY_TO_SELL" &&
          checkpoint.side !== "SELL") ||
        (checkpoint.side_transition === "SELL_TO_BUY" &&
          checkpoint.side !== "BUY")
      );
    }) ||
    sides.reduce((count, side) => count + side.fill_count, 0) !== fillCount ||
    (fillCount === 0 ? sides.length !== 0 : sides.length === 0) ||
    compareExactDecimals(
      sumUSD(sides.map((side) => side.total_fees)) ??
        (fillCount === 0 ? "0" : "invalid"),
      totalFees,
    ) !== 0 ||
    compareExactDecimals(
      sumUSD(sides.map((side) => side.adverse_slippage)) ??
        (fillCount === 0 ? "0" : "invalid"),
      totalAdverseSlippage,
    ) !== 0 ||
    compareExactDecimals(
      sumUSD(sides.map((side) => side.total_explicit_cost)) ??
        (fillCount === 0 ? "0" : "invalid"),
      totalExplicitCost,
    ) !== 0 ||
    compareExactDecimals(
      sumUSD(sides.map((side) => side.provider_reference_notional)) ??
        (fillCount === 0 ? "0" : "invalid"),
      providerReferenceNotional,
    ) !== 0 ||
    compareExactDecimals(
      sumUSD(sides.map((side) => side.gross_notional)) ??
        (fillCount === 0 ? "0" : "invalid"),
      grossNotional,
    ) !== 0 ||
    symbols.reduce((count, symbol) => count + symbol.fill_count, 0) !==
      fillCount ||
    compareExactDecimals(
      sumUSD(symbols.map((symbol) => symbol.total_fees)) ?? "invalid",
      totalFees,
    ) !== 0 ||
    compareExactDecimals(
      sumUSD(symbols.map((symbol) => symbol.adverse_slippage)) ?? "invalid",
      totalAdverseSlippage,
    ) !== 0 ||
    compareExactDecimals(
      sumUSD(symbols.map((symbol) => symbol.total_explicit_cost)) ??
        (fillCount === 0 ? "0" : "invalid"),
      totalExplicitCost,
    ) !== 0 ||
    compareExactDecimals(
      sumUSD(symbols.map((symbol) => symbol.provider_reference_notional)) ??
        (fillCount === 0 ? "0" : "invalid"),
      providerReferenceNotional,
    ) !== 0 ||
    compareExactDecimals(
      sumUSD(symbols.map((symbol) => symbol.gross_notional)) ?? "invalid",
      grossNotional,
    ) !== 0
  )
    return;
  return {
    status: status as PaperExecutionCosts["status"],
    calculation_method: calculationMethod,
    historical_coverage: historicalCoverage,
    total_fees: totalFees,
    total_adverse_slippage: totalAdverseSlippage,
    total_explicit_cost: totalExplicitCost,
    provider_reference_notional: providerReferenceNotional,
    gross_notional: grossNotional,
    all_in_cost_rate_bps: allInCostRateBPS,
    fill_notional_residual: fillNotionalResidual,
    maximum_absolute_fill_residual: maximumAbsoluteFillResidual,
    residual_bound_per_fill: residualBoundPerFill,
    fill_count: fillCount,
    buy_fill_count: buyFillCount,
    sell_fill_count: sellFillCount,
    first_fill_at: firstFillAt,
    last_fill_at: lastFillAt,
    market_providers: marketProviders,
    market_feeds: marketFeeds,
    market_qualities: marketQualities,
    sides,
    symbols,
    timeline_sample_count: timelineSampleCount,
    timeline_capped: timelineCappedValue,
    timeline,
    trade_sequence: tradeSequence,
  };
}

function text(record: RecordValue | undefined, key: string, legacy: string) {
  const value = record?.[key] ?? record?.[legacy];
  return typeof value === "string" ? value : undefined;
}

function number(record: RecordValue | undefined, key: string, legacy: string) {
  const value = record?.[key] ?? record?.[legacy];
  return typeof value === "number" && Number.isFinite(value)
    ? value
    : undefined;
}

function flag(record: RecordValue | undefined, key: string, legacy: string) {
  const value = record?.[key] ?? record?.[legacy];
  return typeof value === "boolean" ? value : undefined;
}

function symbols(mandate: RecordValue) {
  const universe = (mandate.allowed_universe ?? mandate.AllowedUniverse) as
    | RecordValue
    | undefined;
  const values = universe?.symbols ?? universe?.Symbols;
  return Array.isArray(values)
    ? values.filter((value): value is string => typeof value === "string")
    : [];
}

function stringList(value: unknown) {
  return Array.isArray(value)
    ? value.filter((item): item is string => typeof item === "string")
    : [];
}

function record(value: unknown) {
  return value && typeof value === "object" && !Array.isArray(value)
    ? (value as RecordValue)
    : undefined;
}

function exactDecimal(value: unknown) {
  return typeof value === "string" &&
    /^-?(0|[1-9][0-9]*)(\.[0-9]+)?$/.test(value)
    ? value
    : undefined;
}

function nonnegativeInteger(value: unknown) {
  return typeof value === "number" && Number.isSafeInteger(value) && value >= 0
    ? value
    : undefined;
}

function normalizedPaperRealizedOutcome(
  value: unknown,
): PaperRealizedOutcome | undefined {
  const outcome = record(value);
  const status = text(outcome, "status", "Status");
  const fillCount = nonnegativeInteger(
    outcome?.fill_count ?? outcome?.FillCount,
  );
  const sellFillCount = nonnegativeInteger(
    outcome?.sell_fill_count ?? outcome?.SellFillCount,
  );
  const rawSymbols = outcome?.symbols ?? outcome?.Symbols;
  if (
    !["AVAILABLE", "NO_REALIZED_SALES", "UNAVAILABLE"].includes(status ?? "") ||
    fillCount === undefined ||
    sellFillCount === undefined ||
    sellFillCount > fillCount ||
    !Array.isArray(rawSymbols)
  )
    return;
  if (status === "UNAVAILABLE") {
    if (rawSymbols.length !== 0) return;
    return {
      status: "UNAVAILABLE",
      fill_count: fillCount,
      sell_fill_count: sellFillCount,
      symbols: [],
    };
  }
  const calculationMethod = text(
    outcome,
    "calculation_method",
    "CalculationMethod",
  );
  const historicalCoverage = text(
    outcome,
    "historical_coverage",
    "HistoricalCoverage",
  );
  const totalRealizedProfitLoss = exactDecimal(
    outcome?.total_realized_profit_loss ?? outcome?.TotalRealizedProfitLoss,
  );
  const firstFillAt = text(outcome, "first_fill_at", "FirstFillAt");
  const lastFillAt = text(outcome, "last_fill_at", "LastFillAt");
  if (
    calculationMethod !== "AVERAGE_COST_INCLUDED_FEES" ||
    historicalCoverage !== "COMPLETE_FROM_PORTFOLIO_GENESIS" ||
    !totalRealizedProfitLoss ||
    (status === "AVAILABLE" && sellFillCount === 0) ||
    (status === "NO_REALIZED_SALES" && sellFillCount !== 0) ||
    (fillCount > 0 &&
      (!firstFillAt ||
        !lastFillAt ||
        Number.isNaN(Date.parse(firstFillAt)) ||
        Number.isNaN(Date.parse(lastFillAt)))) ||
    (fillCount === 0 && (firstFillAt || lastFillAt))
  )
    return;
  const symbols: PaperRealizedSymbolOutcome[] = [];
  const seenSymbols = new Set<string>();
  for (const rawSymbol of rawSymbols) {
    const symbolOutcome = record(rawSymbol);
    const symbol = text(symbolOutcome, "symbol", "Symbol");
    const instrument = text(symbolOutcome, "instrument", "Instrument");
    const realizedProfitLoss = exactDecimal(
      symbolOutcome?.realized_profit_loss ?? symbolOutcome?.RealizedProfitLoss,
    );
    const buyFillCount = nonnegativeInteger(
      symbolOutcome?.buy_fill_count ?? symbolOutcome?.BuyFillCount,
    );
    const symbolSellFillCount = nonnegativeInteger(
      symbolOutcome?.sell_fill_count ?? symbolOutcome?.SellFillCount,
    );
    const totalFees = exactDecimal(
      symbolOutcome?.total_fees ?? symbolOutcome?.TotalFees,
    );
    const endingPositionQuantity = exactDecimal(
      symbolOutcome?.ending_position_quantity ??
        symbolOutcome?.EndingPositionQuantity,
    );
    const endingAverageCost = exactDecimal(
      symbolOutcome?.ending_average_cost ?? symbolOutcome?.EndingAverageCost,
    );
    if (
      !symbol ||
      !["EQUITY", "CRYPTO"].includes(instrument ?? "") ||
      !realizedProfitLoss ||
      buyFillCount === undefined ||
      symbolSellFillCount === undefined ||
      !totalFees ||
      !endingPositionQuantity ||
      (compareExactDecimals(totalFees, "0") ?? -1) < 0 ||
      (compareExactDecimals(endingPositionQuantity, "0") ?? -1) < 0 ||
      (compareExactDecimals(endingPositionQuantity, "0") === 1 &&
        !endingAverageCost) ||
      seenSymbols.has(`${instrument}:${symbol}`)
    )
      return;
    seenSymbols.add(`${instrument}:${symbol}`);
    symbols.push({
      symbol,
      instrument: instrument as PaperRealizedSymbolOutcome["instrument"],
      realized_profit_loss: realizedProfitLoss,
      buy_fill_count: buyFillCount,
      sell_fill_count: symbolSellFillCount,
      total_fees: totalFees,
      ending_position_quantity: endingPositionQuantity,
      ending_average_cost: endingAverageCost,
    });
  }
  const attributedRealizedProfitLoss = sumUSD(
    symbols.map((symbol) => symbol.realized_profit_loss),
  );
  if (
    symbols.reduce((count, symbol) => count + symbol.sell_fill_count, 0) !==
      sellFillCount ||
    symbols.reduce(
      (count, symbol) => count + symbol.buy_fill_count + symbol.sell_fill_count,
      0,
    ) !== fillCount ||
    !attributedRealizedProfitLoss ||
    compareExactDecimals(
      attributedRealizedProfitLoss,
      totalRealizedProfitLoss,
    ) !== 0
  )
    return;
  return {
    status: status as PaperRealizedOutcome["status"],
    calculation_method: calculationMethod,
    historical_coverage: historicalCoverage,
    total_realized_profit_loss: totalRealizedProfitLoss,
    fill_count: fillCount,
    sell_fill_count: sellFillCount,
    first_fill_at: firstFillAt,
    last_fill_at: lastFillAt,
    symbols,
  };
}

function normalizedPaperPortfolio(value: unknown): PaperPortfolio | undefined {
  const portfolio = record(value);
  const rawPositions = portfolio?.positions ?? portfolio?.Positions;
  const currency = text(portfolio, "currency", "Currency");
  const startingCash = exactDecimal(
    portfolio?.starting_cash ?? portfolio?.StartingCash,
  );
  const cash = exactDecimal(portfolio?.cash ?? portfolio?.Cash);
  const version = number(portfolio, "version", "Version");
  const updatedAt = text(portfolio, "updated_at", "UpdatedAt");
  const strategyInstanceID = text(
    portfolio,
    "strategy_instance_id",
    "StrategyInstanceID",
  );
  const realizedOutcome = normalizedPaperRealizedOutcome(
    portfolio?.realized_outcome ?? portfolio?.RealizedOutcome,
  );
  const executionCosts = normalizedPaperExecutionCosts(
    portfolio?.execution_costs ?? portfolio?.ExecutionCosts,
  );
  if (
    !currency ||
    !startingCash ||
    !cash ||
    compareExactDecimals(startingCash, "0") !== 1 ||
    (compareExactDecimals(cash, "0") ?? -1) < 0 ||
    version === undefined ||
    !updatedAt ||
    Number.isNaN(Date.parse(updatedAt)) ||
    !strategyInstanceID ||
    !Array.isArray(rawPositions)
  )
    return;
  const positions: PaperPosition[] = [];
  for (const rawPosition of rawPositions) {
    const position = record(rawPosition);
    const symbol = text(position, "symbol", "Symbol");
    const instrument = text(position, "instrument", "Instrument");
    const quantity = exactDecimal(position?.quantity ?? position?.Quantity);
    const averagePrice = exactDecimal(
      position?.average_price ?? position?.AveragePrice,
    );
    const isOpen = flag(position, "is_open", "IsOpen");
    const positionUpdatedAt = text(position, "updated_at", "UpdatedAt");
    if (
      !symbol ||
      !["EQUITY", "OPTION", "CRYPTO"].includes(instrument ?? "") ||
      !quantity ||
      !averagePrice ||
      isOpen === undefined ||
      !positionUpdatedAt ||
      Number.isNaN(Date.parse(positionUpdatedAt))
    )
      return;
    const optionType = text(position, "option_type", "OptionType");
    if (optionType && optionType !== "PUT" && optionType !== "CALL") return;
    positions.push({
      symbol,
      instrument: instrument as PaperPosition["instrument"],
      option_type: optionType as PaperPosition["option_type"],
      strike: exactDecimal(position?.strike ?? position?.Strike),
      expiration: text(position, "expiration", "Expiration"),
      quantity,
      average_price: averagePrice,
      is_open: isOpen,
      updated_at: positionUpdatedAt,
    });
  }
  return {
    strategy_instance_id: strategyInstanceID,
    currency,
    starting_cash: startingCash,
    cash,
    version,
    positions,
    realized_outcome: realizedOutcome,
    execution_costs: executionCosts,
    updated_at: updatedAt,
  };
}

function minimumExactDecimals(values: Array<string | undefined>) {
  const present = values.filter((value): value is string => Boolean(value));
  if (present.length !== values.length || present.length === 0) return;
  let minimum = present[0];
  for (const value of present.slice(1)) {
    const comparison = compareExactDecimals(value, minimum);
    if (comparison === undefined) return;
    if (comparison < 0) minimum = value;
  }
  return minimum;
}

function sumUSD(values: string[]) {
  if (values.length === 0) return "0";
  return sumExactMoney(values.map((amount) => ({ amount, currency: "USD" })))
    ?.amount;
}

function paperOutcomeSnapshot(
  decision: RecordValue,
  cashReserve?: string,
  exposureCeiling?: string,
): StrategyFleetOutcomeHistorySnapshot | undefined {
  const rationale = record(
    decision.structured_rationale ?? decision.StructuredRationale,
  );
  const input = record(rationale?.input_evidence ?? rationale?.InputEvidence);
  const id = text(decision, "id", "ID");
  const observedAt = text(decision, "created_at", "CreatedAt");
  const cash = exactDecimal(
    input?.available_cash_usd ?? input?.AvailableCashUSD,
  );
  const rawPositions = input?.positions ?? input?.Positions;
  if (
    !id ||
    !observedAt ||
    Number.isNaN(Date.parse(observedAt)) ||
    !cash ||
    !Array.isArray(rawPositions)
  )
    return;
  const positions: NonNullable<
    StrategyFleetOutcomeHistorySnapshot["paperPositions"]
  > = [];
  for (const rawPosition of rawPositions) {
    const position = record(rawPosition);
    const symbol = text(position, "symbol", "Symbol");
    const marketValue = exactDecimal(
      position?.market_value_usd ?? position?.MarketValueUSD,
    );
    const unrealizedProfitLoss = exactDecimal(
      position?.open_profit_loss_usd ?? position?.OpenProfitLossUSD,
    );
    if (!symbol || !marketValue || !unrealizedProfitLoss) return;
    positions.push({ symbol, marketValue, unrealizedProfitLoss });
  }
  const markedExposure = sumUSD(
    positions.map((position) => position.marketValue),
  );
  const unrealizedProfitLoss = sumUSD(
    positions.map((position) => position.unrealizedProfitLoss),
  );
  const simulatedEquity = markedExposure
    ? sumUSD([cash, markedExposure])
    : undefined;
  if (!markedExposure || !unrealizedProfitLoss || !simulatedEquity) return;
  const rawMarkets = input?.markets ?? input?.Markets;
  const markets = Array.isArray(rawMarkets)
    ? rawMarkets
        .map(record)
        .filter((market): market is RecordValue => Boolean(market))
    : [];
  const marketObservedTimes = markets
    .map((market) => text(market, "observed_at", "ObservedAt"))
    .filter((value): value is string => Boolean(value))
    .sort();
  const marketFeeds = Array.from(
    new Set(
      markets
        .map((market) => text(market, "feed", "Feed"))
        .filter((value): value is string => Boolean(value)),
    ),
  );
  const marketQualities = Array.from(
    new Set(
      markets
        .map((market) => text(market, "quality", "Quality"))
        .filter((value): value is string => Boolean(value)),
    ),
  );
  return {
    id,
    observedAt,
    marketObservedAt: marketObservedTimes[0],
    financialProvider: text(input, "provider", "Provider"),
    marketFeed: marketFeeds.length === 1 ? marketFeeds[0] : undefined,
    marketQuality:
      marketQualities.length === 1 ? marketQualities[0] : undefined,
    paperCash: cash,
    paperMarkedExposure: markedExposure,
    paperSimulatedEquity: simulatedEquity,
    paperUnrealizedProfitLoss: unrealizedProfitLoss,
    paperCashHeadroom:
      cashReserve === undefined
        ? undefined
        : subtractExactDecimals(cash, cashReserve),
    paperExposureHeadroom:
      exposureCeiling === undefined
        ? undefined
        : subtractExactDecimals(exposureCeiling, markedExposure),
    paperFillDisposition:
      text(decision, "execution_status", "ExecutionStatus") ??
      text(decision, "decision_type", "DecisionType"),
    paperPositions: positions,
  };
}

function shadowOutcomeHistory(
  outcomes: RecordValue[],
): StrategyFleetOutcomeHistorySnapshot[] | undefined {
  if (outcomes.length === 0 || outcomes.length >= 200) return;
  const normalized = outcomes
    .map((outcome) => {
      const id = text(outcome, "id", "ID");
      const observedAt = text(outcome, "evaluated_at", "EvaluatedAt");
      const marketObservedAt = text(
        outcome,
        "market_observed_at",
        "MarketObservedAt",
      );
      const horizon = text(outcome, "horizon", "Horizon");
      const symbol = text(outcome, "symbol", "Symbol");
      const directionalChangePercent = exactDecimal(
        outcome.directional_change_percent ?? outcome.DirectionalChangePercent,
      );
      const marketFeed = text(outcome, "market_feed", "MarketFeed");
      const marketQuality = text(outcome, "market_quality", "MarketQuality");
      if (
        !id ||
        !observedAt ||
        Number.isNaN(Date.parse(observedAt)) ||
        !marketObservedAt ||
        Number.isNaN(Date.parse(marketObservedAt)) ||
        !["ONE_HOUR", "TWENTY_FOUR_HOURS"].includes(horizon ?? "") ||
        !symbol ||
        !directionalChangePercent ||
        !marketFeed ||
        !marketQuality
      )
        return;
      return {
        id,
        observedAt,
        marketObservedAt,
        horizon: horizon!,
        symbol,
        directionalChangePercent,
        marketFeed,
        marketQuality,
      };
    })
    .filter((outcome): outcome is NonNullable<typeof outcome> =>
      Boolean(outcome),
    )
    .sort(
      (left, right) =>
        left.observedAt.localeCompare(right.observedAt) ||
        left.id.localeCompare(right.id),
    );
  if (normalized.length !== outcomes.length) return;
  let oneHour = 0;
  let twentyFourHour = 0;
  const snapshots: StrategyFleetOutcomeHistorySnapshot[] = [];
  for (const outcome of normalized) {
    if (outcome.horizon === "ONE_HOUR") oneHour += 1;
    else twentyFourHour += 1;
    snapshots.push({
      id: outcome.id,
      observedAt: outcome.observedAt,
      marketObservedAt: outcome.marketObservedAt,
      marketFeed: outcome.marketFeed,
      marketQuality: outcome.marketQuality,
      shadowOneHourSamples: oneHour,
      shadowTwentyFourHourSamples: twentyFourHour,
      shadowNewMarks: [
        {
          id: outcome.id,
          horizon: outcome.horizon,
          symbol: outcome.symbol,
          directionalChangePercent: outcome.directionalChangePercent,
        },
      ],
    });
  }
  return snapshots.reverse().slice(0, 6);
}

function uniqueStrings(values: Array<string | undefined>) {
  return [
    ...new Set(values.filter((value): value is string => Boolean(value))),
  ];
}

function decisionProvenance(decision: RecordValue | undefined) {
  const rationale = record(
    decision?.structured_rationale ?? decision?.StructuredRationale,
  );
  const inputEvidence = record(
    rationale?.input_evidence ?? rationale?.InputEvidence,
  );
  const rawMarkets = (inputEvidence?.markets ?? inputEvidence?.Markets) as
    | RecordValue[]
    | null
    | undefined;
  const marketRecords = asList(rawMarkets);
  const rawPositions = (inputEvidence?.positions ??
    inputEvidence?.Positions) as RecordValue[] | null | undefined;
  const positionRecords = asList(rawPositions);
  const rawMarketEventCoverage = (inputEvidence?.market_event_coverage ??
    inputEvidence?.MarketEventCoverage) as RecordValue[] | null | undefined;
  const marketEventCoverageRecords = asList(rawMarketEventCoverage);
  const financialProvider = text(inputEvidence, "provider", "Provider");
  const marketObservedAt = text(
    rationale,
    "market_observed_at",
    "MarketObservedAt",
  );
  const marketSymbols = marketRecords.map((market) =>
    text(market, "symbol", "Symbol"),
  );
  const marketFeeds = marketRecords.map((market) =>
    text(market, "feed", "Feed"),
  );
  const marketQualities = marketRecords.map((market) =>
    text(market, "quality", "Quality"),
  );
  const marketObservationTimes = marketRecords.map((market) =>
    text(market, "observed_at", "ObservedAt"),
  );
  const historyStatuses = marketRecords.map((market) =>
    text(market, "history_status", "HistoryStatus"),
  );
  const historyFeeds = marketRecords.map((market) =>
    text(market, "history_feed", "HistoryFeed"),
  );
  const historyQualities = marketRecords.map((market) =>
    text(market, "history_quality", "HistoryQuality"),
  );
  const liquidityStatuses = marketRecords.map((market) =>
    text(market, "liquidity_status", "LiquidityStatus"),
  );
  const positionPerformanceStatuses = positionRecords.map((position) =>
    text(position, "performance_status", "PerformanceStatus"),
  );
  const marketEventCoverageStatuses = marketEventCoverageRecords.map(
    (coverage) => text(coverage, "status", "Status"),
  );
  const marketEventProviders = marketEventCoverageRecords.map((coverage) =>
    text(coverage, "resolver_provider", "ResolverProvider"),
  );
  const marketEventFeeds = marketEventCoverageRecords.map((coverage) =>
    text(coverage, "resolver_feed", "ResolverFeed"),
  );
  const marketEventQualities = marketEventCoverageRecords.map((coverage) =>
    text(coverage, "resolver_quality", "ResolverQuality"),
  );
  const marketEventCounts = marketEventCoverageRecords.map((coverage) =>
    number(coverage, "event_count", "EventCount"),
  );
  const exactMarketRows = Boolean(
    Array.isArray(rawMarkets) &&
      rawMarkets.length > 0 &&
      marketRecords.length === rawMarkets.length &&
      marketRecords.every(
        (_, index) =>
          marketSymbols[index] &&
          marketFeeds[index] &&
          marketQualities[index] &&
          marketObservationTimes[index] &&
          !Number.isNaN(new Date(marketObservationTimes[index]!).valueOf()),
      ) &&
      new Set(marketSymbols).size === marketSymbols.length,
  );
  const exactHistoryAndLiquidity = marketRecords.every(
    (_, index) =>
      historyStatuses[index] &&
      liquidityStatuses[index] &&
      (!["AVAILABLE", "COMPLETE"].includes(historyStatuses[index]!) ||
        (historyFeeds[index] && historyQualities[index])),
  );
  const exactPositionEvidence = Boolean(
    Array.isArray(rawPositions) &&
      positionRecords.length === rawPositions.length &&
      positionRecords.every(
        (position, index) =>
          text(position, "symbol", "Symbol") &&
          positionPerformanceStatuses[index],
      ),
  );
  const exactMarketEventEvidence = Boolean(
    Array.isArray(rawMarketEventCoverage) &&
      marketEventCoverageRecords.length === rawMarketEventCoverage.length &&
      marketEventCoverageRecords.every(
        (coverage, index) =>
          text(coverage, "symbol", "Symbol") &&
          marketEventCoverageStatuses[index] &&
          (marketEventCoverageStatuses[index] !== "AVAILABLE" ||
            (marketEventProviders[index] &&
              marketEventFeeds[index] &&
              marketEventQualities[index] &&
              marketEventCounts[index] !== undefined)),
      ),
  );
  const exactInputCoverage = Boolean(
    exactMarketRows &&
      exactHistoryAndLiquidity &&
      exactPositionEvidence &&
      exactMarketEventEvidence,
  );
  return {
    rationale,
    financialProvider,
    financialContextComplete: Boolean(
      inputEvidence &&
        financialProvider &&
        marketObservedAt &&
        !Number.isNaN(new Date(marketObservedAt).valueOf()) &&
        exactMarketRows,
    ),
    marketSymbols: uniqueStrings(marketSymbols),
    marketFeeds: uniqueStrings(marketFeeds),
    marketQualities: uniqueStrings(marketQualities),
    marketObservedAt,
    inputCoverageComplete: exactInputCoverage,
    historyLiquidityEvidenceComplete: Boolean(
      exactMarketRows && exactHistoryAndLiquidity,
    ),
    historyStatuses: uniqueStrings(historyStatuses),
    historyFeeds: uniqueStrings(historyFeeds),
    historyQualities: uniqueStrings(historyQualities),
    liquidityStatuses: uniqueStrings(liquidityStatuses),
    positionEvidenceComplete: exactPositionEvidence,
    positionCount: positionRecords.length,
    positionPerformanceStatuses: uniqueStrings(positionPerformanceStatuses),
    marketEventEvidenceComplete: exactMarketEventEvidence,
    marketEventCoverageCount: marketEventCoverageRecords.length,
    marketEventCoverageStatuses: uniqueStrings(marketEventCoverageStatuses),
    marketEventProviders: uniqueStrings(marketEventProviders),
    marketEventFeeds: uniqueStrings(marketEventFeeds),
    marketEventQualities: uniqueStrings(marketEventQualities),
    marketEventCount: marketEventCounts.every((count) => count !== undefined)
      ? marketEventCounts.reduce((total, count) => total + (count ?? 0), 0)
      : undefined,
  };
}

function exactCapitalCurrency(value: string | undefined) {
  return Boolean(value && /^[A-Z]{3}$/.test(value));
}

function strategyTitle(mandate: RecordValue) {
  if (text(mandate, "automation_type", "AutomationType") === "AI_AUTONOMOUS") {
    return text(mandate, "execution_mode", "ExecutionMode") === "PAPER"
      ? "AI Paper Engine"
      : "AI Shadow Engine";
  }
  const identifier = text(mandate, "strategy_identifier", "StrategyIdentifier");
  if (!identifier) return "Trading strategy";
  return identifier
    .toLowerCase()
    .split("_")
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(" ");
}

async function fetchOptional<T>(
  url: string,
  headers: { cookie: string },
): Promise<{ available: boolean; status?: number; payload?: T }> {
  try {
    const response = await fetch(url, { headers, cache: "no-store" });
    if (!response.ok) return { available: false, status: response.status };
    return {
      available: true,
      status: response.status,
      payload: (await response.json()) as T,
    };
  } catch {
    return { available: false };
  }
}

function currentInstance(mandate: RecordValue, instances: RecordValue[]) {
  const mandateID = text(mandate, "id", "ID");
  const version = number(mandate, "current_version", "CurrentVersion");
  const matches = instances.filter(
    (instance) =>
      text(instance, "automation_mandate_id", "AutomationMandateID") ===
      mandateID,
  );
  return (
    matches.find((instance) =>
      ["ACTIVE", "PAUSED"].includes(text(instance, "status", "Status") ?? ""),
    ) ??
    matches.find(
      (instance) => text(instance, "status", "Status") === "ERROR",
    ) ??
    matches.find(
      (instance) =>
        number(instance, "mandate_version", "MandateVersion") === version,
    )
  );
}

async function fleetItem(
  mandate: RecordValue,
  accounts: RecordValue[],
  financialConnections: RecordValue[],
  capitalBuckets: RecordValue[],
  capitalReservations: RecordValue[],
  instances: RecordValue[],
  base: string,
  headers: { cookie: string },
  observedAt: Date,
  accountContextAvailable: boolean,
  financialConnectionContextAvailable: boolean,
  capitalBucketContextAvailable: boolean,
  capitalReservationContextAvailable: boolean,
  instanceContextAvailable: boolean,
): Promise<StrategyFleetItem> {
  const id = text(mandate, "id", "ID") ?? "";
  const mutableAutomationType =
    text(mandate, "automation_type", "AutomationType") ?? "UNKNOWN";
  const instance = currentInstance(mandate, instances);
  const instanceID = text(instance, "id", "ID");
  const instanceStatus = text(instance, "status", "Status");
  const instanceIsAIRuntime =
    text(instance, "strategy_identifier", "StrategyIdentifier") === "ai_shadow";
  const runtimeExecutionMode =
    (instanceIsAIRuntime
      ? text(instance, "execution_mode", "ExecutionMode")
      : text(mandate, "execution_mode", "ExecutionMode")) ?? "UNKNOWN";
  const expectsPinnedRuntime =
    Boolean(instanceID) &&
    ["ACTIVE", "PAUSED", "ERROR"].includes(instanceStatus ?? "") &&
    instanceIsAIRuntime;
  const pinnedMandateVersion = number(
    instance,
    "mandate_version",
    "MandateVersion",
  );
  const accountID =
    (expectsPinnedRuntime
      ? text(instance, "financial_account_id", "FinancialAccountID")
      : text(mandate, "financial_account_id", "FinancialAccountID")) ?? "";
  const account = accounts.find(
    (candidate) => text(candidate, "id", "ID") === accountID,
  );
  const financialConnectionID = text(
    account,
    "provider_connection_id",
    "ProviderConnectionID",
  );
  const financialConnection = financialConnections.find(
    (candidate) => text(candidate, "id", "ID") === financialConnectionID,
  );
  const capitalBucketID =
    (expectsPinnedRuntime
      ? text(instance, "capital_bucket_id", "CapitalBucketID")
      : text(mandate, "capital_bucket_id", "CapitalBucketID")) ?? "";
  const capitalBucket = capitalBuckets.find(
    (candidate) => text(candidate, "id", "ID") === capitalBucketID,
  );
  const instanceReservations = capitalReservations.filter(
    (candidate) =>
      text(candidate, "strategy_instance_id", "StrategyInstanceID") ===
      instanceID,
  );
  const activeCapitalReservations = instanceReservations.filter(
    (candidate) => text(candidate, "status", "Status") === "ACTIVE",
  );
  const capitalReservation =
    activeCapitalReservations.length === 1
      ? activeCapitalReservations[0]
      : undefined;
  const capitalAllocationType = text(
    capitalBucket,
    "allocation_type",
    "AllocationType",
  );
  const capitalAllocationValue = text(
    capitalBucket,
    "allocation_value",
    "AllocationValue",
  );
  const capitalCurrency = text(capitalBucket, "currency", "Currency");
  const capitalProtectedAmount = text(
    capitalBucket,
    "protected_amount",
    "ProtectedAmount",
  );
  const capitalAllocationLimit = text(
    capitalBucket,
    "allocation_limit",
    "AllocationLimit",
  );
  const capitalReservationAmount = text(
    capitalReservation,
    "reservation_amount",
    "ReservationAmount",
  );
  const capitalReservationCurrency = text(
    capitalReservation,
    "currency",
    "Currency",
  );
  const capitalReservationAccountLimit = text(
    capitalReservation,
    "account_allocation_limit",
    "AccountAllocationLimit",
  );
  const capitalReservationBasis = text(
    capitalReservation,
    "reservation_basis",
    "ReservationBasis",
  );
  const expectsSchedule =
    Boolean(instanceID) && ["ACTIVE", "PAUSED"].includes(instanceStatus ?? "");
  const expectsDecisionEvidence =
    Boolean(instanceID) &&
    (instanceIsAIRuntime || mutableAutomationType === "AI_AUTONOMOUS");
  const expectsShadowEvidence =
    expectsDecisionEvidence && runtimeExecutionMode === "SHADOW";
  const expectsOperationalData =
    instanceStatus === "ACTIVE" &&
    (instanceIsAIRuntime || mutableAutomationType === "AI_AUTONOMOUS");
  const expectsReconciliation =
    expectsOperationalData && runtimeExecutionMode === "SHADOW";
  const expectsCapitalData =
    Boolean(instanceID) &&
    ["ACTIVE", "PAUSED"].includes(instanceStatus ?? "") &&
    (instanceIsAIRuntime || mutableAutomationType === "AI_AUTONOMOUS");
  const capitalContextAvailable =
    capitalBucketContextAvailable && capitalReservationContextAvailable;
  const capitalBindingValid =
    expectsCapitalData &&
    capitalContextAvailable &&
    Boolean(capitalBucket) &&
    Boolean(capitalReservation) &&
    activeCapitalReservations.length === 1 &&
    Boolean(accountID) &&
    Boolean(capitalBucketID) &&
    Boolean(text(capitalReservation, "id", "ID")) &&
    text(capitalBucket, "financial_account_id", "FinancialAccountID") ===
      accountID &&
    text(instance, "capital_bucket_id", "CapitalBucketID") ===
      capitalBucketID &&
    text(capitalBucket, "status", "Status") === "ACTIVE" &&
    flag(capitalBucket, "is_reserve", "IsReserve") === false &&
    text(capitalReservation, "financial_account_id", "FinancialAccountID") ===
      accountID &&
    text(capitalReservation, "capital_bucket_id", "CapitalBucketID") ===
      capitalBucketID &&
    (runtimeExecutionMode === "PAPER" || runtimeExecutionMode === "SHADOW") &&
    text(capitalReservation, "execution_mode", "ExecutionMode") ===
      runtimeExecutionMode &&
    text(capitalReservation, "status", "Status") === "ACTIVE" &&
    exactCapitalCurrency(capitalCurrency) &&
    capitalReservationCurrency === capitalCurrency &&
    (runtimeExecutionMode === "PAPER"
      ? paperCapitalReservationMatchesPolicy({
          allocationType: capitalAllocationType,
          allocationValue: capitalAllocationValue,
          protectedAmount: capitalProtectedAmount,
          allocationLimit: capitalAllocationLimit,
          reservationAmount: capitalReservationAmount,
          reservationBasis: capitalReservationBasis,
          reservationAccountLimit: capitalReservationAccountLimit,
        })
      : capitalReservationMatchesPolicy({
          allocationType: capitalAllocationType,
          allocationValue: capitalAllocationValue,
          protectedAmount: capitalProtectedAmount,
          allocationLimit: capitalAllocationLimit,
          reservationAmount: capitalReservationAmount,
          reservationBasis: capitalReservationBasis,
          reservationAccountLimit: capitalReservationAccountLimit,
        }));
  const [
    versionResult,
    scheduleResult,
    scheduleHistoryResult,
    scorecardResult,
    shadowOutcomesResult,
    decisionResult,
    paperPortfolioResult,
    reconciliationResult,
  ] = await Promise.all([
    expectsPinnedRuntime
      ? pinnedMandateVersion
        ? fetchOptional<{ version?: RecordValue }>(
            `${base}/api/automations/${encodeURIComponent(id)}/versions/${pinnedMandateVersion}`,
            headers,
          )
        : Promise.resolve({ available: false as const, payload: undefined })
      : Promise.resolve({ available: true as const, payload: undefined }),
    expectsSchedule || expectsPinnedRuntime
      ? fetchOptional<RecordValue>(
          `${base}/api/strategy-instances/${encodeURIComponent(instanceID ?? "")}/schedule`,
          headers,
        )
      : Promise.resolve({ available: true as const, payload: undefined }),
    expectsOperationalData && expectsSchedule
      ? fetchOptional<ScheduleRunWindow>(
          `${base}/api/strategy-instances/${encodeURIComponent(instanceID ?? "")}/schedule-runs?limit=12`,
          headers,
        )
      : Promise.resolve({ available: true as const, payload: undefined }),
    expectsShadowEvidence
      ? fetchOptional<{ scorecard?: RecordValue }>(
          `${base}/api/strategy-instances/${encodeURIComponent(instanceID ?? "")}/shadow-scorecard`,
          headers,
        )
      : Promise.resolve({ available: true as const, payload: undefined }),
    expectsShadowEvidence
      ? fetchOptional<ShadowOutcomeWindow>(
          `${base}/api/strategy-instances/${encodeURIComponent(instanceID ?? "")}/shadow-outcomes`,
          headers,
        )
      : Promise.resolve({ available: true as const, payload: undefined }),
    expectsDecisionEvidence
      ? fetchOptional<DecisionWindow>(
          `${base}/api/strategy-instances/${encodeURIComponent(instanceID ?? "")}/decisions?limit=10`,
          headers,
        )
      : Promise.resolve({ available: true as const, payload: undefined }),
    expectsOperationalData && runtimeExecutionMode === "PAPER"
      ? fetchOptional<PaperPortfolioEnvelope>(
          `${base}/api/strategy-instances/${encodeURIComponent(instanceID ?? "")}/paper-portfolio`,
          headers,
        )
      : Promise.resolve({ available: true as const, payload: undefined }),
    expectsReconciliation && accountID
      ? fetchOptional<ReconciliationEnvelope>(
          `${base}/api/accounts/${encodeURIComponent(accountID)}/reconciliations/latest`,
          headers,
        )
      : Promise.resolve({ available: true as const, payload: undefined }),
  ]);
  if (
    [
      versionResult,
      scheduleResult,
      scheduleHistoryResult,
      scorecardResult,
      shadowOutcomesResult,
      decisionResult,
      paperPortfolioResult,
      reconciliationResult,
    ].some((result) => "status" in result && result.status === 401)
  )
    redirect("/login");
  const runtimeContract = expectsPinnedRuntime
    ? projectPinnedAIRuntime({
        mandate,
        instance: instance ?? {},
        versionAvailable: versionResult.available,
        version: versionResult.payload?.version,
      })
    : undefined;
  const runtimeScheduleBindingValid = expectsPinnedRuntime
    ? scheduleMatchesPinnedAIRuntime({
        contract: runtimeContract!,
        instanceID: instanceID ?? "",
        mandateID: id,
        scheduleAvailable: scheduleResult.available,
        envelope: scheduleResult.payload,
      })
    : undefined;
  const displayConfiguration = expectsPinnedRuntime
    ? runtimeContract?.configuration
    : mandate;
  const automationType =
    text(displayConfiguration, "automation_type", "AutomationType") ??
    mutableAutomationType;
  const schedule = record(scheduleResult.payload?.schedule);
  const scheduleHistoryAvailable =
    expectsOperationalData &&
    scheduleHistoryResult.available &&
    Array.isArray(scheduleHistoryResult.payload?.runs) &&
    scheduleHistoryResult.payload?.history_semantics ===
      "IMMUTABLE_NONLIVE_SCHEDULER_EVIDENCE" &&
    scheduleHistoryResult.payload?.broker_action_available === false &&
    scheduleHistoryResult.payload?.live_execution_available === false;
  const scheduleRecentRuns = scheduleHistoryAvailable
    ? asList(scheduleHistoryResult.payload?.runs).map((run) => ({
        id: text(run, "id", "ID"),
        scheduledFor: text(run, "scheduled_for", "ScheduledFor"),
        completedAt: text(run, "completed_at", "CompletedAt"),
        nextRunAt: text(run, "next_run_at", "NextRunAt"),
        status: text(run, "status", "Status"),
        errorCode: text(run, "error_code", "ErrorCode"),
        aiDecision: text(run, "ai_decision", "AIDecision"),
        executionStatus: text(run, "execution_status", "ExecutionStatus"),
        duplicateRecovered: flag(
          run,
          "duplicate_recovered",
          "DuplicateRecovered",
        ),
        consecutiveFailures: number(
          run,
          "consecutive_failures",
          "ConsecutiveFailures",
        ),
      }))
    : undefined;
  const scorecard = scorecardResult.payload?.scorecard;
  const evidenceGate = (scorecard?.evidence_gate ?? scorecard?.EvidenceGate) as
    | RecordValue
    | undefined;
  const evidenceAvailable =
    expectsShadowEvidence && scorecardResult.available && Boolean(evidenceGate);
  const shadowOutcomeHistoryAvailable =
    expectsShadowEvidence &&
    shadowOutcomesResult.available &&
    shadowOutcomesResult.payload?.performance_semantics ===
      "HYPOTHETICAL_DIRECTIONAL_MARK" &&
    shadowOutcomesResult.payload?.fees_and_slippage_included === false &&
    shadowOutcomesResult.payload?.live_execution_available === false &&
    Array.isArray(shadowOutcomesResult.payload?.outcomes);
  const decisionAvailable =
    expectsDecisionEvidence &&
    decisionResult.available &&
    decisionResult.payload?.decision_history_semantics ===
      "IMMUTABLE_OWNER_STRATEGY_DECISION_HISTORY" &&
    decisionResult.payload.model_rerun === false &&
    decisionResult.payload.financial_provider_called === false &&
    decisionResult.payload.broker_action_available === false &&
    decisionResult.payload.live_execution_available === false;
  const aiDecisions = asList(decisionResult.payload?.decisions).filter(
    (decision) => text(decision, "source", "Source") === "AI",
  );
  const latestAIDecision = aiDecisions[0];
  const priorAIDecision = aiDecisions[1];
  const latestDecisionProvenance = decisionProvenance(latestAIDecision);
  const priorDecisionProvenance = decisionProvenance(priorAIDecision);
  const recentDecisionInputCoverage: StrategyFleetDecisionInputCoverageSnapshot[] =
    decisionAvailable
      ? aiDecisions.slice(0, 6).map((decision) => {
          const provenance = decisionProvenance(decision);
          return {
            decisionID: text(decision, "id", "ID"),
            decisionAt: text(decision, "created_at", "CreatedAt"),
            financialProvider: provenance.financialProvider,
            financialContextComplete: provenance.financialContextComplete,
            inputCoverageComplete: provenance.inputCoverageComplete,
            historyLiquidityEvidenceComplete:
              provenance.historyLiquidityEvidenceComplete,
            marketSymbols: provenance.marketSymbols,
            marketFeeds: provenance.marketFeeds,
            marketQualities: provenance.marketQualities,
            marketObservedAt: provenance.marketObservedAt,
            historyStatuses: provenance.historyStatuses,
            historyFeeds: provenance.historyFeeds,
            historyQualities: provenance.historyQualities,
            liquidityStatuses: provenance.liquidityStatuses,
            positionEvidenceComplete: provenance.positionEvidenceComplete,
            positionCount: provenance.positionCount,
            positionPerformanceStatuses: provenance.positionPerformanceStatuses,
            marketEventEvidenceComplete: provenance.marketEventEvidenceComplete,
            marketEventCoverageCount: provenance.marketEventCoverageCount,
            marketEventCoverageStatuses: provenance.marketEventCoverageStatuses,
            marketEventProviders: provenance.marketEventProviders,
            marketEventFeeds: provenance.marketEventFeeds,
            marketEventQualities: provenance.marketEventQualities,
            marketEventCount: provenance.marketEventCount,
          };
        })
      : [];
  const decisionRationale = latestDecisionProvenance.rationale;
  const priorDecisionRationale = priorDecisionProvenance.rationale;
  const paperPortfolio =
    runtimeExecutionMode === "PAPER"
      ? normalizedPaperPortfolio(paperPortfolioResult.payload?.paper_portfolio)
      : undefined;
  const paperRealizedContractAvailable =
    runtimeExecutionMode === "PAPER" &&
    paperPortfolioResult.payload?.realized_outcome_semantics ===
      "EXACT_IMMUTABLE_AVERAGE_COST_SIMULATION" &&
    paperPortfolioResult.payload?.realized_outcome_includes_fees === true &&
    paperPortfolioResult.payload?.broker_action_available === false &&
    paperPortfolioResult.payload?.live_execution_available === false &&
    Boolean(paperPortfolio?.realized_outcome);
  const paperExecutionCostContractAvailable =
    runtimeExecutionMode === "PAPER" &&
    paperPortfolioResult.payload?.execution_cost_semantics ===
      "EXACT_IMMUTABLE_SIMULATION_FEES_AND_ADVERSE_SLIPPAGE" &&
    paperPortfolioResult.payload?.execution_costs_broker_reported === false &&
    paperPortfolioResult.payload?.broker_action_available === false &&
    paperPortfolioResult.payload?.live_execution_available === false &&
    Boolean(paperPortfolio?.execution_costs);
  const paperPortfolioAvailable =
    expectsOperationalData &&
    runtimeExecutionMode === "PAPER" &&
    paperPortfolioResult.available &&
    Boolean(paperPortfolio);
  const paperPerformance = paperPortfolio
    ? calculatePaperPerformance(
        paperPortfolio,
        extractLatestPaperMarketSnapshots(aiDecisions),
      )
    : undefined;
  const paperOutcomeReconciliation =
    paperPortfolio && paperPerformance
      ? reconcilePaperOutcome({
          currency: paperPortfolio.currency,
          cash: paperPortfolio.cash,
          performance: paperPerformance,
          realized: paperPortfolio.realized_outcome,
          realizedContractAvailable: paperRealizedContractAvailable,
        })
      : undefined;
  const riskParameters = record(
    displayConfiguration?.risk_parameters ??
      displayConfiguration?.RiskParameters,
  );
  const paperCashReserve = exactDecimal(
    riskParameters?.minimum_cash_reserve ?? riskParameters?.MinimumCashReserve,
  );
  const paperExposureCeiling = exactDecimal(
    riskParameters?.max_capital_deployed ?? riskParameters?.MaxCapitalDeployed,
  );
  const paperSymbolCeiling = exactDecimal(
    riskParameters?.max_single_position_amount ??
      riskParameters?.MaxSinglePositionAmount,
  );
  const paperCashHeadroom =
    paperPortfolio?.cash && paperCashReserve
      ? subtractExactDecimals(paperPortfolio.cash, paperCashReserve)
      : undefined;
  const paperExposureHeadroom =
    paperExposureCeiling && paperPerformance?.investedExposure
      ? subtractExactDecimals(
          paperExposureCeiling,
          paperPerformance.investedExposure,
        )
      : undefined;
  const paperProposalHeadroom = minimumExactDecimals([
    paperCashHeadroom,
    paperExposureHeadroom,
    runtimeContract?.maxProposalNotional,
  ]);
  const outcomeHistory =
    runtimeExecutionMode === "PAPER"
      ? decisionAvailable && paperCashReserve && paperExposureCeiling
        ? aiDecisions
            .slice(0, 6)
            .map((decision) =>
              paperOutcomeSnapshot(
                decision,
                paperCashReserve,
                paperExposureCeiling,
              ),
            )
            .filter(
              (snapshot): snapshot is StrategyFleetOutcomeHistorySnapshot =>
                Boolean(snapshot),
            )
        : undefined
      : shadowOutcomeHistoryAvailable
        ? shadowOutcomeHistory(
            asList(shadowOutcomesResult.payload?.outcomes),
          )?.map((snapshot) => ({
            ...snapshot,
            financialProvider: text(account, "provider", "Provider"),
          }))
        : undefined;
  const outcomeHistoryAvailable =
    expectsDecisionEvidence &&
    Boolean(outcomeHistory && outcomeHistory.length >= 2) &&
    (runtimeExecutionMode === "PAPER"
      ? outcomeHistory?.length === Math.min(aiDecisions.length, 6)
      : shadowOutcomeHistoryAvailable);
  const reconciliation = reconciliationResult.payload?.reconciliation;
  const reconciliationObservedAt = text(
    reconciliation,
    "observed_at",
    "ObservedAt",
  );
  const reconciliationAvailable =
    expectsReconciliation &&
    reconciliationResult.available &&
    reconciliationResult.payload?.live_execution_available === false &&
    Boolean(reconciliation);
  const scheduleNextRunAt =
    !expectsPinnedRuntime || runtimeScheduleBindingValid
      ? text(schedule, "next_run_at", "NextRunAt")
      : undefined;
  const scheduleTimingStatus =
    expectsOperationalData && runtimeContract?.scheduleEnabled === true
      ? scheduleResult.available && runtimeScheduleBindingValid
        ? scheduledRunTimingStatus(scheduleNextRunAt, observedAt)
        : "UNAVAILABLE"
      : undefined;

  return {
    id,
    freshnessObservedAt: observedAt.toISOString(),
    strategyInstanceID: instanceID,
    financialAccountID: accountID,
    capitalBucketID: capitalBucketID || undefined,
    capitalReservationID: text(capitalReservation, "id", "ID"),
    title: strategyTitle(displayConfiguration ?? mandate),
    accountName:
      text(account, "display_name", "DisplayName") ?? "Connected account",
    provider: text(account, "provider", "Provider") ?? "connected_account",
    accountStatus: text(account, "status", "Status"),
    financialConnectionAvailable: expectsOperationalData
      ? financialConnectionContextAvailable && Boolean(financialConnection)
      : undefined,
    financialConnectionContextAvailable: expectsOperationalData
      ? financialConnectionContextAvailable
      : undefined,
    financialConnectionStatus: text(financialConnection, "status", "Status"),
    financialAuthorizationExpiresAt: text(
      financialConnection,
      "authorization_expires_at",
      "AuthorizationExpiresAt",
    ),
    capitalContextAvailable: expectsCapitalData
      ? capitalContextAvailable
      : undefined,
    capitalBindingValid: expectsCapitalData ? capitalBindingValid : undefined,
    capitalBucketName: text(capitalBucket, "name", "Name"),
    capitalBucketStatus: text(capitalBucket, "status", "Status"),
    capitalAllocationType,
    capitalAllocationValue,
    capitalCurrency,
    capitalProtectedAmount,
    capitalAllocationLimit,
    capitalReservationStatus: text(capitalReservation, "status", "Status"),
    capitalReservationAmount,
    capitalReservationCurrency,
    capitalReservationBasis,
    capitalReservationAccountLimit,
    runtimeVersionContextAvailable: expectsPinnedRuntime
      ? runtimeContract?.contextAvailable
      : undefined,
    runtimeBindingValid: expectsPinnedRuntime
      ? runtimeContract?.bindingValid
      : undefined,
    runtimeScheduleBindingValid,
    runtimeMandateVersion: runtimeContract?.pinnedVersion,
    currentMandateVersion:
      runtimeContract?.currentVersion ??
      number(mandate, "current_version", "CurrentVersion"),
    runtimeSnapshotStatus: expectsPinnedRuntime
      ? text(runtimeContract?.configuration, "status", "Status")
      : undefined,
    newerDraftAvailable: runtimeContract?.newerDraftAvailable,
    runtimeMaxProposalNotional: runtimeContract?.maxProposalNotional,
    runtimeMaxTradesPerDay: runtimeContract?.maxTradesPerDay,
    runtimeLegacyDailyActionLimitMissing:
      runtimeContract?.legacyDailyActionLimitMissing,
    runtimeScheduleEnabled: runtimeContract?.scheduleEnabled,
    runtimeScheduleIntervalMinutes: runtimeContract?.scheduleIntervalMinutes,
    runtimeScheduleSession: runtimeContract?.scheduleSession,
    automationType,
    mandateStatus: text(mandate, "status", "Status") ?? "UNKNOWN",
    autonomyLevel:
      text(displayConfiguration, "autonomy_level", "AutonomyLevel") ??
      "UNKNOWN",
    executionMode:
      (expectsPinnedRuntime
        ? text(instance, "execution_mode", "ExecutionMode")
        : text(mandate, "execution_mode", "ExecutionMode")) ?? "UNKNOWN",
    modelID: text(displayConfiguration, "ai_model_id", "AIModelID"),
    symbols: displayConfiguration ? symbols(displayConfiguration) : [],
    instanceStatus,
    currentState: text(instance, "current_state", "CurrentState"),
    lastEvaluatedAt: text(instance, "last_evaluated_at", "LastEvaluatedAt"),
    scheduleAvailable: expectsSchedule ? scheduleResult.available : undefined,
    scheduleEnabled:
      !expectsPinnedRuntime || runtimeScheduleBindingValid
        ? flag(schedule, "enabled", "Enabled")
        : undefined,
    scheduleStatus:
      !expectsPinnedRuntime || runtimeScheduleBindingValid
        ? text(schedule, "last_status", "LastStatus")
        : undefined,
    scheduleErrorCode:
      !expectsPinnedRuntime || runtimeScheduleBindingValid
        ? text(schedule, "last_error_code", "LastErrorCode")
        : undefined,
    scheduleLastCompletedAt:
      !expectsPinnedRuntime || runtimeScheduleBindingValid
        ? text(schedule, "last_completed_at", "LastCompletedAt")
        : undefined,
    scheduleTimingStatus,
    consecutiveFailures:
      !expectsPinnedRuntime || runtimeScheduleBindingValid
        ? (number(schedule, "consecutive_failures", "ConsecutiveFailures") ?? 0)
        : 0,
    nextRunAt: scheduleNextRunAt,
    scheduleHistoryAvailable: expectsOperationalData
      ? scheduleHistoryAvailable
      : undefined,
    scheduleRecentRuns,
    evidenceAvailable: expectsShadowEvidence ? evidenceAvailable : undefined,
    evidenceStatus: text(evidenceGate, "status", "Status"),
    oneHourSampleSize: number(
      evidenceGate,
      "one_hour_sample_size",
      "OneHourSampleSize",
    ),
    twentyFourHourSampleSize: number(
      evidenceGate,
      "twenty_four_hour_sample_size",
      "TwentyFourHourSampleSize",
    ),
    minimumSamplePerHorizon: number(
      evidenceGate,
      "minimum_sample_per_horizon",
      "MinimumSamplePerHorizon",
    ),
    evidenceWindowHours: number(
      evidenceGate,
      "evidence_window_hours",
      "EvidenceWindowHours",
    ),
    minimumEvidenceWindowHours: number(
      evidenceGate,
      "minimum_evidence_window_hours",
      "MinimumEvidenceWindowHours",
    ),
    evidenceScheduleHealthy: flag(
      evidenceGate,
      "schedule_healthy",
      "ScheduleHealthy",
    ),
    evidenceBlockers: stringList(
      evidenceGate?.blockers ?? evidenceGate?.Blockers,
    ),
    currentEvidenceReviewed: flag(
      scorecard,
      "current_evidence_reviewed",
      "CurrentEvidenceReviewed",
    ),
    paperPortfolioAvailable:
      runtimeExecutionMode === "PAPER" ? paperPortfolioAvailable : undefined,
    paperPerformanceStatus: paperPerformance?.status,
    paperCurrency: paperPortfolio?.currency,
    paperStartingCash: paperPortfolio?.starting_cash,
    paperCash: paperPortfolio?.cash,
    paperSimulatedEquity: paperPerformance?.simulatedEquity,
    paperInvestedExposure: paperPerformance?.investedExposure,
    paperTotalProfitLoss: paperPerformance?.totalProfitLoss,
    paperTotalReturnPercent: paperPerformance?.totalReturnPercent,
    paperValuedAt: paperPerformance?.valuedAt,
    paperCashReserve,
    paperCashHeadroom,
    paperExposureCeiling,
    paperExposureHeadroom,
    paperSymbolCeiling,
    paperProposalHeadroom,
    paperRealizedContractAvailable:
      runtimeExecutionMode === "PAPER"
        ? paperRealizedContractAvailable
        : undefined,
    paperRealizedOutcomeStatus: paperPortfolio?.realized_outcome?.status,
    paperRealizedProfitLoss:
      paperPortfolio?.realized_outcome?.total_realized_profit_loss,
    paperRealizedFillCount: paperPortfolio?.realized_outcome?.fill_count,
    paperRealizedSellFillCount:
      paperPortfolio?.realized_outcome?.sell_fill_count,
    paperRealizedFirstFillAt: paperPortfolio?.realized_outcome?.first_fill_at,
    paperRealizedLastFillAt: paperPortfolio?.realized_outcome?.last_fill_at,
    paperRealizedSymbolOutcomes: paperPortfolio?.realized_outcome?.symbols.map(
      (symbol) => ({
        symbol: symbol.symbol,
        instrument: symbol.instrument,
        realizedProfitLoss: symbol.realized_profit_loss,
        buyFillCount: symbol.buy_fill_count,
        sellFillCount: symbol.sell_fill_count,
        totalFees: symbol.total_fees,
        endingPositionQuantity: symbol.ending_position_quantity,
        endingAverageCost: symbol.ending_average_cost,
      }),
    ),
    paperExecutionCostsContractAvailable:
      runtimeExecutionMode === "PAPER"
        ? paperExecutionCostContractAvailable
        : undefined,
    paperExecutionCostsStatus: paperPortfolio?.execution_costs?.status,
    paperExecutionTotalFees: paperPortfolio?.execution_costs?.total_fees,
    paperExecutionTotalAdverseSlippage:
      paperPortfolio?.execution_costs?.total_adverse_slippage,
    paperExecutionTotalExplicitCost:
      paperPortfolio?.execution_costs?.total_explicit_cost,
    paperExecutionProviderReferenceNotional:
      paperPortfolio?.execution_costs?.provider_reference_notional,
    paperExecutionGrossNotional:
      paperPortfolio?.execution_costs?.gross_notional,
    paperExecutionAllInCostRateBPS:
      paperPortfolio?.execution_costs?.all_in_cost_rate_bps,
    paperExecutionFillNotionalResidual:
      paperPortfolio?.execution_costs?.fill_notional_residual,
    paperExecutionMaximumAbsoluteFillResidual:
      paperPortfolio?.execution_costs?.maximum_absolute_fill_residual,
    paperExecutionResidualBoundPerFill:
      paperPortfolio?.execution_costs?.residual_bound_per_fill,
    paperExecutionFillCount: paperPortfolio?.execution_costs?.fill_count,
    paperExecutionBuyFillCount: paperPortfolio?.execution_costs?.buy_fill_count,
    paperExecutionSellFillCount:
      paperPortfolio?.execution_costs?.sell_fill_count,
    paperExecutionFirstFillAt: paperPortfolio?.execution_costs?.first_fill_at,
    paperExecutionLastFillAt: paperPortfolio?.execution_costs?.last_fill_at,
    paperExecutionMarketProviders:
      paperPortfolio?.execution_costs?.market_providers,
    paperExecutionMarketFeeds: paperPortfolio?.execution_costs?.market_feeds,
    paperExecutionMarketQualities:
      paperPortfolio?.execution_costs?.market_qualities,
    paperExecutionTimelineSampleCount:
      paperPortfolio?.execution_costs?.timeline_sample_count,
    paperExecutionTimelineCapped:
      paperPortfolio?.execution_costs?.timeline_capped,
    paperTradeSequence: paperPortfolio?.execution_costs?.trade_sequence,
    paperExecutionTimeline: paperPortfolio?.execution_costs?.timeline.map(
      (checkpoint) => ({
        sequence: checkpoint.sequence,
        fillID: checkpoint.fill_id,
        symbol: checkpoint.symbol,
        side: checkpoint.side,
        explicitCost: checkpoint.explicit_cost,
        fee: checkpoint.fee,
        adverseSlippage: checkpoint.adverse_slippage,
        providerReferenceNotional: checkpoint.provider_reference_notional,
        cumulativeExplicitCost: checkpoint.cumulative_explicit_cost,
        cumulativeProviderReferenceNotional:
          checkpoint.cumulative_provider_reference_notional,
        cumulativeAllInCostRateBPS: checkpoint.cumulative_all_in_cost_rate_bps,
        cumulativeRateChange: checkpoint.cumulative_rate_change,
        symbolSequence: checkpoint.symbol_sequence,
        sameSideStreak: checkpoint.same_side_streak,
        sideTransition: checkpoint.side_transition,
        oppositeSideElapsedSeconds: checkpoint.opposite_side_elapsed_seconds,
        marketProvider: checkpoint.market_provider,
        marketFeed: checkpoint.market_feed,
        marketQuality: checkpoint.market_quality,
        marketObservedAt: checkpoint.market_observed_at,
        simulatedAt: checkpoint.simulated_at,
      }),
    ),
    paperExecutionSideCosts: paperPortfolio?.execution_costs?.sides.map(
      (side) => ({
        side: side.side,
        totalFees: side.total_fees,
        adverseSlippage: side.adverse_slippage,
        totalExplicitCost: side.total_explicit_cost,
        providerReferenceNotional: side.provider_reference_notional,
        grossNotional: side.gross_notional,
        allInCostRateBPS: side.all_in_cost_rate_bps,
        fillCount: side.fill_count,
      }),
    ),
    paperExecutionSymbolCosts: paperPortfolio?.execution_costs?.symbols.map(
      (symbol) => ({
        symbol: symbol.symbol,
        instrument: symbol.instrument,
        totalFees: symbol.total_fees,
        adverseSlippage: symbol.adverse_slippage,
        totalExplicitCost: symbol.total_explicit_cost,
        providerReferenceNotional: symbol.provider_reference_notional,
        grossNotional: symbol.gross_notional,
        allInCostRateBPS: symbol.all_in_cost_rate_bps,
        fillCount: symbol.fill_count,
        buyFillCount: symbol.buy_fill_count,
        sellFillCount: symbol.sell_fill_count,
      }),
    ),
    paperOutcomeReconciliationStatus: paperOutcomeReconciliation?.status,
    paperReconciledRealizedProfitLoss:
      paperOutcomeReconciliation?.realizedProfitLoss,
    paperReconciledUnrealizedProfitLoss:
      paperOutcomeReconciliation?.unrealizedProfitLoss,
    paperReconciledClassifiedProfitLoss:
      paperOutcomeReconciliation?.classifiedProfitLoss,
    paperReconciledTotalProfitLoss: paperOutcomeReconciliation?.totalProfitLoss,
    paperOutcomeResidual: paperOutcomeReconciliation?.outcomeResidual,
    paperReconciledSimulatedEquity: paperOutcomeReconciliation?.simulatedEquity,
    paperReconciledCashPlusExposure:
      paperOutcomeReconciliation?.cashPlusExposure,
    paperEquityResidual: paperOutcomeReconciliation?.equityResidual,
    paperOutcomeResidualLimit: paperOutcomeReconciliation?.residualLimit,
    paperOutcomeReconciliationProvider: paperOutcomeReconciliation?.provider,
    paperOutcomeReconciliationFeeds: paperOutcomeReconciliation?.marketFeeds,
    paperOutcomeReconciliationQualities:
      paperOutcomeReconciliation?.marketQualities,
    paperOutcomeReconciliationValuedAt: paperOutcomeReconciliation?.valuedAt,
    paperPositionOutcomes: paperPerformance?.positions.map((position) => ({
      symbol: position.symbol,
      marketValue: position.marketValue,
      unrealizedProfitLoss: position.unrealizedProfitLoss,
      unrealizedProfitLossPercent: position.unrealizedProfitLossPercent,
    })),
    outcomeHistoryAvailable,
    outcomeHistory,
    decisionAvailable: expectsDecisionEvidence ? decisionAvailable : undefined,
    latestDecisionID: text(latestAIDecision, "id", "ID"),
    latestDecisionType: text(latestAIDecision, "decision_type", "DecisionType"),
    latestDecisionAt: text(latestAIDecision, "created_at", "CreatedAt"),
    latestDecisionProposedActionID: text(
      latestAIDecision,
      "proposed_action_id",
      "ProposedActionID",
    ),
    latestDecisionRiskEvaluationID: text(
      latestAIDecision,
      "risk_evaluation_id",
      "RiskEvaluationID",
    ),
    latestDecisionExecutionRecordID: text(
      latestAIDecision,
      "execution_record_id",
      "ExecutionRecordID",
    ),
    latestDecisionSymbol:
      text(latestAIDecision, "symbol", "Symbol") ??
      text(decisionRationale, "symbol", "Symbol"),
    latestDecisionSide:
      text(latestAIDecision, "side", "Side") ??
      text(decisionRationale, "side", "Side"),
    latestDecisionQuantity: text(latestAIDecision, "quantity", "Quantity"),
    latestDecisionRiskDecision: text(
      latestAIDecision,
      "risk_decision",
      "RiskDecision",
    ),
    latestDecisionRiskReasons: stringList(
      latestAIDecision?.risk_reason_codes ?? latestAIDecision?.RiskReasonCodes,
    ),
    latestDecisionExecutionStatus: text(
      latestAIDecision,
      "execution_status",
      "ExecutionStatus",
    ),
    latestDecisionAIProvider: text(
      decisionRationale,
      "ai_provider",
      "AIProvider",
    ),
    latestDecisionAIModelID: text(decisionRationale, "model_id", "ModelID"),
    latestDecisionAIProfile: text(decisionRationale, "profile", "Profile"),
    latestDecisionLatencyMS: number(
      decisionRationale,
      "latency_ms",
      "LatencyMS",
    ),
    latestDecisionInputUsage: number(
      decisionRationale,
      "input_usage",
      "InputUsage",
    ),
    latestDecisionOutputUsage: number(
      decisionRationale,
      "output_usage",
      "OutputUsage",
    ),
    latestDecisionProposedNotional: text(
      decisionRationale,
      "proposed_notional",
      "ProposedNotional",
    ),
    latestDecisionFinancialContextComplete:
      latestDecisionProvenance.financialContextComplete,
    latestDecisionFinancialProvider: latestDecisionProvenance.financialProvider,
    latestDecisionMarketSymbols: latestDecisionProvenance.marketSymbols,
    latestDecisionMarketFeeds: latestDecisionProvenance.marketFeeds,
    latestDecisionMarketQualities: latestDecisionProvenance.marketQualities,
    latestDecisionMarketObservedAt: latestDecisionProvenance.marketObservedAt,
    latestDecisionInputCoverageComplete:
      latestDecisionProvenance.inputCoverageComplete,
    latestDecisionHistoryLiquidityEvidenceComplete:
      latestDecisionProvenance.historyLiquidityEvidenceComplete,
    latestDecisionHistoryStatuses: latestDecisionProvenance.historyStatuses,
    latestDecisionHistoryFeeds: latestDecisionProvenance.historyFeeds,
    latestDecisionHistoryQualities: latestDecisionProvenance.historyQualities,
    latestDecisionLiquidityStatuses: latestDecisionProvenance.liquidityStatuses,
    latestDecisionPositionEvidenceComplete:
      latestDecisionProvenance.positionEvidenceComplete,
    latestDecisionPositionCount: latestDecisionProvenance.positionCount,
    latestDecisionPositionPerformanceStatuses:
      latestDecisionProvenance.positionPerformanceStatuses,
    latestDecisionMarketEventEvidenceComplete:
      latestDecisionProvenance.marketEventEvidenceComplete,
    latestDecisionMarketEventCoverageCount:
      latestDecisionProvenance.marketEventCoverageCount,
    latestDecisionMarketEventCoverageStatuses:
      latestDecisionProvenance.marketEventCoverageStatuses,
    latestDecisionMarketEventProviders:
      latestDecisionProvenance.marketEventProviders,
    latestDecisionMarketEventFeeds: latestDecisionProvenance.marketEventFeeds,
    latestDecisionMarketEventQualities:
      latestDecisionProvenance.marketEventQualities,
    latestDecisionMarketEventCount: latestDecisionProvenance.marketEventCount,
    priorDecisionID: text(priorAIDecision, "id", "ID"),
    priorDecisionType: text(priorAIDecision, "decision_type", "DecisionType"),
    priorDecisionAt: text(priorAIDecision, "created_at", "CreatedAt"),
    priorDecisionSymbol:
      text(priorAIDecision, "symbol", "Symbol") ??
      text(priorDecisionRationale, "symbol", "Symbol"),
    priorDecisionSide:
      text(priorAIDecision, "side", "Side") ??
      text(priorDecisionRationale, "side", "Side"),
    priorDecisionProposedNotional: text(
      priorDecisionRationale,
      "proposed_notional",
      "ProposedNotional",
    ),
    priorDecisionFinancialContextComplete:
      priorDecisionProvenance.financialContextComplete,
    priorDecisionFinancialProvider: priorDecisionProvenance.financialProvider,
    priorDecisionMarketSymbols: priorDecisionProvenance.marketSymbols,
    priorDecisionMarketFeeds: priorDecisionProvenance.marketFeeds,
    priorDecisionMarketQualities: priorDecisionProvenance.marketQualities,
    priorDecisionMarketObservedAt: priorDecisionProvenance.marketObservedAt,
    priorDecisionInputCoverageComplete:
      priorDecisionProvenance.inputCoverageComplete,
    priorDecisionHistoryLiquidityEvidenceComplete:
      priorDecisionProvenance.historyLiquidityEvidenceComplete,
    priorDecisionHistoryStatuses: priorDecisionProvenance.historyStatuses,
    priorDecisionHistoryFeeds: priorDecisionProvenance.historyFeeds,
    priorDecisionHistoryQualities: priorDecisionProvenance.historyQualities,
    priorDecisionLiquidityStatuses: priorDecisionProvenance.liquidityStatuses,
    priorDecisionPositionEvidenceComplete:
      priorDecisionProvenance.positionEvidenceComplete,
    priorDecisionPositionCount: priorDecisionProvenance.positionCount,
    priorDecisionPositionPerformanceStatuses:
      priorDecisionProvenance.positionPerformanceStatuses,
    priorDecisionMarketEventEvidenceComplete:
      priorDecisionProvenance.marketEventEvidenceComplete,
    priorDecisionMarketEventCoverageCount:
      priorDecisionProvenance.marketEventCoverageCount,
    priorDecisionMarketEventCoverageStatuses:
      priorDecisionProvenance.marketEventCoverageStatuses,
    priorDecisionMarketEventProviders:
      priorDecisionProvenance.marketEventProviders,
    priorDecisionMarketEventFeeds: priorDecisionProvenance.marketEventFeeds,
    priorDecisionMarketEventQualities:
      priorDecisionProvenance.marketEventQualities,
    priorDecisionMarketEventCount: priorDecisionProvenance.marketEventCount,
    recentDecisionInputCoverage,
    reconciliationAvailable: expectsReconciliation
      ? reconciliationAvailable
      : undefined,
    reconciliationComparisonStatus: text(
      reconciliation,
      "comparison_status",
      "ComparisonStatus",
    ),
    reconciliationBalancesStatus: text(
      reconciliation,
      "balances_status",
      "BalancesStatus",
    ),
    reconciliationPositionsStatus: text(
      reconciliation,
      "positions_status",
      "PositionsStatus",
    ),
    reconciliationAutonomySignal: text(
      reconciliation,
      "autonomy_signal",
      "AutonomySignal",
    ),
    reconciliationAutonomyEnforcementActive: flag(
      reconciliation,
      "autonomy_enforcement_active",
      "AutonomyEnforcementActive",
    ),
    reconciliationBlocksNewActions: flag(
      reconciliation,
      "blocks_new_actions",
      "BlocksNewActions",
    ),
    reconciliationBlockingChangeCount: number(
      reconciliation,
      "blocking_change_count",
      "BlockingChangeCount",
    ),
    reconciliationObservedAt,
    reconciliationFresh: expectsReconciliation
      ? reconciliationAvailable &&
        reconciliationFreshWithinTwentyFourHours(
          reconciliationObservedAt,
          observedAt,
        )
      : undefined,
    accountContextAvailable: accountContextAvailable && Boolean(account),
    instanceContextAvailable,
  };
}

export default async function Automations() {
  const jar = await cookies();
  const headers = { cookie: jar.toString() };
  const base = process.env.API_BASE_URL ?? "http://localhost:8080";
  const [
    mandatesResult,
    accountsResult,
    financialConnectionsResult,
    capitalBucketsResult,
    capitalReservationsResult,
    instancesResult,
  ] = await Promise.all([
    fetchOptional<{ automations?: RecordValue[] | null }>(
      `${base}/api/automations`,
      headers,
    ),
    fetchOptional<{ accounts?: RecordValue[] | null }>(
      `${base}/api/accounts`,
      headers,
    ),
    fetchOptional<{ connections?: RecordValue[] | null }>(
      `${base}/api/connections/financial`,
      headers,
    ),
    fetchOptional<{ capital_buckets?: RecordValue[] | null }>(
      `${base}/api/capital-buckets`,
      headers,
    ),
    fetchOptional<{ capital_reservations?: RecordValue[] | null }>(
      `${base}/api/strategy-capital-reservations`,
      headers,
    ),
    fetchOptional<{ strategy_instances?: RecordValue[] | null }>(
      `${base}/api/strategy-instances`,
      headers,
    ),
  ]);
  if (
    mandatesResult.status === 401 ||
    accountsResult.status === 401 ||
    financialConnectionsResult.status === 401 ||
    capitalBucketsResult.status === 401 ||
    capitalReservationsResult.status === 401 ||
    instancesResult.status === 401
  )
    redirect("/login");

  const mandates = asList(mandatesResult.payload?.automations);
  const accounts = asList(accountsResult.payload?.accounts);
  const financialConnections = asList(
    financialConnectionsResult.payload?.connections,
  );
  const capitalBuckets = asList(capitalBucketsResult.payload?.capital_buckets);
  const capitalReservations = asList(
    capitalReservationsResult.payload?.capital_reservations,
  );
  const instances = asList(instancesResult.payload?.strategy_instances);
  const observedAt = new Date();
  const items = mandatesResult.available
    ? await Promise.all(
        mandates.map((mandate) =>
          fleetItem(
            mandate,
            accounts,
            financialConnections,
            capitalBuckets,
            capitalReservations,
            instances,
            base,
            headers,
            observedAt,
            accountsResult.available,
            financialConnectionsResult.available,
            capitalBucketsResult.available,
            capitalReservationsResult.available,
            instancesResult.available,
          ),
        ),
      )
    : [];
  const contextWarnings = [
    !accountsResult.available
      ? "Account names and providers could not be refreshed."
      : "",
    !instancesResult.available
      ? "Current engine state could not be refreshed."
      : "",
    !financialConnectionsResult.available
      ? "Financial connection state could not be refreshed."
      : "",
    !capitalBucketsResult.available || !capitalReservationsResult.available
      ? "Capital bucket and reservation state could not be refreshed."
      : "",
  ].filter(Boolean);

  return (
    <main className="connections-page automation-page strategy-fleet-page">
      <AppPageHeader />
      <section className="strategy-fleet-hero">
        <div>
          <p className="eyebrow">YOUR AUTONOMY FLEET</p>
          <h1>Every engine. One command surface.</h1>
          <p className="lede">
            See the account, model, coverage, state, and schedule behind every
            strategy—without opening each mandate first.
          </p>
        </div>
        <div className="strategy-fleet-actions">
          <Link className="button-link" href="/automations/new">
            Launch AI Engine
          </Link>
          <div className="strategy-fleet-secondary-actions">
            <Link href="/capital">Capital budgets →</Link>
            <Link href="/activity">Decision journal →</Link>
          </div>
        </div>
      </section>
      <StrategyFleet
        contextWarnings={contextWarnings}
        inventoryAvailable={mandatesResult.available}
        items={items}
      />
    </main>
  );
}
