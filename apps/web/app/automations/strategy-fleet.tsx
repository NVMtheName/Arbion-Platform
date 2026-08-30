import Link from "next/link";

export type StrategyFleetDecisionInputCoverageSnapshot = {
  decisionID?: string;
  decisionAt?: string;
  financialProvider?: string;
  financialContextComplete?: boolean;
  inputCoverageComplete?: boolean;
  historyLiquidityEvidenceComplete?: boolean;
  marketSymbols?: string[];
  marketFeeds?: string[];
  marketQualities?: string[];
  marketObservedAt?: string;
  historyStatuses?: string[];
  historyFeeds?: string[];
  historyQualities?: string[];
  liquidityStatuses?: string[];
  positionEvidenceComplete?: boolean;
  positionCount?: number;
  positionPerformanceStatuses?: string[];
  marketEventEvidenceComplete?: boolean;
  marketEventCoverageCount?: number;
  marketEventCoverageStatuses?: string[];
  marketEventProviders?: string[];
  marketEventFeeds?: string[];
  marketEventQualities?: string[];
  marketEventCount?: number;
};

export type StrategyFleetItem = {
  id: string;
  strategyInstanceID?: string;
  financialAccountID?: string;
  capitalBucketID?: string;
  capitalReservationID?: string;
  title: string;
  accountName: string;
  provider: string;
  accountStatus?: string;
  financialConnectionAvailable?: boolean;
  financialConnectionContextAvailable?: boolean;
  financialConnectionStatus?: string;
  financialAuthorizationExpiresAt?: string;
  capitalContextAvailable?: boolean;
  capitalBindingValid?: boolean;
  capitalBucketName?: string;
  capitalBucketStatus?: string;
  capitalAllocationType?: string;
  capitalAllocationValue?: string;
  capitalCurrency?: string;
  capitalProtectedAmount?: string;
  capitalAllocationLimit?: string;
  capitalReservationStatus?: string;
  capitalReservationAmount?: string;
  capitalReservationCurrency?: string;
  capitalReservationBasis?: string;
  capitalReservationAccountLimit?: string;
  runtimeVersionContextAvailable?: boolean;
  runtimeBindingValid?: boolean;
  runtimeScheduleBindingValid?: boolean;
  runtimeMandateVersion?: number;
  currentMandateVersion?: number;
  runtimeSnapshotStatus?: string;
  newerDraftAvailable?: boolean;
  runtimeMaxProposalNotional?: string;
  runtimeMaxTradesPerDay?: number;
  runtimeLegacyDailyActionLimitMissing?: boolean;
  runtimeScheduleEnabled?: boolean;
  runtimeScheduleIntervalMinutes?: number;
  runtimeScheduleSession?: string;
  automationType: string;
  mandateStatus: string;
  autonomyLevel: string;
  executionMode: string;
  modelID?: string;
  symbols: string[];
  instanceStatus?: string;
  currentState?: string;
  lastEvaluatedAt?: string;
  scheduleAvailable?: boolean;
  scheduleEnabled?: boolean;
  scheduleStatus?: string;
  scheduleErrorCode?: string;
  scheduleLastCompletedAt?: string;
  scheduleTimingStatus?: "ON_SCHEDULE" | "OVERDUE" | "UNAVAILABLE";
  consecutiveFailures: number;
  nextRunAt?: string;
  scheduleHistoryAvailable?: boolean;
  scheduleRecentRuns?: StrategyFleetScheduleRunEvidence[];
  evidenceAvailable?: boolean;
  evidenceStatus?: string;
  oneHourSampleSize?: number;
  twentyFourHourSampleSize?: number;
  minimumSamplePerHorizon?: number;
  evidenceWindowHours?: number;
  minimumEvidenceWindowHours?: number;
  evidenceScheduleHealthy?: boolean;
  evidenceBlockers?: string[];
  currentEvidenceReviewed?: boolean;
  decisionAvailable?: boolean;
  latestDecisionID?: string;
  latestDecisionType?: string;
  latestDecisionAt?: string;
  latestDecisionProposedActionID?: string;
  latestDecisionRiskEvaluationID?: string;
  latestDecisionExecutionRecordID?: string;
  latestDecisionSymbol?: string;
  latestDecisionSide?: string;
  latestDecisionQuantity?: string;
  latestDecisionRiskDecision?: string;
  latestDecisionRiskReasons?: string[];
  latestDecisionExecutionStatus?: string;
  latestDecisionAIProvider?: string;
  latestDecisionAIModelID?: string;
  latestDecisionAIProfile?: string;
  latestDecisionLatencyMS?: number;
  latestDecisionInputUsage?: number;
  latestDecisionOutputUsage?: number;
  latestDecisionProposedNotional?: string;
  latestDecisionFinancialContextComplete?: boolean;
  latestDecisionFinancialProvider?: string;
  latestDecisionMarketSymbols?: string[];
  latestDecisionMarketFeeds?: string[];
  latestDecisionMarketQualities?: string[];
  latestDecisionMarketObservedAt?: string;
  latestDecisionInputCoverageComplete?: boolean;
  latestDecisionHistoryLiquidityEvidenceComplete?: boolean;
  latestDecisionHistoryStatuses?: string[];
  latestDecisionHistoryFeeds?: string[];
  latestDecisionHistoryQualities?: string[];
  latestDecisionLiquidityStatuses?: string[];
  latestDecisionPositionEvidenceComplete?: boolean;
  latestDecisionPositionCount?: number;
  latestDecisionPositionPerformanceStatuses?: string[];
  latestDecisionMarketEventEvidenceComplete?: boolean;
  latestDecisionMarketEventCoverageCount?: number;
  latestDecisionMarketEventCoverageStatuses?: string[];
  latestDecisionMarketEventProviders?: string[];
  latestDecisionMarketEventFeeds?: string[];
  latestDecisionMarketEventQualities?: string[];
  latestDecisionMarketEventCount?: number;
  priorDecisionID?: string;
  priorDecisionType?: string;
  priorDecisionAt?: string;
  priorDecisionSymbol?: string;
  priorDecisionSide?: string;
  priorDecisionProposedNotional?: string;
  priorDecisionFinancialContextComplete?: boolean;
  priorDecisionFinancialProvider?: string;
  priorDecisionMarketSymbols?: string[];
  priorDecisionMarketFeeds?: string[];
  priorDecisionMarketQualities?: string[];
  priorDecisionMarketObservedAt?: string;
  priorDecisionInputCoverageComplete?: boolean;
  priorDecisionHistoryLiquidityEvidenceComplete?: boolean;
  priorDecisionHistoryStatuses?: string[];
  priorDecisionHistoryFeeds?: string[];
  priorDecisionHistoryQualities?: string[];
  priorDecisionLiquidityStatuses?: string[];
  priorDecisionPositionEvidenceComplete?: boolean;
  priorDecisionPositionCount?: number;
  priorDecisionPositionPerformanceStatuses?: string[];
  priorDecisionMarketEventEvidenceComplete?: boolean;
  priorDecisionMarketEventCoverageCount?: number;
  priorDecisionMarketEventCoverageStatuses?: string[];
  priorDecisionMarketEventProviders?: string[];
  priorDecisionMarketEventFeeds?: string[];
  priorDecisionMarketEventQualities?: string[];
  priorDecisionMarketEventCount?: number;
  recentDecisionInputCoverage?: StrategyFleetDecisionInputCoverageSnapshot[];
  reconciliationAvailable?: boolean;
  reconciliationComparisonStatus?: string;
  reconciliationBalancesStatus?: string;
  reconciliationPositionsStatus?: string;
  reconciliationAutonomySignal?: string;
  reconciliationAutonomyEnforcementActive?: boolean;
  reconciliationBlocksNewActions?: boolean;
  reconciliationBlockingChangeCount?: number;
  reconciliationObservedAt?: string;
  reconciliationFresh?: boolean;
  accountContextAvailable?: boolean;
  instanceContextAvailable?: boolean;
};

type StrategyFleetProps = {
  items: StrategyFleetItem[];
  inventoryAvailable?: boolean;
  contextWarnings?: string[];
};

export type StrategyFleetAccountIsolation = {
  accountID: string;
  accountName: string;
  provider: string;
  status: "VERIFIED" | "UNAVAILABLE";
  engineCount: number;
  paperEngineCount: number;
  shadowEngineCount: number;
  modes: string[];
  currency?: string;
  paperClaimedAmount?: string;
  shadowClaimedAmount?: string;
  accountLimit?: string;
  detail: string;
};

export type StrategyFleetIdentityIsolation = {
  status: "VERIFIED" | "UNAVAILABLE";
  engineCount: number;
  accountCount: number;
  paperEngineCount: number;
  shadowEngineCount: number;
  uniqueInstanceCount: number;
  uniqueBucketCount: number;
  uniqueReservationCount: number;
  crossAccountCollisionCount: number;
};

export type StrategyFleetScheduleReliability = {
  status: "VERIFIED" | "ATTENTION" | "UNAVAILABLE";
  engineCount: number;
  healthyCount: number;
  succeededCount: number;
  safelySkippedCount: number;
  failureCount: number;
  overdueCount: number;
  consecutiveFailures: number;
  nextRunAt?: string;
  engines: Array<{
    id: string;
    title: string;
    accountName: string;
    provider: string;
    executionMode: string;
    lastStatus: string;
    errorCode?: string;
    lastCompletedAt?: string;
    nextRunAt?: string;
    timingStatus: "ON_SCHEDULE" | "OVERDUE" | "UNAVAILABLE";
    consecutiveFailures: number;
  }>;
};

export type StrategyFleetScheduleRunEvidence = {
  id?: string;
  scheduledFor?: string;
  completedAt?: string;
  nextRunAt?: string;
  status?: string;
  errorCode?: string;
  aiDecision?: string;
  executionStatus?: string;
  duplicateRecovered?: boolean;
  consecutiveFailures?: number;
};

export type StrategyFleetScheduleRecovery = {
  status: "VERIFIED" | "ATTENTION" | "UNAVAILABLE";
  engineCount: number;
  verifiedCount: number;
  stableCount: number;
  recoveredCount: number;
  safeWaitCount: number;
  attentionCount: number;
  preservedRunCount: number;
  preservedFailureCount: number;
  engines: Array<{
    id: string;
    title: string;
    accountName: string;
    provider: string;
    executionMode: string;
    state: "STABLE" | "RECOVERED" | "SAFE_WAIT" | "ATTENTION" | "UNAVAILABLE";
    verified: boolean;
    recentRunCount: number;
    preservedFailureCount: number;
    latestStatus?: string;
    latestErrorCode?: string;
    latestCompletedAt?: string;
    nextRunAt?: string;
    recentStatuses: string[];
  }>;
};

export type StrategyFleetDecisionEvidence = {
  status: "VERIFIED" | "UNAVAILABLE";
  engineCount: number;
  verifiedCount: number;
  abstentionCount: number;
  riskEvaluationCount: number;
  nonLiveRecordCount: number;
  engines: Array<{
    id: string;
    title: string;
    accountName: string;
    provider: string;
    executionMode: string;
    decisionID?: string;
    decisionType?: string;
    decisionAt?: string;
    proposedActionID?: string;
    riskEvaluationID?: string;
    executionRecordID?: string;
    riskDecision?: string;
    executionStatus?: string;
    verified: boolean;
  }>;
};

export type StrategyFleetNextAction = {
  key: string;
  priority: number;
  tone: "ATTENTION" | "READY" | "PAUSED";
  eyebrow: string;
  title: string;
  detail: string;
  href: string;
  actionLabel: string;
  accountName?: string;
  provider?: string;
};

export type StrategyFleetOperatingBrief = {
  status: "ON_COURSE" | "REVIEW" | "UNAVAILABLE";
  engineCount: number;
  onCourseCount: number;
  reviewCount: number;
  engines: Array<{
    id: string;
    title: string;
    accountName: string;
    provider: string;
    executionMode: string;
    status: "ON_COURSE" | "REVIEW" | "UNAVAILABLE";
    conclusion: string;
    explanation: string;
    nextStep: string;
    nextRunAt?: string;
    decisionAt?: string;
    detailHref: string;
    reviewHref?: string;
    reviewLabel?: string;
  }>;
};

export type StrategyFleetProvenanceDigest = {
  status: "VERIFIED" | "UNAVAILABLE";
  engineCount: number;
  attributableCount: number;
  changedCount: number;
  heldCourseCount: number;
  firstDecisionCount: number;
  engines: Array<{
    id: string;
    title: string;
    accountName: string;
    provider: string;
    executionMode: string;
    state: "CHANGED" | "HELD_COURSE" | "FIRST_DECISION" | "UNAVAILABLE";
    attributable: boolean;
    latestDecisionType?: string;
    priorDecisionType?: string;
    latestDecisionSymbol?: string;
    priorDecisionSymbol?: string;
    latestDecisionSide?: string;
    priorDecisionSide?: string;
    latestDecisionProposedNotional?: string;
    priorDecisionProposedNotional?: string;
    latestDecisionAt?: string;
    priorDecisionAt?: string;
    aiProvider?: string;
    modelID?: string;
    profile?: string;
    financialProvider?: string;
    marketSymbols: string[];
    marketFeeds: string[];
    marketQualities: string[];
    marketObservedAt?: string;
    followUp: string;
  }>;
};

export type StrategyFleetInputCoverageStatus =
  | "AVAILABLE"
  | "PARTIAL"
  | "UNAVAILABLE"
  | "NOT_APPLICABLE";

export type StrategyFleetInputCoverageMatrix = {
  status: "VERIFIED" | "UNAVAILABLE";
  engineCount: number;
  attributableCount: number;
  availableCategoryCount: number;
  partialCategoryCount: number;
  unavailableCategoryCount: number;
  engines: Array<{
    id: string;
    title: string;
    accountName: string;
    provider: string;
    executionMode: string;
    decisionID?: string;
    decisionAt?: string;
    attributable: boolean;
    financialProvider?: string;
    categories: Array<{
      key: "MARKET_PRICE" | "HISTORY" | "LIQUIDITY" | "POSITION" | "EVENTS";
      label: string;
      status: StrategyFleetInputCoverageStatus;
      evidence: string;
      limitation?: string;
    }>;
  }>;
};

export type StrategyFleetInputCoverageChange =
  | "IMPROVED"
  | "REGRESSED"
  | "UNCHANGED"
  | "CONTEXT_CHANGED"
  | "UNAVAILABLE";

export type StrategyFleetInputCoverageChangeLedger = {
  status: "VERIFIED" | "UNAVAILABLE";
  engineCount: number;
  comparableCount: number;
  improvedCategoryCount: number;
  regressedCategoryCount: number;
  unchangedCategoryCount: number;
  contextChangedCategoryCount: number;
  engines: Array<{
    id: string;
    title: string;
    accountName: string;
    provider: string;
    executionMode: string;
    latestDecisionID?: string;
    priorDecisionID?: string;
    latestDecisionAt?: string;
    priorDecisionAt?: string;
    financialProvider?: string;
    comparable: boolean;
    followUp: string;
    categories: Array<{
      key: "MARKET_PRICE" | "HISTORY" | "LIQUIDITY" | "POSITION" | "EVENTS";
      label: string;
      priorStatus: StrategyFleetInputCoverageStatus;
      currentStatus: StrategyFleetInputCoverageStatus;
      change: StrategyFleetInputCoverageChange;
      priorEvidence: string;
      currentEvidence: string;
    }>;
  }>;
};

export type StrategyFleetPersistentInputGapState =
  | "PERSISTENT"
  | "INTERMITTENT"
  | "NEWLY_MISSING"
  | "CURRENTLY_RESOLVED"
  | "CLEAR"
  | "CONTEXT_CHANGED"
  | "UNAVAILABLE";

export type StrategyFleetPersistentInputGapRegister = {
  status: "VERIFIED" | "UNAVAILABLE";
  engineCount: number;
  attributableCount: number;
  persistentCount: number;
  intermittentCount: number;
  newlyMissingCount: number;
  resolvedCount: number;
  engines: Array<{
    id: string;
    title: string;
    accountName: string;
    provider: string;
    executionMode: string;
    financialProvider?: string;
    attributable: boolean;
    sampleCount: number;
    windowStartedAt?: string;
    windowEndedAt?: string;
    followUp: string;
    categories: Array<{
      key: "HISTORY" | "LIQUIDITY" | "POSITION" | "EVENTS";
      label: string;
      state: StrategyFleetPersistentInputGapState;
      currentStatus: StrategyFleetInputCoverageStatus;
      sampleCount: number;
      gapSampleCount: number;
      firstGapAt?: string;
      latestGapAt?: string;
    }>;
  }>;
};

function providerLabel(provider: string) {
  if (provider === "coinbase") return "Coinbase";
  if (provider === "schwab") return "Charles Schwab";
  return provider === "connected_account" ? "Connected account" : provider;
}

function providerInitial(provider: string) {
  if (provider === "coinbase") return "C";
  if (provider === "schwab") return "S";
  return provider.slice(0, 1).toUpperCase() || "A";
}

const capitalDecimalScale = BigInt("10000000000");

function capitalDecimalUnits(value: string | undefined) {
  if (!value || !/^\d+(\.\d{1,10})?$/.test(value)) return undefined;
  const [whole, fraction = ""] = value.split(".");
  return BigInt(whole) * capitalDecimalScale + BigInt(fraction.padEnd(10, "0"));
}

function capitalDecimalText(value: bigint) {
  const whole = value / capitalDecimalScale;
  const fraction = String(value % capitalDecimalScale)
    .padStart(10, "0")
    .replace(/0+$/, "");
  return `${whole}${fraction ? `.${fraction}` : ""}`;
}

export function projectStrategyFleetAccountIsolation(
  items: StrategyFleetItem[],
): StrategyFleetAccountIsolation[] {
  const operational = items.filter(
    (item) =>
      isAI(item) &&
      Boolean(item.financialAccountID) &&
      Boolean(item.instanceStatus) &&
      ["ACTIVE", "PAUSED"].includes(item.instanceStatus ?? ""),
  );
  const grouped = new Map<string, StrategyFleetItem[]>();
  for (const item of operational) {
    const accountID = item.financialAccountID!;
    grouped.set(accountID, [...(grouped.get(accountID) ?? []), item]);
  }

  const identityAccounts = new Map<string, Set<string>>();
  for (const item of operational) {
    for (const [kind, value] of [
      ["instance", item.strategyInstanceID],
      ["bucket", item.capitalBucketID],
      ["reservation", item.capitalReservationID],
    ]) {
      if (!value) continue;
      const key = `${kind}:${value}`;
      const accounts = identityAccounts.get(key) ?? new Set<string>();
      accounts.add(item.financialAccountID!);
      identityAccounts.set(key, accounts);
    }
  }

  return [...grouped.entries()]
    .map(([accountID, accountItems]) => {
      const first = accountItems[0];
      const modes = [...new Set(accountItems.map((item) => item.executionMode))]
        .filter((mode) => mode === "PAPER" || mode === "SHADOW")
        .sort();
      const currencies = new Set(
        accountItems.map((item) => item.capitalReservationCurrency),
      );
      const reservationIDs = accountItems.map(
        (item) => item.capitalReservationID,
      );
      const bucketIDs = accountItems.map((item) => item.capitalBucketID);
      const instanceIDs = accountItems.map((item) => item.strategyInstanceID);
      const amounts = accountItems.map((item) =>
        capitalDecimalUnits(item.capitalReservationAmount),
      );
      const paperItems = accountItems.filter(
        (item) => item.executionMode === "PAPER",
      );
      const shadowItems = accountItems.filter(
        (item) => item.executionMode === "SHADOW",
      );
      const uniqueIdentity = (values: Array<string | undefined>) =>
        values.every(Boolean) && new Set(values).size === values.length;
      const globallyBoundToOneAccount = (
        kind: string,
        values: Array<string | undefined>,
      ) =>
        values.every(
          (value) =>
            Boolean(value) &&
            identityAccounts.get(`${kind}:${value}`)?.size === 1,
        );
      const exactEvidence =
        accountItems.every(capitalBindingHealthy) &&
        accountItems.every(
          (item) =>
            item.accountContextAvailable !== false &&
            item.instanceContextAvailable !== false &&
            item.provider === first.provider &&
            item.accountName === first.accountName,
        ) &&
        currencies.size === 1 &&
        !currencies.has(undefined) &&
        amounts.every((amount) => amount !== undefined && amount > BigInt(0)) &&
        uniqueIdentity(reservationIDs) &&
        uniqueIdentity(bucketIDs) &&
        uniqueIdentity(instanceIDs) &&
        globallyBoundToOneAccount("reservation", reservationIDs) &&
        globallyBoundToOneAccount("bucket", bucketIDs) &&
        globallyBoundToOneAccount("instance", instanceIDs) &&
        paperItems.every(
          (item) => item.capitalReservationBasis === "PAPER_STARTING_CASH",
        ) &&
        modes.length > 0;
      const amountFor = (item: StrategyFleetItem) =>
        capitalDecimalUnits(item.capitalReservationAmount) ?? BigInt(0);
      const paperTotal = exactEvidence
        ? paperItems.reduce<bigint>(
            (sum, item) => sum + amountFor(item),
            BigInt(0),
          )
        : undefined;
      const shadowTotal = exactEvidence
        ? shadowItems.reduce<bigint>(
            (sum, item) => sum + amountFor(item),
            BigInt(0),
          )
        : undefined;
      const rawLimits = shadowItems.map(
        (item) => item.capitalReservationAccountLimit,
      );
      const limits = rawLimits.map(capitalDecimalUnits);
      const sharedLimit =
        rawLimits.length > 0 &&
        rawLimits.every(Boolean) &&
        new Set(rawLimits).size === 1 &&
        limits.every((limit) => limit !== undefined && limit > BigInt(0))
          ? limits[0]
          : undefined;
      const compatible =
        exactEvidence &&
        (shadowItems.length <= 1 ||
          (sharedLimit !== undefined &&
            shadowTotal !== undefined &&
            shadowTotal <= sharedLimit));

      return {
        accountID,
        accountName: first.accountName,
        provider: first.provider,
        status: compatible ? "VERIFIED" : "UNAVAILABLE",
        engineCount: accountItems.length,
        paperEngineCount: paperItems.length,
        shadowEngineCount: shadowItems.length,
        modes,
        currency: compatible
          ? accountItems[0].capitalReservationCurrency
          : undefined,
        paperClaimedAmount:
          compatible && paperTotal !== undefined
            ? capitalDecimalText(paperTotal)
            : undefined,
        shadowClaimedAmount:
          compatible && shadowTotal !== undefined
            ? capitalDecimalText(shadowTotal)
            : undefined,
        accountLimit:
          compatible && sharedLimit !== undefined
            ? capitalDecimalText(sharedLimit)
            : undefined,
        detail: compatible
          ? paperItems.length > 0 && shadowItems.length > 0
            ? "Paper simulation capital is isolated from the connected account; Shadow uses its own separate policy claim."
            : paperItems.length > 0
              ? "Every Paper engine has its own simulated capital, strategy, bucket, and reservation identity."
              : "Every Shadow engine has a unique strategy, bucket, and reservation identity inside its account policy."
          : "Arbion cannot prove complete, compatible account-level isolation from the current fleet snapshot.",
      } satisfies StrategyFleetAccountIsolation;
    })
    .sort((left, right) => left.accountName.localeCompare(right.accountName));
}

export function projectStrategyFleetIdentityIsolation(
  items: StrategyFleetItem[],
): StrategyFleetIdentityIsolation {
  const operational = items.filter(
    (item) =>
      isAI(item) &&
      Boolean(item.financialAccountID) &&
      Boolean(item.instanceStatus) &&
      ["ACTIVE", "PAUSED"].includes(item.instanceStatus ?? ""),
  );
  const uniqueCount = (values: Array<string | undefined>) =>
    new Set(values.filter((value): value is string => Boolean(value))).size;
  const identityAccounts = new Map<string, Set<string>>();
  for (const item of operational) {
    for (const [kind, value] of [
      ["instance", item.strategyInstanceID],
      ["bucket", item.capitalBucketID],
      ["reservation", item.capitalReservationID],
    ]) {
      if (!value) continue;
      const key = `${kind}:${value}`;
      const accounts = identityAccounts.get(key) ?? new Set<string>();
      accounts.add(item.financialAccountID!);
      identityAccounts.set(key, accounts);
    }
  }
  const crossAccountCollisionCount = [...identityAccounts.values()].filter(
    (accounts) => accounts.size > 1,
  ).length;
  const accounts = projectStrategyFleetAccountIsolation(operational);
  const instanceCount = uniqueCount(
    operational.map((item) => item.strategyInstanceID),
  );
  const bucketCount = uniqueCount(
    operational.map((item) => item.capitalBucketID),
  );
  const reservationCount = uniqueCount(
    operational.map((item) => item.capitalReservationID),
  );
  const complete =
    operational.length > 0 &&
    accounts.length > 0 &&
    accounts.every((account) => account.status === "VERIFIED") &&
    crossAccountCollisionCount === 0 &&
    instanceCount === operational.length &&
    bucketCount === operational.length &&
    reservationCount === operational.length;
  return {
    status: complete ? "VERIFIED" : "UNAVAILABLE",
    engineCount: operational.length,
    accountCount: new Set(
      operational.map((item) => item.financialAccountID).filter(Boolean),
    ).size,
    paperEngineCount: operational.filter(
      (item) => item.executionMode === "PAPER",
    ).length,
    shadowEngineCount: operational.filter(
      (item) => item.executionMode === "SHADOW",
    ).length,
    uniqueInstanceCount: instanceCount,
    uniqueBucketCount: bucketCount,
    uniqueReservationCount: reservationCount,
    crossAccountCollisionCount,
  };
}

function readable(value: string) {
  return value
    .toLowerCase()
    .split("_")
    .filter(Boolean)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(" ");
}

function evidenceBlockerLabel(value: string) {
  const labels: Record<string, string> = {
    ONE_HOUR_SAMPLE_INCOMPLETE: "Collect more 1-hour outcome marks",
    TWENTY_FOUR_HOUR_SAMPLE_INCOMPLETE: "Collect more 24-hour outcome marks",
    EVIDENCE_WINDOW_INCOMPLETE: "Observe the mandate across a longer window",
    SCHEDULE_NOT_VERIFIED: "Complete a healthy scheduled cycle",
    SCHEDULE_UNHEALTHY: "Resolve the current scheduler failure",
  };
  return labels[value] ?? value.replaceAll("_", " ");
}

function readableTime(value?: string) {
  if (!value) return "—";
  const date = new Date(value);
  if (Number.isNaN(date.valueOf())) return "—";
  return new Intl.DateTimeFormat("en-US", {
    month: "short",
    day: "numeric",
    hour: "numeric",
    minute: "2-digit",
    timeZone: "UTC",
    timeZoneName: "short",
  }).format(date);
}

export function reconciliationFreshWithinTwentyFourHours(
  value: string | undefined,
  now: Date,
) {
  if (!value || Number.isNaN(now.valueOf())) return false;
  const observedAt = new Date(value);
  if (Number.isNaN(observedAt.valueOf()) || observedAt > now) return false;
  return now.valueOf() - observedAt.valueOf() <= 24 * 60 * 60 * 1000;
}

export function scheduledRunTimingStatus(
  nextRunAt: string | undefined,
  observedAt: Date,
) {
  if (!nextRunAt || Number.isNaN(observedAt.valueOf())) return "UNAVAILABLE";
  const nextRun = new Date(nextRunAt);
  if (Number.isNaN(nextRun.valueOf())) return "UNAVAILABLE";
  const graceMilliseconds = 5 * 60 * 1000;
  return nextRun.valueOf() < observedAt.valueOf() - graceMilliseconds
    ? "OVERDUE"
    : "ON_SCHEDULE";
}

export function projectStrategyFleetScheduleReliability(
  items: StrategyFleetItem[],
): StrategyFleetScheduleReliability {
  const scheduled = items.filter(
    (item) =>
      isAI(item) &&
      item.instanceStatus === "ACTIVE" &&
      item.runtimeScheduleEnabled === true,
  );
  const engines = scheduled.map((item) => ({
    id: item.id,
    title: item.title,
    accountName: item.accountName,
    provider: item.provider,
    executionMode: item.executionMode,
    lastStatus: item.scheduleStatus ?? "UNAVAILABLE",
    errorCode: item.scheduleErrorCode,
    lastCompletedAt: item.scheduleLastCompletedAt,
    nextRunAt: item.nextRunAt,
    timingStatus: item.scheduleTimingStatus ?? ("UNAVAILABLE" as const),
    consecutiveFailures: item.consecutiveFailures,
  }));
  const unavailable = scheduled.some(
    (item) =>
      item.scheduleAvailable !== true ||
      item.scheduleEnabled !== true ||
      item.runtimeScheduleBindingValid !== true ||
      !item.scheduleStatus ||
      !item.scheduleLastCompletedAt ||
      !item.nextRunAt ||
      item.scheduleTimingStatus === "UNAVAILABLE" ||
      item.scheduleTimingStatus === undefined ||
      !Number.isInteger(item.consecutiveFailures) ||
      item.consecutiveFailures < 0,
  );
  const overdueCount = engines.filter(
    (engine) => engine.timingStatus === "OVERDUE",
  ).length;
  const failureCount = engines.filter(
    (engine) =>
      engine.lastStatus === "FAILED" || engine.consecutiveFailures > 0,
  ).length;
  const healthyCount = engines.filter(
    (engine) =>
      ["SUCCEEDED", "SKIPPED"].includes(engine.lastStatus) &&
      engine.consecutiveFailures === 0 &&
      engine.timingStatus === "ON_SCHEDULE",
  ).length;
  const nextRunAt = engines
    .map((engine) => engine.nextRunAt)
    .filter((value): value is string => Boolean(value))
    .sort(
      (left, right) => new Date(left).valueOf() - new Date(right).valueOf(),
    )[0];
  const status =
    scheduled.length === 0 || unavailable
      ? "UNAVAILABLE"
      : overdueCount > 0 ||
          failureCount > 0 ||
          healthyCount !== scheduled.length
        ? "ATTENTION"
        : "VERIFIED";
  return {
    status,
    engineCount: scheduled.length,
    healthyCount,
    succeededCount: engines.filter(
      (engine) => engine.lastStatus === "SUCCEEDED",
    ).length,
    safelySkippedCount: engines.filter(
      (engine) => engine.lastStatus === "SKIPPED",
    ).length,
    failureCount,
    overdueCount,
    consecutiveFailures: engines.reduce(
      (sum, engine) => sum + engine.consecutiveFailures,
      0,
    ),
    nextRunAt,
    engines,
  };
}

function validScheduleRunEvidence(run: StrategyFleetScheduleRunEvidence) {
  return Boolean(
    run.id &&
      run.scheduledFor &&
      run.completedAt &&
      run.nextRunAt &&
      run.status &&
      ["SUCCEEDED", "FAILED", "SKIPPED"].includes(run.status) &&
      !Number.isNaN(new Date(run.scheduledFor).valueOf()) &&
      !Number.isNaN(new Date(run.completedAt).valueOf()) &&
      !Number.isNaN(new Date(run.nextRunAt).valueOf()) &&
      typeof run.duplicateRecovered === "boolean" &&
      Number.isInteger(run.consecutiveFailures) &&
      (run.consecutiveFailures ?? -1) >= 0,
  );
}

function sameInstant(left?: string, right?: string) {
  if (!left || !right) return false;
  const leftTime = new Date(left).valueOf();
  const rightTime = new Date(right).valueOf();
  return (
    !Number.isNaN(leftTime) &&
    !Number.isNaN(rightTime) &&
    leftTime === rightTime
  );
}

export function projectStrategyFleetScheduleRecovery(
  items: StrategyFleetItem[],
): StrategyFleetScheduleRecovery {
  const scheduled = items.filter(
    (item) =>
      isAI(item) &&
      item.instanceStatus === "ACTIVE" &&
      item.runtimeScheduleEnabled === true,
  );
  const engines = scheduled.map((item) => {
    const recentRuns = item.scheduleRecentRuns ?? [];
    const latest = recentRuns[0];
    const historyVerified = Boolean(
      item.scheduleHistoryAvailable === true &&
        recentRuns.length > 0 &&
        recentRuns.length <= 12 &&
        recentRuns.every(validScheduleRunEvidence) &&
        latest?.status === item.scheduleStatus &&
        latest?.consecutiveFailures === item.consecutiveFailures &&
        sameInstant(latest?.completedAt, item.scheduleLastCompletedAt) &&
        sameInstant(latest?.nextRunAt, item.nextRunAt),
    );
    const preservedFailureCount = recentRuns.filter(
      (run) => run.status === "FAILED",
    ).length;
    const state = !historyVerified
      ? ("UNAVAILABLE" as const)
      : latest?.status === "FAILED" || item.consecutiveFailures > 0
        ? ("ATTENTION" as const)
        : latest?.status === "SKIPPED"
          ? ("SAFE_WAIT" as const)
          : preservedFailureCount > 0
            ? ("RECOVERED" as const)
            : ("STABLE" as const);
    return {
      id: item.id,
      title: item.title,
      accountName: item.accountName,
      provider: item.provider,
      executionMode: item.executionMode,
      state,
      verified: historyVerified,
      recentRunCount: recentRuns.length,
      preservedFailureCount,
      latestStatus: latest?.status,
      latestErrorCode: latest?.errorCode,
      latestCompletedAt: latest?.completedAt,
      nextRunAt: latest?.nextRunAt,
      recentStatuses: recentRuns.map((run) => run.status ?? "UNAVAILABLE"),
    };
  });
  const verifiedCount = engines.filter((engine) => engine.verified).length;
  const attentionCount = engines.filter(
    (engine) => engine.state === "ATTENTION",
  ).length;
  const unavailable = engines.some((engine) => !engine.verified);
  return {
    status:
      scheduled.length === 0 || unavailable
        ? "UNAVAILABLE"
        : attentionCount > 0
          ? "ATTENTION"
          : "VERIFIED",
    engineCount: scheduled.length,
    verifiedCount,
    stableCount: engines.filter((engine) => engine.state === "STABLE").length,
    recoveredCount: engines.filter((engine) => engine.state === "RECOVERED")
      .length,
    safeWaitCount: engines.filter((engine) => engine.state === "SAFE_WAIT")
      .length,
    attentionCount,
    preservedRunCount: engines.reduce(
      (sum, engine) => sum + engine.recentRunCount,
      0,
    ),
    preservedFailureCount: engines.reduce(
      (sum, engine) => sum + engine.preservedFailureCount,
      0,
    ),
    engines,
  };
}

const linkedDecisionContracts: Record<
  string,
  { riskDecision: string; executionStatus: string }
> = {
  DENY_RISK_DENIED: {
    riskDecision: "DENY",
    executionStatus: "RISK_DENIED",
  },
  ALLOW_WOULD_HAVE_SUBMITTED: {
    riskDecision: "ALLOW",
    executionStatus: "WOULD_HAVE_SUBMITTED",
  },
  ALLOW_SIMULATED_FILLED: {
    riskDecision: "ALLOW",
    executionStatus: "SIMULATED_FILLED",
  },
  ALLOW_SIMULATED_REJECTED: {
    riskDecision: "ALLOW",
    executionStatus: "SIMULATED_REJECTED",
  },
};

export function projectStrategyFleetDecisionEvidence(
  items: StrategyFleetItem[],
): StrategyFleetDecisionEvidence {
  const active = items.filter(
    (item) => isAI(item) && item.instanceStatus === "ACTIVE",
  );
  const engines = active.map((item) => {
    const linkedContract = item.latestDecisionType
      ? linkedDecisionContracts[item.latestDecisionType]
      : undefined;
    const abstentionVerified =
      item.latestDecisionType === "ABSTAIN" &&
      Boolean(item.latestDecisionID) &&
      Boolean(item.latestDecisionAt) &&
      !item.latestDecisionProposedActionID &&
      !item.latestDecisionRiskEvaluationID &&
      !item.latestDecisionExecutionRecordID &&
      !item.latestDecisionRiskDecision &&
      !item.latestDecisionExecutionStatus;
    const linkedDecisionVerified = Boolean(
      linkedContract &&
        item.latestDecisionID &&
        item.latestDecisionAt &&
        item.latestDecisionProposedActionID &&
        item.latestDecisionRiskEvaluationID &&
        item.latestDecisionExecutionRecordID &&
        item.latestDecisionRiskDecision === linkedContract.riskDecision &&
        item.latestDecisionExecutionStatus === linkedContract.executionStatus,
    );
    return {
      id: item.id,
      title: item.title,
      accountName: item.accountName,
      provider: item.provider,
      executionMode: item.executionMode,
      decisionID: item.latestDecisionID,
      decisionType: item.latestDecisionType,
      decisionAt: item.latestDecisionAt,
      proposedActionID: item.latestDecisionProposedActionID,
      riskEvaluationID: item.latestDecisionRiskEvaluationID,
      executionRecordID: item.latestDecisionExecutionRecordID,
      riskDecision: item.latestDecisionRiskDecision,
      executionStatus: item.latestDecisionExecutionStatus,
      verified:
        item.decisionAvailable === true &&
        (abstentionVerified || linkedDecisionVerified),
    };
  });
  const verifiedCount = engines.filter((engine) => engine.verified).length;
  return {
    status:
      engines.length > 0 && verifiedCount === engines.length
        ? "VERIFIED"
        : "UNAVAILABLE",
    engineCount: engines.length,
    verifiedCount,
    abstentionCount: engines.filter(
      (engine) => engine.decisionType === "ABSTAIN" && engine.verified,
    ).length,
    riskEvaluationCount: engines.filter(
      (engine) => Boolean(engine.riskEvaluationID) && engine.verified,
    ).length,
    nonLiveRecordCount: engines.filter(
      (engine) => Boolean(engine.executionRecordID) && engine.verified,
    ).length,
    engines,
  };
}

function isAI(item: StrategyFleetItem) {
  return item.automationType === "AI_AUTONOMOUS";
}

function isAIShadow(item: StrategyFleetItem) {
  return isAI(item) && item.executionMode === "SHADOW";
}

function isAutonomous(item: StrategyFleetItem) {
  return (
    item.autonomyLevel === "FULL_AUTONOMOUS" ||
    item.autonomyLevel === "STRATEGY_AUTONOMOUS"
  );
}

function historicalInstance(item: StrategyFleetItem) {
  return Boolean(
    item.instanceStatus &&
      !["ACTIVE", "PAUSED", "ERROR"].includes(item.instanceStatus),
  );
}

function requiresOperationalData(item: StrategyFleetItem) {
  return isAI(item) && item.instanceStatus === "ACTIVE";
}

function requiresReconciliationData(item: StrategyFleetItem) {
  return requiresOperationalData(item) && isAIShadow(item);
}

function requiresShadowEvidence(item: StrategyFleetItem) {
  return isAIShadow(item) && Boolean(item.instanceStatus);
}

function requiresCapitalData(item: StrategyFleetItem) {
  return (
    isAI(item) &&
    Boolean(
      item.instanceStatus && ["ACTIVE", "PAUSED"].includes(item.instanceStatus),
    )
  );
}

function requiresRuntimeContract(item: StrategyFleetItem) {
  return (
    isAI(item) &&
    Boolean(
      item.instanceStatus &&
        ["ACTIVE", "PAUSED", "ERROR"].includes(item.instanceStatus),
    )
  );
}

function runtimeContractHealthy(item: StrategyFleetItem) {
  if (!requiresRuntimeContract(item)) return true;
  return (
    item.runtimeVersionContextAvailable === true &&
    item.runtimeBindingValid === true &&
    item.runtimeScheduleBindingValid === true
  );
}

function capitalBindingHealthy(item: StrategyFleetItem) {
  if (!requiresCapitalData(item)) return true;
  return (
    item.capitalContextAvailable === true && item.capitalBindingValid === true
  );
}

function financialConnectionHealthy(item: StrategyFleetItem) {
  if (!requiresOperationalData(item)) return true;
  return (
    item.financialConnectionAvailable === true &&
    item.accountStatus === "active" &&
    item.financialConnectionStatus === "active"
  );
}

function reconciliationHealthy(item: StrategyFleetItem) {
  if (!requiresReconciliationData(item)) return true;
  return (
    item.reconciliationAvailable === true &&
    item.reconciliationComparisonStatus === "MATCHED" &&
    item.reconciliationBalancesStatus === "READY" &&
    item.reconciliationPositionsStatus === "READY" &&
    item.reconciliationAutonomySignal === "CLEAR" &&
    item.reconciliationAutonomyEnforcementActive === true &&
    item.reconciliationBlocksNewActions === false &&
    item.reconciliationFresh === true
  );
}

function needsReview(item: StrategyFleetItem) {
  return (
    item.instanceStatus === "ERROR" ||
    item.currentState === "ERROR" ||
    item.accountContextAvailable === false ||
    item.instanceContextAvailable === false ||
    !runtimeContractHealthy(item) ||
    !capitalBindingHealthy(item) ||
    !financialConnectionHealthy(item) ||
    !reconciliationHealthy(item) ||
    item.scheduleAvailable === false ||
    item.scheduleHistoryAvailable === false ||
    (requiresShadowEvidence(item) && item.evidenceAvailable === false) ||
    item.decisionAvailable === false ||
    item.scheduleStatus === "FAILED" ||
    item.scheduleTimingStatus === "OVERDUE" ||
    item.scheduleTimingStatus === "UNAVAILABLE" ||
    item.consecutiveFailures > 0
  );
}

function statusLabel(item: StrategyFleetItem) {
  if (item.instanceContextAvailable === false) return "Status unavailable";
  if (item.accountContextAvailable === false) return "Context unavailable";
  if (needsReview(item)) return "Needs review";
  if (item.instanceStatus === "PAUSED") return "Paused";
  if (item.instanceStatus === "ACTIVE" && item.currentState === "AI_MONITORING")
    return "Monitoring";
  if (item.instanceStatus === "ACTIVE") return "Active";
  if (historicalInstance(item)) return "New version required";
  if (item.mandateStatus === "DRAFT") return "Draft";
  if (item.mandateStatus === "READY") return "Ready to initialize";
  return readable(item.mandateStatus || "Unknown");
}

function healthLabel(item: StrategyFleetItem) {
  if (item.instanceContextAvailable === false)
    return "Engine state unavailable";
  if (item.accountContextAvailable === false)
    return "Account context unavailable";
  if (item.runtimeVersionContextAvailable === false)
    return "Runtime contract unavailable";
  if (requiresRuntimeContract(item) && !runtimeContractHealthy(item))
    return "Runtime contract needs review";
  if (item.capitalContextAvailable === false)
    return "Capital context unavailable";
  if (requiresCapitalData(item) && !capitalBindingHealthy(item))
    return "Capital binding needs review";
  if (item.financialConnectionContextAvailable === false)
    return "Connection context unavailable";
  if (requiresOperationalData(item) && !financialConnectionHealthy(item)) {
    if (item.financialConnectionAvailable === false)
      return "Connection status unavailable";
    return "Financial connection needs review";
  }
  if (requiresReconciliationData(item) && !reconciliationHealthy(item)) {
    if (item.reconciliationAvailable === false)
      return "Portfolio evidence unavailable";
    if (item.reconciliationFresh === false)
      return "Portfolio evidence is stale";
    if (
      item.reconciliationBlocksNewActions ||
      item.reconciliationComparisonStatus === "DRIFT_DETECTED"
    )
      return "Portfolio drift blocks proposals";
    return "Portfolio evidence needs review";
  }
  if (item.scheduleAvailable === false) return "Schedule status unavailable";
  if (item.scheduleHistoryAvailable === false)
    return "Scheduler recovery evidence unavailable";
  if (item.scheduleTimingStatus === "UNAVAILABLE")
    return "Schedule timing unavailable";
  if (item.scheduleTimingStatus === "OVERDUE")
    return "Scheduled cycle is overdue";
  if (requiresShadowEvidence(item) && item.evidenceAvailable === false)
    return "Evidence status unavailable";
  if (item.decisionAvailable === false) return "Decision status unavailable";
  if (item.scheduleStatus === "FAILED" || item.consecutiveFailures > 0)
    return "Schedule needs review";
  if (item.instanceStatus === "ERROR" || item.currentState === "ERROR")
    return "Engine needs review";
  if (item.instanceStatus === "PAUSED") return "Paused by owner";
  if (historicalInstance(item)) return "Historical version complete";
  if (item.scheduleEnabled && item.nextRunAt) return "Healthy schedule";
  if (item.instanceStatus === "ACTIVE") return "Manual cycles only";
  if (item.mandateStatus === "READY") return "Ready to initialize";
  return "Draft configuration";
}

export function selectStrategyFleetNextAction(
  item: StrategyFleetItem,
): StrategyFleetNextAction | null {
  const destination = `/automations/${item.id}`;
  const identity = {
    key: item.id,
    accountName: item.accountName,
    provider: item.provider,
  };

  if (
    item.accountContextAvailable === false ||
    item.financialConnectionContextAvailable === false ||
    item.capitalContextAvailable === false ||
    item.runtimeVersionContextAvailable === false ||
    item.instanceContextAvailable === false
  ) {
    return {
      key: "CURRENT_FLEET_CONTEXT",
      priority: 0,
      tone: "ATTENTION",
      eyebrow: "CURRENT CONTEXT UNAVAILABLE",
      title: "Refresh the current fleet context",
      detail:
        "Arbion will not infer owner actions from a partial account or strategy inventory. No mandate or schedule was changed.",
      href: "/automations",
      actionLabel: "Refresh automations",
    };
  }
  if (requiresRuntimeContract(item) && !runtimeContractHealthy(item)) {
    return {
      ...identity,
      key: `runtime-contract:${item.id}`,
      priority: 1,
      tone: "ATTENTION",
      eyebrow: "PINNED RUNTIME NEEDS REVIEW",
      title: `Review ${item.title} runtime contract`,
      detail:
        "Arbion could not verify the exact immutable mandate version and matching non-live schedule row pinned to this engine. Configuration facts remain hidden, and no runtime setting or broker action was changed.",
      href: `${destination}#configuration-controls`,
      actionLabel: "Review runtime contract",
    };
  }
  if (requiresCapitalData(item) && !capitalBindingHealthy(item)) {
    return {
      ...identity,
      key: `capital-binding:${item.id}`,
      priority: 2,
      tone: "ATTENTION",
      eyebrow: "CAPITAL BINDING NEEDS REVIEW",
      title: `Review ${item.title} capital authority`,
      detail:
        "Arbion could not verify one active, non-reserve bucket and its exact non-live reservation for this engine. Existing database controls remain enforced and no broker funds moved.",
      href: "/capital",
      actionLabel: "Review capital control",
    };
  }
  if (requiresOperationalData(item) && !financialConnectionHealthy(item)) {
    return {
      ...identity,
      key: `financial-connection:${item.id}`,
      priority: 3,
      tone: "ATTENTION",
      eyebrow: "FINANCIAL CONNECTION NEEDS REVIEW",
      title: `Restore ${providerLabel(item.provider)} account access`,
      detail:
        item.financialConnectionAvailable === false
          ? "Arbion could not verify the current account and connection record. The engine will not treat this broker context as usable."
          : "The attached account or financial connection is not active. New AI proposals remain fail-closed until access is restored.",
      href: "/connections#financial-accounts",
      actionLabel: "Review connection",
    };
  }
  if (requiresReconciliationData(item) && !reconciliationHealthy(item)) {
    const stale = item.reconciliationFresh === false;
    const drift =
      item.reconciliationBlocksNewActions ||
      item.reconciliationComparisonStatus === "DRIFT_DETECTED";
    return {
      ...identity,
      key: `portfolio-evidence:${item.id}`,
      priority: 4,
      tone: "ATTENTION",
      eyebrow: "PORTFOLIO EVIDENCE NEEDS REVIEW",
      title: drift
        ? `Review ${item.accountName} portfolio drift`
        : stale
          ? `Refresh ${item.accountName} portfolio evidence`
          : `Review ${item.accountName} portfolio evidence`,
      detail:
        item.reconciliationAvailable === false
          ? "The latest immutable reconciliation could not be loaded. Arbion will not infer balances, positions, or proposal readiness."
          : drift
            ? `${item.reconciliationBlockingChangeCount ?? 0} blocking ${item.reconciliationBlockingChangeCount === 1 ? "change" : "changes"} recorded. Deterministic controls hold new proposals until the exact broker evidence is reviewed.`
            : stale
              ? "The latest immutable broker snapshot is older than the 24-hour autonomy threshold. New proposals remain fail-closed until current evidence is recorded."
              : "Balance, position, match, or autonomy-control evidence is incomplete. New proposals remain fail-closed.",
      href: item.financialAccountID
        ? `/accounts/${item.financialAccountID}#reconciliation-title`
        : "/accounts",
      actionLabel: "Review portfolio evidence",
    };
  }
  if (item.instanceStatus === "ERROR" || item.currentState === "ERROR") {
    return {
      ...identity,
      priority: 5,
      tone: "ATTENTION",
      eyebrow: "ENGINE REVIEW REQUIRED",
      title: `Review ${item.title} runtime evidence`,
      detail:
        "The non-live instance is in an error state. Arbion stopped the cycle and did not send a broker order.",
      href: `${destination}#runtime-evidence`,
      actionLabel: "Review runtime evidence",
    };
  }
  if (
    item.scheduleAvailable === false ||
    item.scheduleHistoryAvailable === false ||
    item.scheduleStatus === "FAILED" ||
    item.scheduleTimingStatus === "OVERDUE" ||
    item.scheduleTimingStatus === "UNAVAILABLE" ||
    item.consecutiveFailures > 0
  ) {
    return {
      ...identity,
      priority: 6,
      tone: "ATTENTION",
      eyebrow: "SCHEDULE NEEDS REVIEW",
      title: `Review ${item.title} schedule health`,
      detail:
        item.scheduleAvailable === false
          ? "Arbion could not refresh the current schedule record and will not label this engine healthy."
          : item.scheduleHistoryAvailable === false
            ? "Arbion could not verify the recent immutable scheduler history. The current schedule remains unchanged and no broker order was sent."
            : item.scheduleTimingStatus === "UNAVAILABLE"
              ? "Arbion could not verify the exact next-cycle time and will not infer scheduler health."
              : item.scheduleTimingStatus === "OVERDUE"
                ? "The next guarded cycle is more than five minutes overdue. Arbion has not inferred a result and no broker order was sent."
                : `${item.consecutiveFailures} consecutive ${item.consecutiveFailures === 1 ? "failure" : "failures"}. The scheduler failed closed without broker execution.`,
      href: `${destination}#schedule-controls`,
      actionLabel: "Review schedule",
    };
  }
  if (requiresShadowEvidence(item) && item.evidenceAvailable === false) {
    return {
      ...identity,
      priority: 7,
      tone: "ATTENTION",
      eyebrow: "EVIDENCE STATUS UNAVAILABLE",
      title: `Refresh ${item.title} evidence`,
      detail:
        "Arbion could not refresh the immutable Shadow scorecard and will not infer its sample or review status.",
      href: destination,
      actionLabel: "Refresh evidence",
    };
  }
  if (item.decisionAvailable === false) {
    return {
      ...identity,
      priority: 8,
      tone: "ATTENTION",
      eyebrow: "DECISION STATUS UNAVAILABLE",
      title: `Refresh ${item.title} decision pulse`,
      detail:
        "Arbion could not refresh the bounded immutable journal window and will not infer a recent AI action.",
      href: destination,
      actionLabel: "Refresh decision pulse",
    };
  }
  if (
    requiresShadowEvidence(item) &&
    item.evidenceStatus === "EVIDENCE_REVIEWABLE" &&
    !item.currentEvidenceReviewed
  ) {
    return {
      ...identity,
      priority: 9,
      tone: "READY",
      eyebrow: "SHADOW EVIDENCE REVIEWABLE",
      title: `Review ${item.title} evidence`,
      detail:
        "The exact non-live sample and time-window gate has passed. Review is an acknowledgment only and grants no trading authority.",
      href: `${destination}#shadow-evidence-review`,
      actionLabel: "Open evidence review",
    };
  }
  if (item.mandateStatus === "DRAFT") {
    return {
      ...identity,
      priority: 10,
      tone: "READY",
      eyebrow: "DRAFT REVIEW",
      title: `Finish reviewing ${item.title}`,
      detail:
        "Review the exact saved version and mark it Ready before any non-live instance can initialize.",
      href: `${destination}#mandate-lifecycle-controls`,
      actionLabel: "Review draft",
    };
  }
  if (historicalInstance(item)) {
    return {
      ...identity,
      priority: 20,
      tone: "READY",
      eyebrow: "HISTORICAL VERSION COMPLETE",
      title: `Create the next ${item.title} version`,
      detail:
        "This immutable version already produced a historical instance. A new bounded version is required before initialization.",
      href: `${destination}#configuration-controls`,
      actionLabel: "Open version controls",
    };
  }
  if (!item.instanceStatus && item.mandateStatus === "READY") {
    return {
      ...identity,
      priority: 30,
      tone: "READY",
      eyebrow: "READY FOR NON-LIVE INITIALIZATION",
      title: `Initialize ${item.title}`,
      detail:
        "The mandate is Ready for its exact initialization checks. This creates no broker order or live authority.",
      href: `${destination}#strategy-readiness-check`,
      actionLabel: "Review initialization",
    };
  }
  if (item.instanceStatus === "PAUSED") {
    return {
      ...identity,
      priority: 40,
      tone: "PAUSED",
      eyebrow: "PAUSED BY OWNER",
      title: `Decide when to resume ${item.title}`,
      detail:
        "The non-live capital claim remains protected while evaluations are stopped. Resume only when you choose.",
      href: `${destination}#strategy-instance-controls`,
      actionLabel: "Open resume controls",
    };
  }
  if (item.instanceStatus === "ACTIVE" && isAutonomous(item)) {
    if (!item.scheduleEnabled || !item.nextRunAt) {
      return {
        ...identity,
        priority: 50,
        tone: "READY",
        eyebrow: "AUTOMATIC CYCLES ARE OFF",
        title: `Configure ${item.title} scheduling`,
        detail:
          "The non-live engine is active, but it has no current automatic cycle. Review and save a bounded schedule.",
        href: `${destination}#schedule-controls`,
        actionLabel: "Open schedule controls",
      };
    }
    return null;
  }
  if (item.instanceStatus === "ACTIVE") {
    return {
      ...identity,
      priority: 60,
      tone: "READY",
      eyebrow: "OWNER-INVOKED STRATEGY",
      title: `Evaluate ${item.title} when ready`,
      detail:
        "This strategy waits for an owner-invoked PAPER or SHADOW evaluation through the deterministic risk gate.",
      href: `${destination}#manual-evaluation`,
      actionLabel: "Open evaluation controls",
    };
  }
  return {
    ...identity,
    priority: 70,
    tone: "ATTENTION",
    eyebrow: "MANDATE REVIEW REQUIRED",
    title: `Review ${item.title}`,
    detail:
      "The current lifecycle state does not map to an automatic owner action. Review the immutable mandate before proceeding.",
    href: destination,
    actionLabel: "Open strategy",
  };
}

function operatingConclusion(item: StrategyFleetItem) {
  switch (item.latestDecisionType) {
    case "ABSTAIN":
      return "AI chose to wait.";
    case "DENY_RISK_DENIED":
      return "A proposal was held.";
    case "ALLOW_SIMULATED_FILLED":
      return "A Paper trade was simulated.";
    case "ALLOW_SIMULATED_REJECTED":
      return "A Paper simulation was rejected safely.";
    case "ALLOW_WOULD_HAVE_SUBMITTED":
      return "A Shadow proposal was recorded.";
    default:
      return item.decisionAvailable === true
        ? "Waiting for the first AI conclusion."
        : "The latest conclusion is unavailable.";
  }
}

function operatingExplanation(
  item: StrategyFleetItem,
  action: StrategyFleetNextAction | null,
) {
  if (action) return action.detail;
  switch (item.latestDecisionType) {
    case "ABSTAIN":
      return "The latest immutable journal ended safely without a proposal, so no risk or execution record was needed.";
    case "DENY_RISK_DENIED":
      return "Deterministic controls denied the proposal and preserved the exact reason without creating a broker order.";
    case "ALLOW_SIMULATED_FILLED":
      return "Deterministic controls allowed the proposal, and only the isolated Paper ledger changed.";
    case "ALLOW_SIMULATED_REJECTED":
      return "The isolated simulator rejected the proposal safely; no connected-account holding changed.";
    case "ALLOW_WOULD_HAVE_SUBMITTED":
      return "Deterministic controls allowed the proposal, but Shadow mode only preserved what would have happened.";
    default:
      return "The guarded engine will preserve its first completed AI conclusion in the immutable journal.";
  }
}

export function projectStrategyFleetOperatingBrief(
  items: StrategyFleetItem[],
): StrategyFleetOperatingBrief {
  const active = items.filter(
    (item) =>
      isAI(item) &&
      Boolean(
        item.instanceStatus &&
          ["ACTIVE", "PAUSED"].includes(item.instanceStatus),
      ),
  );
  const engines = active.map((item) => {
    const action = selectStrategyFleetNextAction(item);
    const unavailable =
      item.accountContextAvailable === false ||
      item.instanceContextAvailable === false ||
      item.decisionAvailable === false;
    const status = unavailable
      ? ("UNAVAILABLE" as const)
      : action
        ? ("REVIEW" as const)
        : ("ON_COURSE" as const);
    const nextStep = action
      ? action.title
      : item.instanceStatus === "PAUSED"
        ? "Nothing runs until you choose to resume it."
        : item.scheduleStatus === "SKIPPED" &&
            item.scheduleErrorCode === "OUTSIDE_SESSION"
          ? "Arbion will wait for the next eligible market session."
          : item.scheduleEnabled && item.nextRunAt
            ? "The next guarded cycle runs automatically."
            : "This engine waits for an owner-invoked non-live evaluation.";
    return {
      id: item.id,
      title: item.title,
      accountName: item.accountName,
      provider: item.provider,
      executionMode: item.executionMode,
      status,
      conclusion: operatingConclusion(item),
      explanation: operatingExplanation(item, action),
      nextStep,
      nextRunAt: !action && item.scheduleEnabled ? item.nextRunAt : undefined,
      decisionAt: item.latestDecisionAt,
      detailHref: `/automations/${item.id}#runtime-evidence`,
      reviewHref: action?.href,
      reviewLabel: action?.actionLabel,
    };
  });
  const reviewCount = engines.filter(
    (engine) => engine.status === "REVIEW",
  ).length;
  const unavailable = engines.some((engine) => engine.status === "UNAVAILABLE");
  return {
    status:
      active.length === 0 || unavailable
        ? "UNAVAILABLE"
        : reviewCount > 0
          ? "REVIEW"
          : "ON_COURSE",
    engineCount: active.length,
    onCourseCount: engines.filter((engine) => engine.status === "ON_COURSE")
      .length,
    reviewCount,
    engines,
  };
}

function exactDecisionNotional(
  decisionType: string | undefined,
  value: string | undefined,
) {
  const units = capitalDecimalUnits(value);
  if (units === undefined) return undefined;
  if (decisionType === "ABSTAIN")
    return units === BigInt(0) ? units : undefined;
  return units > BigInt(0) ? units : undefined;
}

function exactDecisionSignature(item: {
  decisionType?: string;
  symbol?: string;
  side?: string;
  proposedNotional?: string;
}) {
  const notional = exactDecisionNotional(
    item.decisionType,
    item.proposedNotional,
  );
  if (
    !item.decisionType ||
    !item.symbol ||
    !item.side ||
    notional === undefined
  )
    return undefined;
  return [item.decisionType, item.symbol, item.side, notional.toString()].join(
    "|",
  );
}

function exactDecisionInstant(value?: string) {
  if (!value) return undefined;
  const parsed = new Date(value);
  return Number.isNaN(parsed.valueOf()) ? undefined : parsed;
}

export function projectStrategyFleetProvenanceDigest(
  items: StrategyFleetItem[],
): StrategyFleetProvenanceDigest {
  const active = items.filter(
    (item) => isAI(item) && item.instanceStatus === "ACTIVE",
  );
  const engines = active.map((item) => {
    const decisionAt = exactDecisionInstant(item.latestDecisionAt);
    const marketObservedAt = exactDecisionInstant(
      item.latestDecisionMarketObservedAt,
    );
    const latestSignature = exactDecisionSignature({
      decisionType: item.latestDecisionType,
      symbol: item.latestDecisionSymbol,
      side: item.latestDecisionSide,
      proposedNotional: item.latestDecisionProposedNotional,
    });
    const hasPrior = Boolean(item.priorDecisionID);
    const priorSignature = hasPrior
      ? exactDecisionSignature({
          decisionType: item.priorDecisionType,
          symbol: item.priorDecisionSymbol,
          side: item.priorDecisionSide,
          proposedNotional: item.priorDecisionProposedNotional,
        })
      : undefined;
    const priorAt = hasPrior
      ? exactDecisionInstant(item.priorDecisionAt)
      : undefined;
    const marketSymbols = item.latestDecisionMarketSymbols ?? [];
    const uniqueMarketSymbols = new Set(marketSymbols);
    const routeAttributed = Boolean(
      item.latestDecisionAIProvider &&
        item.latestDecisionAIModelID &&
        item.latestDecisionAIProfile,
    );
    const financialAttributed = Boolean(
      item.latestDecisionFinancialContextComplete === true &&
        item.latestDecisionFinancialProvider === item.provider &&
        marketSymbols.length > 0 &&
        uniqueMarketSymbols.size === marketSymbols.length &&
        (item.latestDecisionMarketFeeds?.length ?? 0) > 0 &&
        (item.latestDecisionMarketQualities?.length ?? 0) > 0 &&
        decisionAt &&
        marketObservedAt &&
        marketObservedAt.valueOf() <= decisionAt.valueOf() + 5 * 60 * 1000,
    );
    const currentDecisionComplete = Boolean(
      item.decisionAvailable === true &&
        item.latestDecisionID &&
        decisionAt &&
        latestSignature,
    );
    const comparisonComplete = !hasPrior || Boolean(priorAt && priorSignature);
    const attributable = Boolean(
      currentDecisionComplete && routeAttributed && financialAttributed,
    );
    const state =
      !attributable || !comparisonComplete
        ? ("UNAVAILABLE" as const)
        : !hasPrior
          ? ("FIRST_DECISION" as const)
          : latestSignature === priorSignature
            ? ("HELD_COURSE" as const)
            : ("CHANGED" as const);
    const followUp =
      state === "UNAVAILABLE"
        ? "Review the saved evidence before relying on this comparison. This view will not rerun the model."
        : state === "CHANGED"
          ? "No action is required. Compare the two saved conclusions in the journal while the next guarded cycle remains automatic."
          : state === "HELD_COURSE"
            ? "No action is required. The engine held its exact conclusion and its next guarded cycle remains automatic."
            : "No action is required. The next guarded cycle will add the first exact comparison point.";
    return {
      id: item.id,
      title: item.title,
      accountName: item.accountName,
      provider: item.provider,
      executionMode: item.executionMode,
      state,
      attributable,
      latestDecisionType: item.latestDecisionType,
      priorDecisionType: item.priorDecisionType,
      latestDecisionSymbol: item.latestDecisionSymbol,
      priorDecisionSymbol: item.priorDecisionSymbol,
      latestDecisionSide: item.latestDecisionSide,
      priorDecisionSide: item.priorDecisionSide,
      latestDecisionProposedNotional: item.latestDecisionProposedNotional,
      priorDecisionProposedNotional: item.priorDecisionProposedNotional,
      latestDecisionAt: item.latestDecisionAt,
      priorDecisionAt: item.priorDecisionAt,
      aiProvider: item.latestDecisionAIProvider,
      modelID: item.latestDecisionAIModelID,
      profile: item.latestDecisionAIProfile,
      financialProvider: item.latestDecisionFinancialProvider,
      marketSymbols,
      marketFeeds: item.latestDecisionMarketFeeds ?? [],
      marketQualities: item.latestDecisionMarketQualities ?? [],
      marketObservedAt: item.latestDecisionMarketObservedAt,
      followUp,
    };
  });
  const attributableCount = engines.filter(
    (engine) => engine.attributable,
  ).length;
  const unavailable = engines.some((engine) => engine.state === "UNAVAILABLE");
  return {
    status: engines.length > 0 && !unavailable ? "VERIFIED" : "UNAVAILABLE",
    engineCount: engines.length,
    attributableCount,
    changedCount: engines.filter((engine) => engine.state === "CHANGED").length,
    heldCourseCount: engines.filter((engine) => engine.state === "HELD_COURSE")
      .length,
    firstDecisionCount: engines.filter(
      (engine) => engine.state === "FIRST_DECISION",
    ).length,
    engines,
  };
}

function inputCoverageStatus(
  values: string[],
): StrategyFleetInputCoverageStatus {
  if (values.length === 0) return "UNAVAILABLE";
  const normalized = values.map((value) => value.toUpperCase());
  const available = normalized.filter((value) =>
    ["AVAILABLE", "COMPLETE"].includes(value),
  ).length;
  const partial = normalized.filter((value) => value === "PARTIAL").length;
  if (available === normalized.length) return "AVAILABLE";
  if (available > 0 || partial > 0) return "PARTIAL";
  return "UNAVAILABLE";
}

function inputCoverageEvidence(values: string[]) {
  return values.length > 0 ? values.map(readable).join(" · ") : "Unavailable";
}

function inputCoverageLimitation(label: string) {
  return `${label} limits the evidence available to this engine. It does not establish why the model reached its conclusion.`;
}

export function projectStrategyFleetInputCoverageMatrix(
  items: StrategyFleetItem[],
): StrategyFleetInputCoverageMatrix {
  const active = items.filter(
    (item) => isAI(item) && item.instanceStatus === "ACTIVE",
  );
  const engines = active.map((item) => {
    const marketPriceStatus: StrategyFleetInputCoverageStatus =
      item.latestDecisionFinancialContextComplete === true
        ? "AVAILABLE"
        : "UNAVAILABLE";
    const historyStatuses = item.latestDecisionHistoryStatuses ?? [];
    const historySources = [
      ...(item.latestDecisionHistoryFeeds ?? []),
      ...(item.latestDecisionHistoryQualities ?? []),
    ];
    const rawHistoryStatus = inputCoverageStatus(historyStatuses);
    const historyStatus =
      item.latestDecisionHistoryLiquidityEvidenceComplete !== true
        ? ("UNAVAILABLE" as const)
        : rawHistoryStatus === "AVAILABLE" && historySources.length === 0
          ? ("UNAVAILABLE" as const)
          : rawHistoryStatus;
    const liquidityStatuses = item.latestDecisionLiquidityStatuses ?? [];
    const liquidityStatus =
      item.latestDecisionHistoryLiquidityEvidenceComplete === true
        ? inputCoverageStatus(liquidityStatuses)
        : ("UNAVAILABLE" as const);
    const positionStatuses =
      item.latestDecisionPositionPerformanceStatuses ?? [];
    const positionStatus: StrategyFleetInputCoverageStatus =
      item.latestDecisionPositionEvidenceComplete !== true
        ? "UNAVAILABLE"
        : item.latestDecisionPositionCount === 0
          ? "NOT_APPLICABLE"
          : inputCoverageStatus(positionStatuses);
    const eventStatuses = item.latestDecisionMarketEventCoverageStatuses ?? [];
    const eventSources = [
      ...(item.latestDecisionMarketEventProviders ?? []),
      ...(item.latestDecisionMarketEventFeeds ?? []),
      ...(item.latestDecisionMarketEventQualities ?? []),
    ];
    const rawEventStatus = inputCoverageStatus(eventStatuses);
    const eventStatus =
      item.latestDecisionMarketEventEvidenceComplete !== true
        ? ("UNAVAILABLE" as const)
        : rawEventStatus === "AVAILABLE" && eventSources.length < 3
          ? ("UNAVAILABLE" as const)
          : rawEventStatus;
    const marketSymbols = item.latestDecisionMarketSymbols ?? [];
    const marketSource = [
      ...(item.latestDecisionMarketFeeds ?? []),
      ...(item.latestDecisionMarketQualities ?? []),
    ];
    const attributable = Boolean(
      item.decisionAvailable === true &&
        item.latestDecisionID &&
        exactDecisionInstant(item.latestDecisionAt) &&
        item.latestDecisionInputCoverageComplete === true &&
        item.latestDecisionFinancialProvider === item.provider,
    );
    const categories: StrategyFleetInputCoverageMatrix["engines"][number]["categories"] =
      [
        {
          key: "MARKET_PRICE",
          label: "Market price",
          status: marketPriceStatus,
          evidence:
            marketPriceStatus === "AVAILABLE"
              ? `${provenanceExactList(marketSymbols)} · ${provenanceReadableList(marketSource)}`
              : "Exact saved price provenance is unavailable.",
          limitation:
            marketPriceStatus === "AVAILABLE"
              ? undefined
              : inputCoverageLimitation("Missing current market prices"),
        },
        {
          key: "HISTORY",
          label: "Price history",
          status: historyStatus,
          evidence:
            historyStatus === "UNAVAILABLE"
              ? inputCoverageEvidence(historyStatuses)
              : `${inputCoverageEvidence(historyStatuses)} · ${provenanceReadableList(historySources)}`,
          limitation:
            historyStatus === "AVAILABLE"
              ? undefined
              : inputCoverageLimitation("Incomplete saved price history"),
        },
        {
          key: "LIQUIDITY",
          label: "Liquidity",
          status: liquidityStatus,
          evidence: inputCoverageEvidence(liquidityStatuses),
          limitation:
            liquidityStatus === "AVAILABLE"
              ? undefined
              : inputCoverageLimitation("Incomplete saved liquidity context"),
        },
        {
          key: "POSITION",
          label: "Position performance",
          status: positionStatus,
          evidence:
            positionStatus === "NOT_APPLICABLE"
              ? "Exact saved position set is empty."
              : `${item.latestDecisionPositionCount ?? 0} saved position${item.latestDecisionPositionCount === 1 ? "" : "s"} · ${inputCoverageEvidence(positionStatuses)}`,
          limitation:
            positionStatus === "AVAILABLE" ||
            positionStatus === "NOT_APPLICABLE"
              ? undefined
              : inputCoverageLimitation(
                  "Incomplete saved position-performance data",
                ),
        },
        {
          key: "EVENTS",
          label: "Market events",
          status: eventStatus,
          evidence:
            eventStatus === "AVAILABLE"
              ? `${inputCoverageEvidence(eventStatuses)} · ${provenanceReadableList(eventSources)} · ${item.latestDecisionMarketEventCount ?? 0} saved events`
              : item.latestDecisionMarketEventCoverageCount === 0
                ? "No saved market-event coverage record."
                : inputCoverageEvidence(eventStatuses),
          limitation:
            eventStatus === "AVAILABLE"
              ? undefined
              : inputCoverageLimitation("Missing saved market-event coverage"),
        },
      ];
    return {
      id: item.id,
      title: item.title,
      accountName: item.accountName,
      provider: item.provider,
      executionMode: item.executionMode,
      decisionID: item.latestDecisionID,
      decisionAt: item.latestDecisionAt,
      attributable,
      financialProvider: item.latestDecisionFinancialProvider,
      categories,
    };
  });
  const categories = engines.flatMap((engine) => engine.categories);
  const attributableCount = engines.filter(
    (engine) => engine.attributable,
  ).length;
  return {
    status:
      engines.length > 0 && attributableCount === engines.length
        ? "VERIFIED"
        : "UNAVAILABLE",
    engineCount: engines.length,
    attributableCount,
    availableCategoryCount: categories.filter(
      (category) => category.status === "AVAILABLE",
    ).length,
    partialCategoryCount: categories.filter(
      (category) => category.status === "PARTIAL",
    ).length,
    unavailableCategoryCount: categories.filter(
      (category) => category.status === "UNAVAILABLE",
    ).length,
    engines,
  };
}

function priorInputCoverageItem(item: StrategyFleetItem): StrategyFleetItem {
  return {
    ...item,
    decisionAvailable: Boolean(item.priorDecisionID),
    latestDecisionID: item.priorDecisionID,
    latestDecisionAt: item.priorDecisionAt,
    latestDecisionFinancialContextComplete:
      item.priorDecisionFinancialContextComplete,
    latestDecisionFinancialProvider: item.priorDecisionFinancialProvider,
    latestDecisionMarketSymbols: item.priorDecisionMarketSymbols,
    latestDecisionMarketFeeds: item.priorDecisionMarketFeeds,
    latestDecisionMarketQualities: item.priorDecisionMarketQualities,
    latestDecisionMarketObservedAt: item.priorDecisionMarketObservedAt,
    latestDecisionInputCoverageComplete:
      item.priorDecisionInputCoverageComplete,
    latestDecisionHistoryLiquidityEvidenceComplete:
      item.priorDecisionHistoryLiquidityEvidenceComplete,
    latestDecisionHistoryStatuses: item.priorDecisionHistoryStatuses,
    latestDecisionHistoryFeeds: item.priorDecisionHistoryFeeds,
    latestDecisionHistoryQualities: item.priorDecisionHistoryQualities,
    latestDecisionLiquidityStatuses: item.priorDecisionLiquidityStatuses,
    latestDecisionPositionEvidenceComplete:
      item.priorDecisionPositionEvidenceComplete,
    latestDecisionPositionCount: item.priorDecisionPositionCount,
    latestDecisionPositionPerformanceStatuses:
      item.priorDecisionPositionPerformanceStatuses,
    latestDecisionMarketEventEvidenceComplete:
      item.priorDecisionMarketEventEvidenceComplete,
    latestDecisionMarketEventCoverageCount:
      item.priorDecisionMarketEventCoverageCount,
    latestDecisionMarketEventCoverageStatuses:
      item.priorDecisionMarketEventCoverageStatuses,
    latestDecisionMarketEventProviders: item.priorDecisionMarketEventProviders,
    latestDecisionMarketEventFeeds: item.priorDecisionMarketEventFeeds,
    latestDecisionMarketEventQualities: item.priorDecisionMarketEventQualities,
    latestDecisionMarketEventCount: item.priorDecisionMarketEventCount,
  };
}

function inputCoverageChange(
  priorStatus: StrategyFleetInputCoverageStatus,
  currentStatus: StrategyFleetInputCoverageStatus,
  comparable: boolean,
): StrategyFleetInputCoverageChange {
  if (!comparable) return "UNAVAILABLE";
  if (priorStatus === currentStatus) return "UNCHANGED";
  if (priorStatus === "NOT_APPLICABLE" || currentStatus === "NOT_APPLICABLE")
    return "CONTEXT_CHANGED";
  const rank: Record<
    Exclude<StrategyFleetInputCoverageStatus, "NOT_APPLICABLE">,
    number
  > = {
    UNAVAILABLE: 0,
    PARTIAL: 1,
    AVAILABLE: 2,
  };
  return rank[currentStatus] > rank[priorStatus] ? "IMPROVED" : "REGRESSED";
}

export function projectStrategyFleetInputCoverageChangeLedger(
  items: StrategyFleetItem[],
): StrategyFleetInputCoverageChangeLedger {
  const active = items.filter(
    (item) => isAI(item) && item.instanceStatus === "ACTIVE",
  );
  const engines = active.map((item) => {
    const current = projectStrategyFleetInputCoverageMatrix([item]).engines[0];
    const prior = projectStrategyFleetInputCoverageMatrix([
      priorInputCoverageItem(item),
    ]).engines[0];
    const comparable = Boolean(
      current?.attributable &&
        prior?.attributable &&
        item.latestDecisionFinancialProvider === item.provider &&
        item.priorDecisionFinancialProvider === item.provider,
    );
    const categories = (current?.categories ?? []).map((currentCategory) => {
      const priorCategory = prior?.categories.find(
        (category) => category.key === currentCategory.key,
      );
      const priorStatus = priorCategory?.status ?? "UNAVAILABLE";
      return {
        key: currentCategory.key,
        label: currentCategory.label,
        priorStatus,
        currentStatus: currentCategory.status,
        change: inputCoverageChange(
          priorStatus,
          currentCategory.status,
          comparable,
        ),
        priorEvidence: priorCategory?.evidence ?? "Unavailable",
        currentEvidence: currentCategory.evidence,
      };
    });
    const regressed = categories.filter(
      (category) => category.change === "REGRESSED",
    ).length;
    const contextChanged = categories.filter(
      (category) => category.change === "CONTEXT_CHANGED",
    ).length;
    const followUp = !comparable
      ? "Review the two saved records before relying on this comparison. Arbion will not rerun the model to fill the gap."
      : regressed > 0
        ? "Review the exact regressed categories in the two immutable records. The next guarded cycle remains automatic."
        : contextChanged > 0
          ? "Review the saved position-context change without treating it as a quality improvement or regression."
          : "No owner action is required. Input coverage held course or improved, and the next guarded cycle remains automatic.";
    return {
      id: item.id,
      title: item.title,
      accountName: item.accountName,
      provider: item.provider,
      executionMode: item.executionMode,
      latestDecisionID: item.latestDecisionID,
      priorDecisionID: item.priorDecisionID,
      latestDecisionAt: item.latestDecisionAt,
      priorDecisionAt: item.priorDecisionAt,
      financialProvider: item.latestDecisionFinancialProvider,
      comparable,
      followUp,
      categories,
    };
  });
  const categories = engines.flatMap((engine) => engine.categories);
  const comparableCount = engines.filter((engine) => engine.comparable).length;
  return {
    status:
      engines.length > 0 && comparableCount === engines.length
        ? "VERIFIED"
        : "UNAVAILABLE",
    engineCount: engines.length,
    comparableCount,
    improvedCategoryCount: categories.filter(
      (category) => category.change === "IMPROVED",
    ).length,
    regressedCategoryCount: categories.filter(
      (category) => category.change === "REGRESSED",
    ).length,
    unchangedCategoryCount: categories.filter(
      (category) => category.change === "UNCHANGED",
    ).length,
    contextChangedCategoryCount: categories.filter(
      (category) => category.change === "CONTEXT_CHANGED",
    ).length,
    engines,
  };
}

function snapshotInputCoverageItem(
  item: StrategyFleetItem,
  snapshot: StrategyFleetDecisionInputCoverageSnapshot,
): StrategyFleetItem {
  return {
    ...item,
    decisionAvailable: Boolean(snapshot.decisionID),
    latestDecisionID: snapshot.decisionID,
    latestDecisionAt: snapshot.decisionAt,
    latestDecisionFinancialContextComplete: snapshot.financialContextComplete,
    latestDecisionFinancialProvider: snapshot.financialProvider,
    latestDecisionMarketSymbols: snapshot.marketSymbols,
    latestDecisionMarketFeeds: snapshot.marketFeeds,
    latestDecisionMarketQualities: snapshot.marketQualities,
    latestDecisionMarketObservedAt: snapshot.marketObservedAt,
    latestDecisionInputCoverageComplete: snapshot.inputCoverageComplete,
    latestDecisionHistoryLiquidityEvidenceComplete:
      snapshot.historyLiquidityEvidenceComplete,
    latestDecisionHistoryStatuses: snapshot.historyStatuses,
    latestDecisionHistoryFeeds: snapshot.historyFeeds,
    latestDecisionHistoryQualities: snapshot.historyQualities,
    latestDecisionLiquidityStatuses: snapshot.liquidityStatuses,
    latestDecisionPositionEvidenceComplete: snapshot.positionEvidenceComplete,
    latestDecisionPositionCount: snapshot.positionCount,
    latestDecisionPositionPerformanceStatuses:
      snapshot.positionPerformanceStatuses,
    latestDecisionMarketEventEvidenceComplete:
      snapshot.marketEventEvidenceComplete,
    latestDecisionMarketEventCoverageCount: snapshot.marketEventCoverageCount,
    latestDecisionMarketEventCoverageStatuses:
      snapshot.marketEventCoverageStatuses,
    latestDecisionMarketEventProviders: snapshot.marketEventProviders,
    latestDecisionMarketEventFeeds: snapshot.marketEventFeeds,
    latestDecisionMarketEventQualities: snapshot.marketEventQualities,
    latestDecisionMarketEventCount: snapshot.marketEventCount,
  };
}

function persistentInputGapState(
  statuses: StrategyFleetInputCoverageStatus[],
  attributable: boolean,
): StrategyFleetPersistentInputGapState {
  if (!attributable || statuses.length < 2) return "UNAVAILABLE";
  const current = statuses[0];
  const older = statuses.slice(1);
  if (statuses.every((status) => status === "NOT_APPLICABLE")) return "CLEAR";
  if (statuses.some((status) => status === "NOT_APPLICABLE"))
    return "CONTEXT_CHANGED";
  const isGap = (status: StrategyFleetInputCoverageStatus) =>
    status === "PARTIAL" || status === "UNAVAILABLE";
  if (!isGap(current))
    return older.some(isGap) ? "CURRENTLY_RESOLVED" : "CLEAR";
  if (statuses.every(isGap)) return "PERSISTENT";
  if (!older.some(isGap)) return "NEWLY_MISSING";
  return "INTERMITTENT";
}

export function projectStrategyFleetPersistentInputGapRegister(
  items: StrategyFleetItem[],
): StrategyFleetPersistentInputGapRegister {
  const active = items.filter(
    (item) => isAI(item) && item.instanceStatus === "ACTIVE",
  );
  const trackedKeys = ["HISTORY", "LIQUIDITY", "POSITION", "EVENTS"] as const;
  const engines = active.map((item) => {
    const snapshots = [...(item.recentDecisionInputCoverage ?? [])]
      .slice(0, 6)
      .sort(
        (left, right) =>
          new Date(right.decisionAt ?? "").valueOf() -
          new Date(left.decisionAt ?? "").valueOf(),
      );
    const projections = snapshots.map(
      (snapshot) =>
        projectStrategyFleetInputCoverageMatrix([
          snapshotInputCoverageItem(item, snapshot),
        ]).engines[0],
    );
    const decisionIDs = snapshots
      .map((snapshot) => snapshot.decisionID)
      .filter((value): value is string => Boolean(value));
    const decisionTimes = snapshots
      .map((snapshot) => snapshot.decisionAt)
      .filter((value): value is string => Boolean(value));
    const attributable = Boolean(
      snapshots.length >= 2 &&
        decisionIDs.length === snapshots.length &&
        new Set(decisionIDs).size === snapshots.length &&
        decisionTimes.length === snapshots.length &&
        new Set(decisionTimes).size === snapshots.length &&
        decisionTimes.every((value) => exactDecisionInstant(value)) &&
        projections.every(
          (projection) =>
            projection?.attributable &&
            projection.financialProvider === item.provider,
        ),
    );
    const categories = trackedKeys.map((key) => {
      const observations = projections.map((projection, index) => ({
        status:
          projection?.categories.find((category) => category.key === key)
            ?.status ?? ("UNAVAILABLE" as const),
        decisionAt: snapshots[index]?.decisionAt,
      }));
      const gapObservations = observations.filter(
        (observation) =>
          observation.status === "PARTIAL" ||
          observation.status === "UNAVAILABLE",
      );
      const label =
        projections[0]?.categories.find((category) => category.key === key)
          ?.label ?? readable(key);
      return {
        key,
        label,
        state: persistentInputGapState(
          observations.map((observation) => observation.status),
          attributable,
        ),
        currentStatus: observations[0]?.status ?? ("UNAVAILABLE" as const),
        sampleCount: snapshots.length,
        gapSampleCount: attributable ? gapObservations.length : 0,
        firstGapAt: attributable
          ? gapObservations[gapObservations.length - 1]?.decisionAt
          : undefined,
        latestGapAt: attributable ? gapObservations[0]?.decisionAt : undefined,
      };
    });
    const hasCurrentGap = categories.some((category) =>
      ["PERSISTENT", "INTERMITTENT", "NEWLY_MISSING"].includes(category.state),
    );
    const hasContextChange = categories.some(
      (category) => category.state === "CONTEXT_CHANGED",
    );
    const followUp = !attributable
      ? "Review the saved decision window before relying on this register. Arbion will not rerun the model or refresh a provider to fill the gap."
      : hasCurrentGap
        ? "Review the linked immutable records for the registered gaps. The next guarded cycle remains automatic; no live action is available."
        : hasContextChange
          ? "Review the saved position-context change without treating it as an input improvement or regression."
          : "No owner action is required. No current repeated input gap needs intervention, and the next guarded cycle remains automatic.";
    return {
      id: item.id,
      title: item.title,
      accountName: item.accountName,
      provider: item.provider,
      executionMode: item.executionMode,
      financialProvider: snapshots[0]?.financialProvider,
      attributable,
      sampleCount: snapshots.length,
      windowStartedAt: snapshots[snapshots.length - 1]?.decisionAt,
      windowEndedAt: snapshots[0]?.decisionAt,
      followUp,
      categories,
    };
  });
  const categories = engines.flatMap((engine) => engine.categories);
  const attributableCount = engines.filter(
    (engine) => engine.attributable,
  ).length;
  return {
    status:
      engines.length > 0 && attributableCount === engines.length
        ? "VERIFIED"
        : "UNAVAILABLE",
    engineCount: engines.length,
    attributableCount,
    persistentCount: categories.filter(
      (category) => category.state === "PERSISTENT",
    ).length,
    intermittentCount: categories.filter(
      (category) => category.state === "INTERMITTENT",
    ).length,
    newlyMissingCount: categories.filter(
      (category) => category.state === "NEWLY_MISSING",
    ).length,
    resolvedCount: categories.filter(
      (category) => category.state === "CURRENTLY_RESOLVED",
    ).length,
    engines,
  };
}

function healthClass(item: StrategyFleetItem) {
  if (needsReview(item)) return "needs-review";
  return item.instanceStatus === "ACTIVE" ? "healthy" : "neutral";
}

function modelLabel(item: StrategyFleetItem) {
  if (requiresRuntimeContract(item) && !runtimeContractHealthy(item))
    return "Pinned model unavailable";
  if (isAI(item)) return item.modelID ?? "Model not recorded";
  return "Deterministic rules";
}

function fleetCoverageLabel(item: StrategyFleetItem) {
  if (requiresRuntimeContract(item) && !runtimeContractHealthy(item))
    return "Pinned universe unavailable";
  return coverageLabel(item.symbols);
}

function coverageLabel(symbols: string[]) {
  if (symbols.length === 0) return "Not defined";
  if (symbols.length <= 3) return symbols.join(" · ");
  return `${symbols.slice(0, 3).join(" · ")} +${symbols.length - 3}`;
}

function boundedProgress(
  value: number | undefined,
  maximum: number | undefined,
) {
  if (
    value === undefined ||
    maximum === undefined ||
    !Number.isFinite(value) ||
    !Number.isFinite(maximum) ||
    value < 0 ||
    maximum <= 0
  )
    return undefined;
  return { value: Math.min(value, maximum), maximum };
}

function latestDecisionLabel(value: string) {
  if (value === "ABSTAIN") return "Abstained";
  if (value === "DENY_RISK_DENIED") return "Held by controls";
  if (value === "ALLOW_WOULD_HAVE_SUBMITTED") return "Would have submitted";
  if (value === "ALLOW_SIMULATED_FILLED") return "Simulated fill";
  if (value === "ALLOW_SIMULATED_REJECTED") return "Simulation rejected";
  return readable(value);
}

function latestActionLabel(item: StrategyFleetItem) {
  const symbol =
    item.latestDecisionSymbol && item.latestDecisionSymbol !== "NONE"
      ? item.latestDecisionSymbol
      : undefined;
  if (!symbol) return "No action proposed";
  return [
    item.latestDecisionSide && readable(item.latestDecisionSide),
    item.latestDecisionQuantity,
    symbol,
  ]
    .filter(Boolean)
    .join(" · ");
}

function latestRiskLabel(item: StrategyFleetItem) {
  if (item.latestDecisionRiskDecision === "ALLOW")
    return "Allowed by deterministic controls";
  if (item.latestDecisionRiskDecision === "DENY") {
    const [first, ...remaining] = item.latestDecisionRiskReasons ?? [];
    if (!first) return "Denied by deterministic controls";
    return `${readable(first)}${remaining.length ? ` +${remaining.length}` : ""}`;
  }
  return "Risk gate not reached";
}

function latestExecutionLabel(value?: string) {
  if (value === "WOULD_HAVE_SUBMITTED") return "Shadow record only";
  if (value === "SIMULATED_FILLED") return "Paper simulated fill only";
  if (value === "SIMULATED_REJECTED") return "Paper simulation rejected";
  if (!value) return "No execution record";
  return `${readable(value)} · non-live`;
}

function aiProviderLabel(value: string) {
  if (value === "openai") return "OpenAI";
  if (value === "anthropic") return "Claude";
  if (value === "gemini") return "Gemini";
  return readable(value);
}

function latestRouteLabel(item: StrategyFleetItem) {
  if (
    !item.latestDecisionAIProvider ||
    !item.latestDecisionAIModelID ||
    !item.latestDecisionAIProfile
  )
    return "Unattributed legacy route";
  return `${aiProviderLabel(item.latestDecisionAIProvider)} · ${item.latestDecisionAIModelID} · ${readable(item.latestDecisionAIProfile)}`;
}

function latestTelemetryLabel(item: StrategyFleetItem) {
  const values = [
    item.latestDecisionLatencyMS,
    item.latestDecisionInputUsage,
    item.latestDecisionOutputUsage,
  ];
  if (
    values.some(
      (value) => value === undefined || !Number.isFinite(value) || value < 0,
    )
  )
    return "Telemetry unavailable";
  const integer = new Intl.NumberFormat("en-US", { maximumFractionDigits: 0 });
  return `${integer.format(item.latestDecisionLatencyMS ?? 0)} ms · ${integer.format(item.latestDecisionInputUsage ?? 0)} in / ${integer.format(item.latestDecisionOutputUsage ?? 0)} out`;
}

function exactState(value?: string) {
  return value ? readable(value) : "Unavailable";
}

function conciseCapitalDecimal(value?: string) {
  if (!value || !/^\d+(\.\d{1,10})?$/.test(value)) return undefined;
  const [whole, fraction = ""] = value.split(".");
  const grouped = whole.replace(/\B(?=(\d{3})+(?!\d))/g, ",");
  const compact = fraction.replace(/0+$/, "");
  return `${grouped}${compact ? `.${compact}` : ""}`;
}

function capitalMoney(currency?: string, value?: string) {
  const amount = conciseCapitalDecimal(value);
  if (!amount || !currency || !/^[A-Z]{3}$/.test(currency))
    return "Unavailable";
  return currency === "USD" ? `$${amount}` : `${currency} ${amount}`;
}

function operatingStatusLabel(
  status: StrategyFleetOperatingBrief["engines"][number]["status"],
) {
  if (status === "ON_COURSE") return "On course";
  if (status === "REVIEW") return "Review";
  return "Unavailable";
}

function StrategyFleetOperatingBrief({
  items,
}: {
  items: StrategyFleetItem[];
}) {
  const brief = projectStrategyFleetOperatingBrief(items);
  if (brief.engineCount === 0) return null;
  const title =
    brief.status === "ON_COURSE"
      ? "Your AI engines are continuing safely."
      : brief.status === "REVIEW"
        ? "An AI engine has a clear review step."
        : "The current operating brief is incomplete.";
  return (
    <section
      className={`strategy-fleet-operating-brief is-${brief.status.toLowerCase().replace("_", "-")}`}
      aria-labelledby="strategy-fleet-operating-brief-heading"
    >
      <header>
        <div>
          <p className="eyebrow">OWNER OPERATING BRIEF</p>
          <h2 id="strategy-fleet-operating-brief-heading">{title}</h2>
          <p>
            A plain-language view of what each engine concluded, why Arbion is
            holding or continuing, what happens next, and where its immutable
            evidence lives.
          </p>
        </div>
        <span>
          {brief.reviewCount === 0
            ? "No owner action"
            : `${brief.reviewCount} to review`}
        </span>
      </header>
      <div className="strategy-fleet-operating-brief-summary">
        <strong>
          {brief.onCourseCount} of {brief.engineCount} on course
        </strong>
        <span>
          Paper and Shadow remain isolated · no broker order · no live path
        </span>
      </div>
      <ol>
        {brief.engines.map((engine) => (
          <li
            className={`is-${engine.status.toLowerCase().replace("_", "-")}`}
            key={engine.id}
          >
            <header>
              <div>
                <span
                  className={`provider-mark provider-${engine.provider}`}
                  aria-hidden="true"
                >
                  {providerInitial(engine.provider)}
                </span>
                <p>
                  <small>
                    {engine.accountName} · {readable(engine.executionMode)}
                  </small>
                  <strong>{engine.title}</strong>
                </p>
              </div>
              <span>{operatingStatusLabel(engine.status)}</span>
            </header>
            <dl>
              <div>
                <dt>What it concluded</dt>
                <dd>{engine.conclusion}</dd>
                {engine.decisionAt && (
                  <time dateTime={engine.decisionAt}>
                    {readableTime(engine.decisionAt)}
                  </time>
                )}
              </div>
              <div>
                <dt>
                  {engine.status === "ON_COURSE"
                    ? "Why this is okay"
                    : "Why review"}
                </dt>
                <dd>{engine.explanation}</dd>
              </div>
              <div>
                <dt>What happens next</dt>
                <dd>{engine.nextStep}</dd>
                {engine.nextRunAt && (
                  <time dateTime={engine.nextRunAt}>
                    {readableTime(engine.nextRunAt)}
                  </time>
                )}
              </div>
            </dl>
            <footer>
              <div>
                <Link href={engine.detailHref}>Open engine evidence →</Link>
                <Link href="/activity">Open decision journal →</Link>
              </div>
              {engine.reviewHref && engine.reviewLabel && (
                <Link href={engine.reviewHref}>{engine.reviewLabel} →</Link>
              )}
            </footer>
          </li>
        ))}
      </ol>
      <footer>
        Read-only explanation from current owner-scoped records · no model rerun
        · no provider call
      </footer>
    </section>
  );
}

function provenanceDigestStateLabel(
  state: StrategyFleetProvenanceDigest["engines"][number]["state"],
) {
  if (state === "CHANGED") return "Conclusion changed";
  if (state === "HELD_COURSE") return "Held course";
  if (state === "FIRST_DECISION") return "First decision";
  return "Evidence unavailable";
}

function provenanceReadableList(values: string[]) {
  return values.length > 0 ? values.map(readable).join(" · ") : "Unavailable";
}

function provenanceExactList(values: string[]) {
  return values.length > 0 ? values.join(" · ") : "Unavailable";
}

function provenanceDecisionAction(
  decisionType?: string,
  side?: string,
  symbol?: string,
  proposedNotional?: string,
) {
  if (decisionType === "ABSTAIN") return "No action proposed · $0";
  const notional = capitalMoney("USD", proposedNotional);
  if (!side || !symbol || notional === "Unavailable")
    return "Exact action unavailable";
  return `${readable(side)} ${symbol} · ${notional}`;
}

function StrategyFleetProvenanceDigestView({
  items,
}: {
  items: StrategyFleetItem[];
}) {
  const digest = projectStrategyFleetProvenanceDigest(items);
  if (digest.engineCount === 0) return null;
  const verified = digest.status === "VERIFIED";
  return (
    <section
      className={`strategy-fleet-provenance-digest ${verified ? "is-verified" : "needs-review"}`}
      aria-labelledby="strategy-fleet-provenance-digest-heading"
    >
      <header>
        <div>
          <p className="eyebrow">DECISION CHANGE + PROVENANCE</p>
          <h2 id="strategy-fleet-provenance-digest-heading">
            {verified
              ? `${digest.attributableCount} current AI ${digest.attributableCount === 1 ? "decision is" : "decisions are"} fully attributable.`
              : "A current AI decision comparison is incomplete."}
          </h2>
          <p>
            Compare each engine&apos;s two newest immutable conclusions and
            verify the exact model route, financial provider, symbols, feed,
            quality, and observation time behind the current one.
          </p>
        </div>
        <span>{verified ? "Complete" : "Review evidence"}</span>
      </header>
      <dl className="strategy-fleet-provenance-digest-summary">
        <div>
          <dt>Attributable now</dt>
          <dd>
            {digest.attributableCount} / {digest.engineCount}
          </dd>
        </div>
        <div>
          <dt>Changed conclusion</dt>
          <dd>{digest.changedCount}</dd>
        </div>
        <div>
          <dt>Held course</dt>
          <dd>{digest.heldCourseCount}</dd>
        </div>
        <div>
          <dt>First decision</dt>
          <dd>{digest.firstDecisionCount}</dd>
        </div>
      </dl>
      <ol className="strategy-fleet-provenance-digest-list">
        {digest.engines.map((engine) => (
          <li
            className={
              engine.state === "UNAVAILABLE" ? "needs-review" : "is-verified"
            }
            key={engine.id}
          >
            <header>
              <div>
                <span
                  className={`provider-mark provider-${engine.provider}`}
                  aria-hidden="true"
                >
                  {providerInitial(engine.provider)}
                </span>
                <p>
                  <small>
                    {engine.accountName} · {readable(engine.executionMode)}
                  </small>
                  <strong>{engine.title}</strong>
                </p>
              </div>
              <span>{provenanceDigestStateLabel(engine.state)}</span>
            </header>
            <div className="strategy-fleet-provenance-comparison">
              <article>
                <span>Previous</span>
                <strong>
                  {engine.priorDecisionType
                    ? latestDecisionLabel(engine.priorDecisionType)
                    : "No earlier decision"}
                </strong>
                <small>
                  {engine.priorDecisionType
                    ? provenanceDecisionAction(
                        engine.priorDecisionType,
                        engine.priorDecisionSide,
                        engine.priorDecisionSymbol,
                        engine.priorDecisionProposedNotional,
                      )
                    : "First saved comparison point"}
                </small>
                <time dateTime={engine.priorDecisionAt}>
                  {readableTime(engine.priorDecisionAt)}
                </time>
              </article>
              <span aria-hidden="true">→</span>
              <article>
                <span>Current</span>
                <strong>
                  {engine.latestDecisionType
                    ? latestDecisionLabel(engine.latestDecisionType)
                    : "Unavailable"}
                </strong>
                <small>
                  {provenanceDecisionAction(
                    engine.latestDecisionType,
                    engine.latestDecisionSide,
                    engine.latestDecisionSymbol,
                    engine.latestDecisionProposedNotional,
                  )}
                </small>
                <time dateTime={engine.latestDecisionAt}>
                  {readableTime(engine.latestDecisionAt)}
                </time>
              </article>
            </div>
            <dl>
              <div>
                <dt>AI route</dt>
                <dd>
                  {engine.aiProvider && engine.modelID && engine.profile
                    ? `${aiProviderLabel(engine.aiProvider)} · ${engine.modelID} · ${readable(engine.profile)}`
                    : "Unavailable"}
                </dd>
              </div>
              <div>
                <dt>Financial source</dt>
                <dd>
                  {engine.financialProvider
                    ? providerLabel(engine.financialProvider)
                    : "Unavailable"}
                </dd>
              </div>
              <div>
                <dt>Exact markets</dt>
                <dd>{provenanceExactList(engine.marketSymbols)}</dd>
              </div>
              <div>
                <dt>Feed + quality</dt>
                <dd>
                  {provenanceReadableList(engine.marketFeeds)} ·{" "}
                  {provenanceReadableList(engine.marketQualities)}
                </dd>
              </div>
            </dl>
            <footer>
              <p>
                <strong>Safe follow-up</strong>
                <span>{engine.followUp}</span>
              </p>
              <div>
                <time dateTime={engine.marketObservedAt}>
                  Market observed {readableTime(engine.marketObservedAt)}
                </time>
                <Link href="/activity">Compare immutable records →</Link>
              </div>
            </footer>
          </li>
        ))}
      </ol>
      <footer>
        Owner-scoped saved evidence only · no model rerun · no provider refresh
        · no broker order
      </footer>
    </section>
  );
}

function inputCoverageStatusLabel(status: StrategyFleetInputCoverageStatus) {
  if (status === "NOT_APPLICABLE") return "Not applicable";
  return readable(status);
}

function StrategyFleetInputCoverageMatrixView({
  items,
}: {
  items: StrategyFleetItem[];
}) {
  const matrix = projectStrategyFleetInputCoverageMatrix(items);
  if (matrix.engineCount === 0) return null;
  const verified = matrix.status === "VERIFIED";
  return (
    <section
      className={`strategy-fleet-input-matrix ${verified ? "is-verified" : "needs-review"}`}
      aria-labelledby="strategy-fleet-input-matrix-heading"
    >
      <header>
        <div>
          <p className="eyebrow">AI INPUT COVERAGE</p>
          <h2 id="strategy-fleet-input-matrix-heading">
            {verified
              ? "Every current engine input is exactly attributable."
              : "A current engine input record is incomplete."}
          </h2>
          <p>
            See what each Paper or Shadow engine actually had available when it
            saved its newest decision. Missing evidence stays unavailable;
            Arbion never fills a gap by inference.
          </p>
        </div>
        <span>{verified ? "Exact saved inputs" : "Review evidence"}</span>
      </header>
      <dl className="strategy-fleet-input-matrix-summary">
        <div>
          <dt>Attributable engines</dt>
          <dd>
            {matrix.attributableCount} / {matrix.engineCount}
          </dd>
        </div>
        <div>
          <dt>Available categories</dt>
          <dd>{matrix.availableCategoryCount}</dd>
        </div>
        <div>
          <dt>Partial categories</dt>
          <dd>{matrix.partialCategoryCount}</dd>
        </div>
        <div>
          <dt>Unavailable categories</dt>
          <dd>{matrix.unavailableCategoryCount}</dd>
        </div>
      </dl>
      <ol className="strategy-fleet-input-matrix-list">
        {matrix.engines.map((engine) => (
          <li
            className={engine.attributable ? "is-verified" : "needs-review"}
            key={engine.id}
          >
            <header>
              <div>
                <span
                  className={`provider-mark provider-${engine.provider}`}
                  aria-hidden="true"
                >
                  {providerInitial(engine.provider)}
                </span>
                <p>
                  <small>
                    {engine.accountName} · {readable(engine.executionMode)}
                  </small>
                  <strong>{engine.title}</strong>
                </p>
              </div>
              <span>{engine.attributable ? "Attributed" : "Unavailable"}</span>
            </header>
            <div className="strategy-fleet-input-source">
              <span>Financial input provider</span>
              <strong>
                {engine.financialProvider
                  ? providerLabel(engine.financialProvider)
                  : "Unavailable"}
              </strong>
              <time dateTime={engine.decisionAt}>
                Decision saved {readableTime(engine.decisionAt)}
              </time>
            </div>
            <dl>
              {engine.categories.map((category) => (
                <div
                  className={`is-${category.status.toLowerCase().replaceAll("_", "-")}`}
                  key={category.key}
                >
                  <dt>
                    <span>{category.label}</span>
                    <strong>{inputCoverageStatusLabel(category.status)}</strong>
                  </dt>
                  <dd>{category.evidence}</dd>
                  {category.limitation && <small>{category.limitation}</small>}
                </div>
              ))}
            </dl>
            <footer>
              <span>
                An unavailable input limits evidence; it does not prove why the
                model chose its conclusion.
              </span>
              <Link href="/activity">Open immutable evidence →</Link>
            </footer>
          </li>
        ))}
      </ol>
      <footer>
        Read-only current decision evidence · no model rerun · no provider
        refresh · no broker order · no live path
      </footer>
    </section>
  );
}

function inputCoverageChangeLabel(change: StrategyFleetInputCoverageChange) {
  if (change === "CONTEXT_CHANGED") return "Context changed";
  return readable(change);
}

function StrategyFleetInputCoverageChangeLedgerView({
  items,
}: {
  items: StrategyFleetItem[];
}) {
  const ledger = projectStrategyFleetInputCoverageChangeLedger(items);
  if (ledger.engineCount === 0) return null;
  const verified = ledger.status === "VERIFIED";
  return (
    <section
      className={`strategy-fleet-input-change ${verified ? "is-verified" : "needs-review"}`}
      aria-labelledby="strategy-fleet-input-change-heading"
    >
      <header>
        <div>
          <p className="eyebrow">INPUT COVERAGE CHANGE LEDGER</p>
          <h2 id="strategy-fleet-input-change-heading">
            {verified
              ? `${ledger.comparableCount} current AI ${ledger.comparableCount === 1 ? "engine has" : "engines have"} an exact input comparison.`
              : "An AI input comparison is incomplete."}
          </h2>
          <p>
            Compare the two newest immutable input snapshots without rerunning
            the model. Improved, regressed, and unchanged describe evidence
            coverage only—not decision quality or causality.
          </p>
        </div>
        <span>{verified ? "Immutable comparison" : "Review evidence"}</span>
      </header>
      <dl className="strategy-fleet-input-change-summary">
        <div>
          <dt>Comparable engines</dt>
          <dd>
            {ledger.comparableCount} / {ledger.engineCount}
          </dd>
        </div>
        <div>
          <dt>Improved inputs</dt>
          <dd>{ledger.improvedCategoryCount}</dd>
        </div>
        <div>
          <dt>Regressed inputs</dt>
          <dd>{ledger.regressedCategoryCount}</dd>
        </div>
        <div>
          <dt>Unchanged inputs</dt>
          <dd>{ledger.unchangedCategoryCount}</dd>
        </div>
      </dl>
      <ol className="strategy-fleet-input-change-list">
        {ledger.engines.map((engine) => (
          <li
            className={engine.comparable ? "is-verified" : "needs-review"}
            key={engine.id}
          >
            <header>
              <div>
                <span
                  className={`provider-mark provider-${engine.provider}`}
                  aria-hidden="true"
                >
                  {providerInitial(engine.provider)}
                </span>
                <p>
                  <small>
                    {engine.accountName} · {readable(engine.executionMode)}
                  </small>
                  <strong>{engine.title}</strong>
                </p>
              </div>
              <span>{engine.comparable ? "Comparable" : "Unavailable"}</span>
            </header>
            <div className="strategy-fleet-input-change-source">
              <strong>
                {engine.financialProvider
                  ? providerLabel(engine.financialProvider)
                  : "Provider unavailable"}
              </strong>
              <span>
                {readableTime(engine.priorDecisionAt)} →{" "}
                {readableTime(engine.latestDecisionAt)}
              </span>
            </div>
            <dl>
              {engine.categories.map((category) => (
                <div
                  className={`is-${category.change.toLowerCase().replaceAll("_", "-")}`}
                  key={category.key}
                >
                  <dt>
                    <span>{category.label}</span>
                    <strong>{inputCoverageChangeLabel(category.change)}</strong>
                  </dt>
                  <dd>
                    {inputCoverageStatusLabel(category.priorStatus)} →{" "}
                    {inputCoverageStatusLabel(category.currentStatus)}
                  </dd>
                  <small>
                    Before: {category.priorEvidence}
                    <br />
                    Now: {category.currentEvidence}
                  </small>
                </div>
              ))}
            </dl>
            <footer>
              <span>{engine.followUp}</span>
              <Link href="/activity">Compare immutable records →</Link>
            </footer>
          </li>
        ))}
      </ol>
      <footer>
        Evidence coverage only · no causal claim · no model rerun · no provider
        refresh · no broker order · no live path
      </footer>
    </section>
  );
}

function persistentInputGapStateLabel(
  state: StrategyFleetPersistentInputGapState,
) {
  if (state === "NEWLY_MISSING") return "Newly missing";
  if (state === "CURRENTLY_RESOLVED") return "Currently resolved";
  if (state === "CONTEXT_CHANGED") return "Context changed";
  return readable(state);
}

function StrategyFleetPersistentInputGapRegisterView({
  items,
}: {
  items: StrategyFleetItem[];
}) {
  const register = projectStrategyFleetPersistentInputGapRegister(items);
  if (register.engineCount === 0) return null;
  const verified = register.status === "VERIFIED";
  return (
    <section
      className={`strategy-fleet-input-gaps ${verified ? "is-verified" : "needs-review"}`}
      aria-labelledby="strategy-fleet-input-gaps-heading"
    >
      <header>
        <div>
          <p className="eyebrow">PERSISTENT AI INPUT GAPS</p>
          <h2 id="strategy-fleet-input-gaps-heading">
            {verified
              ? `${register.attributableCount} active AI ${register.attributableCount === 1 ? "engine has" : "engines have"} a bounded input history.`
              : "An AI input history is not yet fully attributable."}
          </h2>
          <p>
            Track repeated gaps across up to six recent immutable decisions per
            engine. Persistent, intermittent, and resolved describe saved
            evidence only—they never explain or grade an AI conclusion.
          </p>
        </div>
        <span>{verified ? "Bounded saved history" : "Review evidence"}</span>
      </header>
      <dl className="strategy-fleet-input-gaps-summary">
        <div>
          <dt>Attributable engines</dt>
          <dd>
            {register.attributableCount} / {register.engineCount}
          </dd>
        </div>
        <div>
          <dt>Persistent gaps</dt>
          <dd>{register.persistentCount}</dd>
        </div>
        <div>
          <dt>Intermittent + new</dt>
          <dd>{register.intermittentCount + register.newlyMissingCount}</dd>
        </div>
        <div>
          <dt>Currently resolved</dt>
          <dd>{register.resolvedCount}</dd>
        </div>
      </dl>
      <ol className="strategy-fleet-input-gaps-list">
        {register.engines.map((engine) => (
          <li
            className={engine.attributable ? "is-verified" : "needs-review"}
            key={engine.id}
          >
            <header>
              <div>
                <span
                  className={`provider-mark provider-${engine.provider}`}
                  aria-hidden="true"
                >
                  {providerInitial(engine.provider)}
                </span>
                <p>
                  <small>
                    {engine.accountName} · {readable(engine.executionMode)}
                  </small>
                  <strong>{engine.title}</strong>
                </p>
              </div>
              <span>{engine.attributable ? "Attributed" : "Unavailable"}</span>
            </header>
            <div className="strategy-fleet-input-gaps-window">
              <strong>
                {engine.financialProvider
                  ? providerLabel(engine.financialProvider)
                  : "Provider unavailable"}
              </strong>
              <span>{engine.sampleCount} immutable decision samples</span>
              <time dateTime={engine.windowStartedAt}>
                {readableTime(engine.windowStartedAt)} →{" "}
                {readableTime(engine.windowEndedAt)}
              </time>
            </div>
            <dl>
              {engine.categories.map((category) => (
                <div
                  className={`is-${category.state.toLowerCase().replaceAll("_", "-")}`}
                  key={category.key}
                >
                  <dt>
                    <span>{category.label}</span>
                    <strong>
                      {persistentInputGapStateLabel(category.state)}
                    </strong>
                  </dt>
                  <dd>
                    Current: {inputCoverageStatusLabel(category.currentStatus)}
                    {engine.attributable &&
                      ` · gap samples ${category.gapSampleCount}/${category.sampleCount}`}
                  </dd>
                  <small>
                    {category.gapSampleCount > 0
                      ? `First saved gap ${readableTime(category.firstGapAt)} · latest saved gap ${readableTime(category.latestGapAt)}`
                      : category.state === "CONTEXT_CHANGED"
                        ? "The saved position context changed inside this window."
                        : "No exact gap observation is registered in this window."}
                  </small>
                </div>
              ))}
            </dl>
            <footer>
              <span>{engine.followUp}</span>
              <Link href="/activity">Review immutable window →</Link>
            </footer>
          </li>
        ))}
      </ol>
      <footer>
        Up to six owner-scoped saved decisions per engine · no inferred values ·
        no causal claim · no model rerun · no provider refresh · no broker order
        · no live path
      </footer>
    </section>
  );
}

function StrategyFleetAccountIsolationMap({
  items,
}: {
  items: StrategyFleetItem[];
}) {
  const accounts = projectStrategyFleetAccountIsolation(items);
  const identity = projectStrategyFleetIdentityIsolation(items);
  if (accounts.length === 0) return null;

  return (
    <section
      className="strategy-fleet-isolation"
      aria-labelledby="strategy-fleet-isolation-heading"
    >
      <header>
        <div>
          <p className="eyebrow">ACCOUNT ISOLATION MAP</p>
          <h2 id="strategy-fleet-isolation-heading">
            Every engine stays inside its own policy claim.
          </h2>
          <p>
            A read-only account view of non-live capital boundaries. Shared
            Shadow claims require compatible account policy; Paper claims stay
            in separate simulated ledgers and never consume broker cash.
          </p>
        </div>
        <Link href="/capital">Open capital center →</Link>
      </header>
      <div
        className={`strategy-fleet-identity-proof ${identity.status === "VERIFIED" ? "is-verified" : "needs-review"}`}
        role="region"
        aria-label="Fleet identity isolation proof"
      >
        <header>
          <div>
            <span>FLEET IDENTITY PROOF</span>
            <strong>
              {identity.status === "VERIFIED" ? "Verified" : "Review required"}
            </strong>
          </div>
          <small>
            {identity.paperEngineCount} Paper · {identity.shadowEngineCount}{" "}
            Shadow
          </small>
        </header>
        <dl>
          <div>
            <dt>Operational engines</dt>
            <dd>{identity.engineCount}</dd>
          </div>
          <div>
            <dt>Connected accounts</dt>
            <dd>{identity.accountCount}</dd>
          </div>
          <div>
            <dt>Unique identities</dt>
            <dd>
              {identity.uniqueInstanceCount} engines ·{" "}
              {identity.uniqueBucketCount} budgets ·{" "}
              {identity.uniqueReservationCount} claims
            </dd>
          </div>
          <div>
            <dt>Cross-account collisions</dt>
            <dd>{identity.crossAccountCollisionCount}</dd>
          </div>
        </dl>
        <p>
          {identity.status === "VERIFIED"
            ? "Every active or paused non-live engine, budget, and capital claim belongs to exactly one connected account."
            : "Arbion cannot prove complete one-account ownership for every current non-live identity."}
        </p>
      </div>
      <div className="strategy-fleet-isolation-accounts">
        {accounts.map((account) => (
          <article
            className={
              account.status === "VERIFIED" ? "is-verified" : "needs-review"
            }
            key={account.accountID}
          >
            <header>
              <div className="strategy-fleet-account">
                <span
                  className={`provider-mark provider-${account.provider}`}
                  aria-hidden="true"
                >
                  {providerInitial(account.provider)}
                </span>
                <div>
                  <small>{providerLabel(account.provider)}</small>
                  <strong>{account.accountName}</strong>
                </div>
              </div>
              <span>
                {account.status === "VERIFIED" ? "Verified" : "Review"}
              </span>
            </header>
            <dl>
              <div>
                <dt>Non-live engines</dt>
                <dd>{account.engineCount}</dd>
              </div>
              <div>
                <dt>Modes</dt>
                <dd>
                  {account.modes.length > 0
                    ? account.modes.map(readable).join(" + ")
                    : "Unavailable"}
                </dd>
              </div>
              <div>
                <dt>Paper simulation claim</dt>
                <dd>
                  {account.paperEngineCount === 0
                    ? "Not used"
                    : account.paperClaimedAmount
                      ? capitalMoney(
                          account.currency,
                          account.paperClaimedAmount,
                        )
                      : "Unavailable"}
                </dd>
              </div>
              <div>
                <dt>Shadow policy claim</dt>
                <dd>
                  {account.shadowEngineCount === 0
                    ? "Not used"
                    : account.shadowClaimedAmount
                      ? capitalMoney(
                          account.currency,
                          account.shadowClaimedAmount,
                        )
                      : "Unavailable"}
                </dd>
              </div>
              <div>
                <dt>Shadow account ceiling</dt>
                <dd>
                  {account.shadowEngineCount === 0
                    ? "Not used by Paper"
                    : account.accountLimit
                      ? capitalMoney(account.currency, account.accountLimit)
                      : account.status === "VERIFIED" &&
                          account.shadowEngineCount === 1
                        ? "Exclusive Shadow claim"
                        : "Unavailable"}
                </dd>
              </div>
            </dl>
            <p>{account.detail}</p>
            <small>
              Paper claims never consume broker cash · no broker hold · no order
              action
            </small>
          </article>
        ))}
      </div>
    </section>
  );
}

function scheduleResultLabel(
  engine: StrategyFleetScheduleReliability["engines"][number],
) {
  if (engine.lastStatus === "SUCCEEDED") return "Completed safely";
  if (engine.lastStatus === "SKIPPED")
    return engine.errorCode === "OUTSIDE_SESSION"
      ? "Waiting for market session"
      : "Skipped safely";
  if (engine.lastStatus === "FAILED") return "Failed closed";
  return "Result unavailable";
}

function StrategyFleetScheduleWatchtower({
  items,
}: {
  items: StrategyFleetItem[];
}) {
  const reliability = projectStrategyFleetScheduleReliability(items);
  if (reliability.engineCount === 0) return null;
  const statusLabel =
    reliability.status === "VERIFIED"
      ? "Verified"
      : reliability.status === "ATTENTION"
        ? "Needs review"
        : "Unavailable";
  const title =
    reliability.status === "VERIFIED"
      ? `${reliability.healthyCount} guarded ${reliability.healthyCount === 1 ? "schedule is" : "schedules are"} on course.`
      : reliability.status === "ATTENTION"
        ? "A guarded schedule needs review."
        : "Schedule reliability cannot be verified.";

  return (
    <section
      className={`strategy-fleet-watchtower is-${reliability.status.toLowerCase()}`}
      aria-labelledby="strategy-fleet-watchtower-heading"
    >
      <header>
        <div>
          <p className="eyebrow">SCHEDULER RELIABILITY</p>
          <h2 id="strategy-fleet-watchtower-heading">{title}</h2>
          <p>
            Arbion verifies each active AI engine&apos;s pinned schedule, latest
            completed result, failure streak, and next due time. A cycle more
            than five minutes late is never labeled healthy.
          </p>
        </div>
        <span>{statusLabel}</span>
      </header>
      <dl className="strategy-fleet-watchtower-summary">
        <div>
          <dt>Healthy schedules</dt>
          <dd>
            {reliability.healthyCount} / {reliability.engineCount}
          </dd>
        </div>
        <div>
          <dt>Latest results</dt>
          <dd>
            {reliability.succeededCount} completed ·{" "}
            {reliability.safelySkippedCount} safe wait
          </dd>
        </div>
        <div>
          <dt>Failure streaks</dt>
          <dd>{reliability.consecutiveFailures}</dd>
        </div>
        <div>
          <dt>Next fleet cycle</dt>
          <dd>{readableTime(reliability.nextRunAt)}</dd>
        </div>
      </dl>
      <ol className="strategy-fleet-watchtower-engines">
        {reliability.engines.map((engine) => (
          <li
            className={
              engine.timingStatus === "ON_SCHEDULE" &&
              engine.consecutiveFailures === 0 &&
              ["SUCCEEDED", "SKIPPED"].includes(engine.lastStatus)
                ? "is-healthy"
                : "needs-review"
            }
            key={engine.id}
          >
            <span
              className={`provider-mark provider-${engine.provider}`}
              aria-hidden="true"
            >
              {providerInitial(engine.provider)}
            </span>
            <div>
              <small>
                {engine.accountName} · {readable(engine.executionMode)}
              </small>
              <strong>{engine.title}</strong>
            </div>
            <dl>
              <div>
                <dt>Last completed</dt>
                <dd>{readableTime(engine.lastCompletedAt)}</dd>
              </div>
              <div>
                <dt>Last result</dt>
                <dd>{scheduleResultLabel(engine)}</dd>
              </div>
              <div>
                <dt>Next due</dt>
                <dd>{readableTime(engine.nextRunAt)}</dd>
              </div>
            </dl>
            <span>
              {engine.timingStatus === "ON_SCHEDULE"
                ? "On schedule"
                : engine.timingStatus === "OVERDUE"
                  ? "Overdue"
                  : "Timing unavailable"}
            </span>
          </li>
        ))}
      </ol>
      <footer>
        Read-only schedule proof · Paper or Shadow only · no manual cycle · no
        broker order
      </footer>
    </section>
  );
}

function scheduleRecoveryLabel(
  state: StrategyFleetScheduleRecovery["engines"][number]["state"],
) {
  if (state === "STABLE") return "Stable";
  if (state === "RECOVERED") return "Recovered";
  if (state === "SAFE_WAIT") return "Safe wait";
  if (state === "ATTENTION") return "Review";
  return "Unavailable";
}

function scheduleRecoveryGuidance(
  engine: StrategyFleetScheduleRecovery["engines"][number],
) {
  if (engine.state === "RECOVERED")
    return "No action needed. A later guarded cycle succeeded, while the earlier fail-closed result remains preserved for review.";
  if (engine.state === "SAFE_WAIT")
    return "No action needed. Arbion waited safely and will check again at the next eligible cycle.";
  if (engine.state === "STABLE")
    return "No action needed. Recent guarded cycles are preserved and the next automatic check remains scheduled.";
  if (engine.state === "ATTENTION")
    return "Review the evidence before intervening. The latest cycle failed closed and no broker order was sent.";
  return "Arbion cannot prove the recent recovery path from complete immutable scheduler evidence.";
}

function scheduleRunStatusLabel(status: string) {
  if (status === "SUCCEEDED") return "Completed";
  if (status === "SKIPPED") return "Safe wait";
  if (status === "FAILED") return "Failed closed";
  return "Unavailable";
}

function StrategyFleetScheduleRecoveryProof({
  items,
}: {
  items: StrategyFleetItem[];
}) {
  const recovery = projectStrategyFleetScheduleRecovery(items);
  if (recovery.engineCount === 0) return null;
  const statusLabel =
    recovery.status === "VERIFIED"
      ? "Verified"
      : recovery.status === "ATTENTION"
        ? "Needs review"
        : "Unavailable";
  return (
    <section
      className={`strategy-fleet-recovery is-${recovery.status.toLowerCase()}`}
      aria-labelledby="strategy-fleet-recovery-heading"
    >
      <header>
        <div>
          <p className="eyebrow">AUTOMATIC RECOVERY PROOF</p>
          <h2 id="strategy-fleet-recovery-heading">
            {recovery.status === "VERIFIED"
              ? `${recovery.verifiedCount} guarded ${recovery.verifiedCount === 1 ? "engine has" : "engines have"} a verified recent path.`
              : recovery.status === "ATTENTION"
                ? "A guarded engine is waiting for a safe recovery."
                : "Recent scheduler recovery cannot be verified."}
          </h2>
          <p>
            Arbion keeps failures, safe waits, and later successes together so
            you can see whether an engine recovered automatically without
            replaying a model cycle.
          </p>
        </div>
        <span>{statusLabel}</span>
      </header>
      <dl className="strategy-fleet-recovery-summary">
        <div>
          <dt>Verified engines</dt>
          <dd>
            {recovery.verifiedCount} / {recovery.engineCount}
          </dd>
        </div>
        <div>
          <dt>Recent records</dt>
          <dd>{recovery.preservedRunCount}</dd>
        </div>
        <div>
          <dt>Recovered paths</dt>
          <dd>{recovery.recoveredCount}</dd>
        </div>
        <div>
          <dt>Open failures</dt>
          <dd>{recovery.attentionCount}</dd>
        </div>
      </dl>
      <ol className="strategy-fleet-recovery-engines">
        {recovery.engines.map((engine) => (
          <li
            className={`is-${engine.state.toLowerCase().replace("_", "-")}`}
            key={engine.id}
          >
            <header>
              <div>
                <span
                  className={`provider-mark provider-${engine.provider}`}
                  aria-hidden="true"
                >
                  {providerInitial(engine.provider)}
                </span>
                <p>
                  <small>
                    {engine.accountName} · {readable(engine.executionMode)}
                  </small>
                  <strong>{engine.title}</strong>
                </p>
              </div>
              <span>{scheduleRecoveryLabel(engine.state)}</span>
            </header>
            <ol
              aria-label={`${engine.title} recent immutable scheduler results`}
            >
              {engine.recentStatuses.map((status, index) => (
                <li
                  className={`is-${status.toLowerCase()}`}
                  key={`${status}-${index}`}
                >
                  <span aria-hidden="true" />
                  <small>{scheduleRunStatusLabel(status)}</small>
                </li>
              ))}
            </ol>
            <p>{scheduleRecoveryGuidance(engine)}</p>
            <footer>
              <span>Next automatic check</span>
              <time dateTime={engine.nextRunAt}>
                {readableTime(engine.nextRunAt)}
              </time>
              <Link href={`/automations/${engine.id}#schedule-controls`}>
                Review evidence →
              </Link>
            </footer>
          </li>
        ))}
      </ol>
      <footer>
        Bounded immutable history · no automatic replay · no manual cycle · no
        broker order
      </footer>
    </section>
  );
}

function evidenceID(value?: string) {
  if (!value) return "Not created";
  return `${value.slice(0, 8)}…`;
}

function StrategyFleetDecisionEvidenceChain({
  items,
}: {
  items: StrategyFleetItem[];
}) {
  const evidence = projectStrategyFleetDecisionEvidence(items);
  if (evidence.engineCount === 0) return null;
  const verified = evidence.status === "VERIFIED";
  return (
    <section
      className={`strategy-fleet-evidence-chain ${verified ? "is-verified" : "needs-review"}`}
      aria-labelledby="strategy-fleet-evidence-chain-heading"
    >
      <header>
        <div>
          <p className="eyebrow">IMMUTABLE DECISION TRAILS</p>
          <h2 id="strategy-fleet-evidence-chain-heading">
            {verified
              ? `${evidence.verifiedCount} latest AI ${evidence.verifiedCount === 1 ? "decision has" : "decisions have"} a complete evidence trail.`
              : "A latest AI decision trail cannot be verified."}
          </h2>
          <p>
            Every current AI conclusion is traced from its sealed journal entry
            through the deterministic risk gate to a Paper or Shadow result.
            Abstentions end at the journal without inventing downstream work.
          </p>
        </div>
        <Link href="/activity">Open decision journal →</Link>
      </header>
      <dl className="strategy-fleet-evidence-chain-summary">
        <div>
          <dt>Verified trails</dt>
          <dd>
            {evidence.verifiedCount} / {evidence.engineCount}
          </dd>
        </div>
        <div>
          <dt>Safe abstentions</dt>
          <dd>{evidence.abstentionCount}</dd>
        </div>
        <div>
          <dt>Risk evaluations</dt>
          <dd>{evidence.riskEvaluationCount}</dd>
        </div>
        <div>
          <dt>Non-live records</dt>
          <dd>{evidence.nonLiveRecordCount}</dd>
        </div>
      </dl>
      <ol className="strategy-fleet-evidence-chain-list">
        {evidence.engines.map((engine) => (
          <li
            className={engine.verified ? "is-verified" : "needs-review"}
            key={engine.id}
          >
            <header>
              <div>
                <span
                  className={`provider-mark provider-${engine.provider}`}
                  aria-hidden="true"
                >
                  {providerInitial(engine.provider)}
                </span>
                <p>
                  <small>
                    {engine.accountName} · {readable(engine.executionMode)}
                  </small>
                  <strong>{engine.title}</strong>
                </p>
              </div>
              <span>{engine.verified ? "Trail verified" : "Review"}</span>
            </header>
            <dl>
              <div>
                <dt>01 · Journal</dt>
                <dd>
                  {latestDecisionLabel(engine.decisionType ?? "UNAVAILABLE")}
                </dd>
                <small>{evidenceID(engine.decisionID)}</small>
              </div>
              <div>
                <dt>02 · Proposal</dt>
                <dd>
                  {engine.decisionType === "ABSTAIN"
                    ? "None by design"
                    : engine.proposedActionID
                      ? "Recorded"
                      : "Unavailable"}
                </dd>
                <small>
                  {engine.decisionType === "ABSTAIN"
                    ? "AI chose no action"
                    : evidenceID(engine.proposedActionID)}
                </small>
              </div>
              <div>
                <dt>03 · Risk gate</dt>
                <dd>
                  {engine.decisionType === "ABSTAIN"
                    ? "Not reached"
                    : engine.riskDecision
                      ? readable(engine.riskDecision)
                      : "Unavailable"}
                </dd>
                <small>
                  {engine.decisionType === "ABSTAIN"
                    ? "No proposal to evaluate"
                    : evidenceID(engine.riskEvaluationID)}
                </small>
              </div>
              <div>
                <dt>04 · Non-live result</dt>
                <dd>
                  {engine.decisionType === "ABSTAIN"
                    ? "No record needed"
                    : engine.executionStatus
                      ? latestExecutionLabel(engine.executionStatus)
                      : "Unavailable"}
                </dd>
                <small>
                  {engine.decisionType === "ABSTAIN"
                    ? "Terminal safe decision"
                    : evidenceID(engine.executionRecordID)}
                </small>
              </div>
            </dl>
            <footer>
              <time dateTime={engine.decisionAt}>
                {readableTime(engine.decisionAt)}
              </time>
              <span>No credential data · no broker instruction</span>
            </footer>
          </li>
        ))}
      </ol>
      <footer>
        Owner-scoped immutable evidence only · no model rerun · no provider call
        · no live execution path
      </footer>
    </section>
  );
}

function capitalPolicyLabel(item: StrategyFleetItem) {
  if (item.capitalAllocationType === "PERCENT_OF_AVAILABLE_CASH")
    return `${conciseCapitalDecimal(item.capitalAllocationValue) ?? "Unavailable"}% of available cash`;
  if (item.capitalAllocationType === "PERCENT_OF_BUYING_POWER")
    return `${conciseCapitalDecimal(item.capitalAllocationValue) ?? "Unavailable"}% of buying power`;
  if (item.capitalAllocationType === "FIXED_AMOUNT")
    return capitalMoney(item.capitalCurrency, item.capitalAllocationValue);
  return "Unavailable";
}

function capitalBasisLabel(value?: string) {
  if (value === "PAPER_STARTING_CASH") return "Paper starting cash";
  if (value === "BUCKET_FIXED_CAPACITY") return "Fixed budget capacity";
  if (value === "BUCKET_ABSOLUTE_LIMIT") return "Percentage budget cap";
  if (value === "UNRESOLVED_LEGACY") return "Legacy exclusive policy";
  return "Unavailable";
}

function runtimeScheduleLabel(item: StrategyFleetItem) {
  if (item.runtimeScheduleEnabled === false) return "Owner-invoked only";
  if (
    item.runtimeScheduleEnabled !== true ||
    !item.runtimeScheduleIntervalMinutes ||
    !item.runtimeScheduleSession
  )
    return "Unavailable";
  return `${item.runtimeScheduleIntervalMinutes} min · ${readable(item.runtimeScheduleSession)}`;
}

function StrategyFleetRuntimeContract({ item }: { item: StrategyFleetItem }) {
  if (!requiresRuntimeContract(item)) return null;
  if (!runtimeContractHealthy(item)) {
    return (
      <section
        className="strategy-fleet-runtime needs-review"
        aria-label={`${item.title} immutable runtime contract`}
      >
        <header>
          <span>IMMUTABLE RUNTIME</span>
          <strong>Review required</strong>
        </header>
        <p>
          Model, universe, guardrail, and schedule facts are hidden until the
          exact pinned mandate version and schedule binding can be verified.
        </p>
        <Link href={`/automations/${item.id}#configuration-controls`}>
          Review runtime contract →
        </Link>
        <small>
          Existing database pin remains authoritative · no configuration or
          broker action changed
        </small>
      </section>
    );
  }

  const newerConfigurationAvailable = Boolean(
    item.currentMandateVersion &&
      item.runtimeMandateVersion &&
      item.currentMandateVersion > item.runtimeMandateVersion,
  );
  const currentConfiguration = newerConfigurationAvailable
    ? `v${item.currentMandateVersion} · ${exactState(item.mandateStatus)} separate`
    : `v${item.currentMandateVersion} · Matches runtime`;
  return (
    <section
      className="strategy-fleet-runtime is-verified"
      aria-label={`${item.title} immutable runtime contract`}
    >
      <header>
        <span>IMMUTABLE RUNTIME</span>
        <strong>PINNED v{item.runtimeMandateVersion}</strong>
      </header>
      <dl>
        <div>
          <dt>Snapshot</dt>
          <dd>{exactState(item.runtimeSnapshotStatus)} · immutable</dd>
        </div>
        <div>
          <dt>Model</dt>
          <dd>{item.modelID}</dd>
        </div>
        <div>
          <dt>Universe</dt>
          <dd>{coverageLabel(item.symbols)}</dd>
        </div>
        <div>
          <dt>Proposal ceiling</dt>
          <dd>{capitalMoney("USD", item.runtimeMaxProposalNotional)}</dd>
        </div>
        <div>
          <dt>Daily ceiling</dt>
          <dd>
            {item.runtimeLegacyDailyActionLimitMissing
              ? "Not recorded · legacy"
              : `${item.runtimeMaxTradesPerDay} action${item.runtimeMaxTradesPerDay === 1 ? "" : "s"} / UTC day`}
          </dd>
        </div>
        <div>
          <dt>Schedule contract</dt>
          <dd>{runtimeScheduleLabel(item)}</dd>
        </div>
      </dl>
      <p>
        <span>{currentConfiguration}</span>
        <Link href={`/automations/${item.id}#configuration-controls`}>
          Exact configuration →
        </Link>
      </p>
      <small>
        {item.runtimeLegacyDailyActionLimitMissing
          ? "Legacy daily ceiling is absent and not inferred · version and schedule identities match"
          : "Version and schedule identities match · no model run or provider action"}
      </small>
    </section>
  );
}

function StrategyFleetCapitalAuthority({ item }: { item: StrategyFleetItem }) {
  if (!requiresCapitalData(item)) return null;
  if (!capitalBindingHealthy(item)) {
    return (
      <section
        className="strategy-fleet-capital needs-review"
        aria-label={`${item.title} capital authority`}
      >
        <header>
          <span>CAPITAL AUTHORITY</span>
          <strong>Review required</strong>
        </header>
        <p>
          Exact bucket and reservation facts are hidden until the complete
          owner-scoped capital binding can be verified.
        </p>
        <Link href="/capital">Review capital control →</Link>
        <small>
          Database controls remain enforced · no provider funds moved
        </small>
      </section>
    );
  }

  const accountLimit = item.capitalReservationAccountLimit;
  const bucketCap = accountLimit ? undefined : item.capitalAllocationLimit;
  return (
    <section
      className="strategy-fleet-capital is-verified"
      aria-label={`${item.title} capital authority`}
    >
      <header>
        <span>CAPITAL AUTHORITY</span>
        <strong>Bounded</strong>
      </header>
      <dl>
        <div>
          <dt>Budget</dt>
          <dd>{item.capitalBucketName ?? "Trading budget"}</dd>
        </div>
        <div>
          <dt>Policy</dt>
          <dd>{capitalPolicyLabel(item)}</dd>
        </div>
        <div>
          <dt>Active claim</dt>
          <dd>
            {capitalMoney(
              item.capitalReservationCurrency,
              item.capitalReservationAmount,
            )}{" "}
            · {readable(item.executionMode)}
          </dd>
        </div>
        <div>
          <dt>Protected</dt>
          <dd>
            {capitalMoney(item.capitalCurrency, item.capitalProtectedAmount)}
          </dd>
        </div>
        <div>
          <dt>
            {accountLimit
              ? "Account ceiling"
              : bucketCap
                ? "Budget cap"
                : "Sharing"}
          </dt>
          <dd>
            {accountLimit
              ? capitalMoney(item.capitalCurrency, accountLimit)
              : bucketCap
                ? capitalMoney(item.capitalCurrency, bucketCap)
                : "Exclusive active claim"}
          </dd>
        </div>
        <div>
          <dt>Claim basis</dt>
          <dd>{capitalBasisLabel(item.capitalReservationBasis)}</dd>
        </div>
      </dl>
      <p>
        <span>Database-enforced non-live reservation</span>
        <Link href="/capital">Capital center →</Link>
      </p>
      <small>
        Policy ledger only · no broker custody or execution authority
      </small>
    </section>
  );
}

function StrategyFleetDataHealth({ item }: { item: StrategyFleetItem }) {
  if (!requiresOperationalData(item)) return null;
  const connectionHealthy = financialConnectionHealthy(item);
  if (item.executionMode === "PAPER") {
    return (
      <section
        className={`strategy-fleet-data-health${connectionHealthy ? " is-verified" : " needs-review"}`}
        aria-label={`${item.title} account data health`}
      >
        <header>
          <span>MARKET PRICE SOURCE</span>
          <strong>{connectionHealthy ? "Verified" : "Review required"}</strong>
        </header>
        <dl>
          <div>
            <dt>Connection</dt>
            <dd>
              {item.financialConnectionAvailable === false
                ? "Unavailable"
                : `${exactState(item.accountStatus)} account · ${exactState(item.financialConnectionStatus)} connection`}
            </dd>
          </div>
          <div>
            <dt>Broker portfolio</dt>
            <dd>Not used by Paper</dd>
          </div>
          <div>
            <dt>Cash and positions</dt>
            <dd>Isolated Arbion simulation</dd>
          </div>
          {item.financialAuthorizationExpiresAt && (
            <div>
              <dt>Authorization expiry</dt>
              <dd>{readableTime(item.financialAuthorizationExpiresAt)}</dd>
            </div>
          )}
        </dl>
        <p>
          <span>
            {connectionHealthy
              ? "Connected account supplies current market prices only"
              : "Paper cycles remain fail-closed until the price source is restored"}
          </span>
          {item.financialAccountID && (
            <Link href={`/accounts/${item.financialAccountID}`}>
              View price-source account →
            </Link>
          )}
        </p>
        <small>
          Simulated ledger is isolated · no broker positions, cash, or order
          action
        </small>
      </section>
    );
  }
  const portfolioHealthy = reconciliationHealthy(item);
  const healthy = connectionHealthy && portfolioHealthy;
  const coverage = [
    `Balances ${exactState(item.reconciliationBalancesStatus).toLowerCase()}`,
    `Positions ${exactState(item.reconciliationPositionsStatus).toLowerCase()}`,
  ].join(" · ");
  const freshness = !item.reconciliationObservedAt
    ? "Timestamp unavailable"
    : `${item.reconciliationFresh ? "Fresh ≤24h" : "Stale or invalid"} · ${readableTime(item.reconciliationObservedAt)}`;

  return (
    <section
      className={`strategy-fleet-data-health${healthy ? " is-verified" : " needs-review"}`}
      aria-label={`${item.title} account data health`}
    >
      <header>
        <span>ACCOUNT DATA CONTROL</span>
        <strong>{healthy ? "Verified" : "Review required"}</strong>
      </header>
      <dl>
        <div>
          <dt>Connection</dt>
          <dd>
            {item.financialConnectionAvailable === false
              ? "Unavailable"
              : `${exactState(item.accountStatus)} account · ${exactState(item.financialConnectionStatus)} connection`}
          </dd>
        </div>
        <div>
          <dt>Portfolio match</dt>
          <dd>{exactState(item.reconciliationComparisonStatus)}</dd>
        </div>
        <div>
          <dt>Coverage</dt>
          <dd>{coverage}</dd>
        </div>
        <div>
          <dt>Autonomy signal</dt>
          <dd>{exactState(item.reconciliationAutonomySignal)}</dd>
        </div>
        <div>
          <dt>Evidence freshness</dt>
          <dd>{freshness}</dd>
        </div>
        {item.financialAuthorizationExpiresAt && (
          <div>
            <dt>Authorization expiry</dt>
            <dd>{readableTime(item.financialAuthorizationExpiresAt)}</dd>
          </div>
        )}
      </dl>
      <p>
        <span>
          {portfolioHealthy
            ? "Deterministic proposal gate has current account evidence"
            : item.reconciliationBlocksNewActions
              ? "New AI proposals are held by portfolio evidence"
              : "New AI proposals remain fail-closed"}
        </span>
        {item.financialAccountID && (
          <Link
            href={`/accounts/${item.financialAccountID}#reconciliation-title`}
          >
            Account evidence →
          </Link>
        )}
      </p>
      <small>
        Stored immutable evidence · no provider read or order action
      </small>
    </section>
  );
}

function StrategyFleetDecision({ item }: { item: StrategyFleetItem }) {
  if (!isAI(item)) return null;
  if (!item.instanceStatus) {
    return (
      <section
        className="strategy-fleet-decision is-pending"
        aria-label={`${item.title} latest AI decision`}
      >
        <strong>Decision pulse begins after initialization.</strong>
        <p>No model activity is inferred before an instance exists.</p>
      </section>
    );
  }
  if (item.decisionAvailable === false) {
    return (
      <section
        className="strategy-fleet-decision is-unavailable"
        aria-label={`${item.title} latest AI decision`}
      >
        <strong>Latest decision unavailable</strong>
        <p>No recent AI action is inferred from a failed journal refresh.</p>
      </section>
    );
  }
  if (!item.latestDecisionType) {
    return (
      <section
        className="strategy-fleet-decision is-pending"
        aria-label={`${item.title} latest AI decision`}
      >
        <strong>Awaiting a completed AI decision</strong>
        <p>No AI entry appears in the latest 10 immutable journal records.</p>
      </section>
    );
  }

  const riskTone = ["ALLOW", "DENY"].includes(
    item.latestDecisionRiskDecision ?? "",
  )
    ? item.latestDecisionRiskDecision?.toLowerCase()
    : "neutral";
  return (
    <section
      className={`strategy-fleet-decision is-${riskTone}`}
      aria-label={`${item.title} latest AI decision`}
    >
      <header>
        <span>LATEST AI DECISION</span>
        <strong>{latestDecisionLabel(item.latestDecisionType)}</strong>
      </header>
      <dl>
        <div>
          <dt>Action</dt>
          <dd>{latestActionLabel(item)}</dd>
        </div>
        <div>
          <dt>Risk gate</dt>
          <dd>{latestRiskLabel(item)}</dd>
        </div>
        <div>
          <dt>AI route</dt>
          <dd>{latestRouteLabel(item)}</dd>
        </div>
        <div>
          <dt>Route telemetry</dt>
          <dd>{latestTelemetryLabel(item)}</dd>
        </div>
      </dl>
      <p>
        <time dateTime={item.latestDecisionAt}>
          {readableTime(item.latestDecisionAt)}
        </time>
        <span>{latestExecutionLabel(item.latestDecisionExecutionStatus)}</span>
        <Link href={`/automations/${item.id}#decision-journal`}>
          Decision journal →
        </Link>
      </p>
    </section>
  );
}

function StrategyFleetEvidence({ item }: { item: StrategyFleetItem }) {
  if (!isAIShadow(item)) return null;
  if (!item.instanceStatus) {
    return (
      <section
        className="strategy-fleet-evidence is-pending"
        aria-label={`${item.title} Shadow evidence`}
      >
        <strong>Shadow evidence begins after initialization.</strong>
        <p>No outcome samples are inferred before an instance exists.</p>
      </section>
    );
  }
  if (item.evidenceAvailable === false || !item.evidenceStatus) {
    return (
      <section
        className="strategy-fleet-evidence is-unavailable"
        aria-label={`${item.title} Shadow evidence`}
      >
        <strong>Evidence status unavailable</strong>
        <p>Sample counts are hidden until the exact scorecard refreshes.</p>
      </section>
    );
  }

  const oneHour = boundedProgress(
    item.oneHourSampleSize,
    item.minimumSamplePerHorizon,
  );
  const twentyFourHour = boundedProgress(
    item.twentyFourHourSampleSize,
    item.minimumSamplePerHorizon,
  );
  const window = boundedProgress(
    item.evidenceWindowHours,
    item.minimumEvidenceWindowHours,
  );
  const valid = oneHour && twentyFourHour && window;
  if (!valid) {
    return (
      <section
        className="strategy-fleet-evidence is-unavailable"
        aria-label={`${item.title} Shadow evidence`}
      >
        <strong>Evidence contract incomplete</strong>
        <p>Arbion will not estimate a missing sample target or time window.</p>
      </section>
    );
  }

  const reviewable = item.evidenceStatus === "EVIDENCE_REVIEWABLE";
  return (
    <section
      className={`strategy-fleet-evidence${reviewable ? " is-reviewable" : ""}`}
      aria-label={`${item.title} Shadow evidence`}
    >
      <header>
        <span>SHADOW EVIDENCE</span>
        <strong>
          {reviewable
            ? item.currentEvidenceReviewed
              ? "Current snapshot reviewed"
              : "Reviewable"
            : "Collecting"}
        </strong>
      </header>
      <div>
        <label>
          <span>1-hour marks</span>
          <strong>
            {item.oneHourSampleSize} / {item.minimumSamplePerHorizon}
          </strong>
          <progress
            aria-label="1-hour Shadow outcome sample progress"
            max={oneHour.maximum}
            value={oneHour.value}
          />
        </label>
        <label>
          <span>24-hour marks</span>
          <strong>
            {item.twentyFourHourSampleSize} / {item.minimumSamplePerHorizon}
          </strong>
          <progress
            aria-label="24-hour Shadow outcome sample progress"
            max={twentyFourHour.maximum}
            value={twentyFourHour.value}
          />
        </label>
        <label>
          <span>Evidence window</span>
          <strong>
            {item.evidenceWindowHours} / {item.minimumEvidenceWindowHours}h
          </strong>
          <progress
            aria-label="Shadow evidence window progress"
            max={window.maximum}
            value={window.value}
          />
        </label>
      </div>
      <p>
        Gate schedule {item.evidenceScheduleHealthy ? "verified" : "pending"}
        {item.evidenceBlockers?.length
          ? ` · ${item.evidenceBlockers.length} remaining ${item.evidenceBlockers.length === 1 ? "condition" : "conditions"}`
          : " · exact gate complete"}
        <span>Reviewability only · never live authority</span>
      </p>
      {item.evidenceBlockers && item.evidenceBlockers.length > 0 && (
        <ul
          className="strategy-fleet-evidence-blockers"
          aria-label="Shadow evidence blockers"
        >
          {item.evidenceBlockers.map((blocker) => (
            <li key={blocker}>{evidenceBlockerLabel(blocker)}</li>
          ))}
        </ul>
      )}
    </section>
  );
}

export function StrategyFleet({
  items,
  inventoryAvailable = true,
  contextWarnings = [],
}: StrategyFleetProps) {
  const ordered = [...items].sort((left, right) => {
    const aiOrder = Number(isAI(right)) - Number(isAI(left));
    if (aiOrder !== 0) return aiOrder;
    const activeOrder =
      Number(right.instanceStatus === "ACTIVE") -
      Number(left.instanceStatus === "ACTIVE");
    if (activeOrder !== 0) return activeOrder;
    return left.accountName.localeCompare(right.accountName);
  });
  const monitoring = items.filter(
    (item) =>
      isAI(item) &&
      item.instanceStatus === "ACTIVE" &&
      item.currentState === "AI_MONITORING",
  ).length;
  const healthySchedules = items.filter(
    (item) =>
      item.scheduleAvailable === true &&
      item.scheduleEnabled &&
      Boolean(item.nextRunAt) &&
      item.scheduleStatus !== "FAILED" &&
      item.consecutiveFailures === 0,
  ).length;
  const attention = items.filter(needsReview).length;
  const drafts = items.filter((item) => item.mandateStatus === "DRAFT").length;
  const nextActions = Array.from(
    new Map(
      items
        .map(selectStrategyFleetNextAction)
        .filter((action): action is StrategyFleetNextAction => Boolean(action))
        .sort((left, right) =>
          left.priority !== right.priority
            ? left.priority - right.priority
            : left.title.localeCompare(right.title),
        )
        .map((action) => [action.key, action]),
    ).values(),
  );

  return (
    <>
      <section className="strategy-fleet-metrics" aria-label="Fleet summary">
        <article>
          <span>Monitoring</span>
          <strong>{inventoryAvailable ? monitoring : "—"}</strong>
          <small>AI non-live engines</small>
        </article>
        <article>
          <span>Scheduled</span>
          <strong>{inventoryAvailable ? healthySchedules : "—"}</strong>
          <small>healthy automatic cycles</small>
        </article>
        <article className={attention > 0 ? "needs-review" : undefined}>
          <span>Attention</span>
          <strong>{inventoryAvailable ? attention : "—"}</strong>
          <small>engine health signals</small>
        </article>
        <article>
          <span>Drafts</span>
          <strong>{inventoryAvailable ? drafts : "—"}</strong>
          <small>not initialized</small>
        </article>
      </section>

      {inventoryAvailable && contextWarnings.length > 0 && (
        <section className="strategy-fleet-warning" role="status">
          <strong>Some live strategy context is unavailable.</strong>
          <p>{contextWarnings.join(" ")}</p>
        </section>
      )}

      {inventoryAvailable && <StrategyFleetOperatingBrief items={items} />}

      {inventoryAvailable && (
        <StrategyFleetProvenanceDigestView items={items} />
      )}

      {inventoryAvailable && (
        <StrategyFleetInputCoverageMatrixView items={items} />
      )}

      {inventoryAvailable && (
        <StrategyFleetInputCoverageChangeLedgerView items={items} />
      )}

      {inventoryAvailable && (
        <StrategyFleetPersistentInputGapRegisterView items={items} />
      )}

      {inventoryAvailable && <StrategyFleetAccountIsolationMap items={items} />}

      {inventoryAvailable && <StrategyFleetScheduleWatchtower items={items} />}

      {inventoryAvailable && (
        <StrategyFleetScheduleRecoveryProof items={items} />
      )}

      {inventoryAvailable && (
        <StrategyFleetDecisionEvidenceChain items={items} />
      )}

      {inventoryAvailable && ordered.length > 0 && (
        <section
          className={`strategy-fleet-action-queue${nextActions.length === 0 ? " is-clear" : ""}`}
          aria-labelledby="strategy-fleet-action-heading"
        >
          <header>
            <div>
              <p className="eyebrow">OWNER ACTION QUEUE</p>
              <h2 id="strategy-fleet-action-heading">
                {nextActions.length === 0
                  ? "No owner action right now."
                  : "Your clearest path forward."}
              </h2>
              <p>
                {nextActions.length === 0
                  ? "Every current autonomous engine has a healthy next cycle or no actionable exception."
                  : "Ordered by safety and lifecycle priority from the complete current fleet snapshot."}
              </p>
            </div>
            <span>
              {nextActions.length} owner{" "}
              {nextActions.length === 1 ? "step" : "steps"}
            </span>
          </header>
          {nextActions.length > 0 && (
            <ol>
              {nextActions.map((action, index) => (
                <li
                  className={`is-${action.tone.toLowerCase()}`}
                  key={action.key}
                >
                  <span aria-hidden="true">
                    {String(index + 1).padStart(2, "0")}
                  </span>
                  <div>
                    <small>
                      {action.provider && action.accountName
                        ? `${providerLabel(action.provider)} · ${action.accountName}`
                        : action.eyebrow}
                    </small>
                    <h3>{action.title}</h3>
                    <p>{action.detail}</p>
                  </div>
                  <Link href={action.href}>{action.actionLabel} →</Link>
                </li>
              ))}
            </ol>
          )}
          <footer>
            Guidance only · opening a control does not run a cycle or authorize
            an order
          </footer>
        </section>
      )}

      {!inventoryAvailable ? (
        <section className="strategy-fleet-empty is-unavailable" role="status">
          <p className="eyebrow">LIVE STATUS UNAVAILABLE</p>
          <h2>Strategies could not be loaded.</h2>
          <p>
            Existing mandates and schedules were not changed. Return to the
            dashboard while Arbion restores this read-only view.
          </p>
          <Link className="button-link" href="/dashboard">
            Return to dashboard
          </Link>
        </section>
      ) : ordered.length === 0 ? (
        <section className="strategy-fleet-empty">
          <p className="eyebrow">START NON-LIVE</p>
          <h2>No strategies yet.</h2>
          <p>
            Connect one account and one AI model, define a bounded objective and
            budget, then begin with Paper simulation or Shadow observation.
          </p>
          <Link className="button-link" href="/automations/new">
            Launch an AI Engine
          </Link>
        </section>
      ) : (
        <section className="strategy-fleet-grid" aria-label="Strategy fleet">
          {ordered.map((item) => (
            <article
              className={isAI(item) ? "is-ai-engine" : "is-rules-engine"}
              key={item.id}
            >
              <header>
                <div className="strategy-fleet-account">
                  <span
                    className={`provider-mark provider-${item.provider}`}
                    aria-hidden="true"
                  >
                    {providerInitial(item.provider)}
                  </span>
                  <div>
                    <small>{providerLabel(item.provider)}</small>
                    <strong>{item.accountName}</strong>
                  </div>
                </div>
                <span
                  className={`strategy-fleet-status ${
                    needsReview(item)
                      ? "needs-review"
                      : item.instanceStatus === "ACTIVE"
                        ? "is-active"
                        : ""
                  }`}
                >
                  {statusLabel(item)}
                </span>
              </header>

              <div className="strategy-fleet-identity">
                <p className="eyebrow">
                  {isAI(item) ? "AI AUTONOMOUS" : "RULES-BASED"}
                </p>
                <h2>{item.title}</h2>
                <p>
                  {isAI(item)
                    ? "Model-led analysis inside deterministic risk controls."
                    : "A defined playbook operating inside a non-live account sandbox."}
                </p>
              </div>

              <dl>
                <div>
                  <dt>{isAI(item) ? "Model" : "Decision system"}</dt>
                  <dd>{modelLabel(item)}</dd>
                </div>
                <div>
                  <dt>Coverage</dt>
                  <dd>{fleetCoverageLabel(item)}</dd>
                </div>
                <div>
                  <dt>Mode</dt>
                  <dd>{readable(item.executionMode)}</dd>
                </div>
                <div>
                  <dt>Last cycle</dt>
                  <dd>{readableTime(item.lastEvaluatedAt)}</dd>
                </div>
                <div>
                  <dt>Autonomy</dt>
                  <dd>{readable(item.autonomyLevel)}</dd>
                </div>
                <div>
                  <dt>Next cycle</dt>
                  <dd>
                    {item.nextRunAt
                      ? readableTime(item.nextRunAt)
                      : "Not scheduled"}
                  </dd>
                </div>
              </dl>

              <StrategyFleetRuntimeContract item={item} />
              <StrategyFleetCapitalAuthority item={item} />
              <StrategyFleetDataHealth item={item} />
              <StrategyFleetDecision item={item} />
              <StrategyFleetEvidence item={item} />

              <footer>
                <span className={healthClass(item)}>
                  <i /> {healthLabel(item)}
                </span>
                <Link
                  aria-label={`Open ${item.title} for ${item.accountName}`}
                  href={`/automations/${item.id}`}
                >
                  Open strategy →
                </Link>
              </footer>
            </article>
          ))}
        </section>
      )}

      <section
        className="strategy-fleet-safety"
        aria-label="Execution boundary"
      >
        <strong>Production execution remains locked.</strong>
        <p>
          Shadow records what an engine would consider. Paper simulates a
          position lifecycle. Neither mode can submit a broker order.
        </p>
      </section>
    </>
  );
}
