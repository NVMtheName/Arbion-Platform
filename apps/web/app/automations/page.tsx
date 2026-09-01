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
  PaperAutonomyEvidenceGate,
  PaperAutonomyEvidenceBlocker,
  PaperAutonomyEvidenceThresholdChangeLedger,
  PaperAutonomyEvidenceThresholdCheckpoint,
  PaperActivityCadence,
  PaperActivityWindow,
  PaperDenialEligibility,
  PaperDenialEligibilityFact,
  PaperDenialRiskResultChange,
  PaperDispositionFunnel,
  PaperDispositionFunnelWindow,
  PaperExecutionCosts,
  PaperExecutionCheckpoint,
  PaperExecutionSideCost,
  PaperExecutionSymbolCost,
  PaperEvidenceReview,
  PaperGuardrailCheckCount,
  PaperGuardrailCheckChange,
  PaperGuardrailCheckFact,
  PaperGuardrailCodeCount,
  PaperGuardrailCoverageChange,
  PaperGuardrailCoverageMetricChange,
  PaperGuardrailEvidence,
  PaperGuardrailEvidenceWindow,
  PaperGuardrailProposalFact,
  PaperGuardrailSymbol,
  PaperGuardrailSymbolChange,
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
  activity_cadence_semantics?: string;
  activity_cadence_read_only?: boolean;
  disposition_funnel_semantics?: string;
  disposition_funnel_read_only?: boolean;
  guardrail_evidence_semantics?: string;
  guardrail_evidence_read_only?: boolean;
  guardrail_coverage_semantics?: string;
  guardrail_coverage_read_only?: boolean;
  guardrail_coverage_change_semantics?: string;
  guardrail_coverage_change_read_only?: boolean;
  denial_eligibility_semantics?: string;
  denial_eligibility_read_only?: boolean;
  paper_evidence_readiness_semantics?: string;
  paper_evidence_readiness_read_only?: boolean;
  paper_evidence_readiness_grants_authority?: boolean;
  paper_evidence_review_semantics?: string;
  paper_evidence_review_grants_authority?: boolean;
  broker_action_available?: boolean;
  live_promotion_available?: boolean;
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

function safeInteger(value: unknown) {
  return typeof value === "number" && Number.isSafeInteger(value)
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

function normalizedPaperActivityWindow(
  value: unknown,
  expectedHours: 24 | 168,
): PaperActivityWindow | undefined {
  const window = record(value);
  const status = text(window, "status", "Status");
  const horizonHours = nonnegativeInteger(
    window?.horizon_hours ?? window?.HorizonHours,
  );
  const scheduledCycleCount = nonnegativeInteger(
    window?.scheduled_cycle_count ?? window?.ScheduledCycleCount,
  );
  const succeededCycleCount = nonnegativeInteger(
    window?.succeeded_cycle_count ?? window?.SucceededCycleCount,
  );
  const failedCycleCount = nonnegativeInteger(
    window?.failed_cycle_count ?? window?.FailedCycleCount,
  );
  const safeWaitCycleCount = nonnegativeInteger(
    window?.safe_wait_cycle_count ?? window?.SafeWaitCycleCount,
  );
  const abstentionCount = nonnegativeInteger(
    window?.abstention_count ?? window?.AbstentionCount,
  );
  const deterministicDenyCount = nonnegativeInteger(
    window?.deterministic_deny_count ?? window?.DeterministicDenyCount,
  );
  const simulatedFillCount = nonnegativeInteger(
    window?.simulated_fill_count ?? window?.SimulatedFillCount,
  );
  const otherSucceededCount = nonnegativeInteger(
    window?.other_succeeded_count ?? window?.OtherSucceededCount,
  );
  const windowStartedAt = text(window, "window_started_at", "WindowStartedAt");
  const windowEndedAt = text(window, "window_ended_at", "WindowEndedAt");
  const observedStartedAt = text(
    window,
    "observed_started_at",
    "ObservedStartedAt",
  );
  const observedEndedAt = text(window, "observed_ended_at", "ObservedEndedAt");
  if (
    !["AVAILABLE", "UNAVAILABLE"].includes(status ?? "") ||
    horizonHours !== expectedHours ||
    scheduledCycleCount === undefined ||
    succeededCycleCount === undefined ||
    failedCycleCount === undefined ||
    safeWaitCycleCount === undefined ||
    abstentionCount === undefined ||
    deterministicDenyCount === undefined ||
    simulatedFillCount === undefined ||
    otherSucceededCount === undefined ||
    scheduledCycleCount !==
      succeededCycleCount + failedCycleCount + safeWaitCycleCount ||
    succeededCycleCount !==
      abstentionCount +
        deterministicDenyCount +
        simulatedFillCount +
        otherSucceededCount ||
    (status === "AVAILABLE" &&
      (!windowStartedAt ||
        !windowEndedAt ||
        Number.isNaN(Date.parse(windowStartedAt)) ||
        Number.isNaN(Date.parse(windowEndedAt)))) ||
    (observedStartedAt && Number.isNaN(Date.parse(observedStartedAt))) ||
    (observedEndedAt && Number.isNaN(Date.parse(observedEndedAt)))
  )
    return;
  return {
    status: status as PaperActivityWindow["status"],
    horizon_hours: expectedHours,
    window_started_at: windowStartedAt,
    window_ended_at: windowEndedAt,
    observed_started_at: observedStartedAt,
    observed_ended_at: observedEndedAt,
    scheduled_cycle_count: scheduledCycleCount,
    succeeded_cycle_count: succeededCycleCount,
    failed_cycle_count: failedCycleCount,
    safe_wait_cycle_count: safeWaitCycleCount,
    abstention_count: abstentionCount,
    deterministic_deny_count: deterministicDenyCount,
    simulated_fill_count: simulatedFillCount,
    other_succeeded_count: otherSucceededCount,
  };
}

function exactDispositionRateMatches(
  count: number,
  total: number,
  value?: string,
) {
  if (total === 0) return value === undefined;
  const expected = divideExactDecimals(
    multiplyExactDecimals(String(count), "100") ?? "invalid",
    String(total),
    10,
  );
  return Boolean(
    expected && value && compareExactDecimals(expected, value) === 0,
  );
}

function normalizedPaperDispositionWindow(
  value: unknown,
  expectedHours: 24 | 168,
): PaperDispositionFunnelWindow | undefined {
  const window = record(value);
  const status = text(window, "status", "Status");
  const horizonHours = nonnegativeInteger(
    window?.horizon_hours ?? window?.HorizonHours,
  );
  const count = (snake: string, pascal: string) =>
    nonnegativeInteger(window?.[snake] ?? window?.[pascal]);
  const scheduled = count("scheduled_cycle_count", "ScheduledCycleCount");
  const completed = count("completed_cycle_count", "CompletedCycleCount");
  const succeeded = count(
    "succeeded_evaluation_count",
    "SucceededEvaluationCount",
  );
  const failed = count("failed_cycle_count", "FailedCycleCount");
  const safeWait = count("safe_wait_cycle_count", "SafeWaitCycleCount");
  const decisions = count("decision_count", "DecisionCount");
  const abstentions = count("abstention_count", "AbstentionCount");
  const proposals = count("proposal_count", "ProposalCount");
  const denials = count("deterministic_deny_count", "DeterministicDenyCount");
  const fills = count("simulated_fill_count", "SimulatedFillCount");
  const other = count(
    "other_proposal_outcome_count",
    "OtherProposalOutcomeCount",
  );
  const rate = (snake: string, pascal: string) =>
    exactDecimal(window?.[snake] ?? window?.[pascal]);
  const completionRate = rate(
    "completion_rate_percent",
    "CompletionRatePercent",
  );
  const succeededRate = rate(
    "succeeded_evaluation_rate_percent",
    "SucceededEvaluationRatePercent",
  );
  const decisionRate = rate("decision_rate_percent", "DecisionRatePercent");
  const abstentionRate = rate(
    "abstention_rate_percent",
    "AbstentionRatePercent",
  );
  const proposalRate = rate("proposal_rate_percent", "ProposalRatePercent");
  const denialRate = rate(
    "deterministic_deny_rate_percent",
    "DeterministicDenyRatePercent",
  );
  const fillRate = rate(
    "simulated_fill_rate_percent",
    "SimulatedFillRatePercent",
  );
  const otherRate = rate(
    "other_proposal_outcome_rate_percent",
    "OtherProposalOutcomeRatePercent",
  );
  const windowStartedAt = text(window, "window_started_at", "WindowStartedAt");
  const windowEndedAt = text(window, "window_ended_at", "WindowEndedAt");
  if (
    !["AVAILABLE", "UNAVAILABLE"].includes(status ?? "") ||
    horizonHours !== expectedHours ||
    [
      scheduled,
      completed,
      succeeded,
      failed,
      safeWait,
      decisions,
      abstentions,
      proposals,
      denials,
      fills,
      other,
    ].some((value) => value === undefined)
  )
    return;
  const normalized: PaperDispositionFunnelWindow = {
    status: status as PaperDispositionFunnelWindow["status"],
    horizon_hours: expectedHours,
    window_started_at: windowStartedAt,
    window_ended_at: windowEndedAt,
    scheduled_cycle_count: scheduled!,
    completed_cycle_count: completed!,
    succeeded_evaluation_count: succeeded!,
    failed_cycle_count: failed!,
    safe_wait_cycle_count: safeWait!,
    decision_count: decisions!,
    abstention_count: abstentions!,
    proposal_count: proposals!,
    deterministic_deny_count: denials!,
    simulated_fill_count: fills!,
    other_proposal_outcome_count: other!,
    completion_rate_percent: completionRate,
    succeeded_evaluation_rate_percent: succeededRate,
    decision_rate_percent: decisionRate,
    abstention_rate_percent: abstentionRate,
    proposal_rate_percent: proposalRate,
    deterministic_deny_rate_percent: denialRate,
    simulated_fill_rate_percent: fillRate,
    other_proposal_outcome_rate_percent: otherRate,
  };
  if (status === "UNAVAILABLE") {
    if (
      [
        completionRate,
        succeededRate,
        decisionRate,
        abstentionRate,
        proposalRate,
        denialRate,
        fillRate,
        otherRate,
      ].some(Boolean)
    )
      return;
    return normalized;
  }
  if (
    !windowStartedAt ||
    !windowEndedAt ||
    Number.isNaN(Date.parse(windowStartedAt)) ||
    Number.isNaN(Date.parse(windowEndedAt)) ||
    scheduled! < 1 ||
    completed !== scheduled ||
    scheduled !== succeeded! + failed! + safeWait! ||
    decisions !== succeeded ||
    decisions !== abstentions! + proposals! ||
    proposals !== denials! + fills! + other! ||
    !exactDispositionRateMatches(completed!, scheduled!, completionRate) ||
    !exactDispositionRateMatches(succeeded!, scheduled!, succeededRate) ||
    !exactDispositionRateMatches(decisions!, scheduled!, decisionRate) ||
    !exactDispositionRateMatches(abstentions!, decisions!, abstentionRate) ||
    !exactDispositionRateMatches(proposals!, decisions!, proposalRate) ||
    !exactDispositionRateMatches(denials!, proposals!, denialRate) ||
    !exactDispositionRateMatches(fills!, proposals!, fillRate) ||
    !exactDispositionRateMatches(other!, proposals!, otherRate)
  )
    return;
  return normalized;
}

function normalizedPaperDispositionFunnel(
  value: unknown,
): PaperDispositionFunnel | undefined {
  const funnel = record(value);
  const status = text(funnel, "status", "Status");
  const method = text(funnel, "calculation_method", "CalculationMethod");
  const twentyFourHours = normalizedPaperDispositionWindow(
    funnel?.twenty_four_hours ?? funnel?.TwentyFourHours,
    24,
  );
  const sevenDays = normalizedPaperDispositionWindow(
    funnel?.seven_days ?? funnel?.SevenDays,
    168,
  );
  if (
    !["AVAILABLE", "UNAVAILABLE"].includes(status ?? "") ||
    !twentyFourHours ||
    !sevenDays ||
    (status === "AVAILABLE" &&
      (method !== "IMMUTABLE_PAPER_EVALUATION_DISPOSITION_FUNNEL" ||
        twentyFourHours.status !== "AVAILABLE"))
  )
    return;
  return {
    status: status as PaperDispositionFunnel["status"],
    calculation_method: method as PaperDispositionFunnel["calculation_method"],
    twenty_four_hours: twentyFourHours,
    seven_days: sevenDays,
  };
}

function normalizedPaperGuardrailCodeCounts(
  value: unknown,
): PaperGuardrailCodeCount[] | undefined {
  if (!Array.isArray(value)) return;
  const result: PaperGuardrailCodeCount[] = [];
  const seen = new Set<string>();
  for (const raw of value) {
    const item = record(raw);
    const code = text(item, "code", "Code");
    const count = nonnegativeInteger(item?.count ?? item?.Count);
    if (!code || count === undefined || count < 1 || seen.has(code)) return;
    seen.add(code);
    result.push({ code, count });
  }
  if (
    result.some(
      (item, index) => index > 0 && result[index - 1].code >= item.code,
    )
  )
    return;
  return result;
}

function normalizedPaperGuardrailCheckCounts(
  value: unknown,
): PaperGuardrailCheckCount[] | undefined {
  if (!Array.isArray(value)) return;
  const result: PaperGuardrailCheckCount[] = [];
  const seen = new Set<string>();
  for (const raw of value) {
    const item = record(raw);
    const code = text(item, "code", "Code");
    const evaluationCount = nonnegativeInteger(
      item?.evaluation_count ?? item?.EvaluationCount,
    );
    const passCount = nonnegativeInteger(item?.pass_count ?? item?.PassCount);
    const failCount = nonnegativeInteger(item?.fail_count ?? item?.FailCount);
    const warnCount = nonnegativeInteger(item?.warn_count ?? item?.WarnCount);
    if (
      !code ||
      evaluationCount === undefined ||
      evaluationCount < 1 ||
      passCount === undefined ||
      failCount === undefined ||
      warnCount === undefined ||
      evaluationCount !== passCount + failCount + warnCount ||
      seen.has(code)
    )
      return;
    seen.add(code);
    result.push({
      code,
      evaluation_count: evaluationCount,
      pass_count: passCount,
      fail_count: failCount,
      warn_count: warnCount,
    });
  }
  if (
    result.some(
      (item, index) => index > 0 && result[index - 1].code >= item.code,
    )
  )
    return;
  return result;
}

function normalizedPaperGuardrailWindow(
  value: unknown,
  expectedHorizon: 24 | 168,
): PaperGuardrailEvidenceWindow | undefined {
  const window = record(value);
  const status = text(window, "status", "Status");
  const coverageStatus = text(window, "coverage_status", "CoverageStatus");
  const horizon = nonnegativeInteger(
    window?.horizon_hours ?? window?.HorizonHours,
  );
  const startedAt = text(window, "window_started_at", "WindowStartedAt");
  const endedAt = text(window, "window_ended_at", "WindowEndedAt");
  const proposalCount = nonnegativeInteger(
    window?.proposal_count ?? window?.ProposalCount,
  );
  const allowCount = nonnegativeInteger(
    window?.allow_count ?? window?.AllowCount,
  );
  const denyCount = nonnegativeInteger(window?.deny_count ?? window?.DenyCount);
  const fillCount = nonnegativeInteger(
    window?.simulated_fill_count ?? window?.SimulatedFillCount,
  );
  const minimum = exactDecimal(
    window?.minimum_proposed_notional ?? window?.MinimumProposedNotional,
  );
  const median = exactDecimal(
    window?.median_proposed_notional ?? window?.MedianProposedNotional,
  );
  const maximum = exactDecimal(
    window?.maximum_proposed_notional ?? window?.MaximumProposedNotional,
  );
  const reasonCounts = normalizedPaperGuardrailCodeCounts(
    window?.denial_reason_codes ?? window?.DenialReasonCodes,
  );
  const failedCounts = normalizedPaperGuardrailCodeCounts(
    window?.failed_check_codes ?? window?.FailedCheckCodes,
  );
  const expectedCheckCodes = stringList(
    window?.expected_check_codes ?? window?.ExpectedCheckCodes,
  );
  const checkResults = normalizedPaperGuardrailCheckCounts(
    window?.check_results ?? window?.CheckResults,
  );
  const fullyEvaluatedCount = nonnegativeInteger(
    window?.fully_evaluated_count ?? window?.FullyEvaluatedCount,
  );
  const failClosedPrefixCount = nonnegativeInteger(
    window?.fail_closed_prefix_count ?? window?.FailClosedPrefixCount,
  );
  const checkSetDriftCount = nonnegativeInteger(
    window?.check_set_drift_count ?? window?.CheckSetDriftCount,
  );
  const rawSymbols = window?.symbols ?? window?.Symbols;
  const rawProposals = window?.proposals ?? window?.Proposals;
  if (
    !["AVAILABLE", "UNAVAILABLE"].includes(status ?? "") ||
    !["COMPLETE", "DRIFT_DETECTED", "UNAVAILABLE"].includes(
      coverageStatus ?? "",
    ) ||
    horizon !== expectedHorizon ||
    proposalCount === undefined ||
    allowCount === undefined ||
    denyCount === undefined ||
    fillCount === undefined ||
    !reasonCounts ||
    !failedCounts ||
    !checkResults ||
    fullyEvaluatedCount === undefined ||
    failClosedPrefixCount === undefined ||
    checkSetDriftCount === undefined ||
    !Array.isArray(rawSymbols) ||
    !Array.isArray(rawProposals)
  )
    return;
  const unavailable = {
    status: "UNAVAILABLE" as const,
    coverage_status: "UNAVAILABLE" as const,
    horizon_hours: expectedHorizon,
    window_started_at: startedAt,
    window_ended_at: endedAt,
    proposal_count: 0,
    allow_count: 0,
    deny_count: 0,
    simulated_fill_count: 0,
    denial_reason_codes: [] as PaperGuardrailCodeCount[],
    failed_check_codes: [] as PaperGuardrailCodeCount[],
    expected_check_codes: [] as string[],
    check_results: [] as PaperGuardrailCheckCount[],
    fully_evaluated_count: 0,
    fail_closed_prefix_count: 0,
    check_set_drift_count: 0,
    symbols: [] as PaperGuardrailSymbol[],
    proposals: [] as PaperGuardrailProposalFact[],
  };
  if (status === "UNAVAILABLE") return unavailable;
  if (
    !startedAt ||
    !endedAt ||
    Number.isNaN(Date.parse(startedAt)) ||
    Number.isNaN(Date.parse(endedAt)) ||
    Date.parse(startedAt) >= Date.parse(endedAt) ||
    !["COMPLETE", "DRIFT_DETECTED"].includes(coverageStatus ?? "") ||
    expectedCheckCodes.length < 1 ||
    new Set(expectedCheckCodes).size !== expectedCheckCodes.length ||
    proposalCount !== allowCount + denyCount ||
    proposalCount !==
      fullyEvaluatedCount + failClosedPrefixCount + checkSetDriftCount ||
    (coverageStatus === "COMPLETE"
      ? checkSetDriftCount !== 0
      : checkSetDriftCount === 0) ||
    allowCount !== fillCount ||
    (proposalCount === 0
      ? minimum !== undefined || median !== undefined || maximum !== undefined
      : !minimum || !median || !maximum) ||
    (minimum &&
      median &&
      maximum &&
      (compareExactDecimals(minimum, median) === 1 ||
        compareExactDecimals(median, maximum) === 1)) ||
    rawProposals.length !== proposalCount
  )
    return;
  const proposals: PaperGuardrailProposalFact[] = [];
  const identities = [
    new Set<string>(),
    new Set<string>(),
    new Set<string>(),
    new Set<string>(),
  ];
  for (const raw of rawProposals) {
    const item = record(raw);
    const decisionID = text(
      item,
      "decision_journal_entry_id",
      "DecisionJournalEntryID",
    );
    const actionID = text(item, "proposed_action_id", "ProposedActionID");
    const riskID = text(item, "risk_evaluation_id", "RiskEvaluationID");
    const executionID = text(item, "execution_record_id", "ExecutionRecordID");
    const createdAt = text(item, "created_at", "CreatedAt");
    const symbol = text(item, "symbol", "Symbol");
    const instrument = text(item, "instrument", "Instrument");
    const side = text(item, "side", "Side");
    const rawQuantity = item?.proposed_quantity ?? item?.ProposedQuantity;
    const quantity = exactDecimal(rawQuantity);
    const notional = exactDecimal(
      item?.proposed_notional ?? item?.ProposedNotional,
    );
    const riskDecision = text(item, "risk_decision", "RiskDecision");
    const executionStatus = text(item, "execution_status", "ExecutionStatus");
    const denialReasons = stringList(
      item?.denial_reason_codes ?? item?.DenialReasonCodes,
    );
    const failedChecks = stringList(
      item?.failed_check_codes ?? item?.FailedCheckCodes,
    );
    const rawChecks = item?.checks ?? item?.Checks;
    const proposalCoverageStatus = text(
      item,
      "coverage_status",
      "CoverageStatus",
    );
    const terminalCheckStage = text(
      item,
      "terminal_check_stage",
      "TerminalCheckStage",
    );
    const provider = text(item, "financial_provider", "FinancialProvider");
    const feed = text(item, "market_feed", "MarketFeed");
    const quality = text(item, "market_quality", "MarketQuality");
    const marketAt = text(item, "market_observed_at", "MarketObservedAt");
    const ids = [decisionID, actionID, riskID, executionID];
    if (!Array.isArray(rawChecks)) return;
    const checks: PaperGuardrailCheckFact[] = [];
    const seenChecks = new Set<string>();
    for (const rawCheck of rawChecks) {
      const check = record(rawCheck);
      const code = text(check, "code", "Code");
      const result = text(check, "result", "Result");
      if (
        !code ||
        !["PASS", "FAIL", "WARN"].includes(result ?? "") ||
        seenChecks.has(code)
      )
        return;
      seenChecks.add(code);
      checks.push({ code, result: result as "PASS" | "FAIL" | "WARN" });
    }
    if (
      ids.some((id) => !id) ||
      ids.some((id, index) => identities[index].has(id!)) ||
      !createdAt ||
      Number.isNaN(Date.parse(createdAt)) ||
      Date.parse(createdAt) < Date.parse(startedAt) ||
      Date.parse(createdAt) > Date.parse(endedAt) ||
      !symbol ||
      !["EQUITY", "CRYPTO"].includes(instrument ?? "") ||
      !["BUY", "SELL"].includes(side ?? "") ||
      (rawQuantity !== undefined &&
        (!quantity || compareExactDecimals(quantity, "0") !== 1)) ||
      !notional ||
      compareExactDecimals(notional, "0") !== 1 ||
      !provider ||
      !feed ||
      !quality ||
      !marketAt ||
      Number.isNaN(Date.parse(marketAt)) ||
      Date.parse(marketAt) > Date.parse(createdAt) ||
      checks.length < 1 ||
      !["FULL_EVALUATION", "FAIL_CLOSED_PREFIX", "DRIFT_DETECTED"].includes(
        proposalCoverageStatus ?? "",
      ) ||
      (proposalCoverageStatus !== "DRIFT_DETECTED" && !terminalCheckStage) ||
      !(
        (riskDecision === "ALLOW" &&
          executionStatus === "SIMULATED_FILLED" &&
          denialReasons.length === 0 &&
          failedChecks.length === 0) ||
        (riskDecision === "DENY" &&
          executionStatus === "RISK_DENIED" &&
          denialReasons.length > 0 &&
          denialReasons.join("|") === failedChecks.join("|"))
      )
    )
      return;
    ids.forEach((id, index) => identities[index].add(id!));
    proposals.push({
      decision_journal_entry_id: decisionID!,
      proposed_action_id: actionID!,
      risk_evaluation_id: riskID!,
      execution_record_id: executionID!,
      created_at: createdAt,
      symbol,
      instrument: instrument as "EQUITY" | "CRYPTO",
      side: side as "BUY" | "SELL",
      proposed_quantity: quantity ?? "",
      proposed_notional: notional,
      risk_decision: riskDecision as "ALLOW" | "DENY",
      execution_status: executionStatus as "SIMULATED_FILLED" | "RISK_DENIED",
      denial_reason_codes: denialReasons,
      failed_check_codes: failedChecks,
      checks,
      coverage_status:
        proposalCoverageStatus as PaperGuardrailProposalFact["coverage_status"],
      terminal_check_stage: terminalCheckStage ?? "",
      financial_provider: provider,
      market_feed: feed,
      market_quality: quality,
      market_observed_at: marketAt,
    });
  }
  if (
    proposals.filter((item) => item.risk_decision === "ALLOW").length !==
      allowCount ||
    proposals.filter((item) => item.risk_decision === "DENY").length !==
      denyCount ||
    proposals.filter((item) => item.coverage_status === "FULL_EVALUATION")
      .length !== fullyEvaluatedCount ||
    proposals.filter((item) => item.coverage_status === "FAIL_CLOSED_PREFIX")
      .length !== failClosedPrefixCount ||
    proposals.filter((item) => item.coverage_status === "DRIFT_DETECTED")
      .length !== checkSetDriftCount ||
    proposals.some(
      (item) =>
        (item.coverage_status === "FULL_EVALUATION" &&
          (item.risk_decision !== "ALLOW" ||
            item.terminal_check_stage !== "ALL_REQUIRED_CHECKS" ||
            item.checks.length !== expectedCheckCodes.length ||
            item.checks.some((check) => check.result === "FAIL"))) ||
        (item.coverage_status === "FAIL_CLOSED_PREFIX" &&
          (item.risk_decision !== "DENY" ||
            item.checks.length > expectedCheckCodes.length ||
            item.checks[item.checks.length - 1]?.result !== "FAIL" ||
            item.checks.slice(0, -1).some((check) => check.result === "FAIL"))),
    ) ||
    proposals.some(
      (item, index) =>
        index > 0 &&
        (Date.parse(proposals[index - 1].created_at) >
          Date.parse(item.created_at) ||
          (proposals[index - 1].created_at === item.created_at &&
            proposals[index - 1].decision_journal_entry_id >=
              item.decision_journal_entry_id)),
    )
  )
    return;
  const sortedNotionals = proposals
    .map((item) => item.proposed_notional)
    .sort((left, right) => compareExactDecimals(left, right) ?? 0);
  if (proposalCount > 0) {
    const middle = Math.floor(sortedNotionals.length / 2);
    const expectedMedian =
      sortedNotionals.length % 2 === 1
        ? sortedNotionals[middle]
        : divideExactDecimals(
            sumExactMoney([
              { amount: sortedNotionals[middle - 1], currency: "USD" },
              { amount: sortedNotionals[middle], currency: "USD" },
            ])?.amount ?? "invalid",
            "2",
            10,
          );
    if (
      compareExactDecimals(sortedNotionals[0], minimum!) !== 0 ||
      compareExactDecimals(
        sortedNotionals[sortedNotionals.length - 1],
        maximum!,
      ) !== 0 ||
      !expectedMedian ||
      compareExactDecimals(expectedMedian, median!) !== 0
    )
      return;
  }
  const exactReasonCounts = new Map<string, number>();
  const exactFailedCounts = new Map<string, number>();
  const exactCheckCounts = new Map<
    string,
    {
      evaluation_count: number;
      pass_count: number;
      fail_count: number;
      warn_count: number;
    }
  >();
  for (const proposal of proposals) {
    proposal.denial_reason_codes.forEach((code) =>
      exactReasonCounts.set(code, (exactReasonCounts.get(code) ?? 0) + 1),
    );
    proposal.failed_check_codes.forEach((code) =>
      exactFailedCounts.set(code, (exactFailedCounts.get(code) ?? 0) + 1),
    );
    proposal.checks.forEach((check) => {
      const counts = exactCheckCounts.get(check.code) ?? {
        evaluation_count: 0,
        pass_count: 0,
        fail_count: 0,
        warn_count: 0,
      };
      counts.evaluation_count += 1;
      if (check.result === "PASS") counts.pass_count += 1;
      if (check.result === "FAIL") counts.fail_count += 1;
      if (check.result === "WARN") counts.warn_count += 1;
      exactCheckCounts.set(check.code, counts);
    });
  }
  if (
    reasonCounts.length !== exactReasonCounts.size ||
    failedCounts.length !== exactFailedCounts.size ||
    reasonCounts.some(
      (item) => exactReasonCounts.get(item.code) !== item.count,
    ) ||
    failedCounts.some(
      (item) => exactFailedCounts.get(item.code) !== item.count,
    ) ||
    checkResults.length !== exactCheckCounts.size ||
    checkResults.some((item) => {
      const exact = exactCheckCounts.get(item.code);
      return (
        !exact ||
        exact.evaluation_count !== item.evaluation_count ||
        exact.pass_count !== item.pass_count ||
        exact.fail_count !== item.fail_count ||
        exact.warn_count !== item.warn_count
      );
    })
  )
    return;
  const symbols: PaperGuardrailSymbol[] = [];
  for (const raw of rawSymbols) {
    const item = record(raw);
    const symbol = text(item, "symbol", "Symbol");
    const instrument = text(item, "instrument", "Instrument");
    const symbolProposals = nonnegativeInteger(
      item?.proposal_count ?? item?.ProposalCount,
    );
    const symbolAllows = nonnegativeInteger(
      item?.allow_count ?? item?.AllowCount,
    );
    const symbolDenies = nonnegativeInteger(
      item?.deny_count ?? item?.DenyCount,
    );
    const symbolFills = nonnegativeInteger(
      item?.simulated_fill_count ?? item?.SimulatedFillCount,
    );
    const symbolNotional = exactDecimal(
      item?.proposed_notional ?? item?.ProposedNotional,
    );
    if (
      !symbol ||
      !["EQUITY", "CRYPTO"].includes(instrument ?? "") ||
      symbolProposals === undefined ||
      symbolAllows === undefined ||
      symbolDenies === undefined ||
      symbolFills === undefined ||
      !symbolNotional ||
      symbolProposals < 1 ||
      symbolProposals !== symbolAllows + symbolDenies ||
      symbolAllows !== symbolFills
    )
      return;
    symbols.push({
      symbol,
      instrument: instrument as "EQUITY" | "CRYPTO",
      proposal_count: symbolProposals,
      allow_count: symbolAllows,
      deny_count: symbolDenies,
      simulated_fill_count: symbolFills,
      proposed_notional: symbolNotional,
    });
  }
  if (
    symbols.reduce((sum, item) => sum + item.proposal_count, 0) !==
      proposalCount ||
    symbols.some(
      (item, index) =>
        index > 0 &&
        `${symbols[index - 1].instrument}:${symbols[index - 1].symbol}` >=
          `${item.instrument}:${item.symbol}`,
    )
  )
    return;
  for (const symbol of symbols) {
    const facts = proposals.filter(
      (proposal) =>
        proposal.symbol === symbol.symbol &&
        proposal.instrument === symbol.instrument,
    );
    const exactSymbolNotional = sumExactMoney(
      facts.map((proposal) => ({
        amount: proposal.proposed_notional,
        currency: "USD",
      })),
    )?.amount;
    if (
      facts.length !== symbol.proposal_count ||
      facts.filter((fact) => fact.risk_decision === "ALLOW").length !==
        symbol.allow_count ||
      facts.filter((fact) => fact.risk_decision === "DENY").length !==
        symbol.deny_count ||
      !exactSymbolNotional ||
      compareExactDecimals(exactSymbolNotional, symbol.proposed_notional) !== 0
    )
      return;
  }
  return {
    status: "AVAILABLE",
    coverage_status: coverageStatus as "COMPLETE" | "DRIFT_DETECTED",
    horizon_hours: expectedHorizon,
    window_started_at: startedAt,
    window_ended_at: endedAt,
    proposal_count: proposalCount,
    allow_count: allowCount,
    deny_count: denyCount,
    simulated_fill_count: fillCount,
    minimum_proposed_notional: minimum,
    median_proposed_notional: median,
    maximum_proposed_notional: maximum,
    denial_reason_codes: reasonCounts,
    failed_check_codes: failedCounts,
    expected_check_codes: expectedCheckCodes,
    check_results: checkResults,
    fully_evaluated_count: fullyEvaluatedCount,
    fail_closed_prefix_count: failClosedPrefixCount,
    check_set_drift_count: checkSetDriftCount,
    symbols,
    proposals,
  };
}

function paperGuardrailShareDirection(
  currentCount: number,
  currentTotal: number,
  baselineCount: number,
  baselineTotal: number,
) {
  const current = currentCount * baselineTotal;
  const baseline = baselineCount * currentTotal;
  return current > baseline
    ? "INCREASED"
    : current < baseline
      ? "DECREASED"
      : "UNCHANGED";
}

function normalizedPaperGuardrailCoverageChange(
  value: unknown,
  twentyFour: PaperGuardrailEvidenceWindow,
  sevenDays: PaperGuardrailEvidenceWindow,
): PaperGuardrailCoverageChange | undefined {
  const change = record(value);
  const status = text(change, "status", "Status");
  const method = text(change, "calculation_method", "CalculationMethod");
  const baselineHorizon = nonnegativeInteger(
    change?.baseline_horizon_hours ?? change?.BaselineHorizonHours,
  );
  const currentHorizon = nonnegativeInteger(
    change?.current_horizon_hours ?? change?.CurrentHorizonHours,
  );
  const baselineStarted = text(
    change,
    "baseline_window_started_at",
    "BaselineWindowStartedAt",
  );
  const baselineEnded = text(
    change,
    "baseline_window_ended_at",
    "BaselineWindowEndedAt",
  );
  const currentStarted = text(
    change,
    "current_window_started_at",
    "CurrentWindowStartedAt",
  );
  const currentEnded = text(
    change,
    "current_window_ended_at",
    "CurrentWindowEndedAt",
  );
  const baselineProposalCount = nonnegativeInteger(
    change?.baseline_proposal_count ?? change?.BaselineProposalCount,
  );
  const currentProposalCount = nonnegativeInteger(
    change?.current_proposal_count ?? change?.CurrentProposalCount,
  );
  const proposalCountDelta = safeInteger(
    change?.proposal_count_delta ?? change?.ProposalCountDelta,
  );
  const providers = stringList(
    change?.financial_providers ?? change?.FinancialProviders,
  );
  const firstEvidenceAt = text(change, "first_evidence_at", "FirstEvidenceAt");
  const latestEvidenceAt = text(
    change,
    "latest_evidence_at",
    "LatestEvidenceAt",
  );
  const firstMarketObservedAt = text(
    change,
    "first_market_observed_at",
    "FirstMarketObservedAt",
  );
  const latestMarketObservedAt = text(
    change,
    "latest_market_observed_at",
    "LatestMarketObservedAt",
  );
  const firstDriftAt = text(
    change,
    "first_check_set_drift_at",
    "FirstCheckSetDriftAt",
  );
  const latestDriftAt = text(
    change,
    "latest_check_set_drift_at",
    "LatestCheckSetDriftAt",
  );
  const rawMetrics = change?.coverage_metrics ?? change?.CoverageMetrics;
  const rawChecks = change?.check_changes ?? change?.CheckChanges;
  const rawSymbols = change?.symbol_changes ?? change?.SymbolChanges;
  if (
    !["AVAILABLE", "UNAVAILABLE"].includes(status ?? "") ||
    baselineHorizon !== 168 ||
    currentHorizon !== 24 ||
    baselineProposalCount === undefined ||
    currentProposalCount === undefined ||
    proposalCountDelta === undefined ||
    !Array.isArray(rawMetrics) ||
    !Array.isArray(rawChecks) ||
    !Array.isArray(rawSymbols)
  )
    return;
  if (status === "UNAVAILABLE") {
    if (rawMetrics.length || rawChecks.length || rawSymbols.length) return;
    return {
      status: "UNAVAILABLE",
      baseline_horizon_hours: 168,
      current_horizon_hours: 24,
      baseline_window_started_at: baselineStarted,
      baseline_window_ended_at: baselineEnded,
      current_window_started_at: currentStarted,
      current_window_ended_at: currentEnded,
      baseline_proposal_count: baselineProposalCount,
      current_proposal_count: currentProposalCount,
      proposal_count_delta: proposalCountDelta,
      financial_providers: [],
      coverage_metrics: [],
      check_changes: [],
      symbol_changes: [],
    };
  }
  if (
    method !==
      "IMMUTABLE_24_HOUR_AND_SEVEN_DAY_GUARDRAIL_COVERAGE_COMPARISON" ||
    twentyFour.status !== "AVAILABLE" ||
    sevenDays.status !== "AVAILABLE" ||
    baselineProposalCount !== sevenDays.proposal_count ||
    currentProposalCount !== twentyFour.proposal_count ||
    baselineProposalCount <= 0 ||
    currentProposalCount <= 0 ||
    proposalCountDelta !== currentProposalCount - baselineProposalCount ||
    baselineStarted !== sevenDays.window_started_at ||
    baselineEnded !== sevenDays.window_ended_at ||
    currentStarted !== twentyFour.window_started_at ||
    currentEnded !== twentyFour.window_ended_at ||
    !firstEvidenceAt ||
    !latestEvidenceAt ||
    !firstMarketObservedAt ||
    !latestMarketObservedAt ||
    [
      baselineStarted,
      baselineEnded,
      currentStarted,
      currentEnded,
      firstEvidenceAt,
      latestEvidenceAt,
      firstMarketObservedAt,
      latestMarketObservedAt,
    ].some((timestamp) => Number.isNaN(Date.parse(timestamp ?? ""))) ||
    providers.length === 0 ||
    new Set(providers).size !== providers.length ||
    providers.some((provider, index) =>
      index > 0 ? providers[index - 1] >= provider : !provider,
    ) ||
    Boolean(firstDriftAt) !== Boolean(latestDriftAt) ||
    (firstDriftAt &&
      (Number.isNaN(Date.parse(firstDriftAt)) ||
        Number.isNaN(Date.parse(latestDriftAt!)) ||
        Date.parse(firstDriftAt) > Date.parse(latestDriftAt!)))
  )
    return;

  const expectedMetrics = [
    [
      "FULL_EVALUATION",
      sevenDays.fully_evaluated_count,
      twentyFour.fully_evaluated_count,
    ],
    [
      "FAIL_CLOSED_PREFIX",
      sevenDays.fail_closed_prefix_count,
      twentyFour.fail_closed_prefix_count,
    ],
    [
      "CHECK_SET_DRIFT",
      sevenDays.check_set_drift_count,
      twentyFour.check_set_drift_count,
    ],
  ] as const;
  const metrics: PaperGuardrailCoverageMetricChange[] = [];
  for (const [index, expected] of expectedMetrics.entries()) {
    const item = record(rawMetrics[index]);
    const metric = text(item, "metric", "Metric");
    const baselineCount = nonnegativeInteger(
      item?.baseline_count ?? item?.BaselineCount,
    );
    const currentCount = nonnegativeInteger(
      item?.current_count ?? item?.CurrentCount,
    );
    const countDelta = safeInteger(item?.count_delta ?? item?.CountDelta);
    const baselineRate = exactDecimal(
      item?.baseline_share_percent ?? item?.BaselineSharePercent,
    );
    const currentRate = exactDecimal(
      item?.current_share_percent ?? item?.CurrentSharePercent,
    );
    const shareChange = text(item, "share_change", "ShareChange");
    if (
      metric !== expected[0] ||
      baselineCount !== expected[1] ||
      currentCount !== expected[2] ||
      countDelta !== currentCount - baselineCount ||
      !baselineRate ||
      !currentRate ||
      !exactDispositionRateMatches(
        baselineCount,
        baselineProposalCount,
        baselineRate,
      ) ||
      !exactDispositionRateMatches(
        currentCount,
        currentProposalCount,
        currentRate,
      ) ||
      shareChange !==
        paperGuardrailShareDirection(
          currentCount,
          currentProposalCount,
          baselineCount,
          baselineProposalCount,
        )
    )
      return;
    metrics.push({
      metric,
      baseline_count: baselineCount,
      current_count: currentCount,
      count_delta: countDelta,
      baseline_share_percent: baselineRate,
      current_share_percent: currentRate,
      share_change: shareChange,
    } as PaperGuardrailCoverageMetricChange);
  }
  if (rawMetrics.length !== metrics.length) return;
  const driftMetric = metrics[2];
  if (
    (driftMetric.baseline_count === 0 && (firstDriftAt || latestDriftAt)) ||
    (driftMetric.baseline_count > 0 && (!firstDriftAt || !latestDriftAt))
  )
    return;

  const checks: PaperGuardrailCheckChange[] = [];
  for (const [index, raw] of rawChecks.entries()) {
    const item = record(raw);
    const code = text(item, "code", "Code");
    const counts = [
      nonnegativeInteger(
        item?.baseline_evaluation_count ?? item?.BaselineEvaluationCount,
      ),
      nonnegativeInteger(
        item?.current_evaluation_count ?? item?.CurrentEvaluationCount,
      ),
      safeInteger(item?.evaluation_count_delta ?? item?.EvaluationCountDelta),
      nonnegativeInteger(item?.baseline_pass_count ?? item?.BaselinePassCount),
      nonnegativeInteger(item?.current_pass_count ?? item?.CurrentPassCount),
      safeInteger(item?.pass_count_delta ?? item?.PassCountDelta),
      nonnegativeInteger(item?.baseline_fail_count ?? item?.BaselineFailCount),
      nonnegativeInteger(item?.current_fail_count ?? item?.CurrentFailCount),
      safeInteger(item?.fail_count_delta ?? item?.FailCountDelta),
      nonnegativeInteger(item?.baseline_warn_count ?? item?.BaselineWarnCount),
      nonnegativeInteger(item?.current_warn_count ?? item?.CurrentWarnCount),
      safeInteger(item?.warn_count_delta ?? item?.WarnCountDelta),
    ];
    const baselineRate = exactDecimal(
      item?.baseline_evaluation_percent ?? item?.BaselineEvaluationPercent,
    );
    const currentRate = exactDecimal(
      item?.current_evaluation_percent ?? item?.CurrentEvaluationPercent,
    );
    const shareChange = text(
      item,
      "evaluation_share_change",
      "EvaluationShareChange",
    );
    if (
      !code ||
      code !== twentyFour.expected_check_codes[index] ||
      counts.some((count) => count === undefined) ||
      !baselineRate ||
      !currentRate
    )
      return;
    const [
      baselineEvaluated,
      currentEvaluated,
      evaluatedDelta,
      baselinePass,
      currentPass,
      passDelta,
      baselineFail,
      currentFail,
      failDelta,
      baselineWarn,
      currentWarn,
      warnDelta,
    ] = counts as number[];
    if (
      baselineEvaluated !== baselinePass + baselineFail + baselineWarn ||
      currentEvaluated !== currentPass + currentFail + currentWarn ||
      evaluatedDelta !== currentEvaluated - baselineEvaluated ||
      passDelta !== currentPass - baselinePass ||
      failDelta !== currentFail - baselineFail ||
      warnDelta !== currentWarn - baselineWarn ||
      !exactDispositionRateMatches(
        baselineEvaluated,
        baselineProposalCount,
        baselineRate,
      ) ||
      !exactDispositionRateMatches(
        currentEvaluated,
        currentProposalCount,
        currentRate,
      ) ||
      shareChange !==
        paperGuardrailShareDirection(
          currentEvaluated,
          currentProposalCount,
          baselineEvaluated,
          baselineProposalCount,
        )
    )
      return;
    checks.push({
      code,
      baseline_evaluation_count: baselineEvaluated,
      current_evaluation_count: currentEvaluated,
      evaluation_count_delta: evaluatedDelta,
      baseline_pass_count: baselinePass,
      current_pass_count: currentPass,
      pass_count_delta: passDelta,
      baseline_fail_count: baselineFail,
      current_fail_count: currentFail,
      fail_count_delta: failDelta,
      baseline_warn_count: baselineWarn,
      current_warn_count: currentWarn,
      warn_count_delta: warnDelta,
      baseline_evaluation_percent: baselineRate,
      current_evaluation_percent: currentRate,
      evaluation_share_change: shareChange,
    } as PaperGuardrailCheckChange);
  }
  if (
    checks.length !== twentyFour.expected_check_codes.length ||
    checks.length !== sevenDays.expected_check_codes.length
  )
    return;

  const symbols: PaperGuardrailSymbolChange[] = [];
  for (const raw of rawSymbols) {
    const item = record(raw);
    const symbol = text(item, "symbol", "Symbol");
    const instrument = text(item, "instrument", "Instrument");
    const counts = [
      nonnegativeInteger(
        item?.baseline_proposal_count ?? item?.BaselineProposalCount,
      ),
      nonnegativeInteger(
        item?.current_proposal_count ?? item?.CurrentProposalCount,
      ),
      safeInteger(item?.proposal_count_delta ?? item?.ProposalCountDelta),
      nonnegativeInteger(
        item?.baseline_allow_count ?? item?.BaselineAllowCount,
      ),
      nonnegativeInteger(item?.current_allow_count ?? item?.CurrentAllowCount),
      nonnegativeInteger(item?.baseline_deny_count ?? item?.BaselineDenyCount),
      nonnegativeInteger(item?.current_deny_count ?? item?.CurrentDenyCount),
      nonnegativeInteger(
        item?.baseline_simulated_fill_count ?? item?.BaselineSimulatedFillCount,
      ),
      nonnegativeInteger(
        item?.current_simulated_fill_count ?? item?.CurrentSimulatedFillCount,
      ),
    ];
    const baselineRate = exactDecimal(
      item?.baseline_proposal_percent ?? item?.BaselineProposalPercent,
    );
    const currentRate = exactDecimal(
      item?.current_proposal_percent ?? item?.CurrentProposalPercent,
    );
    const shareChange = text(
      item,
      "proposal_share_change",
      "ProposalShareChange",
    );
    const baselineNotional = exactDecimal(
      item?.baseline_proposed_notional ?? item?.BaselineProposedNotional,
    );
    const currentNotional = exactDecimal(
      item?.current_proposed_notional ?? item?.CurrentProposedNotional,
    );
    const notionalDelta = exactDecimal(
      item?.proposed_notional_delta ?? item?.ProposedNotionalDelta,
    );
    if (
      !symbol ||
      !["EQUITY", "CRYPTO"].includes(instrument ?? "") ||
      counts.some((count) => count === undefined) ||
      !baselineRate ||
      !currentRate ||
      !baselineNotional ||
      !currentNotional ||
      !notionalDelta
    )
      return;
    const [
      baselineProposals,
      currentProposals,
      proposalDelta,
      baselineAllow,
      currentAllow,
      baselineDeny,
      currentDeny,
      baselineFills,
      currentFills,
    ] = counts as number[];
    if (
      baselineProposals !== baselineAllow + baselineDeny ||
      currentProposals !== currentAllow + currentDeny ||
      baselineAllow !== baselineFills ||
      currentAllow !== currentFills ||
      proposalDelta !== currentProposals - baselineProposals ||
      subtractExactDecimals(currentNotional, baselineNotional) !==
        notionalDelta ||
      !exactDispositionRateMatches(
        baselineProposals,
        baselineProposalCount,
        baselineRate,
      ) ||
      !exactDispositionRateMatches(
        currentProposals,
        currentProposalCount,
        currentRate,
      ) ||
      shareChange !==
        paperGuardrailShareDirection(
          currentProposals,
          currentProposalCount,
          baselineProposals,
          baselineProposalCount,
        )
    )
      return;
    symbols.push({
      symbol,
      instrument: instrument as "EQUITY" | "CRYPTO",
      baseline_proposal_count: baselineProposals,
      current_proposal_count: currentProposals,
      proposal_count_delta: proposalDelta,
      baseline_proposal_percent: baselineRate,
      current_proposal_percent: currentRate,
      proposal_share_change: shareChange,
      baseline_allow_count: baselineAllow,
      current_allow_count: currentAllow,
      baseline_deny_count: baselineDeny,
      current_deny_count: currentDeny,
      baseline_simulated_fill_count: baselineFills,
      current_simulated_fill_count: currentFills,
      baseline_proposed_notional: baselineNotional,
      current_proposed_notional: currentNotional,
      proposed_notional_delta: notionalDelta,
    } as PaperGuardrailSymbolChange);
  }
  if (
    symbols.reduce((sum, symbol) => sum + symbol.baseline_proposal_count, 0) !==
      baselineProposalCount ||
    symbols.reduce((sum, symbol) => sum + symbol.current_proposal_count, 0) !==
      currentProposalCount ||
    symbols.some(
      (symbol, index) =>
        index > 0 &&
        `${symbols[index - 1].instrument}:${symbols[index - 1].symbol}` >=
          `${symbol.instrument}:${symbol.symbol}`,
    )
  )
    return;

  return {
    status: "AVAILABLE",
    calculation_method:
      method as PaperGuardrailCoverageChange["calculation_method"],
    baseline_horizon_hours: 168,
    current_horizon_hours: 24,
    baseline_window_started_at: baselineStarted,
    baseline_window_ended_at: baselineEnded,
    current_window_started_at: currentStarted,
    current_window_ended_at: currentEnded,
    baseline_proposal_count: baselineProposalCount,
    current_proposal_count: currentProposalCount,
    proposal_count_delta: proposalCountDelta,
    financial_providers: providers,
    first_evidence_at: firstEvidenceAt,
    latest_evidence_at: latestEvidenceAt,
    first_market_observed_at: firstMarketObservedAt,
    latest_market_observed_at: latestMarketObservedAt,
    first_check_set_drift_at: firstDriftAt,
    latest_check_set_drift_at: latestDriftAt,
    coverage_metrics: metrics,
    check_changes: checks,
    symbol_changes: symbols,
  };
}

function normalizedPaperDenialEligibility(
  value: unknown,
  source: PaperGuardrailEvidenceWindow,
): PaperDenialEligibility | undefined {
  const ledger = record(value);
  const status = text(ledger, "status", "Status");
  const method = text(ledger, "calculation_method", "CalculationMethod");
  const horizon = nonnegativeInteger(
    ledger?.horizon_hours ?? ledger?.HorizonHours,
  );
  const startedAt = text(ledger, "window_started_at", "WindowStartedAt");
  const endedAt = text(ledger, "window_ended_at", "WindowEndedAt");
  const denialCount = nonnegativeInteger(
    ledger?.denial_count ?? ledger?.DenialCount,
  );
  const laterAllowedCount = nonnegativeInteger(
    ledger?.later_allowed_count ?? ledger?.LaterAllowedCount,
  );
  const laterDeniedCount = nonnegativeInteger(
    ledger?.later_denied_count ?? ledger?.LaterDeniedCount,
  );
  const noLaterCount = nonnegativeInteger(
    ledger?.no_later_comparable_proposal_count ??
      ledger?.NoLaterComparableProposalCount,
  );
  const providers = stringList(
    ledger?.financial_providers ?? ledger?.FinancialProviders,
  );
  const firstDenialAt = text(ledger, "first_denial_at", "FirstDenialAt");
  const latestDenialAt = text(ledger, "latest_denial_at", "LatestDenialAt");
  const rawDenials = ledger?.denials ?? ledger?.Denials;
  if (
    !["AVAILABLE", "UNAVAILABLE"].includes(status ?? "") ||
    horizon === undefined ||
    denialCount === undefined ||
    laterAllowedCount === undefined ||
    laterDeniedCount === undefined ||
    noLaterCount === undefined ||
    !providers ||
    new Set(providers).size !== providers.length ||
    !Array.isArray(rawDenials)
  )
    return;
  const unavailable: PaperDenialEligibility = {
    status: "UNAVAILABLE",
    horizon_hours: horizon,
    window_started_at: startedAt,
    window_ended_at: endedAt,
    denial_count: 0,
    later_allowed_count: 0,
    later_denied_count: 0,
    no_later_comparable_proposal_count: 0,
    financial_providers: [],
    denials: [],
  };
  if (status === "UNAVAILABLE") {
    if (
      denialCount !== 0 ||
      laterAllowedCount !== 0 ||
      laterDeniedCount !== 0 ||
      noLaterCount !== 0 ||
      providers.length !== 0 ||
      rawDenials.length !== 0
    )
      return;
    return unavailable;
  }
  if (
    method !== "IMMUTABLE_PAPER_DETERMINISTIC_DENIAL_AND_LATER_ELIGIBILITY" ||
    source.status !== "AVAILABLE" ||
    source.coverage_status !== "COMPLETE" ||
    horizon !== source.horizon_hours ||
    !startedAt ||
    !endedAt ||
    startedAt !== source.window_started_at ||
    endedAt !== source.window_ended_at ||
    Number.isNaN(Date.parse(startedAt)) ||
    Number.isNaN(Date.parse(endedAt)) ||
    denialCount !== source.deny_count ||
    rawDenials.length !== denialCount ||
    laterAllowedCount + laterDeniedCount + noLaterCount !== denialCount
  )
    return;

  const denials: PaperDenialEligibilityFact[] = [];
  const evidenceProviders = new Set<string>();
  for (const raw of rawDenials) {
    const item = record(raw);
    const decisionID = text(
      item,
      "decision_journal_entry_id",
      "DecisionJournalEntryID",
    );
    const actionID = text(item, "proposed_action_id", "ProposedActionID");
    const riskID = text(item, "risk_evaluation_id", "RiskEvaluationID");
    const executionID = text(item, "execution_record_id", "ExecutionRecordID");
    const createdAt = text(item, "created_at", "CreatedAt");
    const symbol = text(item, "symbol", "Symbol");
    const instrument = text(item, "instrument", "Instrument");
    const side = text(item, "side", "Side");
    const quantity = exactDecimal(
      item?.proposed_quantity ?? item?.ProposedQuantity,
    );
    const notional = exactDecimal(
      item?.proposed_notional ?? item?.ProposedNotional,
    );
    const reasons = stringList(
      item?.denial_reason_codes ?? item?.DenialReasonCodes,
    );
    const failed = stringList(
      item?.failed_check_codes ?? item?.FailedCheckCodes,
    );
    const terminal = text(item, "terminal_check_stage", "TerminalCheckStage");
    const provider = text(item, "financial_provider", "FinancialProvider");
    const feed = text(item, "market_feed", "MarketFeed");
    const quality = text(item, "market_quality", "MarketQuality");
    const marketAt = text(item, "market_observed_at", "MarketObservedAt");
    const disposition = text(item, "later_disposition", "LaterDisposition");
    const sourceIndex = source.proposals.findIndex(
      (proposal) => proposal.decision_journal_entry_id === decisionID,
    );
    const sourceDenial = source.proposals[sourceIndex];
    if (
      !decisionID ||
      !actionID ||
      !riskID ||
      !executionID ||
      !createdAt ||
      Number.isNaN(Date.parse(createdAt)) ||
      !symbol ||
      !["EQUITY", "CRYPTO"].includes(instrument ?? "") ||
      !["BUY", "SELL"].includes(side ?? "") ||
      !quantity ||
      compareExactDecimals(quantity, "0") !== 1 ||
      !notional ||
      compareExactDecimals(notional, "0") !== 1 ||
      reasons.length < 1 ||
      reasons.join("|") !== failed.join("|") ||
      !terminal ||
      !provider ||
      !feed ||
      !quality ||
      !marketAt ||
      Number.isNaN(Date.parse(marketAt)) ||
      Date.parse(marketAt) > Date.parse(createdAt) ||
      ![
        "LATER_ALLOWED",
        "LATER_DENIED",
        "NO_LATER_COMPARABLE_PROPOSAL",
      ].includes(disposition ?? "") ||
      !sourceDenial ||
      sourceDenial.risk_decision !== "DENY" ||
      sourceDenial.coverage_status !== "FAIL_CLOSED_PREFIX" ||
      sourceDenial.proposed_action_id !== actionID ||
      sourceDenial.risk_evaluation_id !== riskID ||
      sourceDenial.execution_record_id !== executionID ||
      sourceDenial.created_at !== createdAt ||
      sourceDenial.symbol !== symbol ||
      sourceDenial.instrument !== instrument ||
      sourceDenial.side !== side ||
      sourceDenial.proposed_quantity !== quantity ||
      sourceDenial.proposed_notional !== notional ||
      sourceDenial.denial_reason_codes.join("|") !== reasons.join("|") ||
      sourceDenial.failed_check_codes.join("|") !== failed.join("|") ||
      sourceDenial.terminal_check_stage !== terminal ||
      sourceDenial.financial_provider !== provider ||
      sourceDenial.market_feed !== feed ||
      sourceDenial.market_quality !== quality ||
      sourceDenial.market_observed_at !== marketAt
    )
      return;
    evidenceProviders.add(provider);
    const firstLater = source.proposals
      .slice(sourceIndex + 1)
      .find(
        (proposal) =>
          proposal.symbol === symbol &&
          proposal.instrument === instrument &&
          proposal.side === side,
      );
    const rawChanges = item?.changed_risk_results ?? item?.ChangedRiskResults;
    if (!Array.isArray(rawChanges)) return;
    const changes: PaperDenialRiskResultChange[] = [];
    for (const rawChange of rawChanges) {
      const change = record(rawChange);
      const stage = text(change, "stage", "Stage");
      const previousCode = text(change, "previous_code", "PreviousCode");
      const laterCode = text(change, "later_code", "LaterCode");
      const previousResult = text(change, "previous_result", "PreviousResult");
      const laterResult = text(change, "later_result", "LaterResult");
      if (
        !stage ||
        !previousCode ||
        !laterCode ||
        !["PASS", "FAIL", "WARN"].includes(previousResult ?? "") ||
        !["PASS", "FAIL", "WARN"].includes(laterResult ?? "") ||
        (previousCode === laterCode && previousResult === laterResult)
      )
        return;
      changes.push({
        stage,
        previous_code: previousCode,
        later_code: laterCode,
        previous_result: previousResult as "PASS" | "FAIL" | "WARN",
        later_result: laterResult as "PASS" | "FAIL" | "WARN",
      });
    }
    const expectedChanges: PaperDenialRiskResultChange[] = [];
    if (firstLater) {
      const comparableCheckCount = Math.min(
        sourceDenial.checks.length,
        firstLater.checks.length,
      );
      for (let index = 0; index < comparableCheckCount; index += 1) {
        const previous = sourceDenial.checks[index];
        const later = firstLater.checks[index];
        if (previous.code !== later.code || previous.result !== later.result) {
          const stage = source.expected_check_codes[index];
          if (!stage) return;
          expectedChanges.push({
            stage,
            previous_code: previous.code,
            later_code: later.code,
            previous_result: previous.result,
            later_result: later.result,
          });
        }
      }
    }
    if (JSON.stringify(changes) !== JSON.stringify(expectedChanges)) return;
    const fact: PaperDenialEligibilityFact = {
      decision_journal_entry_id: decisionID,
      proposed_action_id: actionID,
      risk_evaluation_id: riskID,
      execution_record_id: executionID,
      created_at: createdAt,
      symbol,
      instrument: instrument as "EQUITY" | "CRYPTO",
      side: side as "BUY" | "SELL",
      proposed_quantity: quantity,
      proposed_notional: notional,
      denial_reason_codes: reasons,
      failed_check_codes: failed,
      terminal_check_stage: terminal,
      financial_provider: provider,
      market_feed: feed,
      market_quality: quality,
      market_observed_at: marketAt,
      later_disposition:
        disposition as PaperDenialEligibilityFact["later_disposition"],
      changed_risk_results: changes,
    };
    if (disposition === "NO_LATER_COMPARABLE_PROPOSAL") {
      if (firstLater || changes.length !== 0) return;
    } else {
      const laterDecisionID = text(
        item,
        "later_decision_journal_entry_id",
        "LaterDecisionJournalEntryID",
      );
      const laterActionID = text(
        item,
        "later_proposed_action_id",
        "LaterProposedActionID",
      );
      const laterRiskID = text(
        item,
        "later_risk_evaluation_id",
        "LaterRiskEvaluationID",
      );
      const laterExecutionID = text(
        item,
        "later_execution_record_id",
        "LaterExecutionRecordID",
      );
      const laterCreatedAt = text(item, "later_created_at", "LaterCreatedAt");
      const laterQuantity = exactDecimal(
        item?.later_proposed_quantity ?? item?.LaterProposedQuantity,
      );
      const laterNotional = exactDecimal(
        item?.later_proposed_notional ?? item?.LaterProposedNotional,
      );
      const laterRiskDecision = text(
        item,
        "later_risk_decision",
        "LaterRiskDecision",
      );
      const laterExecutionStatus = text(
        item,
        "later_execution_status",
        "LaterExecutionStatus",
      );
      const laterProvider = text(
        item,
        "later_financial_provider",
        "LaterFinancialProvider",
      );
      const laterFeed = text(item, "later_market_feed", "LaterMarketFeed");
      const laterQuality = text(
        item,
        "later_market_quality",
        "LaterMarketQuality",
      );
      const laterMarketAt = text(
        item,
        "later_market_observed_at",
        "LaterMarketObservedAt",
      );
      const elapsed = nonnegativeInteger(
        item?.elapsed_seconds ?? item?.ElapsedSeconds,
      );
      if (
        !firstLater ||
        !laterDecisionID ||
        firstLater.decision_journal_entry_id !== laterDecisionID ||
        firstLater.proposed_action_id !== laterActionID ||
        firstLater.risk_evaluation_id !== laterRiskID ||
        firstLater.execution_record_id !== laterExecutionID ||
        firstLater.created_at !== laterCreatedAt ||
        firstLater.proposed_quantity !== laterQuantity ||
        firstLater.proposed_notional !== laterNotional ||
        firstLater.risk_decision !== laterRiskDecision ||
        firstLater.execution_status !== laterExecutionStatus ||
        firstLater.financial_provider !== laterProvider ||
        firstLater.market_feed !== laterFeed ||
        firstLater.market_quality !== laterQuality ||
        firstLater.market_observed_at !== laterMarketAt ||
        elapsed === undefined ||
        elapsed <= 0 ||
        Math.floor(
          (Date.parse(laterCreatedAt!) - Date.parse(createdAt)) / 1000,
        ) !== elapsed ||
        (disposition === "LATER_ALLOWED"
          ? laterRiskDecision !== "ALLOW" ||
            laterExecutionStatus !== "SIMULATED_FILLED"
          : laterRiskDecision !== "DENY" ||
            laterExecutionStatus !== "RISK_DENIED")
      )
        return;
      evidenceProviders.add(laterProvider!);
      Object.assign(fact, {
        later_decision_journal_entry_id: laterDecisionID,
        later_proposed_action_id: laterActionID,
        later_risk_evaluation_id: laterRiskID,
        later_execution_record_id: laterExecutionID,
        later_created_at: laterCreatedAt,
        later_proposed_quantity: laterQuantity,
        later_proposed_notional: laterNotional,
        later_risk_decision: laterRiskDecision,
        later_execution_status: laterExecutionStatus,
        later_financial_provider: laterProvider,
        later_market_feed: laterFeed,
        later_market_quality: laterQuality,
        later_market_observed_at: laterMarketAt,
        elapsed_seconds: elapsed,
      });
    }
    denials.push(fact);
  }
  const exactProviders = [...evidenceProviders].sort();
  const denialTimes = denials.map((item) => item.created_at).sort();
  if (
    exactProviders.join("|") !== providers.join("|") ||
    (denialCount === 0
      ? firstDenialAt !== undefined || latestDenialAt !== undefined
      : !firstDenialAt ||
        !latestDenialAt ||
        firstDenialAt !== denialTimes[0] ||
        latestDenialAt !== denialTimes[denialTimes.length - 1]) ||
    denials.filter((item) => item.later_disposition === "LATER_ALLOWED")
      .length !== laterAllowedCount ||
    denials.filter((item) => item.later_disposition === "LATER_DENIED")
      .length !== laterDeniedCount ||
    denials.filter(
      (item) => item.later_disposition === "NO_LATER_COMPARABLE_PROPOSAL",
    ).length !== noLaterCount
  )
    return;
  return {
    status: "AVAILABLE",
    calculation_method: method as PaperDenialEligibility["calculation_method"],
    horizon_hours: horizon,
    window_started_at: startedAt,
    window_ended_at: endedAt,
    denial_count: denialCount,
    later_allowed_count: laterAllowedCount,
    later_denied_count: laterDeniedCount,
    no_later_comparable_proposal_count: noLaterCount,
    financial_providers: providers,
    first_denial_at: firstDenialAt,
    latest_denial_at: latestDenialAt,
    denials,
  };
}

function normalizedPaperGuardrailEvidence(
  value: unknown,
): PaperGuardrailEvidence | undefined {
  const evidence = record(value);
  const status = text(evidence, "status", "Status");
  const method = text(evidence, "calculation_method", "CalculationMethod");
  const asOf = text(evidence, "as_of", "AsOf");
  const twentyFour = normalizedPaperGuardrailWindow(
    evidence?.twenty_four_hours ?? evidence?.TwentyFourHours,
    24,
  );
  const sevenDays = normalizedPaperGuardrailWindow(
    evidence?.seven_days ?? evidence?.SevenDays,
    168,
  );
  const coverageChange =
    twentyFour && sevenDays
      ? normalizedPaperGuardrailCoverageChange(
          evidence?.coverage_change ?? evidence?.CoverageChange,
          twentyFour,
          sevenDays,
        )
      : undefined;
  const denialSource =
    sevenDays?.status === "AVAILABLE" ? sevenDays : twentyFour;
  const denialEligibility = denialSource
    ? normalizedPaperDenialEligibility(
        evidence?.denial_eligibility ?? evidence?.DenialEligibility,
        denialSource,
      )
    : undefined;
  if (
    !["AVAILABLE", "UNAVAILABLE"].includes(status ?? "") ||
    !twentyFour ||
    !sevenDays ||
    !coverageChange ||
    !denialEligibility
  )
    return;
  if (
    status === "AVAILABLE" &&
    (method !== "IMMUTABLE_PAPER_PROPOSAL_RISK_AND_SIMULATION_ATTRIBUTION" ||
      !asOf ||
      Number.isNaN(Date.parse(asOf)) ||
      twentyFour.status !== "AVAILABLE")
  )
    return;
  return {
    status: status as "AVAILABLE" | "UNAVAILABLE",
    calculation_method: method as PaperGuardrailEvidence["calculation_method"],
    as_of: asOf,
    twenty_four_hours: twentyFour,
    seven_days: sevenDays,
    coverage_change: coverageChange,
    denial_eligibility: denialEligibility,
  };
}

function normalizedPaperActivityCadence(
  value: unknown,
  expectedFillCount?: number,
): PaperActivityCadence | undefined {
  const cadence = record(value);
  const status = text(cadence, "status", "Status");
  const calculationMethod = text(
    cadence,
    "calculation_method",
    "CalculationMethod",
  );
  const asOf = text(cadence, "as_of", "AsOf");
  const scheduleIntervalMinutes = nonnegativeInteger(
    cadence?.schedule_interval_minutes ?? cadence?.ScheduleIntervalMinutes,
  );
  const twentyFourHours = normalizedPaperActivityWindow(
    cadence?.twenty_four_hours ?? cadence?.TwentyFourHours,
    24,
  );
  const sevenDays = normalizedPaperActivityWindow(
    cadence?.seven_days ?? cadence?.SevenDays,
    168,
  );
  const dispositionFunnel = normalizedPaperDispositionFunnel(
    cadence?.disposition_funnel ?? cadence?.DispositionFunnel,
  );
  const timing = record(cadence?.fill_timing ?? cadence?.FillTiming);
  const timingStatus = text(timing, "status", "Status");
  const timingCoverage = text(
    timing,
    "historical_coverage",
    "HistoricalCoverage",
  );
  const timingFillCount = nonnegativeInteger(
    timing?.fill_count ?? timing?.FillCount,
  );
  const rawSymbols = timing?.symbols ?? timing?.Symbols;
  const timingFirstFillAt = text(timing, "first_fill_at", "FirstFillAt");
  const timingLastFillAt = text(timing, "last_fill_at", "LastFillAt");
  const minimumInterFillSeconds = exactDecimal(
    timing?.minimum_inter_fill_seconds ?? timing?.MinimumInterFillSeconds,
  );
  const medianInterFillSeconds = exactDecimal(
    timing?.median_inter_fill_seconds ?? timing?.MedianInterFillSeconds,
  );
  const maximumInterFillSeconds = exactDecimal(
    timing?.maximum_inter_fill_seconds ?? timing?.MaximumInterFillSeconds,
  );
  const noFill = record(
    cadence?.longest_no_fill_interval ?? cadence?.LongestNoFillInterval,
  );
  const noFillStatus = text(noFill, "status", "Status");
  const noFillCycleCount = nonnegativeInteger(
    noFill?.cycle_count ?? noFill?.CycleCount,
  );
  const noFillIntervalSeconds = exactDecimal(
    noFill?.interval_seconds ?? noFill?.IntervalSeconds,
  );
  const noFillStartedAt = text(
    noFill,
    "scheduled_started_at",
    "ScheduledStartedAt",
  );
  const noFillEndedAt = text(noFill, "completed_ended_at", "CompletedEndedAt");
  if (
    !["AVAILABLE", "UNAVAILABLE"].includes(status ?? "") ||
    scheduleIntervalMinutes === undefined ||
    scheduleIntervalMinutes < 30 ||
    !twentyFourHours ||
    !sevenDays ||
    !dispositionFunnel ||
    ![
      "AVAILABLE",
      "NO_FILLS",
      "INSUFFICIENT_INTERVALS",
      "UNAVAILABLE",
    ].includes(timingStatus ?? "") ||
    timingFillCount === undefined ||
    (expectedFillCount !== undefined &&
      timingFillCount !== expectedFillCount) ||
    !Array.isArray(rawSymbols) ||
    !["AVAILABLE", "NO_FILLS", "UNAVAILABLE"].includes(noFillStatus ?? "") ||
    noFillCycleCount === undefined
  )
    return;
  if (status === "UNAVAILABLE") {
    return {
      status: "UNAVAILABLE",
      schedule_interval_minutes: scheduleIntervalMinutes,
      twenty_four_hours: twentyFourHours,
      seven_days: sevenDays,
      disposition_funnel: dispositionFunnel,
      fill_timing: {
        status: "UNAVAILABLE",
        fill_count: timingFillCount,
        symbols: [],
      },
      longest_no_fill_interval: {
        status: "UNAVAILABLE",
        cycle_count: noFillCycleCount,
      },
    };
  }
  if (
    calculationMethod !== "IMMUTABLE_SCHEDULE_AND_SIMULATION_CHRONOLOGY" ||
    dispositionFunnel.status !== "AVAILABLE" ||
    !asOf ||
    Number.isNaN(Date.parse(asOf)) ||
    timingCoverage !== "COMPLETE_FROM_PORTFOLIO_GENESIS" ||
    timingStatus === "UNAVAILABLE" ||
    (timingFillCount > 0 &&
      (!timingFirstFillAt ||
        !timingLastFillAt ||
        Number.isNaN(Date.parse(timingFirstFillAt)) ||
        Number.isNaN(Date.parse(timingLastFillAt)))) ||
    (timingFillCount > 1 &&
      (!minimumInterFillSeconds ||
        !medianInterFillSeconds ||
        !maximumInterFillSeconds)) ||
    (noFillStatus === "AVAILABLE" &&
      (!noFillIntervalSeconds ||
        noFillCycleCount < 1 ||
        !noFillStartedAt ||
        !noFillEndedAt ||
        Number.isNaN(Date.parse(noFillStartedAt)) ||
        Number.isNaN(Date.parse(noFillEndedAt))))
  )
    return;
  const symbols = [] as PaperActivityCadence["fill_timing"]["symbols"];
  for (const rawSymbol of rawSymbols) {
    const symbol = record(rawSymbol);
    const symbolStatus = text(symbol, "status", "Status");
    const symbolName = text(symbol, "symbol", "Symbol");
    const instrument = text(symbol, "instrument", "Instrument");
    const fillCount = nonnegativeInteger(
      symbol?.fill_count ?? symbol?.FillCount,
    );
    const firstFillAt = text(symbol, "first_fill_at", "FirstFillAt");
    const lastFillAt = text(symbol, "last_fill_at", "LastFillAt");
    const minimum = exactDecimal(
      symbol?.minimum_inter_fill_seconds ?? symbol?.MinimumInterFillSeconds,
    );
    const median = exactDecimal(
      symbol?.median_inter_fill_seconds ?? symbol?.MedianInterFillSeconds,
    );
    const maximum = exactDecimal(
      symbol?.maximum_inter_fill_seconds ?? symbol?.MaximumInterFillSeconds,
    );
    if (
      !["AVAILABLE", "INSUFFICIENT_INTERVALS"].includes(symbolStatus ?? "") ||
      !symbolName ||
      !["EQUITY", "CRYPTO"].includes(instrument ?? "") ||
      fillCount === undefined ||
      fillCount < 1 ||
      !firstFillAt ||
      !lastFillAt ||
      Number.isNaN(Date.parse(firstFillAt)) ||
      Number.isNaN(Date.parse(lastFillAt)) ||
      (symbolStatus === "AVAILABLE" &&
        (fillCount < 2 || !minimum || !median || !maximum)) ||
      (symbolStatus === "INSUFFICIENT_INTERVALS" && fillCount !== 1)
    )
      return;
    symbols.push({
      status: symbolStatus as "AVAILABLE" | "INSUFFICIENT_INTERVALS",
      symbol: symbolName,
      instrument: instrument as "EQUITY" | "CRYPTO",
      fill_count: fillCount,
      first_fill_at: firstFillAt,
      last_fill_at: lastFillAt,
      minimum_inter_fill_seconds: minimum,
      median_inter_fill_seconds: median,
      maximum_inter_fill_seconds: maximum,
    });
  }
  if (
    symbols.reduce((count, symbol) => count + symbol.fill_count, 0) !==
    timingFillCount
  )
    return;
  return {
    status: "AVAILABLE",
    calculation_method: calculationMethod,
    as_of: asOf,
    schedule_interval_minutes: scheduleIntervalMinutes,
    twenty_four_hours: twentyFourHours,
    seven_days: sevenDays,
    disposition_funnel: dispositionFunnel,
    fill_timing: {
      status: timingStatus as PaperActivityCadence["fill_timing"]["status"],
      historical_coverage: timingCoverage,
      fill_count: timingFillCount,
      first_fill_at: timingFirstFillAt,
      last_fill_at: timingLastFillAt,
      minimum_inter_fill_seconds: minimumInterFillSeconds,
      median_inter_fill_seconds: medianInterFillSeconds,
      maximum_inter_fill_seconds: maximumInterFillSeconds,
      symbols,
    },
    longest_no_fill_interval: {
      status: noFillStatus as "AVAILABLE" | "NO_FILLS",
      cycle_count: noFillCycleCount,
      interval_seconds: noFillIntervalSeconds,
      scheduled_started_at: noFillStartedAt,
      completed_ended_at: noFillEndedAt,
    },
  };
}

function normalizedPaperEvidenceThresholdChangeLedger(
  value: unknown,
): PaperAutonomyEvidenceThresholdChangeLedger | undefined {
  const ledger = record(value);
  const status = text(ledger, "status", "Status");
  const method = text(ledger, "calculation_method", "CalculationMethod");
  const strategyInstanceID = text(
    ledger,
    "strategy_instance_id",
    "StrategyInstanceID",
  );
  const executionMode = text(ledger, "execution_mode", "ExecutionMode");
  const sourceRunCount = number(ledger, "source_run_count", "SourceRunCount");
  const checkpointCount = number(ledger, "checkpoint_count", "CheckpointCount");
  const capped = flag(ledger, "capped", "Capped");
  const grantsAuthority = flag(ledger, "grants_authority", "GrantsAuthority");
  const livePromotionAvailable = flag(
    ledger,
    "live_promotion_available",
    "LivePromotionAvailable",
  );
  const rawCheckpoints = ledger?.checkpoints ?? ledger?.Checkpoints;
  if (
    !["AVAILABLE", "UNAVAILABLE"].includes(status ?? "") ||
    method !== "IMMUTABLE_PAPER_EVIDENCE_THRESHOLD_CHANGE_LEDGER" ||
    !strategyInstanceID ||
    executionMode !== "PAPER" ||
    sourceRunCount === undefined ||
    !Number.isInteger(sourceRunCount) ||
    sourceRunCount < 0 ||
    checkpointCount === undefined ||
    !Number.isInteger(checkpointCount) ||
    checkpointCount < 0 ||
    capped === undefined ||
    grantsAuthority !== false ||
    livePromotionAvailable !== false ||
    !Array.isArray(rawCheckpoints)
  )
    return;
  if (status === "UNAVAILABLE") {
    if (checkpointCount !== 0 || rawCheckpoints.length !== 0) return;
    return {
      status,
      calculation_method: method,
      strategy_instance_id: strategyInstanceID,
      execution_mode: "PAPER",
      source_run_count: sourceRunCount,
      checkpoint_count: 0,
      capped,
      checkpoints: [],
      grants_authority: false,
      live_promotion_available: false,
    };
  }
  if (
    checkpointCount === 0 ||
    checkpointCount > 6 ||
    checkpointCount !== rawCheckpoints.length ||
    sourceRunCount < checkpointCount ||
    capped !== sourceRunCount > 6
  )
    return;

  const checkpoints: PaperAutonomyEvidenceThresholdCheckpoint[] = [];
  const seenRunIDs = new Set<string>();
  for (const rawCheckpoint of rawCheckpoints) {
    const checkpoint = record(rawCheckpoint);
    const scheduleRunID = text(checkpoint, "schedule_run_id", "ScheduleRunID");
    const previousScheduleRunID = text(
      checkpoint,
      "previous_schedule_run_id",
      "PreviousScheduleRunID",
    );
    const asOf = text(checkpoint, "as_of", "AsOf");
    const elapsedSeconds = number(
      checkpoint,
      "elapsed_seconds",
      "ElapsedSeconds",
    );
    const remainingSeconds = number(
      checkpoint,
      "remaining_seconds",
      "RemainingSeconds",
    );
    const evidenceWindowHours = number(
      checkpoint,
      "evidence_window_hours",
      "EvidenceWindowHours",
    );
    const decisionCount = number(checkpoint, "decision_count", "DecisionCount");
    const decisionDelta = number(checkpoint, "decision_delta", "DecisionDelta");
    const routeContinuityStatus = text(
      checkpoint,
      "route_continuity_status",
      "RouteContinuityStatus",
    );
    const routeContinuityChange = text(
      checkpoint,
      "route_continuity_change",
      "RouteContinuityChange",
    );
    const inputCoverageStatus = text(
      checkpoint,
      "input_coverage_status",
      "InputCoverageStatus",
    );
    const inputCoverageChange = text(
      checkpoint,
      "input_coverage_change",
      "InputCoverageChange",
    );
    const inputFreshnessStatus = text(
      checkpoint,
      "input_freshness_status",
      "InputFreshnessStatus",
    );
    const schedulerStatus = text(
      checkpoint,
      "scheduler_status",
      "SchedulerStatus",
    );
    const consecutiveFailures = number(
      checkpoint,
      "consecutive_failures",
      "ConsecutiveFailures",
    );
    const schedulerChange = text(
      checkpoint,
      "scheduler_change",
      "SchedulerChange",
    );
    const evidenceStatus = text(
      checkpoint,
      "evidence_status",
      "EvidenceStatus",
    );
    const progressClassification = text(
      checkpoint,
      "progress_classification",
      "ProgressClassification",
    );
    const rawRoutes = checkpoint?.routes ?? checkpoint?.Routes;
    const rawBlockers = checkpoint?.blockers ?? checkpoint?.Blockers;
    const rawAdded =
      checkpoint?.added_blocker_codes ?? checkpoint?.AddedBlockerCodes;
    const rawResolved =
      checkpoint?.resolved_blocker_codes ?? checkpoint?.ResolvedBlockerCodes;
    if (
      !scheduleRunID ||
      seenRunIDs.has(scheduleRunID) ||
      !asOf ||
      Number.isNaN(Date.parse(asOf)) ||
      elapsedSeconds === undefined ||
      !Number.isInteger(elapsedSeconds) ||
      elapsedSeconds < 0 ||
      remainingSeconds === undefined ||
      !Number.isInteger(remainingSeconds) ||
      remainingSeconds < 0 ||
      evidenceWindowHours === undefined ||
      !Number.isInteger(evidenceWindowHours) ||
      evidenceWindowHours < 0 ||
      evidenceWindowHours > 168 ||
      decisionCount === undefined ||
      !Number.isInteger(decisionCount) ||
      decisionCount < 0 ||
      decisionDelta === undefined ||
      !Number.isInteger(decisionDelta) ||
      !["STABLE", "CONTEXT_CHANGED", "REVIEW_REQUIRED", "UNAVAILABLE"].includes(
        routeContinuityStatus ?? "",
      ) ||
      ![
        "BASELINE",
        "UNCHANGED",
        "IMPROVED",
        "REGRESSED",
        "CONTEXT_CHANGED",
      ].includes(routeContinuityChange ?? "") ||
      !["COMPLETE", "REVIEW_REQUIRED", "UNAVAILABLE"].includes(
        inputCoverageStatus ?? "",
      ) ||
      ![
        "BASELINE",
        "UNCHANGED",
        "IMPROVED",
        "REGRESSED",
        "CONTEXT_CHANGED",
      ].includes(inputCoverageChange ?? "") ||
      !["CURRENT_AT_DECISION", "REVIEW_REQUIRED", "UNAVAILABLE"].includes(
        inputFreshnessStatus ?? "",
      ) ||
      !["SUCCEEDED", "FAILED", "SKIPPED"].includes(schedulerStatus ?? "") ||
      consecutiveFailures === undefined ||
      !Number.isInteger(consecutiveFailures) ||
      consecutiveFailures < 0 ||
      ![
        "BASELINE",
        "UNCHANGED",
        "INCIDENT_OPENED",
        "INCIDENT_CONTINUES",
        "RECOVERED",
        "SAFE_WAIT",
        "CONTEXT_CHANGED",
      ].includes(schedulerChange ?? "") ||
      ![
        "COLLECTING_EVIDENCE",
        "EVIDENCE_REVIEWABLE",
        "REVIEW_REQUIRED",
        "UNAVAILABLE",
      ].includes(evidenceStatus ?? "") ||
      ![
        "BASELINE",
        "NORMAL_COLLECTION",
        "HELD",
        "REVIEW_REGRESSION",
        "RECOVERED",
        "CONTEXT_CHANGED",
      ].includes(progressClassification ?? "") ||
      !Array.isArray(rawRoutes) ||
      !Array.isArray(rawBlockers) ||
      !Array.isArray(rawAdded) ||
      !Array.isArray(rawResolved)
    )
      return;
    seenRunIDs.add(scheduleRunID);

    const routes: PaperAutonomyEvidenceGate["routes"] = [];
    let attributedCount = 0;
    let previousRouteKey = "";
    for (const rawRoute of rawRoutes) {
      const route = record(rawRoute);
      const aiProvider = text(route, "ai_provider", "AIProvider");
      const modelID = text(route, "model_id", "ModelID");
      const profile = text(route, "profile", "Profile");
      const financialProvider = text(
        route,
        "financial_provider",
        "FinancialProvider",
      );
      const routeDecisionCount = number(
        route,
        "decision_count",
        "DecisionCount",
      );
      const routeKey = `${aiProvider}\u0000${modelID}\u0000${profile}\u0000${financialProvider}`;
      if (
        !aiProvider ||
        !modelID ||
        !profile ||
        !financialProvider ||
        routeDecisionCount === undefined ||
        !Number.isInteger(routeDecisionCount) ||
        routeDecisionCount <= 0 ||
        (previousRouteKey && routeKey <= previousRouteKey)
      )
        return;
      previousRouteKey = routeKey;
      attributedCount += routeDecisionCount;
      routes.push({
        ai_provider: aiProvider,
        model_id: modelID,
        profile,
        financial_provider: financialProvider,
        decision_count: routeDecisionCount,
      });
    }
    if (
      attributedCount > decisionCount ||
      (inputCoverageStatus === "COMPLETE" &&
        attributedCount !== decisionCount) ||
      (routeContinuityStatus === "STABLE" && routes.length !== 1)
    )
      return;

    const blockers: PaperAutonomyEvidenceBlocker[] = [];
    let previousBlockerCode = "";
    for (const rawBlocker of rawBlockers) {
      const blocker = record(rawBlocker);
      const code = text(blocker, "code", "Code");
      const category = text(blocker, "category", "Category");
      const detail = text(blocker, "detail", "Detail");
      if (
        !code ||
        (previousBlockerCode && code <= previousBlockerCode) ||
        !["COLLECTION", "REVIEW", "UNAVAILABLE"].includes(category ?? "") ||
        !detail
      )
        return;
      previousBlockerCode = code;
      blockers.push({
        code,
        category: category as PaperAutonomyEvidenceBlocker["category"],
        detail,
      });
    }
    const hasUnavailable = blockers.some(
      (blocker) => blocker.category === "UNAVAILABLE",
    );
    const hasReview = blockers.some((blocker) => blocker.category === "REVIEW");
    const hasCollection = blockers.some(
      (blocker) => blocker.category === "COLLECTION",
    );
    if (
      (evidenceStatus === "EVIDENCE_REVIEWABLE" && blockers.length !== 0) ||
      (evidenceStatus === "COLLECTING_EVIDENCE" &&
        (!hasCollection || hasReview || hasUnavailable)) ||
      (evidenceStatus === "REVIEW_REQUIRED" &&
        (!hasReview || hasUnavailable)) ||
      (evidenceStatus === "UNAVAILABLE" && !hasUnavailable)
    )
      return;

    const normalizedCodeList = (raw: unknown[]) => {
      const codes: string[] = [];
      for (const value of raw) {
        if (typeof value !== "string" || !value || codes.includes(value))
          return;
        codes.push(value);
      }
      if ([...codes].sort().some((code, index) => code !== codes[index]))
        return;
      return codes;
    };
    const addedBlockerCodes = normalizedCodeList(rawAdded);
    const resolvedBlockerCodes = normalizedCodeList(rawResolved);
    if (
      !addedBlockerCodes ||
      !resolvedBlockerCodes ||
      addedBlockerCodes.some((code) => resolvedBlockerCodes.includes(code))
    )
      return;

    const prior = checkpoints.at(-1);
    if (
      (!prior && !capped && previousScheduleRunID) ||
      (prior && previousScheduleRunID !== prior.schedule_run_id) ||
      (!prior && decisionDelta !== 0) ||
      (prior && decisionDelta !== decisionCount - prior.decision_count) ||
      (prior && Date.parse(asOf) <= Date.parse(prior.as_of)) ||
      (prior && elapsedSeconds <= prior.elapsed_seconds) ||
      (prior && remainingSeconds > prior.remaining_seconds)
    )
      return;
    if (prior) {
      const previousCodes = new Set(
        prior.blockers.map((blocker) => blocker.code),
      );
      const currentCodes = new Set(blockers.map((blocker) => blocker.code));
      const exactAdded = [...currentCodes]
        .filter((code) => !previousCodes.has(code))
        .sort();
      const exactResolved = [...previousCodes]
        .filter((code) => !currentCodes.has(code))
        .sort();
      if (
        exactAdded.join("\u0000") !== addedBlockerCodes.join("\u0000") ||
        exactResolved.join("\u0000") !== resolvedBlockerCodes.join("\u0000")
      )
        return;
    }

    checkpoints.push({
      schedule_run_id: scheduleRunID,
      previous_schedule_run_id: previousScheduleRunID,
      as_of: asOf,
      elapsed_seconds: elapsedSeconds,
      remaining_seconds: remainingSeconds,
      evidence_window_hours: evidenceWindowHours,
      decision_count: decisionCount,
      decision_delta: decisionDelta,
      route_continuity_status:
        routeContinuityStatus as PaperAutonomyEvidenceThresholdCheckpoint["route_continuity_status"],
      route_continuity_change:
        routeContinuityChange as PaperAutonomyEvidenceThresholdCheckpoint["route_continuity_change"],
      input_coverage_status:
        inputCoverageStatus as PaperAutonomyEvidenceThresholdCheckpoint["input_coverage_status"],
      input_coverage_change:
        inputCoverageChange as PaperAutonomyEvidenceThresholdCheckpoint["input_coverage_change"],
      input_freshness_status:
        inputFreshnessStatus as PaperAutonomyEvidenceThresholdCheckpoint["input_freshness_status"],
      scheduler_status:
        schedulerStatus as PaperAutonomyEvidenceThresholdCheckpoint["scheduler_status"],
      consecutive_failures: consecutiveFailures,
      scheduler_change:
        schedulerChange as PaperAutonomyEvidenceThresholdCheckpoint["scheduler_change"],
      evidence_status:
        evidenceStatus as PaperAutonomyEvidenceThresholdCheckpoint["evidence_status"],
      progress_classification:
        progressClassification as PaperAutonomyEvidenceThresholdCheckpoint["progress_classification"],
      routes,
      blockers,
      added_blocker_codes: addedBlockerCodes,
      resolved_blocker_codes: resolvedBlockerCodes,
    });
  }

  return {
    status: "AVAILABLE",
    calculation_method: method,
    strategy_instance_id: strategyInstanceID,
    execution_mode: "PAPER",
    source_run_count: sourceRunCount,
    checkpoint_count: checkpointCount,
    capped,
    checkpoints,
    grants_authority: false,
    live_promotion_available: false,
  };
}

function normalizedPaperEvidenceReadiness(
  value: unknown,
): PaperAutonomyEvidenceGate | undefined {
  const gate = record(value);
  const status = text(gate, "status", "Status");
  const method = text(gate, "calculation_method", "CalculationMethod");
  const reviewScope = text(gate, "review_scope", "ReviewScope");
  const boundary = text(gate, "execution_boundary", "ExecutionBoundary");
  const asOf = text(gate, "as_of", "AsOf");
  const firstDecisionAt = text(gate, "first_decision_at", "FirstDecisionAt");
  const latestDecisionAt = text(gate, "latest_decision_at", "LatestDecisionAt");
  const count = (key: string, legacy: string) => {
    const value = number(gate, key, legacy);
    return value !== undefined && Number.isInteger(value) && value >= 0
      ? value
      : undefined;
  };
  const minimumDecisionCount = count(
    "minimum_decision_count",
    "MinimumDecisionCount",
  );
  const minimumWindowHours = count(
    "minimum_evidence_window_hours",
    "MinimumEvidenceWindowHours",
  );
  const evidenceWindowHours = count(
    "evidence_window_hours",
    "EvidenceWindowHours",
  );
  const decisionCount = count("decision_count", "DecisionCount");
  const abstentionCount = count("abstention_count", "AbstentionCount");
  const proposalCount = count("proposal_count", "ProposalCount");
  const denyCount = count("deterministic_deny_count", "DeterministicDenyCount");
  const fillCount = count("simulated_fill_count", "SimulatedFillCount");
  const failures = count(
    "consecutive_schedule_failures",
    "ConsecutiveScheduleFailures",
  );
  const attributed = count(
    "attributed_decision_count",
    "AttributedDecisionCount",
  );
  const telemetry = count("telemetry_complete_count", "TelemetryCompleteCount");
  const boundedMemory = count("bounded_memory_count", "BoundedMemoryCount");
  const rawRoutes = gate?.routes ?? gate?.Routes;
  const rawBlockers = gate?.blockers ?? gate?.Blockers;
  const safety = record(gate?.safety ?? gate?.Safety);
  const reviewPacket = record(gate?.review_packet ?? gate?.ReviewPacket);
  const thresholdChangeLedger = normalizedPaperEvidenceThresholdChangeLedger(
    reviewPacket?.threshold_change_ledger ??
      reviewPacket?.ThresholdChangeLedger,
  );
  const ledgerReconciled = flag(
    gate,
    "ledger_contracts_reconciled",
    "LedgerContractsReconciled",
  );
  const liveExecutionAvailable = flag(
    gate,
    "live_execution_available",
    "LiveExecutionAvailable",
  );
  if (
    ![
      "COLLECTING_EVIDENCE",
      "EVIDENCE_REVIEWABLE",
      "REVIEW_REQUIRED",
      "UNAVAILABLE",
    ].includes(status ?? "") ||
    method !== "IMMUTABLE_PAPER_AUTONOMY_EVIDENCE_READINESS_GATE" ||
    reviewScope !== "OWNER_REVIEW_EVIDENCE_ONLY" ||
    boundary !== "PAPER_SIMULATION_ONLY" ||
    minimumDecisionCount !== 20 ||
    minimumWindowHours !== 168 ||
    evidenceWindowHours === undefined ||
    decisionCount === undefined ||
    abstentionCount === undefined ||
    proposalCount === undefined ||
    denyCount === undefined ||
    fillCount === undefined ||
    failures === undefined ||
    attributed === undefined ||
    telemetry === undefined ||
    boundedMemory === undefined ||
    !thresholdChangeLedger ||
    ledgerReconciled === undefined ||
    liveExecutionAvailable !== false ||
    !Array.isArray(rawRoutes) ||
    !Array.isArray(rawBlockers) ||
    decisionCount !== abstentionCount + proposalCount ||
    denyCount + fillCount > proposalCount ||
    attributed > decisionCount ||
    telemetry > decisionCount ||
    boundedMemory > decisionCount ||
    (asOf && Number.isNaN(Date.parse(asOf))) ||
    (firstDecisionAt && Number.isNaN(Date.parse(firstDecisionAt))) ||
    (latestDecisionAt && Number.isNaN(Date.parse(latestDecisionAt)))
  )
    return;

  const routes: PaperAutonomyEvidenceGate["routes"] = [];
  let routeDecisionCount = 0;
  for (const rawRoute of rawRoutes) {
    const route = record(rawRoute);
    const routeCount = number(route, "decision_count", "DecisionCount");
    const aiProvider = text(route, "ai_provider", "AIProvider");
    const modelID = text(route, "model_id", "ModelID");
    const profile = text(route, "profile", "Profile");
    const financialProvider = text(
      route,
      "financial_provider",
      "FinancialProvider",
    );
    if (
      !aiProvider ||
      !modelID ||
      !profile ||
      !financialProvider ||
      routeCount === undefined ||
      !Number.isInteger(routeCount) ||
      routeCount <= 0
    )
      return;
    routeDecisionCount += routeCount;
    routes.push({
      ai_provider: aiProvider,
      model_id: modelID,
      profile,
      financial_provider: financialProvider,
      decision_count: routeCount,
    });
  }
  if (routeDecisionCount !== attributed) return;

  const blockers: PaperAutonomyEvidenceGate["blockers"] = [];
  for (const rawBlocker of rawBlockers) {
    const blocker = record(rawBlocker);
    const code = text(blocker, "code", "Code");
    const category = text(blocker, "category", "Category");
    const detail = text(blocker, "detail", "Detail");
    if (
      !code ||
      !["COLLECTION", "REVIEW", "UNAVAILABLE"].includes(category ?? "") ||
      !detail
    )
      return;
    blockers.push({
      code,
      category:
        category as PaperAutonomyEvidenceGate["blockers"][number]["category"],
      detail,
    });
  }
  const hasUnavailable = blockers.some(
    (blocker) => blocker.category === "UNAVAILABLE",
  );
  const hasReview = blockers.some((blocker) => blocker.category === "REVIEW");
  const hasCollection = blockers.some(
    (blocker) => blocker.category === "COLLECTION",
  );
  if (
    (status === "EVIDENCE_REVIEWABLE" && blockers.length !== 0) ||
    (status === "COLLECTING_EVIDENCE" &&
      (!hasCollection || hasReview || hasUnavailable)) ||
    (status === "REVIEW_REQUIRED" && (!hasReview || hasUnavailable)) ||
    (status === "UNAVAILABLE" && !hasUnavailable)
  )
    return;

  const safetyCount = (key: string, legacy: string) => {
    const value = number(safety, key, legacy);
    return value !== undefined && Number.isInteger(value) && value >= 0
      ? value
      : undefined;
  };
  const safetyStatus = text(safety, "status", "Status");
  const liveMandates = safetyCount("live_mandate_count", "LiveMandateCount");
  const aiOrderIntents = safetyCount(
    "ai_order_intent_count",
    "AIOrderIntentCount",
  );
  const invalidStrategyModes = safetyCount(
    "invalid_strategy_mode_count",
    "InvalidStrategyModeCount",
  );
  const invalidExecutionModes = safetyCount(
    "invalid_execution_mode_count",
    "InvalidExecutionModeCount",
  );
  const executableRisks = safetyCount(
    "platform_executable_risk_count",
    "PlatformExecutableRiskCount",
  );
  const nonSimulationFills = safetyCount(
    "non_simulation_fill_count",
    "NonSimulationFillCount",
  );
  if (
    !["CLEAR", "REVIEW_REQUIRED"].includes(safetyStatus ?? "") ||
    liveMandates === undefined ||
    aiOrderIntents === undefined ||
    invalidStrategyModes === undefined ||
    invalidExecutionModes === undefined ||
    executableRisks === undefined ||
    nonSimulationFills === undefined
  )
    return;
  const safetyTotal =
    liveMandates +
    aiOrderIntents +
    invalidStrategyModes +
    invalidExecutionModes +
    executableRisks +
    nonSimulationFills;
  if ((safetyStatus === "CLEAR") !== (safetyTotal === 0)) return;

  const packetCount = (key: string, legacy: string) => {
    const value = number(reviewPacket, key, legacy);
    return value !== undefined && Number.isInteger(value) && value >= 0
      ? value
      : undefined;
  };
  const packetStatus = text(reviewPacket, "status", "Status");
  const packetMethod = text(
    reviewPacket,
    "calculation_method",
    "CalculationMethod",
  );
  const evidenceStartedAt = text(
    reviewPacket,
    "evidence_started_at",
    "EvidenceStartedAt",
  );
  const evidenceEligibleAt = text(
    reviewPacket,
    "evidence_eligible_at",
    "EvidenceEligibleAt",
  );
  const packetAsOf = text(reviewPacket, "as_of", "AsOf");
  const elapsedSeconds = packetCount("elapsed_seconds", "ElapsedSeconds");
  const remainingSeconds = packetCount("remaining_seconds", "RemainingSeconds");
  const schedulerSampleCount = packetCount(
    "scheduler_sample_count",
    "SchedulerSampleCount",
  );
  const schedulerSuccessCount = packetCount(
    "scheduler_success_count",
    "SchedulerSuccessCount",
  );
  const schedulerFailureCount = packetCount(
    "scheduler_failure_count",
    "SchedulerFailureCount",
  );
  const schedulerSafeWaitCount = packetCount(
    "scheduler_safe_wait_count",
    "SchedulerSafeWaitCount",
  );
  const freshnessThresholdSeconds = packetCount(
    "freshness_threshold_seconds",
    "FreshnessThresholdSeconds",
  );
  const marketObservationCount = packetCount(
    "market_observation_count",
    "MarketObservationCount",
  );
  const freshMarketDecisionCount = packetCount(
    "fresh_market_decision_count",
    "FreshMarketDecisionCount",
  );
  const maximumMarketAgeSeconds = packetCount(
    "maximum_market_age_seconds",
    "MaximumMarketAgeSeconds",
  );
  const firstMarketObservedAt = text(
    reviewPacket,
    "first_market_observed_at",
    "FirstMarketObservedAt",
  );
  const latestMarketObservedAt = text(
    reviewPacket,
    "latest_market_observed_at",
    "LatestMarketObservedAt",
  );
  const routeContinuityStatus = text(
    reviewPacket,
    "route_continuity_status",
    "RouteContinuityStatus",
  );
  const inputCoverageStatus = text(
    reviewPacket,
    "input_coverage_status",
    "InputCoverageStatus",
  );
  const inputFreshnessStatus = text(
    reviewPacket,
    "input_freshness_status",
    "InputFreshnessStatus",
  );
  const ledgerContractStatus = text(
    reviewPacket,
    "ledger_contract_status",
    "LedgerContractStatus",
  );
  const noLiveSafetyStatus = text(
    reviewPacket,
    "no_live_safety_status",
    "NoLiveSafetyStatus",
  );
  const evidenceReadyForHumanReview = flag(
    reviewPacket,
    "evidence_ready_for_human_review",
    "EvidenceReadyForHumanReview",
  );
  const ownerGuidance = text(reviewPacket, "owner_guidance", "OwnerGuidance");
  const grantsAuthority = flag(
    reviewPacket,
    "grants_authority",
    "GrantsAuthority",
  );
  const livePromotionAvailable = flag(
    reviewPacket,
    "live_promotion_available",
    "LivePromotionAvailable",
  );
  const startedMillis = evidenceStartedAt
    ? Date.parse(evidenceStartedAt)
    : Number.NaN;
  const eligibleMillis = evidenceEligibleAt
    ? Date.parse(evidenceEligibleAt)
    : Number.NaN;
  const asOfMillis = packetAsOf ? Date.parse(packetAsOf) : Number.NaN;
  if (
    packetStatus !== status ||
    packetMethod !== "IMMUTABLE_PAPER_AUTONOMY_EVIDENCE_REVIEW_PACKET" ||
    !evidenceStartedAt ||
    !evidenceEligibleAt ||
    !packetAsOf ||
    Number.isNaN(startedMillis) ||
    Number.isNaN(eligibleMillis) ||
    Number.isNaN(asOfMillis) ||
    packetAsOf !== asOf ||
    Math.floor((eligibleMillis - startedMillis) / 1000) !== 168 * 60 * 60 ||
    elapsedSeconds !== Math.floor((asOfMillis - startedMillis) / 1000) ||
    remainingSeconds !==
      Math.max(0, Math.floor((eligibleMillis - asOfMillis) / 1000)) ||
    schedulerSampleCount === undefined ||
    schedulerSuccessCount === undefined ||
    schedulerFailureCount === undefined ||
    schedulerSafeWaitCount === undefined ||
    schedulerSampleCount !==
      schedulerSuccessCount + schedulerFailureCount + schedulerSafeWaitCount ||
    !["STABLE", "CONTEXT_CHANGED", "REVIEW_REQUIRED", "UNAVAILABLE"].includes(
      routeContinuityStatus ?? "",
    ) ||
    !["COMPLETE", "REVIEW_REQUIRED", "UNAVAILABLE"].includes(
      inputCoverageStatus ?? "",
    ) ||
    !["CURRENT_AT_DECISION", "REVIEW_REQUIRED", "UNAVAILABLE"].includes(
      inputFreshnessStatus ?? "",
    ) ||
    freshnessThresholdSeconds !== 300 ||
    marketObservationCount === undefined ||
    freshMarketDecisionCount === undefined ||
    freshMarketDecisionCount > decisionCount ||
    maximumMarketAgeSeconds === undefined ||
    !firstMarketObservedAt ||
    !latestMarketObservedAt ||
    Number.isNaN(Date.parse(firstMarketObservedAt)) ||
    Number.isNaN(Date.parse(latestMarketObservedAt)) ||
    !["RECONCILED", "REVIEW_REQUIRED", "UNAVAILABLE"].includes(
      ledgerContractStatus ?? "",
    ) ||
    noLiveSafetyStatus !== safetyStatus ||
    evidenceReadyForHumanReview !== (status === "EVIDENCE_REVIEWABLE") ||
    !ownerGuidance ||
    grantsAuthority !== false ||
    livePromotionAvailable !== false ||
    ledgerReconciled !== (ledgerContractStatus === "RECONCILED") ||
    (attributed === decisionCount) !== (inputCoverageStatus === "COMPLETE") ||
    (freshMarketDecisionCount === decisionCount) !==
      (inputFreshnessStatus === "CURRENT_AT_DECISION")
  )
    return;

  return {
    status: status as PaperAutonomyEvidenceGate["status"],
    calculation_method: method,
    as_of: asOf,
    review_scope: reviewScope,
    execution_boundary: boundary,
    minimum_decision_count: minimumDecisionCount,
    minimum_evidence_window_hours: minimumWindowHours,
    evidence_window_hours: evidenceWindowHours,
    decision_count: decisionCount,
    abstention_count: abstentionCount,
    proposal_count: proposalCount,
    deterministic_deny_count: denyCount,
    simulated_fill_count: fillCount,
    first_decision_at: firstDecisionAt,
    latest_decision_at: latestDecisionAt,
    last_schedule_status: text(
      gate,
      "last_schedule_status",
      "LastScheduleStatus",
    ),
    consecutive_schedule_failures: failures,
    attributed_decision_count: attributed,
    telemetry_complete_count: telemetry,
    bounded_memory_count: boundedMemory,
    routes,
    ledger_contracts_reconciled: ledgerReconciled,
    safety: {
      status: safetyStatus as PaperAutonomyEvidenceGate["safety"]["status"],
      live_mandate_count: liveMandates,
      ai_order_intent_count: aiOrderIntents,
      invalid_strategy_mode_count: invalidStrategyModes,
      invalid_execution_mode_count: invalidExecutionModes,
      platform_executable_risk_count: executableRisks,
      non_simulation_fill_count: nonSimulationFills,
    },
    review_packet: {
      status:
        packetStatus as PaperAutonomyEvidenceGate["review_packet"]["status"],
      calculation_method: packetMethod,
      evidence_started_at: evidenceStartedAt,
      evidence_eligible_at: evidenceEligibleAt,
      as_of: packetAsOf,
      elapsed_seconds: elapsedSeconds,
      remaining_seconds: remainingSeconds,
      scheduler_sample_count: schedulerSampleCount,
      scheduler_success_count: schedulerSuccessCount,
      scheduler_failure_count: schedulerFailureCount,
      scheduler_safe_wait_count: schedulerSafeWaitCount,
      route_continuity_status:
        routeContinuityStatus as PaperAutonomyEvidenceGate["review_packet"]["route_continuity_status"],
      input_coverage_status:
        inputCoverageStatus as PaperAutonomyEvidenceGate["review_packet"]["input_coverage_status"],
      input_freshness_status:
        inputFreshnessStatus as PaperAutonomyEvidenceGate["review_packet"]["input_freshness_status"],
      freshness_threshold_seconds: freshnessThresholdSeconds,
      market_observation_count: marketObservationCount,
      fresh_market_decision_count: freshMarketDecisionCount,
      maximum_market_age_seconds: maximumMarketAgeSeconds,
      first_market_observed_at: firstMarketObservedAt,
      latest_market_observed_at: latestMarketObservedAt,
      ledger_contract_status:
        ledgerContractStatus as PaperAutonomyEvidenceGate["review_packet"]["ledger_contract_status"],
      no_live_safety_status:
        noLiveSafetyStatus as PaperAutonomyEvidenceGate["review_packet"]["no_live_safety_status"],
      evidence_ready_for_human_review: evidenceReadyForHumanReview,
      owner_guidance: ownerGuidance,
      grants_authority: false,
      live_promotion_available: false,
      threshold_change_ledger: thresholdChangeLedger,
    },
    blockers,
    live_execution_available: false,
  };
}

function normalizedPaperEvidenceReview(
  value: unknown,
): PaperEvidenceReview | undefined {
  const review = record(value);
  if (!review) return;
  const id = text(review, "id", "ID");
  const strategyInstanceID = text(
    review,
    "strategy_instance_id",
    "StrategyInstanceID",
  );
  const financialAccountID = text(
    review,
    "financial_account_id",
    "FinancialAccountID",
  );
  const mandateID = text(review, "mandate_id", "MandateID");
  const mandateVersion = number(review, "mandate_version", "MandateVersion");
  const fingerprint = text(
    review,
    "evidence_fingerprint",
    "EvidenceFingerprint",
  );
  const gateStatus = text(review, "gate_status", "GateStatus");
  const evidenceStartedAt = text(
    review,
    "evidence_started_at",
    "EvidenceStartedAt",
  );
  const evidenceEligibleAt = text(
    review,
    "evidence_eligible_at",
    "EvidenceEligibleAt",
  );
  const evidenceAsOf = text(review, "evidence_as_of", "EvidenceAsOf");
  const evidenceWindowHours = number(
    review,
    "evidence_window_hours",
    "EvidenceWindowHours",
  );
  const decisionCount = number(review, "decision_count", "DecisionCount");
  const portfolioVersion = number(
    review,
    "portfolio_version",
    "PortfolioVersion",
  );
  const portfolioUpdatedAt = text(
    review,
    "portfolio_updated_at",
    "PortfolioUpdatedAt",
  );
  const latestCheckpointRunID = text(
    review,
    "latest_checkpoint_run_id",
    "LatestCheckpointRunID",
  );
  const latestCheckpointAsOf = text(
    review,
    "latest_checkpoint_as_of",
    "LatestCheckpointAsOf",
  );
  const schedulerSampleCount = number(
    review,
    "scheduler_sample_count",
    "SchedulerSampleCount",
  );
  const schedulerSuccessCount = number(
    review,
    "scheduler_success_count",
    "SchedulerSuccessCount",
  );
  const schedulerFailureCount = number(
    review,
    "scheduler_failure_count",
    "SchedulerFailureCount",
  );
  const lastScheduleStatus = text(
    review,
    "last_schedule_status",
    "LastScheduleStatus",
  );
  const consecutiveFailures = number(
    review,
    "consecutive_schedule_failures",
    "ConsecutiveScheduleFailures",
  );
  const routeContinuityStatus = text(
    review,
    "route_continuity_status",
    "RouteContinuityStatus",
  );
  const inputCoverageStatus = text(
    review,
    "input_coverage_status",
    "InputCoverageStatus",
  );
  const inputFreshnessStatus = text(
    review,
    "input_freshness_status",
    "InputFreshnessStatus",
  );
  const ledgerContractStatus = text(
    review,
    "ledger_contract_status",
    "LedgerContractStatus",
  );
  const noLiveSafetyStatus = text(
    review,
    "no_live_safety_status",
    "NoLiveSafetyStatus",
  );
  const executionBoundary = text(
    review,
    "execution_boundary",
    "ExecutionBoundary",
  );
  const reviewScope = text(review, "review_scope", "ReviewScope");
  const grantsAuthority = flag(review, "grants_authority", "GrantsAuthority");
  const livePromotionAvailable = flag(
    review,
    "live_promotion_available",
    "LivePromotionAvailable",
  );
  const mfaMethod = text(review, "mfa_method", "MFAMethod");
  const reviewedAt = text(review, "reviewed_at", "ReviewedAt");
  const createdAt = text(review, "created_at", "CreatedAt");
  const timestamps = [
    evidenceStartedAt,
    evidenceEligibleAt,
    evidenceAsOf,
    portfolioUpdatedAt,
    latestCheckpointAsOf,
    reviewedAt,
    createdAt,
  ];
  const counts = [
    mandateVersion,
    evidenceWindowHours,
    decisionCount,
    portfolioVersion,
    schedulerSampleCount,
    schedulerSuccessCount,
    schedulerFailureCount,
    consecutiveFailures,
  ];
  if (
    !id ||
    !strategyInstanceID ||
    !financialAccountID ||
    !mandateID ||
    !latestCheckpointRunID ||
    !fingerprint ||
    !/^[0-9a-f]{64}$/.test(fingerprint) ||
    gateStatus !== "EVIDENCE_REVIEWABLE" ||
    timestamps.some(
      (timestamp) => !timestamp || Number.isNaN(Date.parse(timestamp)),
    ) ||
    counts.some(
      (count) =>
        count === undefined || !Number.isSafeInteger(count) || count < 0,
    ) ||
    mandateVersion! < 1 ||
    evidenceWindowHours! < 168 ||
    decisionCount! < 20 ||
    portfolioVersion! < 1 ||
    schedulerSampleCount! < 20 ||
    schedulerSuccessCount! + schedulerFailureCount! > schedulerSampleCount! ||
    lastScheduleStatus !== "SUCCEEDED" ||
    consecutiveFailures !== 0 ||
    !["STABLE", "CONTEXT_CHANGED"].includes(routeContinuityStatus ?? "") ||
    inputCoverageStatus !== "COMPLETE" ||
    inputFreshnessStatus !== "CURRENT_AT_DECISION" ||
    ledgerContractStatus !== "RECONCILED" ||
    noLiveSafetyStatus !== "CLEAR" ||
    executionBoundary !== "PAPER_SIMULATION_ONLY" ||
    reviewScope !== "PAPER_NON_LIVE_EVIDENCE_ONLY" ||
    grantsAuthority !== false ||
    livePromotionAvailable !== false ||
    mfaMethod !== "totp" ||
    Date.parse(evidenceEligibleAt!) - Date.parse(evidenceStartedAt!) !==
      168 * 60 * 60 * 1000 ||
    evidenceAsOf !== latestCheckpointAsOf ||
    Date.parse(reviewedAt!) < Date.parse(evidenceAsOf!) ||
    Date.parse(createdAt!) < Date.parse(reviewedAt!)
  )
    return;
  return {
    id,
    strategy_instance_id: strategyInstanceID,
    financial_account_id: financialAccountID,
    mandate_id: mandateID,
    mandate_version: mandateVersion!,
    evidence_fingerprint: fingerprint,
    gate_status: "EVIDENCE_REVIEWABLE",
    evidence_started_at: evidenceStartedAt!,
    evidence_eligible_at: evidenceEligibleAt!,
    evidence_as_of: evidenceAsOf!,
    evidence_window_hours: evidenceWindowHours!,
    decision_count: decisionCount!,
    portfolio_version: portfolioVersion!,
    portfolio_updated_at: portfolioUpdatedAt!,
    latest_checkpoint_run_id: latestCheckpointRunID!,
    latest_checkpoint_as_of: latestCheckpointAsOf!,
    scheduler_sample_count: schedulerSampleCount!,
    scheduler_success_count: schedulerSuccessCount!,
    scheduler_failure_count: schedulerFailureCount!,
    last_schedule_status: "SUCCEEDED",
    consecutive_schedule_failures: 0,
    route_continuity_status: routeContinuityStatus as
      | "STABLE"
      | "CONTEXT_CHANGED",
    input_coverage_status: "COMPLETE",
    input_freshness_status: "CURRENT_AT_DECISION",
    ledger_contract_status: "RECONCILED",
    no_live_safety_status: "CLEAR",
    execution_boundary: "PAPER_SIMULATION_ONLY",
    review_scope: "PAPER_NON_LIVE_EVIDENCE_ONLY",
    grants_authority: false,
    live_promotion_available: false,
    mfa_method: "totp",
    reviewed_at: reviewedAt!,
    created_at: createdAt!,
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
  const activityCadence = normalizedPaperActivityCadence(
    portfolio?.activity_cadence ?? portfolio?.ActivityCadence,
    executionCosts?.fill_count,
  );
  const guardrailEvidence = normalizedPaperGuardrailEvidence(
    portfolio?.guardrail_evidence ?? portfolio?.GuardrailEvidence,
  );
  const evidenceReadiness = normalizedPaperEvidenceReadiness(
    portfolio?.evidence_readiness ?? portfolio?.EvidenceReadiness,
  );
  const rawEvidenceReviewFingerprint = text(
    portfolio,
    "evidence_review_fingerprint",
    "EvidenceReviewFingerprint",
  );
  const evidenceReviewFingerprint = rawEvidenceReviewFingerprint || undefined;
  const rawLatestEvidenceReview =
    portfolio?.latest_evidence_review ?? portfolio?.LatestEvidenceReview;
  const latestEvidenceReview = normalizedPaperEvidenceReview(
    rawLatestEvidenceReview,
  );
  const currentEvidenceReviewed = flag(
    portfolio,
    "current_evidence_reviewed",
    "CurrentEvidenceReviewed",
  );
  const latestReviewWasSupplied = rawLatestEvidenceReview != null;
  const fingerprintValid =
    evidenceReviewFingerprint === undefined ||
    /^[0-9a-f]{64}$/.test(evidenceReviewFingerprint);
  const reviewContractValid =
    currentEvidenceReviewed !== undefined &&
    fingerprintValid &&
    (!latestReviewWasSupplied || Boolean(latestEvidenceReview)) &&
    (evidenceReadiness?.status !== "EVIDENCE_REVIEWABLE" ||
      Boolean(evidenceReviewFingerprint)) &&
    (!currentEvidenceReviewed ||
      Boolean(
        latestEvidenceReview &&
          evidenceReviewFingerprint &&
          latestEvidenceReview.evidence_fingerprint ===
            evidenceReviewFingerprint,
      )) &&
    !(
      !currentEvidenceReviewed &&
      latestEvidenceReview &&
      evidenceReviewFingerprint &&
      latestEvidenceReview.evidence_fingerprint === evidenceReviewFingerprint
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
    (evidenceReadiness &&
      evidenceReadiness.review_packet.threshold_change_ledger
        .strategy_instance_id !== strategyInstanceID) ||
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
    activity_cadence: activityCadence,
    guardrail_evidence: guardrailEvidence,
    evidence_readiness: evidenceReadiness,
    evidence_review_fingerprint: reviewContractValid
      ? evidenceReviewFingerprint
      : undefined,
    latest_evidence_review: reviewContractValid
      ? latestEvidenceReview
      : undefined,
    current_evidence_reviewed: reviewContractValid
      ? currentEvidenceReviewed
      : undefined,
    evidence_review_contract_valid: reviewContractValid,
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
  const paperActivityCadenceContractAvailable =
    runtimeExecutionMode === "PAPER" &&
    paperPortfolioResult.payload?.activity_cadence_semantics ===
      "EXACT_IMMUTABLE_SCHEDULE_AND_SIMULATION_CHRONOLOGY" &&
    paperPortfolioResult.payload?.activity_cadence_read_only === true &&
    paperPortfolioResult.payload?.disposition_funnel_semantics ===
      "EXACT_IMMUTABLE_PAPER_EVALUATION_DISPOSITION_FUNNEL" &&
    paperPortfolioResult.payload?.disposition_funnel_read_only === true &&
    paperPortfolioResult.payload?.broker_action_available === false &&
    paperPortfolioResult.payload?.live_execution_available === false &&
    Boolean(paperPortfolio?.activity_cadence);
  const paperGuardrailEvidenceContractAvailable =
    runtimeExecutionMode === "PAPER" &&
    paperPortfolioResult.payload?.guardrail_evidence_semantics ===
      "EXACT_IMMUTABLE_PAPER_PROPOSAL_RISK_AND_SIMULATION_ATTRIBUTION" &&
    paperPortfolioResult.payload?.guardrail_evidence_read_only === true &&
    paperPortfolioResult.payload?.guardrail_coverage_semantics ===
      "EXACT_ORDERED_PAPER_CHECK_PLAN_AND_FAIL_CLOSED_PREFIX_ATTESTATION" &&
    paperPortfolioResult.payload?.guardrail_coverage_read_only === true &&
    paperPortfolioResult.payload?.guardrail_coverage_change_semantics ===
      "EXACT_IMMUTABLE_24_HOUR_AND_SEVEN_DAY_COVERAGE_COMPARISON" &&
    paperPortfolioResult.payload?.guardrail_coverage_change_read_only ===
      true &&
    paperPortfolioResult.payload?.denial_eligibility_semantics ===
      "EXACT_IMMUTABLE_PAPER_DETERMINISTIC_DENIAL_AND_LATER_ELIGIBILITY" &&
    paperPortfolioResult.payload?.denial_eligibility_read_only === true &&
    paperPortfolioResult.payload?.broker_action_available === false &&
    paperPortfolioResult.payload?.live_execution_available === false &&
    Boolean(paperPortfolio?.guardrail_evidence);
  const paperEvidenceReadinessContractAvailable =
    runtimeExecutionMode === "PAPER" &&
    paperPortfolioResult.payload?.paper_evidence_readiness_semantics ===
      "IMMUTABLE_PAPER_AUTONOMY_EVIDENCE_READINESS_GATE" &&
    paperPortfolioResult.payload?.paper_evidence_readiness_read_only === true &&
    paperPortfolioResult.payload?.paper_evidence_readiness_grants_authority ===
      false &&
    paperPortfolioResult.payload?.broker_action_available === false &&
    paperPortfolioResult.payload?.live_promotion_available === false &&
    paperPortfolioResult.payload?.live_execution_available === false &&
    Boolean(paperPortfolio?.evidence_readiness);
  const paperEvidenceReviewContractAvailable =
    runtimeExecutionMode === "PAPER" &&
    paperPortfolioResult.payload?.paper_evidence_review_semantics ===
      "IMMUTABLE_MFA_BACKED_PAPER_OWNER_REVIEW" &&
    paperPortfolioResult.payload?.paper_evidence_review_grants_authority ===
      false &&
    paperPortfolioResult.payload?.broker_action_available === false &&
    paperPortfolioResult.payload?.live_promotion_available === false &&
    paperPortfolioResult.payload?.live_execution_available === false &&
    paperPortfolio?.evidence_review_contract_valid === true;
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
    financialConnectionID,
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
    financialConnectionLastVerifiedAt: text(
      financialConnection,
      "last_synced_at",
      "LastSyncedAt",
    ),
    financialAccountLastSyncedAt: text(
      account,
      "last_synced_at",
      "LastSyncedAt",
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
    paperActivityCadenceContractAvailable:
      runtimeExecutionMode === "PAPER"
        ? paperActivityCadenceContractAvailable
        : undefined,
    paperActivityCadence: paperPortfolio?.activity_cadence,
    paperGuardrailEvidenceContractAvailable:
      runtimeExecutionMode === "PAPER"
        ? paperGuardrailEvidenceContractAvailable
        : undefined,
    paperGuardrailEvidence: paperPortfolio?.guardrail_evidence,
    paperEvidenceReadinessContractAvailable:
      runtimeExecutionMode === "PAPER"
        ? paperEvidenceReadinessContractAvailable
        : undefined,
    paperEvidenceReadiness: paperPortfolio?.evidence_readiness,
    paperEvidenceReviewContractAvailable:
      runtimeExecutionMode === "PAPER"
        ? paperEvidenceReviewContractAvailable
        : undefined,
    paperEvidenceReviewFingerprint: paperPortfolio?.evidence_review_fingerprint,
    paperLatestEvidenceReview: paperPortfolio?.latest_evidence_review,
    paperCurrentEvidenceReviewed: paperPortfolio?.current_evidence_reviewed,
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
