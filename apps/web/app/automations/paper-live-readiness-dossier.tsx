import Link from "next/link";

import { compareExactDecimals } from "../exact-money";
import type { StrategyFleetItem } from "./strategy-fleet";

export type PaperLiveReadinessState =
  | "PROVEN"
  | "COLLECTING"
  | "BLOCKED"
  | "UNAVAILABLE";

type PaperLiveReadinessSignal = {
  key: string;
  label: string;
  state: PaperLiveReadinessState;
  detail: string;
  evidence: string;
};

export type PaperLiveReadinessDossierProjection = {
  status:
    | "PLATFORM_BLOCKED"
    | "EVIDENCE_COLLECTING"
    | "REVIEW_REQUIRED"
    | "EVIDENCE_UNAVAILABLE";
  paperEngineCount: number;
  provenCount: number;
  collectingCount: number;
  blockedCount: number;
  unavailableCount: number;
  engines: Array<{
    id: string;
    instanceID: string;
    title: string;
    accountName: string;
    provider: string;
    modelRoute: string;
    status: PaperLiveReadinessDossierProjection["status"];
    ownerAction: string;
    signals: PaperLiveReadinessSignal[];
    shadowInstanceID?: string;
    shadowMandateID?: string;
    detailHref: string;
  }>;
};

const uuidPattern =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;

function isAI(item: StrategyFleetItem) {
  return item.automationType === "AI_AUTONOMOUS";
}

function exactTimestamp(value: string | undefined, observedAt: string) {
  if (!value) return false;
  const parsed = Date.parse(value);
  const observed = Date.parse(observedAt);
  return (
    Number.isFinite(parsed) && Number.isFinite(observed) && parsed <= observed
  );
}

function exactFutureTimestamp(value: string | undefined, observedAt: string) {
  if (!value) return true;
  const parsed = Date.parse(value);
  const observed = Date.parse(observedAt);
  return (
    Number.isFinite(parsed) && Number.isFinite(observed) && parsed > observed
  );
}

function readableTime(value?: string) {
  if (!value) return "not time-bounded";
  const parsed = new Date(value);
  if (Number.isNaN(parsed.valueOf())) return "UNAVAILABLE";
  return new Intl.DateTimeFormat("en-US", {
    month: "short",
    day: "numeric",
    hour: "numeric",
    minute: "2-digit",
    timeZone: "UTC",
    timeZoneName: "short",
  }).format(parsed);
}

function exactNonNegative(value?: string) {
  const comparison = value ? compareExactDecimals(value, "0") : undefined;
  return comparison !== undefined && comparison >= 0;
}

function exactAtMost(value?: string, ceiling?: string) {
  if (!value || !ceiling) return false;
  const positive = compareExactDecimals(value, "0");
  const comparison = compareExactDecimals(value, ceiling);
  return (
    positive !== undefined &&
    positive > 0 &&
    comparison !== undefined &&
    comparison <= 0
  );
}

function exactSymbols(actual?: string[], expected?: string[]) {
  if (!actual || !expected || actual.length === 0) return false;
  const left = [...new Set(actual.map((value) => value.toUpperCase()))].sort();
  const right = [
    ...new Set(expected.map((value) => value.toUpperCase())),
  ].sort();
  return (
    left.length === actual.length &&
    right.length === expected.length &&
    left.length === right.length &&
    left.every((value, index) => value === right[index])
  );
}

function exactFleetIdentity(items: StrategyFleetItem[]) {
  const engines = items.filter(
    (item) => isAI(item) && item.instanceStatus === "ACTIVE",
  );
  const instanceIDs = new Set<string>();
  const reservationIDs = new Set<string>();
  const bucketIDs = new Set<string>();
  return engines.every((item) => {
    const exact = Boolean(
      item.strategyInstanceID &&
        uuidPattern.test(item.strategyInstanceID) &&
        item.financialAccountID &&
        uuidPattern.test(item.financialAccountID) &&
        item.financialConnectionID &&
        uuidPattern.test(item.financialConnectionID) &&
        item.capitalBucketID &&
        uuidPattern.test(item.capitalBucketID) &&
        item.capitalReservationID &&
        uuidPattern.test(item.capitalReservationID) &&
        !instanceIDs.has(item.strategyInstanceID) &&
        !bucketIDs.has(item.capitalBucketID) &&
        !reservationIDs.has(item.capitalReservationID),
    );
    if (!exact) return false;
    instanceIDs.add(item.strategyInstanceID!);
    bucketIDs.add(item.capitalBucketID!);
    reservationIDs.add(item.capitalReservationID!);
    return true;
  });
}

function connectionSignal(item: StrategyFleetItem): PaperLiveReadinessSignal {
  const observedAt = item.freshnessObservedAt ?? "";
  const evidenceComplete = Boolean(
    item.financialConnectionContextAvailable === true &&
      item.financialConnectionAvailable === true &&
      item.financialConnectionID &&
      uuidPattern.test(item.financialConnectionID) &&
      exactTimestamp(item.financialConnectionLastVerifiedAt, observedAt) &&
      exactFutureTimestamp(item.financialAuthorizationExpiresAt, observedAt),
  );
  const active = item.financialConnectionStatus === "active";
  return {
    key: "AUTHORIZATION",
    label: "Current provider authorization",
    state: !evidenceComplete ? "UNAVAILABLE" : active ? "PROVEN" : "BLOCKED",
    detail: !evidenceComplete
      ? "The current connection identity, verification time, or saved expiry is incomplete. Arbion will not infer credential scope or provider permission."
      : active
        ? `${item.provider} is active from saved connection evidence; expiry ${readableTime(item.financialAuthorizationExpiresAt)}.`
        : `The saved ${item.provider} connection is ${item.financialConnectionStatus ?? "UNAVAILABLE"}.`,
    evidence: `${item.financialConnectionID ?? "UNAVAILABLE"} · last verified ${readableTime(item.financialConnectionLastVerifiedAt)}`,
  };
}

function capitalSignal(
  item: StrategyFleetItem,
  fleetIdentityExact: boolean,
): PaperLiveReadinessSignal {
  const proven = Boolean(
    fleetIdentityExact &&
      item.capitalContextAvailable === true &&
      item.capitalBindingValid === true &&
      item.capitalReservationStatus === "ACTIVE" &&
      item.capitalReservationBasis === "PAPER_STARTING_CASH" &&
      item.paperPortfolioAvailable === true &&
      item.paperStartingCash &&
      item.capitalReservationAmount &&
      compareExactDecimals(
        item.paperStartingCash,
        item.capitalReservationAmount,
      ) === 0,
  );
  return {
    key: "CAPITAL",
    label: "Account and capital isolation",
    state: proven ? "PROVEN" : "UNAVAILABLE",
    detail: proven
      ? "The Paper ledger, account, strategy instance, bucket, and simulation-only capital claim form one exact owner-scoped chain."
      : "The fleet identity, Paper ledger, account, bucket, or capital-reservation chain is incomplete or collides with another active engine.",
    evidence: `${item.financialAccountID ?? "UNAVAILABLE"} · ${item.capitalReservationID ?? "UNAVAILABLE"}`,
  };
}

function scheduleSignal(item: StrategyFleetItem): PaperLiveReadinessSignal {
  const evidenceComplete = Boolean(
    item.scheduleAvailable === true &&
      item.scheduleHistoryAvailable === true &&
      item.runtimeScheduleBindingValid === true &&
      item.scheduleEnabled === true &&
      item.nextRunAt &&
      exactFutureTimestamp(item.nextRunAt, item.freshnessObservedAt ?? "") &&
      item.scheduleLastCompletedAt &&
      exactTimestamp(
        item.scheduleLastCompletedAt,
        item.freshnessObservedAt ?? "",
      ),
  );
  const healthy = Boolean(
    evidenceComplete &&
      item.scheduleStatus === "SUCCEEDED" &&
      item.consecutiveFailures === 0 &&
      item.scheduleTimingStatus === "ON_SCHEDULE",
  );
  return {
    key: "SCHEDULER",
    label: "Automatic-cycle reliability",
    state: !evidenceComplete ? "UNAVAILABLE" : healthy ? "PROVEN" : "BLOCKED",
    detail: healthy
      ? `Latest automatic cycle succeeded with zero consecutive failures; next guarded cycle ${readableTime(item.nextRunAt)}.`
      : evidenceComplete
        ? `The saved scheduler is ${item.scheduleStatus ?? "UNAVAILABLE"} with ${item.consecutiveFailures} consecutive failures and timing ${item.scheduleTimingStatus ?? "UNAVAILABLE"}.`
        : "The pinned schedule, immutable run history, completion time, or next due time is incomplete.",
    evidence: `${item.scheduleStatus ?? "UNAVAILABLE"} · completed ${readableTime(item.scheduleLastCompletedAt)}`,
  };
}

function provenanceSignal(item: StrategyFleetItem): PaperLiveReadinessSignal {
  const complete = Boolean(
    item.decisionAvailable === true &&
      item.latestDecisionID &&
      uuidPattern.test(item.latestDecisionID) &&
      exactTimestamp(item.latestDecisionAt, item.freshnessObservedAt ?? "") &&
      item.latestDecisionAIProvider &&
      item.latestDecisionAIModelID === item.modelID &&
      item.latestDecisionAIProfile &&
      item.latestDecisionFinancialContextComplete === true &&
      item.latestDecisionFinancialProvider === item.provider &&
      exactSymbols(item.latestDecisionMarketSymbols, item.symbols) &&
      item.latestDecisionMarketFeeds?.every(Boolean) &&
      item.latestDecisionMarketFeeds.length > 0 &&
      item.latestDecisionMarketQualities?.every(Boolean) &&
      item.latestDecisionMarketQualities.length > 0 &&
      exactTimestamp(
        item.latestDecisionMarketObservedAt,
        item.freshnessObservedAt ?? "",
      ) &&
      item.latestDecisionLatencyMS !== undefined &&
      item.latestDecisionLatencyMS >= 0 &&
      item.latestDecisionInputUsage !== undefined &&
      item.latestDecisionInputUsage >= 0 &&
      item.latestDecisionOutputUsage !== undefined &&
      item.latestDecisionOutputUsage >= 0,
  );
  return {
    key: "PROVENANCE",
    label: "AI route and financial inputs",
    state: complete ? "PROVEN" : "UNAVAILABLE",
    detail: complete
      ? `${item.latestDecisionAIProvider} / ${item.latestDecisionAIModelID} / ${item.latestDecisionAIProfile} used exact ${item.provider} inputs for ${item.symbols.join(" / ")}.`
      : "The newest immutable decision does not prove the exact model route, financial provider, market universe, timestamp, and telemetry together.",
    evidence: `${item.latestDecisionID ?? "UNAVAILABLE"} · ${readableTime(item.latestDecisionAt)}`,
  };
}

function boundarySignal(item: StrategyFleetItem): PaperLiveReadinessSignal {
  const abstention = Boolean(
    item.latestDecisionType === "ABSTAIN" &&
      compareExactDecimals(item.latestDecisionProposedNotional ?? "", "0") ===
        0 &&
      !item.latestDecisionProposedActionID &&
      !item.latestDecisionRiskEvaluationID &&
      !item.latestDecisionExecutionRecordID,
  );
  const linkedContract = {
    DENY_RISK_DENIED: {
      riskDecision: "DENY",
      executionStatus: "RISK_DENIED",
    },
    ALLOW_SIMULATED_FILLED: {
      riskDecision: "ALLOW",
      executionStatus: "SIMULATED_FILLED",
    },
    ALLOW_SIMULATED_REJECTED: {
      riskDecision: "ALLOW",
      executionStatus: "SIMULATED_REJECTED",
    },
  }[item.latestDecisionType ?? ""];
  const proposal = Boolean(
    linkedContract &&
      item.latestDecisionProposedActionID &&
      item.latestDecisionRiskEvaluationID &&
      item.latestDecisionExecutionRecordID &&
      exactAtMost(
        item.latestDecisionProposedNotional,
        item.runtimeMaxProposalNotional,
      ) &&
      item.latestDecisionRiskDecision === linkedContract?.riskDecision &&
      item.latestDecisionExecutionStatus === linkedContract?.executionStatus,
  );
  const gate = item.paperEvidenceReadiness;
  const noLive = Boolean(
    item.paperEvidenceReadinessContractAvailable === true &&
      gate &&
      gate.live_execution_available === false &&
      gate.safety.status === "CLEAR" &&
      gate.safety.live_mandate_count === 0 &&
      gate.safety.ai_order_intent_count === 0 &&
      gate.safety.invalid_strategy_mode_count === 0 &&
      gate.safety.invalid_execution_mode_count === 0 &&
      gate.safety.platform_executable_risk_count === 0 &&
      gate.safety.non_simulation_fill_count === 0,
  );
  const proven = noLive && (abstention || proposal);
  return {
    key: "BOUNDARY",
    label: "Deterministic non-live boundary",
    state: proven ? "PROVEN" : "UNAVAILABLE",
    detail: proven
      ? abstention
        ? "The newest decision abstained before risk or simulation, and the fleet no-live counters remain exactly clear."
        : item.latestDecisionRiskDecision === "DENY"
          ? "The newest proposal was deterministically denied before simulation; the fleet no-live counters remain clear."
          : item.latestDecisionExecutionStatus === "SIMULATED_REJECTED"
            ? "The isolated Paper simulator rejected the newest allowed proposal; the fleet no-live counters remain clear."
            : "The newest proposal produced only an isolated simulated fill after deterministic risk review; the fleet no-live counters remain clear."
      : "The newest decision boundary and exact fleet no-live safety counters could not be proven together.",
    evidence: `Decision ${item.latestDecisionID ?? "UNAVAILABLE"} · risk ${item.latestDecisionRiskEvaluationID ?? "not reached"}`,
  };
}

function paperEvidenceSignal(
  item: StrategyFleetItem,
): PaperLiveReadinessSignal {
  const gate = item.paperEvidenceReadiness;
  if (
    item.paperEvidenceReadinessContractAvailable !== true ||
    !gate ||
    gate.calculation_method !==
      "IMMUTABLE_PAPER_AUTONOMY_EVIDENCE_READINESS_GATE" ||
    gate.execution_boundary !== "PAPER_SIMULATION_ONLY" ||
    gate.review_scope !== "OWNER_REVIEW_EVIDENCE_ONLY" ||
    gate.live_execution_available !== false ||
    gate.review_packet.grants_authority !== false ||
    gate.review_packet.live_promotion_available !== false
  ) {
    return {
      key: "PAPER_EVIDENCE",
      label: "Paper evidence and owner review",
      state: "UNAVAILABLE",
      detail:
        "The exact seven-day Paper evidence, non-authorizing review packet, or execution boundary is unavailable.",
      evidence: "UNAVAILABLE",
    };
  }
  const review = item.paperLatestEvidenceReview;
  const reviewBindingExact = Boolean(
    review &&
      item.strategyInstanceID &&
      item.financialAccountID &&
      item.runtimeMandateVersion &&
      review.strategy_instance_id === item.strategyInstanceID &&
      review.financial_account_id === item.financialAccountID &&
      review.mandate_id === item.id &&
      review.mandate_version === item.runtimeMandateVersion &&
      review.gate_status === "EVIDENCE_REVIEWABLE" &&
      review.evidence_window_hours >= 168 &&
      review.decision_count >= 20 &&
      review.portfolio_version >= 1 &&
      review.latest_checkpoint_run_id &&
      review.evidence_as_of === review.latest_checkpoint_as_of &&
      review.scheduler_sample_count >= 20 &&
      review.scheduler_success_count + review.scheduler_failure_count <=
        review.scheduler_sample_count &&
      review.last_schedule_status === "SUCCEEDED" &&
      review.consecutive_schedule_failures === 0 &&
      ["STABLE", "CONTEXT_CHANGED"].includes(review.route_continuity_status) &&
      review.input_coverage_status === "COMPLETE" &&
      review.input_freshness_status === "CURRENT_AT_DECISION" &&
      review.ledger_contract_status === "RECONCILED" &&
      review.no_live_safety_status === "CLEAR" &&
      review.execution_boundary === "PAPER_SIMULATION_ONLY" &&
      review.review_scope === "PAPER_NON_LIVE_EVIDENCE_ONLY" &&
      review.grants_authority === false &&
      review.live_promotion_available === false &&
      review.mfa_method === "totp" &&
      exactTimestamp(review.evidence_started_at, gate.as_of ?? "") &&
      exactTimestamp(review.evidence_eligible_at, gate.as_of ?? "") &&
      exactTimestamp(review.evidence_as_of, gate.as_of ?? "") &&
      exactTimestamp(review.portfolio_updated_at, gate.as_of ?? "") &&
      exactTimestamp(review.latest_checkpoint_as_of, gate.as_of ?? "") &&
      exactTimestamp(review.reviewed_at, item.freshnessObservedAt ?? "") &&
      exactTimestamp(review.created_at, item.freshnessObservedAt ?? "") &&
      Date.parse(review.evidence_eligible_at) -
        Date.parse(review.evidence_started_at) ===
        168 * 60 * 60 * 1000 &&
      Date.parse(review.reviewed_at) >= Date.parse(review.evidence_as_of),
  );
  if (review && !reviewBindingExact) {
    return {
      key: "PAPER_EVIDENCE",
      label: "Paper evidence and owner review",
      state: "UNAVAILABLE",
      detail:
        "The latest owner review does not bind exactly to this Paper engine, account, mandate version, checkpoint, or non-live evidence contract.",
      evidence: review.id,
    };
  }
  const reviewed = Boolean(
    gate.status === "EVIDENCE_REVIEWABLE" &&
      item.paperEvidenceReviewContractAvailable === true &&
      item.paperCurrentEvidenceReviewed === true &&
      item.paperEvidenceReviewFingerprint &&
      /^[0-9a-f]{64}$/.test(item.paperEvidenceReviewFingerprint) &&
      reviewBindingExact &&
      review?.evidence_fingerprint === item.paperEvidenceReviewFingerprint,
  );
  const state: PaperLiveReadinessState =
    gate.status === "REVIEW_REQUIRED"
      ? "BLOCKED"
      : gate.status === "UNAVAILABLE"
        ? "UNAVAILABLE"
        : reviewed
          ? "PROVEN"
          : gate.status === "EVIDENCE_REVIEWABLE"
            ? "BLOCKED"
            : "COLLECTING";
  return {
    key: "PAPER_EVIDENCE",
    label: "Paper evidence and owner review",
    state,
    detail: reviewed
      ? `${gate.decision_count} automatic decisions across ${gate.evidence_window_hours} hours are reviewable and the current fingerprint has an MFA-backed owner acknowledgment.`
      : gate.status === "EVIDENCE_REVIEWABLE"
        ? "The saved Paper checkpoint is reviewable, but its current exact fingerprint has not been owner-acknowledged with fresh MFA. That acknowledgment grants no authority."
        : gate.status === "COLLECTING_EVIDENCE"
          ? `${gate.decision_count}/${gate.minimum_decision_count} decisions and ${gate.evidence_window_hours}/${gate.minimum_evidence_window_hours} evidence hours are saved.`
          : "The saved Paper evidence gate requires review before it can be relied upon.",
    evidence: `${gate.status} · as of ${readableTime(gate.as_of)}`,
  };
}

function accountingSignal(item: StrategyFleetItem): PaperLiveReadinessSignal {
  const mismatch = item.paperOutcomeReconciliationStatus === "MISMATCH";
  const complete = Boolean(
    item.paperPortfolioAvailable === true &&
      item.paperRealizedContractAvailable === true &&
      ["AVAILABLE", "NO_REALIZED_SALES"].includes(
        item.paperRealizedOutcomeStatus ?? "",
      ) &&
      item.paperExecutionCostsContractAvailable === true &&
      ["AVAILABLE", "NO_FILLS"].includes(
        item.paperExecutionCostsStatus ?? "",
      ) &&
      item.paperActivityCadenceContractAvailable === true &&
      item.paperActivityCadence?.status === "AVAILABLE" &&
      ["RECONCILED_EXACT", "RECONCILED_WITH_DECIMAL_RESIDUAL"].includes(
        item.paperOutcomeReconciliationStatus ?? "",
      ) &&
      exactNonNegative(item.paperCashHeadroom) &&
      exactNonNegative(item.paperExposureHeadroom) &&
      exactNonNegative(item.paperProposalHeadroom),
  );
  return {
    key: "ACCOUNTING",
    label: "Paper accounting and operating evidence",
    state: mismatch ? "BLOCKED" : complete ? "PROVEN" : "UNAVAILABLE",
    detail: mismatch
      ? "The exact realized, unrealized, total, cash, and exposure paths do not reconcile inside the strict saved bound."
      : complete
        ? "The isolated ledger, realized and unrealized outcomes, explicit simulated costs, activity cadence, and capital headroom reconcile from immutable evidence."
        : "One or more Paper ledger, outcome, cost, cadence, or exact headroom contracts are unavailable; no value is reconstructed.",
    evidence: `${item.paperOutcomeReconciliationStatus ?? "UNAVAILABLE"} · ${item.paperExecutionFillCount ?? "UNAVAILABLE"} simulation-only fills`,
  };
}

function shadowSignal(
  paper: StrategyFleetItem,
  items: StrategyFleetItem[],
): PaperLiveReadinessSignal & {
  shadowInstanceID?: string;
  shadowMandateID?: string;
} {
  const matches = items.filter(
    (item) =>
      isAI(item) &&
      item.executionMode === "SHADOW" &&
      item.instanceStatus === "ACTIVE" &&
      item.financialAccountID === paper.financialAccountID &&
      item.provider === paper.provider,
  );
  if (matches.length !== 1) {
    return {
      key: "SHADOW_EVIDENCE",
      label: "Companion Shadow evidence",
      state: "UNAVAILABLE",
      detail:
        "Exactly one active, same-account Shadow engine was not available; Arbion will not borrow evidence from another account or provider.",
      evidence: `${matches.length} exact same-account matches`,
    };
  }
  const shadow = matches[0];
  const countsExact = Boolean(
    shadow.evidenceAvailable === true &&
      shadow.oneHourSampleSize !== undefined &&
      shadow.twentyFourHourSampleSize !== undefined &&
      shadow.minimumSamplePerHorizon !== undefined &&
      shadow.evidenceWindowHours !== undefined &&
      shadow.minimumEvidenceWindowHours !== undefined,
  );
  if (!countsExact) {
    return {
      key: "SHADOW_EVIDENCE",
      label: "Companion Shadow evidence",
      state: "UNAVAILABLE",
      detail:
        "The same-account Shadow scorecard or exact sample targets are unavailable.",
      evidence: shadow.strategyInstanceID ?? "UNAVAILABLE",
      shadowInstanceID: shadow.strategyInstanceID,
      shadowMandateID: shadow.id,
    };
  }
  const reviewable = Boolean(
    shadow.evidenceStatus === "EVIDENCE_REVIEWABLE" &&
      shadow.evidenceScheduleHealthy === true &&
      shadow.oneHourSampleSize! >= shadow.minimumSamplePerHorizon! &&
      shadow.twentyFourHourSampleSize! >= shadow.minimumSamplePerHorizon! &&
      shadow.evidenceWindowHours! >= shadow.minimumEvidenceWindowHours!,
  );
  const blocked = Boolean(
    shadow.evidenceStatus === "REVIEW_REQUIRED" ||
      shadow.evidenceScheduleHealthy === false,
  );
  return {
    key: "SHADOW_EVIDENCE",
    label: "Companion Shadow evidence",
    state: reviewable ? "PROVEN" : blocked ? "BLOCKED" : "COLLECTING",
    detail: reviewable
      ? `${shadow.oneHourSampleSize} one-hour and ${shadow.twentyFourHourSampleSize} 24-hour marks span ${shadow.evidenceWindowHours} saved hours on the same account.`
      : blocked
        ? "The same-account Shadow evidence or scheduler health requires review."
        : `${shadow.oneHourSampleSize}/${shadow.minimumSamplePerHorizon} one-hour and ${shadow.twentyFourHourSampleSize}/${shadow.minimumSamplePerHorizon} 24-hour marks are saved.`,
    evidence: shadow.strategyInstanceID ?? "UNAVAILABLE",
    shadowInstanceID: shadow.strategyInstanceID,
    shadowMandateID: shadow.id,
  };
}

function liveBoundarySignal(item: StrategyFleetItem): PaperLiveReadinessSignal {
  const gate = item.paperEvidenceReadiness;
  const exact = Boolean(
    gate?.live_execution_available === false &&
      gate.review_packet.live_promotion_available === false &&
      gate.review_packet.grants_authority === false,
  );
  return {
    key: "LIVE_CAPABILITY",
    label: "Live platform capability",
    state: "BLOCKED",
    detail: exact
      ? "The saved platform contract explicitly keeps live execution, promotion, and authority unavailable. Any future live system requires a separately scoped engineering, security, broker-permission, and legal review."
      : "The platform does not expose a live path, and the exact non-authorizing contract is unavailable for this snapshot.",
    evidence: exact
      ? "live_execution_available=false · live_promotion_available=false · grants_authority=false"
      : "UNAVAILABLE · live remains disabled",
  };
}

function engineStatus(signals: PaperLiveReadinessSignal[]) {
  const nonPlatform = signals.filter(
    (signal) => signal.key !== "LIVE_CAPABILITY",
  );
  if (nonPlatform.some((signal) => signal.state === "BLOCKED"))
    return "REVIEW_REQUIRED" as const;
  if (nonPlatform.some((signal) => signal.state === "UNAVAILABLE"))
    return "EVIDENCE_UNAVAILABLE" as const;
  if (nonPlatform.some((signal) => signal.state === "COLLECTING"))
    return "EVIDENCE_COLLECTING" as const;
  return "PLATFORM_BLOCKED" as const;
}

function ownerAction(status: PaperLiveReadinessDossierProjection["status"]) {
  if (status === "REVIEW_REQUIRED")
    return "Review the exact blocked non-live evidence. Do not enable or simulate a live path.";
  if (status === "EVIDENCE_UNAVAILABLE")
    return "Restore the missing saved evidence contract before relying on this dossier; no readiness is inferred.";
  if (status === "EVIDENCE_COLLECTING")
    return "Let the next normal Paper and Shadow schedules continue collecting. Do not run a manual cycle.";
  return "The non-live evidence is complete, but live capability remains intentionally unavailable pending a separately scoped review and implementation.";
}

export function projectPaperToLiveReadinessDossier(
  items: StrategyFleetItem[],
): PaperLiveReadinessDossierProjection {
  const paperEngines = items.filter(
    (item) =>
      isAI(item) &&
      item.executionMode === "PAPER" &&
      item.instanceStatus === "ACTIVE",
  );
  const fleetIdentityExact = exactFleetIdentity(items);
  const engines = paperEngines.map((item) => {
    const shadow = shadowSignal(item, items);
    const signals: PaperLiveReadinessSignal[] = [
      connectionSignal(item),
      capitalSignal(item, fleetIdentityExact),
      scheduleSignal(item),
      provenanceSignal(item),
      boundarySignal(item),
      paperEvidenceSignal(item),
      accountingSignal(item),
      shadow,
      liveBoundarySignal(item),
    ];
    const status = engineStatus(signals);
    return {
      id: item.id,
      instanceID: item.strategyInstanceID!,
      title: item.title,
      accountName: item.accountName,
      provider: item.provider,
      modelRoute: `${item.latestDecisionAIProvider ?? "UNAVAILABLE"} / ${item.latestDecisionAIModelID ?? item.modelID ?? "UNAVAILABLE"} / ${item.latestDecisionAIProfile ?? "UNAVAILABLE"}`,
      status,
      ownerAction: ownerAction(status),
      signals,
      shadowInstanceID: shadow.shadowInstanceID,
      shadowMandateID: shadow.shadowMandateID,
      detailHref: `/automations/${encodeURIComponent(item.id)}#runtime-evidence`,
    };
  });
  const signals = engines.flatMap((engine) => engine.signals);
  const unavailableCount = signals.filter(
    (signal) => signal.state === "UNAVAILABLE",
  ).length;
  const blockedCount = signals.filter(
    (signal) => signal.state === "BLOCKED",
  ).length;
  const collectingCount = signals.filter(
    (signal) => signal.state === "COLLECTING",
  ).length;
  const status = engines.some((engine) => engine.status === "REVIEW_REQUIRED")
    ? "REVIEW_REQUIRED"
    : engines.some((engine) => engine.status === "EVIDENCE_UNAVAILABLE")
      ? "EVIDENCE_UNAVAILABLE"
      : engines.some((engine) => engine.status === "EVIDENCE_COLLECTING")
        ? "EVIDENCE_COLLECTING"
        : "PLATFORM_BLOCKED";
  return {
    status,
    paperEngineCount: engines.length,
    provenCount: signals.filter((signal) => signal.state === "PROVEN").length,
    collectingCount,
    blockedCount,
    unavailableCount,
    engines,
  };
}

function statusLabel(status: PaperLiveReadinessDossierProjection["status"]) {
  if (status === "REVIEW_REQUIRED") return "Non-live review required";
  if (status === "EVIDENCE_UNAVAILABLE")
    return "Readiness evidence unavailable";
  if (status === "EVIDENCE_COLLECTING") return "Evidence collecting";
  return "Live platform blocked";
}

function signalLabel(state: PaperLiveReadinessState) {
  if (state === "PROVEN") return "Proven";
  if (state === "COLLECTING") return "Collecting";
  if (state === "BLOCKED") return "Blocked";
  return "Unavailable";
}

export function PaperToLiveReadinessDossier({
  items,
}: {
  items: StrategyFleetItem[];
}) {
  const dossier = projectPaperToLiveReadinessDossier(items);
  if (dossier.paperEngineCount === 0) return null;
  return (
    <section
      className={`paper-live-readiness-dossier is-${dossier.status.toLowerCase().replaceAll("_", "-")}`}
      aria-labelledby="paper-live-readiness-dossier-title"
    >
      <header>
        <div>
          <p className="eyebrow">PAPER-TO-LIVE READINESS DOSSIER</p>
          <h2 id="paper-live-readiness-dossier-title">
            Live execution remains intentionally unavailable.
          </h2>
          <p>
            Exact proof of what is ready, still collecting, unavailable, or
            structurally blocked—without treating Paper outcomes as
            profitability, suitability, broker permission, or authority.
          </p>
        </div>
        <span>{statusLabel(dossier.status)}</span>
      </header>
      <div className="paper-live-readiness-dossier-metrics">
        <article>
          <strong>{dossier.provenCount}</strong>
          <span>proven controls</span>
        </article>
        <article>
          <strong>{dossier.collectingCount}</strong>
          <span>collecting</span>
        </article>
        <article>
          <strong>{dossier.blockedCount}</strong>
          <span>blocked</span>
        </article>
        <article>
          <strong>{dossier.unavailableCount}</strong>
          <span>unavailable</span>
        </article>
      </div>
      <ol>
        {dossier.engines.map((engine) => {
          const attention = engine.status !== "PLATFORM_BLOCKED";
          return (
            <li key={engine.instanceID}>
              <header>
                <div>
                  <strong>{engine.title}</strong>
                  <small>
                    {engine.accountName} · {engine.provider} · PAPER
                  </small>
                </div>
                <span>{statusLabel(engine.status)}</span>
              </header>
              <p>{engine.ownerAction}</p>
              <dl>
                <div>
                  <dt>Paper engine</dt>
                  <dd>{engine.instanceID}</dd>
                </div>
                <div>
                  <dt>Exact AI route</dt>
                  <dd>{engine.modelRoute}</dd>
                </div>
                <div>
                  <dt>Companion Shadow</dt>
                  <dd>{engine.shadowInstanceID ?? "UNAVAILABLE"}</dd>
                </div>
              </dl>
              <details open={attention}>
                <summary>
                  Exact prerequisite evidence
                  <span>{engine.signals.length} controls</span>
                </summary>
                <ol>
                  {engine.signals.map((signal) => (
                    <li
                      className={`is-${signal.state.toLowerCase()}`}
                      key={signal.key}
                    >
                      <header>
                        <strong>{signal.label}</strong>
                        <span>{signalLabel(signal.state)}</span>
                      </header>
                      <p>{signal.detail}</p>
                      <small>{signal.evidence}</small>
                    </li>
                  ))}
                </ol>
              </details>
              <footer>
                <Link href={engine.detailHref}>
                  Open immutable Paper evidence →
                </Link>
                {engine.shadowMandateID && (
                  <Link
                    href={`/automations/${encodeURIComponent(engine.shadowMandateID)}#runtime-evidence`}
                  >
                    Open companion Shadow evidence →
                  </Link>
                )}
              </footer>
            </li>
          );
        })}
      </ol>
      <footer>
        Saved non-live evidence only · no promotion control · no credential
        inference · no provider contact · no model rerun · no manual cycle · no
        account mutation · no broker order · no live path
      </footer>
    </section>
  );
}
