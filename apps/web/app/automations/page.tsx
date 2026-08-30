import Link from "next/link";
import { cookies } from "next/headers";
import { redirect } from "next/navigation";

import { AppPageHeader } from "../app-page-header";
import {
  capitalReservationMatchesPolicy,
  paperCapitalReservationMatchesPolicy,
} from "./capital-authority";
import { asList } from "./response";
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
    decisionResult,
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
    expectsDecisionEvidence
      ? fetchOptional<DecisionWindow>(
          `${base}/api/strategy-instances/${encodeURIComponent(instanceID ?? "")}/decisions?limit=10`,
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
      decisionResult,
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
