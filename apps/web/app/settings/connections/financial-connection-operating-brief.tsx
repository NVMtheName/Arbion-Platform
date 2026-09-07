import Link from "next/link";

import {
  ConnectionSyncEvidenceCenter,
  type ConnectionSyncEvidenceInput,
  projectConnectionSyncEvidence,
} from "./connection-sync-evidence-center";
import {
  FinancialContinuityCenter,
  SchwabMarketDataReadinessView,
  type FinancialContinuityEngine,
  projectFinancialContinuityCenter,
} from "./financial-continuity-center";
import type { FinancialAccount, FinancialConnection } from "./page";

type OperatingState = "ON_COURSE" | "COLLECTING" | "REVIEW" | "UNAVAILABLE";

export type FinancialAuthorizationReceipt = {
  id: string;
  attempt_id?: string;
  provider: string;
  status: "STARTED" | "COMPLETED" | "FAILED";
  connection_id?: string;
  authorization_expires_at?: string;
  current_last_verified_at?: string;
  authorization_expiry_matches_current_connection?: boolean;
  occurred_at: string;
};

type AuthorizationRenewalState =
  | "CURRENT"
  | "EXPIRING"
  | "EXPIRED"
  | "PENDING"
  | "FAILED"
  | "UNAVAILABLE";

export type FinancialAuthorizationTimelineProjection = {
  status: "VERIFIED" | "ATTENTION" | "UNAVAILABLE";
  currentCount: number;
  attentionCount: number;
  unavailableCount: number;
  connections: Array<{
    id: string;
    provider: string;
    displayName: string;
    state: AuthorizationRenewalState;
    label: string;
    guidance: string;
    latestReceiptAt?: string;
    lastSuccessfulAt?: string;
    lastVerifiedAt?: string;
    authorizationExpiresAt?: string;
    remainingMilliseconds?: number;
    pendingAgeMilliseconds?: number;
    attemptCount: number;
    pairedAttemptCount: number;
    latestAttemptDurationMilliseconds?: number;
    attempts: Array<{
      key: string;
      status: "PENDING" | "COMPLETED" | "FAILED";
      startedAt?: string;
      terminalAt?: string;
      durationMilliseconds?: number;
      eventIDs: string[];
    }>;
  }>;
};

type AuthorizationIncidentState = "OPEN" | "RECOVERED" | "UNAVAILABLE";
type AuthorizationIncidentKind =
  | "FAILED_ATTEMPT"
  | "LONG_PENDING"
  | "EXPIRED_AUTHORIZATION";

export type FinancialAuthorizationRuntimeIncidentProjection = {
  status: "VERIFIED" | "ATTENTION" | "UNAVAILABLE";
  incidentCount: number;
  openCount: number;
  recoveredCount: number;
  unavailableCount: number;
  unboundProviderEventCount: number;
  connections: Array<{
    id: string;
    provider: string;
    displayName: string;
    state: "CLEAR" | AuthorizationIncidentState;
    incidentCount: number;
    incidents: Array<{
      id: string;
      kind: AuthorizationIncidentKind;
      state: AuthorizationIncidentState;
      startedAt: string;
      latestAt: string;
      recoveredAt?: string;
      durationMilliseconds?: number;
      currentAgeMilliseconds?: number;
      authorizationDeadline?: string;
      attemptIDs: string[];
      eventIDs: string[];
      runtimeStatus:
        | "PROTECTED"
        | "SAFE_WAIT"
        | "NEEDS_REVIEW"
        | "NO_POST_INCIDENT_SAMPLE"
        | "NO_BOUND_ENGINE"
        | "UNAVAILABLE";
      engines: Array<{
        instanceID: string;
        mandateID: string;
        accountName: string;
        executionMode: "PAPER" | "SHADOW";
        succeededCount: number;
        failedCount: number;
        safeWaitCount: number;
        blockedCount: number;
        latestStatus: string;
        latestErrorCode?: string;
        latestCompletedAt?: string;
      }>;
    }>;
  }>;
};

type AuthorizationContinuityState =
  | "CURRENT"
  | "RENEW_NOW"
  | "PENDING"
  | "FAILED"
  | "EXPIRED"
  | "UNAVAILABLE";

export type FinancialAuthorizationContinuitySLOProjection = {
  status: "VERIFIED" | "ATTENTION" | "UNAVAILABLE";
  currentCount: number;
  attentionCount: number;
  unavailableCount: number;
  connections: Array<{
    id: string;
    provider: string;
    displayName: string;
    state: AuthorizationContinuityState;
    label: string;
    guidance: string;
    lastVerifiedAt?: string;
    authorizationExpiresAt?: string;
    renewalWindowStartsAt?: string;
    remainingMilliseconds?: number;
    attemptCount: number;
    completedCount: number;
    pendingCount: number;
    failedCount: number;
    expiredCount: number;
    pairedAttemptCount: number;
    latestTerminalLatencyMilliseconds?: number;
    medianTerminalLatencyMilliseconds?: number;
    maximumTerminalLatencyMilliseconds?: number;
    openIncidentCount: number;
    recoveredIncidentCount: number;
    unavailableIncidentCount: number;
    unboundProviderEventCount: number;
    engines: Array<{
      instanceID: string;
      mandateID: string;
      accountName: string;
      executionMode: "PAPER" | "SHADOW";
      latestStatus: string;
      latestErrorCode?: string;
      latestCompletedAt: string;
      nextRunAt: string;
    }>;
  }>;
};

type AuthorizationRecoveryState =
  | "CURRENT"
  | "RENEWAL_WINDOW"
  | "PENDING"
  | "FAILED"
  | "EXPIRED"
  | "UNAVAILABLE";

export type FinancialAuthorizationRecoveryPlanProjection = {
  status: "VERIFIED" | "ATTENTION" | "UNAVAILABLE";
  currentCount: number;
  attentionCount: number;
  unavailableCount: number;
  accountCount: number;
  engineCount: number;
  capitalClaimCount: number;
  connections: Array<{
    id: string;
    provider: string;
    displayName: string;
    state: AuthorizationRecoveryState;
    label: string;
    deadline?: string;
    ownerAction: string;
    beforeExpiry: string;
    atExpiry: string;
    afterReconnect: string;
    incidentState: string;
    accounts: Array<{
      id: string;
      displayName: string;
      status: string;
    }>;
    engines: Array<{
      instanceID: string;
      mandateID: string;
      capitalBucketID: string;
      accountID: string;
      accountName: string;
      executionMode: "PAPER" | "SHADOW";
      runtimeState: "PROTECTED" | "SAFE_WAIT" | "FAILED_CLOSED";
      latestStatus: string;
      latestErrorCode?: string;
      latestCompletedAt: string;
      nextRunAt: string;
    }>;
  }>;
};

export type FinancialConnectionOperatingBriefProjection = {
  status: "VERIFIED" | "ATTENTION" | "UNAVAILABLE";
  onCourseCount: number;
  collectingCount: number;
  reviewCount: number;
  unavailableCount: number;
  connections: Array<{
    id: string;
    provider: string;
    displayName: string;
    state: OperatingState;
    label: string;
    guidance: string;
    connectionStatus: string;
    authorizationExpiresAt?: string;
    authorizationReceiptStatus:
      | "CURRENT"
      | "EXPIRING"
      | "EXPIRED"
      | "PENDING"
      | "FAILED"
      | "UNAVAILABLE";
    authorizationReceiptLabel: string;
    authorizationReceiptAt?: string;
    accountCount: number;
    latestPortfolioObservedAt?: string;
    syncAttemptCount: number;
    syncSuccessCount: number;
    recoveredFailureCount: number;
    currentFailureCount: number;
    paperEngineCount: number;
    shadowEngineCount: number;
    nextRunAt?: string;
    accountIDs: string[];
    mandateIDs: string[];
  }>;
};

function providerName(provider: string) {
  if (provider === "schwab") return "Charles Schwab";
  if (provider === "coinbase") return "Coinbase";
  return provider;
}

function readableTime(value?: string) {
  if (!value) return "UNAVAILABLE";
  const parsed = new Date(value);
  if (Number.isNaN(parsed.valueOf())) return "UNAVAILABLE";
  return new Intl.DateTimeFormat("en-US", {
    month: "short",
    day: "numeric",
    year: "numeric",
    hour: "numeric",
    minute: "2-digit",
    timeZone: "UTC",
    timeZoneName: "short",
  }).format(parsed);
}

function readableDuration(value?: number) {
  if (value === undefined || !Number.isFinite(value) || value < 0)
    return "UNAVAILABLE";
  if (value < 60_000) return `${Math.round(value / 1000)} seconds`;
  if (value < 3_600_000) return `${Math.round(value / 60_000)} minutes`;
  if (value < 86_400_000)
    return `${(value / 3_600_000).toLocaleString("en-US", { maximumFractionDigits: 1 })} hours`;
  return `${(value / 86_400_000).toLocaleString("en-US", { maximumFractionDigits: 1 })} days`;
}

function exactDuration(value?: number) {
  if (value === undefined || !Number.isFinite(value) || value < 0)
    return "UNAVAILABLE";
  return `${value.toLocaleString("en-US", { maximumFractionDigits: 3 })} ms · ${readableDuration(value)}`;
}

function earliestTime(values: Array<string | undefined>) {
  const available = values.filter((value): value is string => Boolean(value));
  if (available.length === 0) return;
  return available.sort(
    (left, right) => Date.parse(left) - Date.parse(right),
  )[0];
}

function latestTime(values: Array<string | undefined>) {
  const available = values.filter((value): value is string => Boolean(value));
  if (available.length === 0) return;
  return available.sort(
    (left, right) => Date.parse(right) - Date.parse(left),
  )[0];
}

const uuidPattern =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;
const attemptIDPattern = /^[a-f0-9]{64}$/;
const providerPattern = /^[a-z][a-z0-9_]{0,31}$/;
const renewalWindowMilliseconds = 24 * 60 * 60 * 1000;
const longPendingAuthorizationMilliseconds = 15 * 60 * 1000;
const authorizationIncidentLimit = 6;

type AuthorizationAttempt = {
  key: string;
  provider: string;
  connectionID?: string;
  started?: FinancialAuthorizationReceipt;
  terminal?: FinancialAuthorizationReceipt;
  eventIDs: string[];
};

function authorizationAttemptGroups(receipts: FinancialAuthorizationReceipt[]) {
  const attempts = new Map<string, AuthorizationAttempt>();
  for (const receipt of receipts) {
    const key = receipt.attempt_id ?? `legacy:${receipt.id}`;
    const attempt = attempts.get(key) ?? {
      key,
      provider: receipt.provider,
      eventIDs: [],
    };
    if (
      attempt.provider !== receipt.provider ||
      (attempt.connectionID &&
        receipt.connection_id &&
        attempt.connectionID !== receipt.connection_id) ||
      attempt.eventIDs.includes(receipt.id)
    )
      return;
    if (receipt.connection_id) attempt.connectionID = receipt.connection_id;
    if (receipt.status === "STARTED") {
      if (attempt.started) return;
      attempt.started = receipt;
    } else {
      if (attempt.terminal) return;
      attempt.terminal = receipt;
    }
    attempt.eventIDs.push(receipt.id);
    attempts.set(key, attempt);
  }
  const values = [...attempts.values()];
  if (
    values.some(
      (attempt) =>
        attempt.eventIDs.length > 2 ||
        (attempt.started &&
          attempt.terminal &&
          Date.parse(attempt.terminal.occurred_at) <=
            Date.parse(attempt.started.occurred_at)),
    )
  )
    return;
  return values;
}

function validAuthorizationEvidence({
  connections,
  receipts,
  observedAt,
  evidenceAvailable,
}: {
  connections: FinancialConnection[];
  receipts: FinancialAuthorizationReceipt[];
  observedAt: string;
  evidenceAvailable: boolean;
}) {
  const observed = Date.parse(observedAt);
  const receiptIDs = new Set<string>();
  const connectionIDs = new Set<string>();
  const connectionProviders = new Map<string, string>();
  if (!Number.isFinite(observed) || receipts.length > 20) return false;
  for (const connection of connections) {
    if (
      !uuidPattern.test(connection.id) ||
      !providerPattern.test(connection.provider) ||
      connectionIDs.has(connection.id)
    )
      return false;
    connectionIDs.add(connection.id);
    connectionProviders.set(connection.id, connection.provider);
  }
  for (const receipt of receipts) {
    const occurred = Date.parse(receipt.occurred_at);
    const expires = receipt.authorization_expires_at
      ? Date.parse(receipt.authorization_expires_at)
      : undefined;
    const verified = receipt.current_last_verified_at
      ? Date.parse(receipt.current_last_verified_at)
      : undefined;
    if (
      !uuidPattern.test(receipt.id) ||
      receiptIDs.has(receipt.id) ||
      !providerPattern.test(receipt.provider) ||
      !["STARTED", "COMPLETED", "FAILED"].includes(receipt.status) ||
      !Number.isFinite(occurred) ||
      occurred > observed ||
      (receipt.attempt_id !== undefined &&
        !attemptIDPattern.test(receipt.attempt_id)) ||
      (receipt.connection_id !== undefined &&
        (!uuidPattern.test(receipt.connection_id) ||
          (connectionProviders.has(receipt.connection_id) &&
            connectionProviders.get(receipt.connection_id) !==
              receipt.provider))) ||
      (receipt.authorization_expires_at !== undefined &&
        (!Number.isFinite(expires) || (expires ?? 0) <= occurred)) ||
      (receipt.current_last_verified_at !== undefined &&
        (!Number.isFinite(verified) || (verified ?? 0) > observed)) ||
      (receipt.status === "COMPLETED" &&
        (!receipt.connection_id ||
          !receipt.current_last_verified_at ||
          typeof receipt.authorization_expiry_matches_current_connection !==
            "boolean")) ||
      (receipt.status !== "COMPLETED" &&
        (receipt.authorization_expires_at !== undefined ||
          receipt.current_last_verified_at !== undefined ||
          receipt.authorization_expiry_matches_current_connection !==
            undefined))
    )
      return false;
    receiptIDs.add(receipt.id);
  }
  return Boolean(
    evidenceAvailable && authorizationAttemptGroups(receipts) !== undefined,
  );
}

export function projectFinancialAuthorizationTimeline({
  connections,
  receipts,
  observedAt,
  evidenceAvailable,
}: {
  connections: FinancialConnection[];
  receipts: FinancialAuthorizationReceipt[];
  observedAt: string;
  evidenceAvailable: boolean;
}): FinancialAuthorizationTimelineProjection {
  const observed = Date.parse(observedAt);
  const exactEvidence =
    evidenceAvailable &&
    validAuthorizationEvidence({
      connections,
      receipts,
      observedAt,
      evidenceAvailable,
    });
  const safeReceipts = exactEvidence ? receipts : [];
  const groups = exactEvidence ? authorizationAttemptGroups(receipts) : [];
  const projected = connections.map((connection) => {
    const lastVerifiedAt = connection.last_synced_at ?? undefined;
    const lastVerified = lastVerifiedAt
      ? Date.parse(lastVerifiedAt)
      : Number.NaN;
    const expiresAt = connection.authorization_expires_at ?? undefined;
    const expires = expiresAt ? Date.parse(expiresAt) : undefined;
    const boundAttempts = (groups ?? [])
      .filter((attempt) => attempt.connectionID === connection.id)
      .toSorted((left, right) => {
        const leftAt = Date.parse(
          left.terminal?.occurred_at ?? left.started?.occurred_at ?? "",
        );
        const rightAt = Date.parse(
          right.terminal?.occurred_at ?? right.started?.occurred_at ?? "",
        );
        return rightAt - leftAt;
      });
    const completions = safeReceipts
      .filter(
        (receipt) =>
          receipt.provider === connection.provider &&
          receipt.connection_id === connection.id &&
          receipt.status === "COMPLETED",
      )
      .toSorted(
        (left, right) =>
          Date.parse(right.occurred_at) - Date.parse(left.occurred_at),
      );
    const currentCompletion = completions.find(
      (receipt) =>
        receipt.authorization_expiry_matches_current_connection === true &&
        Date.parse(receipt.current_last_verified_at ?? "") === lastVerified,
    );
    const currentCompletionAt = currentCompletion
      ? Date.parse(currentCompletion.occurred_at)
      : undefined;
    const latestAttempt = boundAttempts[0];
    const latestAttemptAt = latestAttempt
      ? Date.parse(
          latestAttempt.terminal?.occurred_at ??
            latestAttempt.started?.occurred_at ??
            "",
        )
      : undefined;
    const latestPending = Boolean(
      latestAttempt?.started &&
        !latestAttempt.terminal &&
        latestAttemptAt !== undefined &&
        (currentCompletionAt === undefined ||
          latestAttemptAt > currentCompletionAt),
    );
    const latestFailed = Boolean(
      latestAttempt?.terminal?.status === "FAILED" &&
        latestAttemptAt !== undefined &&
        (currentCompletionAt === undefined ||
          latestAttemptAt > currentCompletionAt),
    );
    const attempts = boundAttempts.slice(0, 6).map((attempt) => {
      const startedAt = attempt.started?.occurred_at;
      const terminalAt = attempt.terminal?.occurred_at;
      const durationMilliseconds =
        startedAt && terminalAt
          ? Date.parse(terminalAt) - Date.parse(startedAt)
          : undefined;
      return {
        key: attempt.key,
        status: attempt.terminal
          ? attempt.terminal.status === "COMPLETED"
            ? ("COMPLETED" as const)
            : ("FAILED" as const)
          : ("PENDING" as const),
        startedAt,
        terminalAt,
        durationMilliseconds,
        eventIDs: [...attempt.eventIDs],
      };
    });

    let state: AuthorizationRenewalState = "CURRENT";
    let label = "Authorization current";
    let guidance =
      "No owner action is required. The latest saved completion matches this connection.";
    if (
      !exactEvidence ||
      !uuidPattern.test(connection.id) ||
      !providerPattern.test(connection.provider) ||
      !Number.isFinite(lastVerified) ||
      lastVerified > observed ||
      (expiresAt !== undefined && !Number.isFinite(expires)) ||
      !currentCompletion
    ) {
      state = "UNAVAILABLE";
      label = "Authorization evidence unavailable";
      guidance =
        "Review the saved receipts. Arbion will not infer a renewal, expiry, or provider outcome from an incomplete chain.";
    } else if (
      ["expired", "revoked"].includes(connection.status) ||
      (expires !== undefined && expires <= observed)
    ) {
      state = "EXPIRED";
      label = "Authorization expired";
      guidance =
        "Use the existing provider reconnect control. Existing holdings and non-live engines remain preserved and fail closed.";
    } else if (latestPending) {
      state = "PENDING";
      label = "Authorization callback pending";
      guidance =
        "Finish the provider callback in the same browser session. Arbion has not inferred a renewal and has not changed this account.";
    } else if (latestFailed) {
      state = "FAILED";
      label = "Latest authorization attempt failed closed";
      guidance =
        "Reconnect from the provider card when ready. The failed receipt is preserved without changing holdings or guarded engines.";
    } else if (
      expires !== undefined &&
      expires - observed <= renewalWindowMilliseconds
    ) {
      state = "EXPIRING";
      label = "Authorization expires within 24 hours";
      guidance =
        "Use the existing provider reconnect control before the exact deadline to avoid a fail-closed interruption.";
    }
    const pairedAttempts = attempts.filter(
      (attempt) => attempt.durationMilliseconds !== undefined,
    );
    return {
      id: connection.id,
      provider: connection.provider,
      displayName: connection.display_name,
      state,
      label,
      guidance,
      latestReceiptAt:
        latestAttempt?.terminal?.occurred_at ??
        latestAttempt?.started?.occurred_at ??
        currentCompletion?.occurred_at,
      lastSuccessfulAt: currentCompletion?.occurred_at,
      lastVerifiedAt: exactEvidence ? lastVerifiedAt : undefined,
      authorizationExpiresAt: exactEvidence ? expiresAt : undefined,
      remainingMilliseconds:
        exactEvidence && expires !== undefined ? expires - observed : undefined,
      pendingAgeMilliseconds:
        state === "PENDING" && latestAttempt?.started
          ? observed - Date.parse(latestAttempt.started.occurred_at)
          : undefined,
      attemptCount: boundAttempts.length,
      pairedAttemptCount: pairedAttempts.length,
      latestAttemptDurationMilliseconds:
        pairedAttempts[0]?.durationMilliseconds,
      attempts,
    };
  });
  const unavailableCount = projected.filter(
    (connection) => connection.state === "UNAVAILABLE",
  ).length;
  const attentionCount = projected.filter(
    (connection) => !["CURRENT", "UNAVAILABLE"].includes(connection.state),
  ).length;
  return {
    status:
      unavailableCount > 0
        ? "UNAVAILABLE"
        : attentionCount > 0
          ? "ATTENTION"
          : "VERIFIED",
    currentCount: projected.filter(
      (connection) => connection.state === "CURRENT",
    ).length,
    attentionCount,
    unavailableCount,
    connections: projected,
  };
}

function validIncidentRuntimeEvidence({
  connections,
  engines,
  observedAt,
}: {
  connections: FinancialConnection[];
  engines: FinancialContinuityEngine[];
  observedAt: string;
}) {
  const observed = Date.parse(observedAt);
  const connectionProviders = new Map(
    connections.map((connection) => [connection.id, connection.provider]),
  );
  const instanceIDs = new Set<string>();
  return engines.every((engine) => {
    const connectionID = engine.connection_id;
    const instanceID = engine.instance_id;
    const mandateID = engine.mandate_id;
    const provider = engine.provider;
    if (
      !connectionID ||
      !instanceID ||
      !mandateID ||
      !uuidPattern.test(connectionID) ||
      !uuidPattern.test(instanceID) ||
      !uuidPattern.test(mandateID) ||
      instanceIDs.has(instanceID) ||
      !provider ||
      connectionProviders.get(connectionID) !== provider ||
      !["PAPER", "SHADOW"].includes(engine.execution_mode ?? "") ||
      engine.instance_status !== "ACTIVE" ||
      engine.current_state !== "AI_MONITORING" ||
      !engine.schedule_available ||
      !engine.schedule_history_available ||
      engine.recent_runs.length > 12
    )
      return false;
    instanceIDs.add(instanceID);
    const runIDs = new Set<string>();
    let priorCompletedAt = Number.POSITIVE_INFINITY;
    return engine.recent_runs.every((run) => {
      const scheduledAt = Date.parse(run.scheduled_for ?? "");
      const completedAt = Date.parse(run.completed_at ?? "");
      const nextRunAt = Date.parse(run.next_run_at ?? "");
      const exact = Boolean(
        run.id &&
          uuidPattern.test(run.id) &&
          !runIDs.has(run.id) &&
          Number.isFinite(scheduledAt) &&
          Number.isFinite(completedAt) &&
          Number.isFinite(nextRunAt) &&
          scheduledAt <= completedAt &&
          completedAt <= observed &&
          completedAt < priorCompletedAt &&
          nextRunAt > scheduledAt &&
          run.status &&
          ["SUCCEEDED", "FAILED", "SKIPPED"].includes(run.status) &&
          typeof run.duplicate_recovered === "boolean" &&
          Number.isInteger(run.consecutive_failures) &&
          (run.consecutive_failures ?? -1) >= 0 &&
          ((run.status === "SUCCEEDED" && !run.error_code) ||
            (["FAILED", "SKIPPED"].includes(run.status) &&
              Boolean(run.error_code))),
      );
      if (run.id) runIDs.add(run.id);
      priorCompletedAt = completedAt;
      return exact;
    });
  });
}

export function projectFinancialAuthorizationRuntimeIncidents({
  connections,
  receipts,
  engines,
  observedAt,
  evidenceAvailable,
}: {
  connections: FinancialConnection[];
  receipts: FinancialAuthorizationReceipt[];
  engines: FinancialContinuityEngine[];
  observedAt: string;
  evidenceAvailable: boolean;
}): FinancialAuthorizationRuntimeIncidentProjection {
  const observed = Date.parse(observedAt);
  const authorizationEvidenceExact = validAuthorizationEvidence({
    connections,
    receipts,
    observedAt,
    evidenceAvailable,
  });
  const runtimeEvidenceExact = validIncidentRuntimeEvidence({
    connections,
    engines,
    observedAt,
  });
  const exactEvidence = authorizationEvidenceExact && runtimeEvidenceExact;
  const groups = exactEvidence
    ? (authorizationAttemptGroups(receipts) ?? [])
    : [];
  const unboundProviderEventCount = exactEvidence
    ? groups
        .filter((attempt) => !attempt.connectionID)
        .reduce((count, attempt) => count + attempt.eventIDs.length, 0)
    : 0;

  const projected = connections.map((connection) => {
    if (!exactEvidence) {
      return {
        id: connection.id,
        provider: connection.provider,
        displayName: connection.display_name,
        state: "UNAVAILABLE" as const,
        incidentCount: 0,
        incidents: [],
      };
    }
    const boundGroups = groups
      .filter(
        (attempt) =>
          attempt.connectionID === connection.id &&
          attempt.provider === connection.provider,
      )
      .toSorted(
        (left, right) =>
          Date.parse(
            left.terminal?.occurred_at ?? left.started?.occurred_at ?? "",
          ) -
          Date.parse(
            right.terminal?.occurred_at ?? right.started?.occurred_at ?? "",
          ),
      );
    const completionGroups = boundGroups.filter(
      (attempt) => attempt.terminal?.status === "COMPLETED",
    );
    const completedGroups = completionGroups.filter(
      (attempt) =>
        attempt.started &&
        attempt.terminal &&
        !attempt.key.startsWith("legacy:"),
    );
    const connectionEngines = engines.filter(
      (engine) => engine.connection_id === connection.id,
    );

    const makeIncident = ({
      id,
      kind,
      startedAt,
      latestAt,
      authorizationDeadline,
      attemptIDs,
      eventIDs,
      recoveryCandidates = completedGroups,
    }: {
      id: string;
      kind: AuthorizationIncidentKind;
      startedAt: string;
      latestAt: string;
      authorizationDeadline?: string;
      attemptIDs: string[];
      eventIDs: string[];
      recoveryCandidates?: AuthorizationAttempt[];
    }) => {
      const started = Date.parse(startedAt);
      const recoveryThreshold = Date.parse(latestAt);
      const recovery = recoveryCandidates.find(
        (attempt) =>
          Date.parse(attempt.terminal?.occurred_at ?? "") > recoveryThreshold &&
          !attemptIDs.includes(attempt.key),
      );
      const recoveredAt = recovery?.terminal?.occurred_at;
      const windowEnd = observed;
      const engineRows = connectionEngines.map((engine) => {
        const runs = engine.recent_runs.filter((run) => {
          const completedAt = Date.parse(run.completed_at ?? "");
          return completedAt >= started && completedAt <= windowEnd;
        });
        const succeededCount = runs.filter(
          (run) => run.status === "SUCCEEDED",
        ).length;
        const failedCount = runs.filter(
          (run) => run.status === "FAILED",
        ).length;
        const safeWaitCount = runs.filter(
          (run) =>
            run.status === "SKIPPED" && run.error_code === "OUTSIDE_SESSION",
        ).length;
        const blockedCount = runs.filter(
          (run) =>
            run.status === "SKIPPED" && run.error_code !== "OUTSIDE_SESSION",
        ).length;
        const latest = runs[0];
        return {
          instanceID: engine.instance_id!,
          mandateID: engine.mandate_id!,
          accountName: engine.account_name ?? "Financial account",
          executionMode: engine.execution_mode as "PAPER" | "SHADOW",
          succeededCount,
          failedCount,
          safeWaitCount,
          blockedCount,
          latestStatus: latest?.status ?? "NO SAVED SAMPLE",
          latestErrorCode: latest?.error_code ?? undefined,
          latestCompletedAt: latest?.completed_at ?? undefined,
        };
      });
      const sampledRows = engineRows.filter(
        (engine) =>
          engine.succeededCount +
            engine.failedCount +
            engine.safeWaitCount +
            engine.blockedCount >
          0,
      );
      let runtimeStatus:
        | "PROTECTED"
        | "SAFE_WAIT"
        | "NEEDS_REVIEW"
        | "NO_POST_INCIDENT_SAMPLE"
        | "NO_BOUND_ENGINE"
        | "UNAVAILABLE" = "PROTECTED";
      if (connectionEngines.length === 0) runtimeStatus = "NO_BOUND_ENGINE";
      else if (sampledRows.length !== connectionEngines.length)
        runtimeStatus = "NO_POST_INCIDENT_SAMPLE";
      else if (
        sampledRows.some(
          (engine) =>
            engine.latestStatus === "FAILED" ||
            engine.failedCount > 0 ||
            engine.blockedCount > 0,
        )
      )
        runtimeStatus = "NEEDS_REVIEW";
      else if (
        sampledRows.length > 0 &&
        sampledRows.every((engine) => engine.latestStatus === "SKIPPED")
      )
        runtimeStatus = "SAFE_WAIT";
      return {
        id,
        kind,
        state: recoveredAt ? ("RECOVERED" as const) : ("OPEN" as const),
        startedAt,
        latestAt,
        recoveredAt,
        durationMilliseconds: recoveredAt
          ? Date.parse(recoveredAt) - started
          : undefined,
        currentAgeMilliseconds: recoveredAt ? undefined : observed - started,
        authorizationDeadline,
        attemptIDs:
          recovery && !recovery.key.startsWith("legacy:")
            ? [...attemptIDs, recovery.key]
            : [...attemptIDs],
        eventIDs: recovery
          ? [...eventIDs, ...recovery.eventIDs]
          : [...eventIDs],
        runtimeStatus,
        engines: engineRows,
      };
    };

    const incidents: FinancialAuthorizationRuntimeIncidentProjection["connections"][number]["incidents"] =
      [];
    const failedBuckets = new Map<string, AuthorizationAttempt[]>();
    for (const attempt of boundGroups.filter(
      (candidate) => candidate.terminal?.status === "FAILED",
    )) {
      const terminalAt = attempt.terminal?.occurred_at;
      if (!terminalAt) continue;
      if (attempt.key.startsWith("legacy:")) {
        const incident = makeIncident({
          id: `failed:${attempt.key}`,
          kind: "FAILED_ATTEMPT",
          startedAt: attempt.started?.occurred_at ?? terminalAt,
          latestAt: terminalAt,
          attemptIDs: [],
          eventIDs: attempt.eventIDs,
        });
        incidents.push({
          ...incident,
          state: "UNAVAILABLE",
          recoveredAt: undefined,
          durationMilliseconds: undefined,
          currentAgeMilliseconds: undefined,
          runtimeStatus: "UNAVAILABLE",
        });
        continue;
      }
      const recoveryKey =
        completedGroups.find(
          (candidate) =>
            Date.parse(candidate.terminal?.occurred_at ?? "") >
            Date.parse(terminalAt),
        )?.key ?? "OPEN";
      failedBuckets.set(recoveryKey, [
        ...(failedBuckets.get(recoveryKey) ?? []),
        attempt,
      ]);
    }
    for (const [recoveryKey, failedAttempts] of failedBuckets) {
      const first = failedAttempts[0];
      const latest = failedAttempts.at(-1);
      const startedAt =
        first.started?.occurred_at ?? first.terminal?.occurred_at;
      const latestAt = latest?.terminal?.occurred_at;
      if (!startedAt || !latestAt) continue;
      incidents.push(
        makeIncident({
          id: `failed:${first.key}:${recoveryKey}`,
          kind: "FAILED_ATTEMPT",
          startedAt,
          latestAt,
          attemptIDs: failedAttempts.map((attempt) => attempt.key),
          eventIDs: failedAttempts.flatMap((attempt) => attempt.eventIDs),
        }),
      );
    }
    const pendingBuckets = new Map<string, AuthorizationAttempt[]>();
    for (const attempt of boundGroups.filter(
      (candidate) =>
        candidate.started &&
        !candidate.terminal &&
        observed - Date.parse(candidate.started.occurred_at) >=
          longPendingAuthorizationMilliseconds,
    )) {
      const startedAt = attempt.started!.occurred_at;
      const recoveryKey =
        completedGroups.find(
          (candidate) =>
            Date.parse(candidate.terminal?.occurred_at ?? "") >
            Date.parse(startedAt),
        )?.key ?? "OPEN";
      pendingBuckets.set(recoveryKey, [
        ...(pendingBuckets.get(recoveryKey) ?? []),
        attempt,
      ]);
    }
    for (const [recoveryKey, pendingAttempts] of pendingBuckets) {
      const first = pendingAttempts[0];
      const latest = pendingAttempts.at(-1);
      const startedAt = first.started?.occurred_at;
      const latestAt = latest?.started?.occurred_at;
      if (!startedAt || !latestAt) continue;
      incidents.push(
        makeIncident({
          id: `pending:${first.key}:${recoveryKey}`,
          kind: "LONG_PENDING",
          startedAt,
          latestAt,
          attemptIDs: pendingAttempts.map((attempt) => attempt.key),
          eventIDs: pendingAttempts.flatMap((attempt) => attempt.eventIDs),
        }),
      );
    }
    const completionReceipts = receipts
      .filter(
        (receipt) =>
          receipt.connection_id === connection.id &&
          receipt.provider === connection.provider &&
          receipt.status === "COMPLETED" &&
          receipt.authorization_expires_at &&
          Date.parse(receipt.authorization_expires_at) <= observed,
      )
      .toSorted(
        (left, right) =>
          Date.parse(left.authorization_expires_at ?? "") -
          Date.parse(right.authorization_expires_at ?? ""),
      );
    for (const receipt of completionReceipts) {
      const attempt = boundGroups.find((candidate) =>
        candidate.eventIDs.includes(receipt.id),
      );
      incidents.push(
        makeIncident({
          id: `expired:${receipt.id}`,
          kind: "EXPIRED_AUTHORIZATION",
          startedAt: receipt.authorization_expires_at!,
          latestAt: receipt.authorization_expires_at!,
          authorizationDeadline: receipt.authorization_expires_at,
          attemptIDs:
            attempt && !attempt.key.startsWith("legacy:") ? [attempt.key] : [],
          eventIDs: [receipt.id],
          recoveryCandidates: completionGroups,
        }),
      );
    }
    const uniqueIncidents = incidents
      .filter(
        (incident, index, values) =>
          values.findIndex((candidate) => candidate.id === incident.id) ===
          index,
      )
      .toSorted(
        (left, right) =>
          Date.parse(right.startedAt) - Date.parse(left.startedAt),
      )
      .slice(0, authorizationIncidentLimit);
    const open = uniqueIncidents.some((incident) => incident.state === "OPEN");
    const unavailable = uniqueIncidents.some(
      (incident) => incident.state === "UNAVAILABLE",
    );
    return {
      id: connection.id,
      provider: connection.provider,
      displayName: connection.display_name,
      state: unavailable
        ? ("UNAVAILABLE" as const)
        : open
          ? ("OPEN" as const)
          : uniqueIncidents.length > 0
            ? ("RECOVERED" as const)
            : ("CLEAR" as const),
      incidentCount: uniqueIncidents.length,
      incidents: uniqueIncidents,
    };
  });
  const incidents = projected.flatMap((connection) => connection.incidents);
  const unavailableCount = projected.filter(
    (connection) => connection.state === "UNAVAILABLE",
  ).length;
  const openCount = incidents.filter(
    (incident) => incident.state === "OPEN",
  ).length;
  return {
    status:
      unavailableCount > 0
        ? "UNAVAILABLE"
        : openCount > 0
          ? "ATTENTION"
          : "VERIFIED",
    incidentCount: incidents.length,
    openCount,
    recoveredCount: incidents.filter(
      (incident) => incident.state === "RECOVERED",
    ).length,
    unavailableCount,
    unboundProviderEventCount,
    connections: projected,
  };
}

function exactMedian(values: number[]) {
  if (values.length === 0) return;
  const sorted = values.toSorted((left, right) => left - right);
  const middle = Math.floor(sorted.length / 2);
  return sorted.length % 2 === 0
    ? (sorted[middle - 1] + sorted[middle]) / 2
    : sorted[middle];
}

export function projectFinancialAuthorizationContinuitySLO({
  connections,
  receipts,
  engines,
  observedAt,
  evidenceAvailable,
}: {
  connections: FinancialConnection[];
  receipts: FinancialAuthorizationReceipt[];
  engines: FinancialContinuityEngine[];
  observedAt: string;
  evidenceAvailable: boolean;
}): FinancialAuthorizationContinuitySLOProjection {
  const observed = Date.parse(observedAt);
  const timeline = projectFinancialAuthorizationTimeline({
    connections,
    receipts,
    observedAt,
    evidenceAvailable,
  });
  const incidents = projectFinancialAuthorizationRuntimeIncidents({
    connections,
    receipts,
    engines,
    observedAt,
    evidenceAvailable,
  });
  const structurallyExact =
    validAuthorizationEvidence({
      connections,
      receipts,
      observedAt,
      evidenceAvailable,
    }) && validIncidentRuntimeEvidence({ connections, engines, observedAt });
  const groups = structurallyExact
    ? authorizationAttemptGroups(receipts)
    : undefined;
  const projected = connections.map((connection) => {
    const authorization = timeline.connections.find(
      (candidate) => candidate.id === connection.id,
    );
    const incidentEvidence = incidents.connections.find(
      (candidate) => candidate.id === connection.id,
    );
    const exact = Boolean(
      authorization &&
        authorization.state !== "UNAVAILABLE" &&
        incidentEvidence &&
        incidentEvidence.state !== "UNAVAILABLE" &&
        groups,
    );
    const attempts = exact
      ? (groups ?? []).filter(
          (attempt) =>
            attempt.connectionID === connection.id &&
            attempt.provider === connection.provider,
        )
      : [];
    const terminalLatencies = attempts
      .filter(
        (attempt) =>
          attempt.started &&
          attempt.terminal &&
          !attempt.key.startsWith("legacy:"),
      )
      .map(
        (attempt) =>
          Date.parse(attempt.terminal!.occurred_at) -
          Date.parse(attempt.started!.occurred_at),
      );
    const latestPaired = attempts
      .filter(
        (attempt) =>
          attempt.started &&
          attempt.terminal &&
          !attempt.key.startsWith("legacy:"),
      )
      .toSorted(
        (left, right) =>
          Date.parse(right.terminal!.occurred_at) -
          Date.parse(left.terminal!.occurred_at),
      )[0];
    const connectionReceipts = exact
      ? receipts.filter(
          (receipt) =>
            receipt.connection_id === connection.id &&
            receipt.provider === connection.provider,
        )
      : [];
    const exactEngines = exact
      ? engines
          .filter((engine) => engine.connection_id === connection.id)
          .map((engine) => {
            const latest = engine.recent_runs[0];
            return {
              instanceID: engine.instance_id!,
              mandateID: engine.mandate_id!,
              accountName: engine.account_name ?? "Financial account",
              executionMode: engine.execution_mode as "PAPER" | "SHADOW",
              latestStatus: latest.status!,
              latestErrorCode: latest.error_code ?? undefined,
              latestCompletedAt: latest.completed_at!,
              nextRunAt: latest.next_run_at!,
            };
          })
      : [];
    const authorizationExpiresAt = exact
      ? authorization?.authorizationExpiresAt
      : undefined;
    const expires = authorizationExpiresAt
      ? Date.parse(authorizationExpiresAt)
      : undefined;
    let state: AuthorizationContinuityState = "CURRENT";
    let label = "Authorization is outside the renewal window";
    let guidance =
      "No owner action is required. Arbion will keep checking the saved authorization and guarded scheduler evidence.";
    if (!exact || !Number.isFinite(observed)) {
      state = "UNAVAILABLE";
      label = "Authorization continuity evidence is unavailable";
      guidance =
        "Review the immutable receipts and connection state. Arbion will not infer a renewal, deadline, provider outcome, or engine impact.";
    } else if (authorization?.state === "EXPIRED") {
      state = "EXPIRED";
      label = "Authorization has expired";
      guidance =
        "Open the existing reconnect control now. Linked accounts and non-live engines stay preserved and fail closed.";
    } else if (authorization?.state === "PENDING") {
      state = "PENDING";
      label = "Authorization callback is pending";
      guidance =
        "Finish the existing provider callback. Arbion has not inferred a renewal and will not start another attempt automatically.";
    } else if (authorization?.state === "FAILED") {
      state = "FAILED";
      label = "The latest authorization attempt failed closed";
      guidance =
        "Open the existing reconnect control when ready. The failed attempt remains immutable and no account setting changed.";
    } else if (authorization?.state === "EXPIRING") {
      state = "RENEW_NOW";
      label = "Renew within the next 24 hours";
      guidance =
        "Open the existing reconnect control before the exact deadline. Arbion will not reconnect or contact the provider automatically.";
    }
    const incidentRows = incidentEvidence?.incidents ?? [];
    return {
      id: connection.id,
      provider: connection.provider,
      displayName: connection.display_name,
      state,
      label,
      guidance,
      lastVerifiedAt: exact ? authorization?.lastVerifiedAt : undefined,
      authorizationExpiresAt,
      renewalWindowStartsAt:
        exact && expires !== undefined
          ? new Date(expires - renewalWindowMilliseconds).toISOString()
          : undefined,
      remainingMilliseconds:
        exact && expires !== undefined ? expires - observed : undefined,
      attemptCount: attempts.length,
      completedCount: attempts.filter(
        (attempt) => attempt.terminal?.status === "COMPLETED",
      ).length,
      pendingCount: attempts.filter((attempt) => !attempt.terminal).length,
      failedCount: attempts.filter(
        (attempt) => attempt.terminal?.status === "FAILED",
      ).length,
      expiredCount: connectionReceipts.filter(
        (receipt) =>
          receipt.status === "COMPLETED" &&
          receipt.authorization_expires_at &&
          Date.parse(receipt.authorization_expires_at) <= observed,
      ).length,
      pairedAttemptCount: terminalLatencies.length,
      latestTerminalLatencyMilliseconds:
        latestPaired?.started && latestPaired.terminal
          ? Date.parse(latestPaired.terminal.occurred_at) -
            Date.parse(latestPaired.started.occurred_at)
          : undefined,
      medianTerminalLatencyMilliseconds: exactMedian(terminalLatencies),
      maximumTerminalLatencyMilliseconds:
        terminalLatencies.length > 0
          ? Math.max(...terminalLatencies)
          : undefined,
      openIncidentCount: incidentRows.filter(
        (incident) => incident.state === "OPEN",
      ).length,
      recoveredIncidentCount: incidentRows.filter(
        (incident) => incident.state === "RECOVERED",
      ).length,
      unavailableIncidentCount: incidentRows.filter(
        (incident) => incident.state === "UNAVAILABLE",
      ).length,
      unboundProviderEventCount: exact
        ? (groups ?? [])
            .filter(
              (attempt) =>
                !attempt.connectionID &&
                attempt.provider === connection.provider,
            )
            .reduce((count, attempt) => count + attempt.eventIDs.length, 0)
        : 0,
      engines: exactEngines,
    };
  });
  const unavailableCount = projected.filter(
    (connection) => connection.state === "UNAVAILABLE",
  ).length;
  const attentionCount = projected.filter((connection) =>
    ["RENEW_NOW", "PENDING", "FAILED", "EXPIRED"].includes(connection.state),
  ).length;
  return {
    status:
      unavailableCount > 0
        ? "UNAVAILABLE"
        : attentionCount > 0
          ? "ATTENTION"
          : "VERIFIED",
    currentCount: projected.filter(
      (connection) => connection.state === "CURRENT",
    ).length,
    attentionCount,
    unavailableCount,
    connections: projected,
  };
}

export function projectFinancialAuthorizationRecoveryPlan({
  connections,
  accounts,
  receipts,
  engines,
  observedAt,
  evidenceAvailable,
}: {
  connections: FinancialConnection[];
  accounts: FinancialAccount[];
  receipts: FinancialAuthorizationReceipt[];
  engines: FinancialContinuityEngine[];
  observedAt: string;
  evidenceAvailable: boolean;
}): FinancialAuthorizationRecoveryPlanProjection {
  const countdown = projectFinancialAuthorizationContinuitySLO({
    connections,
    receipts,
    engines,
    observedAt,
    evidenceAvailable,
  });
  const incidents = projectFinancialAuthorizationRuntimeIncidents({
    connections,
    receipts,
    engines,
    observedAt,
    evidenceAvailable,
  });
  const accountIDs = new Set<string>();
  const capitalBucketIDs = new Set<string>();
  const connectionProviders = new Map(
    connections.map((connection) => [connection.id, connection.provider]),
  );
  let identityExact = true;
  for (const account of accounts) {
    if (
      !uuidPattern.test(account.id) ||
      accountIDs.has(account.id) ||
      !uuidPattern.test(account.provider_connection_id) ||
      connectionProviders.get(account.provider_connection_id) !==
        account.provider
    ) {
      identityExact = false;
      break;
    }
    accountIDs.add(account.id);
  }
  if (identityExact) {
    for (const engine of engines) {
      if (
        !engine.account_id ||
        !engine.connection_id ||
        !engine.capital_bucket_id ||
        engine.recent_runs.length === 0 ||
        !accountIDs.has(engine.account_id) ||
        !uuidPattern.test(engine.capital_bucket_id) ||
        capitalBucketIDs.has(engine.capital_bucket_id) ||
        !accounts.some(
          (account) =>
            account.id === engine.account_id &&
            account.provider_connection_id === engine.connection_id &&
            account.provider === engine.provider,
        )
      ) {
        identityExact = false;
        break;
      }
      capitalBucketIDs.add(engine.capital_bucket_id);
    }
  }
  const projected = connections.map((connection) => {
    const authorization = countdown.connections.find(
      (candidate) => candidate.id === connection.id,
    );
    const incident = incidents.connections.find(
      (candidate) => candidate.id === connection.id,
    );
    const exact = Boolean(
      identityExact &&
        authorization &&
        authorization.state !== "UNAVAILABLE" &&
        incident &&
        incident.state !== "UNAVAILABLE",
    );
    const linkedAccounts = exact
      ? accounts
          .filter(
            (account) =>
              account.provider_connection_id === connection.id &&
              account.provider === connection.provider,
          )
          .map((account) => ({
            id: account.id,
            displayName: account.display_name,
            status: account.status,
          }))
      : [];
    const linkedEngines = exact
      ? engines
          .filter((engine) => engine.connection_id === connection.id)
          .map((engine) => {
            const latest = engine.recent_runs[0];
            const safeWait =
              latest.status === "SKIPPED" &&
              latest.error_code === "OUTSIDE_SESSION";
            return {
              instanceID: engine.instance_id!,
              mandateID: engine.mandate_id!,
              capitalBucketID: engine.capital_bucket_id!,
              accountID: engine.account_id!,
              accountName: engine.account_name ?? "Financial account",
              executionMode: engine.execution_mode as "PAPER" | "SHADOW",
              runtimeState:
                latest.status === "SUCCEEDED"
                  ? ("PROTECTED" as const)
                  : safeWait
                    ? ("SAFE_WAIT" as const)
                    : ("FAILED_CLOSED" as const),
              latestStatus: latest.status!,
              latestErrorCode: latest.error_code ?? undefined,
              latestCompletedAt: latest.completed_at!,
              nextRunAt: latest.next_run_at!,
            };
          })
      : [];
    let state: AuthorizationRecoveryState = "CURRENT";
    if (!exact) state = "UNAVAILABLE";
    else if (authorization?.state === "RENEW_NOW") state = "RENEWAL_WINDOW";
    else if (authorization?.state === "PENDING") state = "PENDING";
    else if (authorization?.state === "FAILED") state = "FAILED";
    else if (authorization?.state === "EXPIRED") state = "EXPIRED";
    const failedClosed = linkedEngines.some(
      (engine) => engine.runtimeState === "FAILED_CLOSED",
    );
    const safelyWaiting =
      linkedEngines.length > 0 &&
      linkedEngines.every((engine) => engine.runtimeState === "SAFE_WAIT");
    let label = "Authorization and protected runtimes are on course";
    if (state === "UNAVAILABLE") label = "Blast-radius evidence is unavailable";
    else if (state === "EXPIRED")
      label = "Authorization expired; guarded runtimes remain fail closed";
    else if (state === "FAILED")
      label = "The latest authorization attempt failed closed";
    else if (state === "PENDING")
      label = "Provider callback is pending; no renewal is inferred";
    else if (state === "RENEWAL_WINDOW")
      label = "Renewal window is open before the exact deadline";
    else if (failedClosed)
      label = "Authorization is current; a guarded runtime failed closed";
    else if (safelyWaiting)
      label = "Authorization is current; every bound runtime is safely waiting";
    const ownerAction =
      state === "UNAVAILABLE"
        ? "Review the immutable connection, receipt, and engine identities before relying on this plan."
        : ["EXPIRED", "FAILED", "RENEWAL_WINDOW"].includes(state)
          ? "Use the existing provider reconnect control; Arbion will not start or repeat authorization automatically."
          : state === "PENDING"
            ? "Finish the existing provider callback; do not start a second authorization attempt."
            : failedClosed
              ? "Review the saved engine failure and let the next guarded schedule evaluate automatically."
              : "No owner action is required before the 24-hour renewal window opens.";
    return {
      id: connection.id,
      provider: connection.provider,
      displayName: connection.display_name,
      state,
      label,
      deadline: exact ? authorization?.authorizationExpiresAt : undefined,
      ownerAction,
      beforeExpiry:
        state === "UNAVAILABLE"
          ? "UNAVAILABLE — Arbion will not infer authorization runway or protected scope."
          : "Linked accounts, Paper/Shadow engine identities, capital claims, and saved evidence remain unchanged. Renew only through the existing owner control.",
      atExpiry:
        state === "UNAVAILABLE"
          ? "UNAVAILABLE — no provider or runtime outcome is predicted."
          : "New guarded evaluations must fail closed when current provider authorization is unavailable. Existing simulated or shadow evidence is preserved; expiry is not an order-cancellation mechanism.",
      afterReconnect:
        state === "UNAVAILABLE"
          ? "UNAVAILABLE — recovery requires a complete connection-bound evidence chain."
          : "Recovery is proven only by a later valid COMPLETED receipt bound to this connection, matching updated verification/expiry state, followed by the next saved automatic Paper or Shadow scheduler result.",
      incidentState: exact ? (incident?.state ?? "CLEAR") : "UNAVAILABLE",
      accounts: linkedAccounts,
      engines: linkedEngines,
    };
  });
  const unavailableCount = projected.filter(
    (connection) => connection.state === "UNAVAILABLE",
  ).length;
  const attentionCount = projected.filter(
    (connection) =>
      ["RENEWAL_WINDOW", "PENDING", "FAILED", "EXPIRED"].includes(
        connection.state,
      ) ||
      connection.engines.some(
        (engine) => engine.runtimeState === "FAILED_CLOSED",
      ),
  ).length;
  return {
    status:
      unavailableCount > 0
        ? "UNAVAILABLE"
        : attentionCount > 0
          ? "ATTENTION"
          : "VERIFIED",
    currentCount: projected.filter(
      (connection) => connection.state === "CURRENT",
    ).length,
    attentionCount,
    unavailableCount,
    accountCount: projected.reduce(
      (count, connection) => count + connection.accounts.length,
      0,
    ),
    engineCount: projected.reduce(
      (count, connection) => count + connection.engines.length,
      0,
    ),
    capitalClaimCount: projected.reduce(
      (count, connection) => count + connection.engines.length,
      0,
    ),
    connections: projected,
  };
}

export function projectFinancialConnectionOperatingBrief({
  connections,
  accounts,
  engines,
  syncInputs,
  observedAt,
  contextAvailable,
  expectedBindingCount,
  authorizationReceipts,
  authorizationEvidenceAvailable,
}: {
  connections: FinancialConnection[];
  accounts: FinancialAccount[];
  engines: FinancialContinuityEngine[];
  syncInputs: ConnectionSyncEvidenceInput[];
  observedAt: string;
  contextAvailable: boolean;
  expectedBindingCount: number;
  authorizationReceipts: FinancialAuthorizationReceipt[];
  authorizationEvidenceAvailable: boolean;
}): FinancialConnectionOperatingBriefProjection {
  const continuity = projectFinancialContinuityCenter({
    connections,
    accounts,
    engines,
    observedAt,
  });
  const authorizationTimeline = projectFinancialAuthorizationTimeline({
    connections,
    receipts: authorizationReceipts,
    observedAt,
    evidenceAvailable: authorizationEvidenceAvailable,
  });
  const syncEvidence = projectConnectionSyncEvidence({
    inputs: syncInputs,
    observedAt,
    expectedBindingCount,
  });
  const projected = continuity.connections.map((connection) => {
    const authorizationReceipt = authorizationTimeline.connections.find(
      (candidate) => candidate.id === connection.id,
    ) ?? {
      state: "UNAVAILABLE" as const,
      label: "Authorization evidence unavailable",
      guidance:
        "Review the saved receipts. Arbion will not infer a renewal from an incomplete chain.",
      latestReceiptAt: undefined,
    };
    const linkedAccountIDs = accounts
      .filter((account) => account.provider_connection_id === connection.id)
      .map((account) => account.id)
      .sort();
    const linkedSync = syncEvidence.accounts.filter((account) =>
      linkedAccountIDs.includes(account.accountID),
    );
    const completeSyncScope =
      linkedAccountIDs.length > 0 &&
      linkedSync.length === linkedAccountIDs.length &&
      new Set(linkedSync.map((account) => account.accountID)).size ===
        linkedAccountIDs.length;
    const unavailable =
      !contextAvailable ||
      connection.state === "UNAVAILABLE" ||
      !completeSyncScope ||
      authorizationReceipt.state === "UNAVAILABLE" ||
      linkedSync.some((account) => account.state === "UNAVAILABLE");
    const currentFailureCount = linkedSync.reduce(
      (count, account) => count + account.currentFailureCount,
      0,
    );
    const collecting = linkedSync.some(
      (account) => account.state === "COLLECTING",
    );
    const review =
      connection.attention ||
      !["CURRENT", "UNAVAILABLE"].includes(authorizationReceipt.state) ||
      currentFailureCount > 0 ||
      linkedSync.some((account) => account.state === "REVIEW");
    let state: OperatingState = "ON_COURSE";
    let label = "Connected and operating on course";
    let guidance =
      "No owner action is required. Saved account evidence is current and every bound Paper or Shadow engine keeps its existing guarded schedule.";
    if (unavailable) {
      state = "UNAVAILABLE";
      label = "Operating evidence is unavailable";
      guidance =
        "Review the detailed saved evidence. Arbion will not infer connection, portfolio, sync, or runtime safety from an incomplete chain.";
    } else if (review) {
      state = "REVIEW";
      label =
        currentFailureCount > 0
          ? "The newest account sync failed closed"
          : authorizationReceipt.state === "PENDING"
            ? "The latest authorization callback is still pending"
            : authorizationReceipt.state === "FAILED"
              ? "The latest authorization attempt failed closed"
              : authorizationReceipt.state === "EXPIRING" ||
                  authorizationReceipt.state === "EXPIRED"
                ? authorizationReceipt.label
                : connection.label;
      guidance =
        currentFailureCount > 0
          ? "Existing holdings and non-live engines are unchanged. Review the saved failure stage; a future normal sync can prove recovery."
          : authorizationReceipt.state === "PENDING"
            ? "Finish the provider callback in the same browser session. Existing holdings and non-live engines remain unchanged while Arbion waits for an exact completion receipt."
            : authorizationReceipt.state === "FAILED"
              ? "Reconnect from the provider card when ready. Existing holdings and non-live engines remain unchanged; Arbion will not infer a renewal from an incomplete attempt."
              : authorizationReceipt.state === "EXPIRING" ||
                  authorizationReceipt.state === "EXPIRED"
                ? authorizationReceipt.guidance
                : connection.guidance;
    } else if (collecting) {
      state = "COLLECTING";
      label = "Sync reliability evidence is collecting forward";
      guidance =
        "No owner action is required. Existing saved portfolios and non-live runtimes remain intact; a future normal sync can add the first immutable attempt receipt.";
    }
    const mandateIDs = connection.engines
      .map((engine) => engine.mandate_id)
      .filter((value): value is string => Boolean(value));
    return {
      id: connection.id,
      provider: connection.provider,
      displayName: connection.displayName,
      state,
      label,
      guidance,
      connectionStatus: connection.connectionStatus ?? "UNAVAILABLE",
      authorizationExpiresAt: connection.authorizationExpiresAt,
      authorizationReceiptStatus: authorizationReceipt.state,
      authorizationReceiptLabel: authorizationReceipt.label,
      authorizationReceiptAt: authorizationReceipt.latestReceiptAt,
      accountCount: linkedAccountIDs.length,
      latestPortfolioObservedAt: latestTime(
        linkedSync.map((account) => account.latestPortfolioObservedAt),
      ),
      syncAttemptCount: linkedSync.reduce(
        (count, account) => count + account.syncAttemptCount,
        0,
      ),
      syncSuccessCount: linkedSync.reduce(
        (count, account) => count + account.syncSuccessCount,
        0,
      ),
      recoveredFailureCount: linkedSync.reduce(
        (count, account) => count + account.recoveredFailureCount,
        0,
      ),
      currentFailureCount,
      paperEngineCount: connection.paperEngineCount,
      shadowEngineCount: connection.shadowEngineCount,
      nextRunAt: earliestTime(
        connection.engines.map((engine) => engine.schedule_next_run_at),
      ),
      accountIDs: linkedAccountIDs,
      mandateIDs: [...new Set(mandateIDs)].sort(),
    };
  });
  const unavailableCount = projected.filter(
    (connection) => connection.state === "UNAVAILABLE",
  ).length;
  const reviewCount = projected.filter(
    (connection) => connection.state === "REVIEW",
  ).length;
  const collectingCount = projected.filter(
    (connection) => connection.state === "COLLECTING",
  ).length;
  return {
    status:
      unavailableCount > 0
        ? "UNAVAILABLE"
        : reviewCount > 0 || collectingCount > 0
          ? "ATTENTION"
          : "VERIFIED",
    onCourseCount: projected.filter(
      (connection) => connection.state === "ON_COURSE",
    ).length,
    collectingCount,
    reviewCount,
    unavailableCount,
    connections: projected,
  };
}

export function FinancialAuthorizationContinuitySLO({
  connections,
  receipts,
  engines,
  observedAt,
  evidenceAvailable,
}: {
  connections: FinancialConnection[];
  receipts: FinancialAuthorizationReceipt[];
  engines: FinancialContinuityEngine[];
  observedAt: string;
  evidenceAvailable: boolean;
}) {
  const projection = projectFinancialAuthorizationContinuitySLO({
    connections,
    receipts,
    engines,
    observedAt,
    evidenceAvailable,
  });
  return (
    <section
      className={`financial-authorization-continuity-slo is-${projection.status.toLowerCase()}`}
      aria-labelledby="financial-authorization-continuity-slo-title"
    >
      <header>
        <div>
          <p className="eyebrow">AUTHORIZATION CONTINUITY + OWNER COUNTDOWN</p>
          <h2 id="financial-authorization-continuity-slo-title">
            {projection.status === "VERIFIED"
              ? "Every saved authorization is outside its renewal window."
              : projection.status === "ATTENTION"
                ? "A financial authorization needs owner attention."
                : "Authorization continuity evidence is unavailable."}
          </h2>
          <p>
            Exact saved authorization runway, receipt history, incidents, and
            guarded scheduler outcomes—without contacting a provider.
          </p>
        </div>
        <span>
          {projection.currentCount} current · {projection.attentionCount}{" "}
          attention · {projection.unavailableCount} unavailable
        </span>
      </header>
      <ol>
        {projection.connections.map((connection) => {
          const attention = connection.state !== "CURRENT";
          return (
            <li
              className={`is-${connection.state.toLowerCase().replaceAll("_", "-")}`}
              key={connection.id}
            >
              <header>
                <div>
                  <span
                    className={`provider-mark provider-${connection.provider}`}
                    aria-hidden="true"
                  >
                    {providerName(connection.provider).slice(0, 1)}
                  </span>
                  <div>
                    <strong>{connection.displayName}</strong>
                    <small>{providerName(connection.provider)}</small>
                  </div>
                </div>
                <span>{connection.state.replaceAll("_", " ")}</span>
              </header>
              <div className="financial-authorization-countdown">
                <div>
                  <small>OWNER COUNTDOWN</small>
                  <strong>
                    {connection.authorizationExpiresAt
                      ? readableDuration(connection.remainingMilliseconds)
                      : connection.state === "UNAVAILABLE"
                        ? "UNAVAILABLE"
                        : "NO FIXED EXPIRY"}
                  </strong>
                </div>
                <span>
                  {connection.authorizationExpiresAt
                    ? `Exact deadline ${readableTime(connection.authorizationExpiresAt)}`
                    : "No provider deadline is saved for this connection"}
                </span>
              </div>
              <h3>{connection.label}</h3>
              <p>{connection.guidance}</p>
              <div className="financial-authorization-owner-actions">
                <Link href={`#financial-provider-${connection.provider}`}>
                  {attention && connection.state !== "UNAVAILABLE"
                    ? "Open reconnect control →"
                    : "Open connection control →"}
                </Link>
                <Link href="/settings/security#security-activity">
                  Open security evidence →
                </Link>
              </div>
              <details open={attention}>
                <summary>
                  Continuity SLO and protected engines
                  <span>{connection.attemptCount} saved attempts</span>
                </summary>
                <dl>
                  <div>
                    <dt>Provider verification</dt>
                    <dd>{readableTime(connection.lastVerifiedAt)}</dd>
                    <small>
                      Renewal window starts{" "}
                      {readableTime(connection.renewalWindowStartsAt)}
                    </small>
                  </div>
                  <div>
                    <dt>Exact runway</dt>
                    <dd>
                      {connection.authorizationExpiresAt
                        ? exactDuration(connection.remainingMilliseconds)
                        : connection.state === "UNAVAILABLE"
                          ? "UNAVAILABLE"
                          : "NO FIXED EXPIRY"}
                    </dd>
                    <small>24-hour owner renewal window</small>
                  </div>
                  <div>
                    <dt>Receipt outcomes</dt>
                    <dd>
                      {connection.completedCount} completed ·{" "}
                      {connection.pendingCount} pending ·{" "}
                      {connection.failedCount} failed
                    </dd>
                    <small>{connection.expiredCount} saved expiries</small>
                  </div>
                  <div>
                    <dt>Terminal latency</dt>
                    <dd>
                      Latest{" "}
                      {exactDuration(
                        connection.latestTerminalLatencyMilliseconds,
                      )}
                    </dd>
                    <small>
                      Median{" "}
                      {exactDuration(
                        connection.medianTerminalLatencyMilliseconds,
                      )}{" "}
                      · max{" "}
                      {exactDuration(
                        connection.maximumTerminalLatencyMilliseconds,
                      )}
                    </small>
                  </div>
                  <div>
                    <dt>Renewal incidents</dt>
                    <dd>
                      {connection.openIncidentCount} open ·{" "}
                      {connection.recoveredIncidentCount} recovered
                    </dd>
                    <small>
                      {connection.unavailableIncidentCount} unavailable
                    </small>
                  </div>
                  <div>
                    <dt>Unbound provider events</dt>
                    <dd>{connection.unboundProviderEventCount}</dd>
                    <small>Never assigned to this account or its engines</small>
                  </div>
                </dl>
                {connection.engines.length === 0 ? (
                  <p>No active Paper or Shadow engine is bound.</p>
                ) : (
                  <ol>
                    {connection.engines.map((engine) => (
                      <li key={engine.instanceID}>
                        <div>
                          <strong>
                            {engine.executionMode} · {engine.accountName}
                          </strong>
                          <span>
                            Latest {engine.latestStatus}
                            {engine.latestErrorCode
                              ? ` · ${engine.latestErrorCode}`
                              : ""}
                          </span>
                        </div>
                        <div>
                          <span>
                            Completed {readableTime(engine.latestCompletedAt)}
                          </span>
                          <small>
                            Next automatic cycle{" "}
                            {readableTime(engine.nextRunAt)}
                          </small>
                        </div>
                        <Link
                          href={`/automations/${encodeURIComponent(engine.mandateID)}#runtime-evidence`}
                        >
                          Open engine evidence →
                        </Link>
                      </li>
                    ))}
                  </ol>
                )}
              </details>
            </li>
          );
        })}
      </ol>
      <footer>
        Saved evidence only · exact connection isolation · owner-controlled
        reconnect · Paper and Shadow remain non-live · no provider contact · no
        sync · no broker action
      </footer>
    </section>
  );
}

export function FinancialAuthorizationRecoveryPlan({
  connections,
  accounts,
  receipts,
  engines,
  observedAt,
  evidenceAvailable,
}: {
  connections: FinancialConnection[];
  accounts: FinancialAccount[];
  receipts: FinancialAuthorizationReceipt[];
  engines: FinancialContinuityEngine[];
  observedAt: string;
  evidenceAvailable: boolean;
}) {
  const projection = projectFinancialAuthorizationRecoveryPlan({
    connections,
    accounts,
    receipts,
    engines,
    observedAt,
    evidenceAvailable,
  });
  return (
    <section
      className={`financial-authorization-recovery-plan is-${projection.status.toLowerCase()}`}
      aria-labelledby="financial-authorization-recovery-plan-title"
    >
      <header>
        <div>
          <p className="eyebrow">EXPIRY BLAST RADIUS + SAFE RECOVERY</p>
          <h2 id="financial-authorization-recovery-plan-title">
            {projection.status === "VERIFIED"
              ? "Every connection has an exact protected recovery plan."
              : projection.status === "ATTENTION"
                ? "A connection or guarded runtime needs review."
                : "Some protected recovery evidence is unavailable."}
          </h2>
          <p>
            What stays preserved before expiry, what fails closed at expiry, and
            the exact saved evidence required after owner-controlled reconnect.
          </p>
        </div>
        <span>
          {projection.accountCount} accounts · {projection.engineCount} engines
          · {projection.capitalClaimCount} capital claims
        </span>
      </header>
      <ol>
        {projection.connections.map((connection) => {
          const attention =
            connection.state !== "CURRENT" ||
            connection.engines.some(
              (engine) => engine.runtimeState === "FAILED_CLOSED",
            );
          return (
            <li
              className={`is-${connection.state.toLowerCase().replaceAll("_", "-")}`}
              key={connection.id}
            >
              <header>
                <div>
                  <strong>{connection.displayName}</strong>
                  <small>{providerName(connection.provider)}</small>
                </div>
                <span>{connection.state.replaceAll("_", " ")}</span>
              </header>
              <h3>{connection.label}</h3>
              <p>{connection.ownerAction}</p>
              <div className="financial-authorization-recovery-sequence">
                <article>
                  <span>1</span>
                  <div>
                    <strong>Before expiry</strong>
                    <p>{connection.beforeExpiry}</p>
                  </div>
                </article>
                <article>
                  <span>2</span>
                  <div>
                    <strong>At expiry</strong>
                    <p>{connection.atExpiry}</p>
                  </div>
                </article>
                <article>
                  <span>3</span>
                  <div>
                    <strong>After reconnect</strong>
                    <p>{connection.afterReconnect}</p>
                  </div>
                </article>
              </div>
              <dl>
                <div>
                  <dt>Exact deadline</dt>
                  <dd>{readableTime(connection.deadline)}</dd>
                  <small>Current saved authorization evidence</small>
                </div>
                <div>
                  <dt>Bound scope</dt>
                  <dd>
                    {connection.accounts.length} accounts ·{" "}
                    {connection.engines.length} engines
                  </dd>
                  <small>
                    {connection.engines.length} exact capital claims
                  </small>
                </div>
                <div>
                  <dt>Incident evidence</dt>
                  <dd>{connection.incidentState.replaceAll("_", " ")}</dd>
                  <small>No provider outcome or continuity is inferred</small>
                </div>
              </dl>
              <details open={attention}>
                <summary>
                  Exact protected scope and scheduler evidence
                  <span>{connection.engines.length} guarded engines</span>
                </summary>
                {connection.accounts.length === 0 ? (
                  <p>No connection-bound financial account is available.</p>
                ) : (
                  <ol className="financial-authorization-recovery-accounts">
                    {connection.accounts.map((account) => (
                      <li key={account.id}>
                        <div>
                          <strong>{account.displayName}</strong>
                          <span>{account.status}</span>
                        </div>
                        <small>Account {account.id}</small>
                      </li>
                    ))}
                  </ol>
                )}
                {connection.engines.length === 0 ? (
                  <p>No active Paper or Shadow engine is bound.</p>
                ) : (
                  <ol className="financial-authorization-recovery-engines">
                    {connection.engines.map((engine) => (
                      <li key={engine.instanceID}>
                        <header>
                          <div>
                            <strong>
                              {engine.executionMode} · {engine.accountName}
                            </strong>
                            <small>
                              {engine.runtimeState.replaceAll("_", " ")}
                            </small>
                          </div>
                          <span>
                            {engine.latestStatus}
                            {engine.latestErrorCode
                              ? ` · ${engine.latestErrorCode}`
                              : ""}
                          </span>
                        </header>
                        <p>
                          Latest completion{" "}
                          {readableTime(engine.latestCompletedAt)}
                          {" · "}next guarded cycle{" "}
                          {readableTime(engine.nextRunAt)}
                        </p>
                        <small>
                          Instance {engine.instanceID} · capital claim{" "}
                          {engine.capitalBucketID}
                        </small>
                        <Link
                          href={`/automations/${encodeURIComponent(engine.mandateID)}#runtime-evidence`}
                        >
                          Open immutable engine evidence →
                        </Link>
                      </li>
                    ))}
                  </ol>
                )}
              </details>
              <footer>
                <Link href={`#financial-provider-${connection.provider}`}>
                  Open existing provider control →
                </Link>
                <Link href="/settings/security#security-activity">
                  Open immutable security evidence →
                </Link>
              </footer>
            </li>
          );
        })}
      </ol>
      <footer>
        Saved evidence only · no provider forecast · no continuity claim · no
        reconnect · no sync · no model rerun · no account mutation · no broker
        order · no live path
      </footer>
    </section>
  );
}

export function FinancialAuthorizationTimeline({
  connections,
  receipts,
  observedAt,
  evidenceAvailable,
}: {
  connections: FinancialConnection[];
  receipts: FinancialAuthorizationReceipt[];
  observedAt: string;
  evidenceAvailable: boolean;
}) {
  const projection = projectFinancialAuthorizationTimeline({
    connections,
    receipts,
    observedAt,
    evidenceAvailable,
  });
  return (
    <section
      className={`financial-authorization-timeline is-${projection.status.toLowerCase()}`}
      aria-labelledby="financial-authorization-timeline-title"
    >
      <header>
        <div>
          <p className="eyebrow">AUTHORIZATION TIMELINE + RENEWAL SLA</p>
          <h2 id="financial-authorization-timeline-title">
            {projection.status === "VERIFIED"
              ? "Every saved financial authorization is current."
              : projection.status === "ATTENTION"
                ? "A financial authorization needs timely owner attention."
                : "Some authorization evidence is unavailable."}
          </h2>
          <p>
            Exact saved start, completion, failure, verification, and expiry
            evidence—without contacting a provider.
          </p>
        </div>
        <span>
          {projection.currentCount} current · {projection.attentionCount}{" "}
          attention · {projection.unavailableCount} unavailable
        </span>
      </header>
      <ol>
        {projection.connections.map((connection) => {
          const attention = connection.state !== "CURRENT";
          return (
            <li
              className={`is-${connection.state.toLowerCase()}`}
              key={connection.id}
            >
              <header>
                <div>
                  <strong>{connection.displayName}</strong>
                  <small>{providerName(connection.provider)}</small>
                </div>
                <span>{connection.state}</span>
              </header>
              <h3>{connection.label}</h3>
              <p>{connection.guidance}</p>
              <dl>
                <div>
                  <dt>Last completion</dt>
                  <dd>{readableTime(connection.lastSuccessfulAt)}</dd>
                  <small>
                    Verified {readableTime(connection.lastVerifiedAt)}
                  </small>
                </div>
                <div>
                  <dt>Renewal lead time</dt>
                  <dd>
                    {connection.authorizationExpiresAt
                      ? readableDuration(connection.remainingMilliseconds)
                      : "NO FIXED EXPIRY"}
                  </dd>
                  <small>
                    {connection.authorizationExpiresAt
                      ? `Deadline ${readableTime(connection.authorizationExpiresAt)}`
                      : "No fixed deadline is saved"}
                  </small>
                </div>
                <div>
                  <dt>Saved attempts</dt>
                  <dd>{connection.attemptCount}</dd>
                  <small>{connection.pairedAttemptCount} exact durations</small>
                </div>
                <div>
                  <dt>Latest attempt SLA</dt>
                  <dd>
                    {connection.state === "PENDING"
                      ? readableDuration(connection.pendingAgeMilliseconds)
                      : readableDuration(
                          connection.latestAttemptDurationMilliseconds,
                        )}
                  </dd>
                  <small>
                    {connection.state === "PENDING"
                      ? "Current pending age"
                      : "Start to first terminal receipt"}
                  </small>
                </div>
              </dl>
              <details open={attention}>
                <summary>
                  Saved attempt receipts
                  <span>{connection.attempts.length} shown</span>
                </summary>
                {connection.attempts.length === 0 ? (
                  <p>
                    No connection-bound attempt history is available in the
                    current 20-receipt evidence window.
                  </p>
                ) : (
                  <ol>
                    {connection.attempts.map((attempt) => (
                      <li key={attempt.key}>
                        <div>
                          <strong>{attempt.status}</strong>
                          <span>
                            {attempt.startedAt
                              ? `Started ${readableTime(attempt.startedAt)}`
                              : "Start receipt unavailable"}
                          </span>
                        </div>
                        <div>
                          <span>
                            {attempt.terminalAt
                              ? `Terminal ${readableTime(attempt.terminalAt)}`
                              : "Terminal receipt pending"}
                          </span>
                          <small>
                            {attempt.durationMilliseconds === undefined
                              ? "Duration UNAVAILABLE"
                              : `Exact duration ${readableDuration(attempt.durationMilliseconds)}`}
                          </small>
                        </div>
                      </li>
                    ))}
                  </ol>
                )}
              </details>
              <footer>
                <Link href="/settings/security#security-activity">
                  Open immutable activity →
                </Link>
                <span>Millisecond-canonical evidence</span>
              </footer>
            </li>
          );
        })}
      </ol>
      <footer>
        Saved evidence only · no provider contact · no reconnect · no sync · no
        credentials · no account mutation · no broker order · no live path
      </footer>
    </section>
  );
}

function incidentKindLabel(kind: AuthorizationIncidentKind) {
  if (kind === "FAILED_ATTEMPT") return "Authorization attempt failed";
  if (kind === "LONG_PENDING") return "Authorization callback stayed pending";
  return "Saved authorization expired";
}

function runtimeStatusLabel(
  status: FinancialAuthorizationRuntimeIncidentProjection["connections"][number]["incidents"][number]["runtimeStatus"],
) {
  if (status === "PROTECTED") return "Saved non-live cycles continued safely";
  if (status === "SAFE_WAIT") return "Saved non-live cycles waited safely";
  if (status === "NEEDS_REVIEW") return "A saved cycle failed closed";
  if (status === "NO_POST_INCIDENT_SAMPLE")
    return "A later scheduler sample is not saved yet";
  if (status === "NO_BOUND_ENGINE")
    return "No active Paper or Shadow engine is bound";
  return "Runtime impact evidence unavailable";
}

export function FinancialAuthorizationRuntimeIncidents({
  connections,
  receipts,
  engines,
  observedAt,
  evidenceAvailable,
}: {
  connections: FinancialConnection[];
  receipts: FinancialAuthorizationReceipt[];
  engines: FinancialContinuityEngine[];
  observedAt: string;
  evidenceAvailable: boolean;
}) {
  const projection = projectFinancialAuthorizationRuntimeIncidents({
    connections,
    receipts,
    engines,
    observedAt,
    evidenceAvailable,
  });
  return (
    <section
      className={`financial-authorization-incidents is-${projection.status.toLowerCase()}`}
      aria-labelledby="financial-authorization-incidents-title"
    >
      <header>
        <div>
          <p className="eyebrow">RENEWAL INCIDENTS + PROTECTED RUNTIME</p>
          <h2 id="financial-authorization-incidents-title">
            {projection.status === "VERIFIED"
              ? projection.incidentCount > 0
                ? "Every saved authorization incident recovered."
                : "No connection-bound authorization incident is active."
              : projection.status === "ATTENTION"
                ? "A saved authorization incident is still open."
                : "Authorization incident evidence is unavailable."}
          </h2>
          <p>
            Exact saved authorization events are compared with bounded Paper and
            Shadow scheduler history—without inferring broker impact.
          </p>
        </div>
        <span>
          {projection.openCount} open · {projection.recoveredCount} recovered ·{" "}
          {projection.unavailableCount} unavailable
        </span>
      </header>
      {projection.unboundProviderEventCount > 0 ? (
        <aside>
          <strong>
            {projection.unboundProviderEventCount} provider-level event
            {projection.unboundProviderEventCount === 1 ? "" : "s"} isolated
          </strong>
          <span>
            These saved events do not identify a financial connection, so they
            are not attributed to any account or engine.
          </span>
        </aside>
      ) : null}
      <ol>
        {projection.connections.map((connection) => (
          <li
            className={`is-${connection.state.toLowerCase()}`}
            key={connection.id}
          >
            <header>
              <div>
                <strong>{connection.displayName}</strong>
                <small>{providerName(connection.provider)}</small>
              </div>
              <span>{connection.state}</span>
            </header>
            {connection.incidents.length === 0 ? (
              <p>
                {connection.state === "UNAVAILABLE"
                  ? "Saved authorization or scheduler evidence is incomplete or inconsistent. Arbion will not infer an incident, recovery, or runtime effect."
                  : "No failed, expired, or 15-minute pending connection-bound authorization attempt appears in the current saved evidence window."}
              </p>
            ) : (
              <ol>
                {connection.incidents.map((incident) => {
                  const attention =
                    incident.state !== "RECOVERED" ||
                    [
                      "NEEDS_REVIEW",
                      "NO_POST_INCIDENT_SAMPLE",
                      "UNAVAILABLE",
                    ].includes(incident.runtimeStatus);
                  return (
                    <li
                      className={`is-${incident.state.toLowerCase()}`}
                      key={incident.id}
                    >
                      <header>
                        <div>
                          <strong>{incidentKindLabel(incident.kind)}</strong>
                          <small>
                            Started {readableTime(incident.startedAt)}
                          </small>
                        </div>
                        <span>{incident.state}</span>
                      </header>
                      <dl>
                        <div>
                          <dt>Incident timing</dt>
                          <dd>
                            {readableDuration(
                              incident.durationMilliseconds ??
                                incident.currentAgeMilliseconds,
                            )}
                          </dd>
                          <small>
                            {incident.recoveredAt
                              ? `Recovered ${readableTime(incident.recoveredAt)}`
                              : "Current saved age"}
                          </small>
                        </div>
                        <div>
                          <dt>Renewal deadline</dt>
                          <dd>
                            {readableTime(incident.authorizationDeadline)}
                          </dd>
                          <small>
                            {incident.authorizationDeadline
                              ? "Exact saved expiry"
                              : "Not part of this incident"}
                          </small>
                        </div>
                        <div>
                          <dt>Attempt identity</dt>
                          <dd>{incident.attemptIDs.length} exact IDs</dd>
                          <small>
                            {incident.eventIDs.length} immutable events
                          </small>
                        </div>
                        <div>
                          <dt>Protected runtime evidence</dt>
                          <dd>{incident.runtimeStatus.replaceAll("_", " ")}</dd>
                          <small>
                            {runtimeStatusLabel(incident.runtimeStatus)}
                          </small>
                        </div>
                      </dl>
                      <details open={attention}>
                        <summary>
                          Paper and Shadow evidence
                          <span>{incident.engines.length} engines</span>
                        </summary>
                        {incident.engines.length === 0 ? (
                          <p>
                            No active Paper or Shadow engine is bound to this
                            financial connection.
                          </p>
                        ) : (
                          <ol>
                            {incident.engines.map((engine) => (
                              <li key={engine.instanceID}>
                                <div>
                                  <strong>
                                    {engine.executionMode} ·{" "}
                                    {engine.accountName}
                                  </strong>
                                  <span>
                                    Latest {engine.latestStatus}
                                    {engine.latestErrorCode
                                      ? ` · ${engine.latestErrorCode}`
                                      : ""}
                                  </span>
                                </div>
                                <div>
                                  <span>
                                    {engine.succeededCount} succeeded ·{" "}
                                    {engine.failedCount} failed closed ·{" "}
                                    {engine.safeWaitCount} safe waits ·{" "}
                                    {engine.blockedCount} other safe blocks
                                  </span>
                                  <small>
                                    Latest completion{" "}
                                    {readableTime(engine.latestCompletedAt)}
                                  </small>
                                </div>
                                <Link
                                  href={`/automations/${encodeURIComponent(engine.mandateID)}#runtime-evidence`}
                                >
                                  Open engine evidence →
                                </Link>
                              </li>
                            ))}
                          </ol>
                        )}
                      </details>
                    </li>
                  );
                })}
              </ol>
            )}
            <footer>
              <Link href="/settings/security#security-activity">
                Open immutable authorization evidence →
              </Link>
              <span>
                {connection.incidentCount} bounded incident
                {connection.incidentCount === 1 ? "" : "s"}
              </span>
            </footer>
          </li>
        ))}
      </ol>
      <footer>
        Saved evidence only · connection-bound attribution · Paper and Shadow
        remain non-live · no provider contact · no reconnect · no sync · no
        broker action
      </footer>
    </section>
  );
}

export function FinancialConnectionOperatingWorkspace({
  connections,
  accounts,
  engines,
  syncInputs,
  observedAt,
  contextAvailable,
  expectedBindingCount,
  authorizationReceipts,
  authorizationEvidenceAvailable,
}: {
  connections: FinancialConnection[];
  accounts: FinancialAccount[];
  engines: FinancialContinuityEngine[];
  syncInputs: ConnectionSyncEvidenceInput[];
  observedAt: string;
  contextAvailable: boolean;
  expectedBindingCount: number;
  authorizationReceipts: FinancialAuthorizationReceipt[];
  authorizationEvidenceAvailable: boolean;
}) {
  if (connections.length === 0) return null;
  const brief = projectFinancialConnectionOperatingBrief({
    connections,
    accounts,
    engines,
    syncInputs,
    observedAt,
    contextAvailable,
    expectedBindingCount,
    authorizationReceipts,
    authorizationEvidenceAvailable,
  });
  const attention = brief.status !== "VERIFIED";
  const headline =
    brief.status === "VERIFIED"
      ? "Every financial connection is operating on course."
      : brief.status === "ATTENTION"
        ? "Your connections are safe; some saved evidence is collecting or needs review."
        : "Some connection operating evidence is unavailable.";
  return (
    <section
      className={`financial-connection-workspace is-${brief.status.toLowerCase()}`}
      aria-label={headline}
    >
      <header>
        <div>
          <p className="eyebrow">FINANCIAL CONNECTION OPERATING BRIEF</p>
          <h2>{headline}</h2>
          <p>
            Current authorization, saved portfolio, sync reliability, and every
            bound Paper or Shadow engine—summarized without contacting a
            provider.
          </p>
        </div>
        <span>
          {brief.onCourseCount} on course · {brief.collectingCount} collecting ·{" "}
          {brief.reviewCount} review · {brief.unavailableCount} unavailable
        </span>
      </header>
      <FinancialAuthorizationContinuitySLO
        connections={connections}
        receipts={authorizationReceipts}
        engines={engines}
        observedAt={observedAt}
        evidenceAvailable={authorizationEvidenceAvailable && contextAvailable}
      />
      <FinancialAuthorizationRecoveryPlan
        connections={connections}
        accounts={accounts}
        receipts={authorizationReceipts}
        engines={engines}
        observedAt={observedAt}
        evidenceAvailable={authorizationEvidenceAvailable && contextAvailable}
      />
      <ol>
        {brief.connections.map((connection) => (
          <li
            className={`is-${connection.state.toLowerCase().replaceAll("_", "-")}`}
            key={connection.id}
          >
            <header>
              <div>
                <span
                  className={`provider-mark provider-${connection.provider}`}
                  aria-hidden="true"
                >
                  {providerName(connection.provider).slice(0, 1)}
                </span>
                <div>
                  <strong>{connection.displayName}</strong>
                  <small>{providerName(connection.provider)}</small>
                </div>
              </div>
              <span>{connection.state.replaceAll("_", " ")}</span>
            </header>
            <h3>{connection.label}</h3>
            <p>{connection.guidance}</p>
            <dl>
              <div>
                <dt>Connection</dt>
                <dd>{connection.connectionStatus}</dd>
                <small>
                  {connection.authorizationExpiresAt
                    ? `Authorization expires ${readableTime(connection.authorizationExpiresAt)}`
                    : "No fixed authorization expiry saved"}
                </small>
              </div>
              <div>
                <dt>Authorization receipt</dt>
                <dd>{connection.authorizationReceiptStatus}</dd>
                <small>
                  {connection.authorizationReceiptLabel}
                  {connection.authorizationReceiptAt
                    ? ` · ${readableTime(connection.authorizationReceiptAt)}`
                    : ""}
                </small>
              </div>
              <div>
                <dt>Saved portfolio</dt>
                <dd>{connection.accountCount} linked accounts</dd>
                <small>
                  Latest observation{" "}
                  {readableTime(connection.latestPortfolioObservedAt)}
                </small>
              </div>
              <div>
                <dt>Sync reliability</dt>
                <dd>
                  {connection.syncAttemptCount === 0
                    ? "COLLECTING FORWARD"
                    : `${connection.syncSuccessCount} of ${connection.syncAttemptCount} saved`}
                </dd>
                <small>
                  {connection.currentFailureCount} current ·{" "}
                  {connection.recoveredFailureCount} recovered failures
                </small>
              </div>
              <div>
                <dt>Guarded engines</dt>
                <dd>
                  {connection.paperEngineCount} Paper ·{" "}
                  {connection.shadowEngineCount} Shadow
                </dd>
                <small>
                  Next automatic cycle {readableTime(connection.nextRunAt)}
                </small>
              </div>
            </dl>
            <footer>
              {connection.accountIDs[0] ? (
                <Link
                  href={`/accounts/${encodeURIComponent(connection.accountIDs[0])}#account-sync-history-title`}
                >
                  Open account evidence →
                </Link>
              ) : (
                <span>Account evidence unavailable</span>
              )}
              {connection.mandateIDs[0] ? (
                <Link
                  href={`/automations/${encodeURIComponent(connection.mandateIDs[0])}#runtime-evidence`}
                >
                  Open engine evidence →
                </Link>
              ) : (
                <span>No bound engine</span>
              )}
            </footer>
          </li>
        ))}
      </ol>
      <details className="financial-connection-diagnostics" open={attention}>
        <summary>
          Detailed continuity, sync, and market-readiness evidence
          <span>{attention ? "OPEN FOR REVIEW" : "AVAILABLE"}</span>
        </summary>
        <div>
          <FinancialAuthorizationTimeline
            connections={connections}
            receipts={authorizationReceipts}
            observedAt={observedAt}
            evidenceAvailable={authorizationEvidenceAvailable}
          />
          <FinancialAuthorizationRuntimeIncidents
            connections={connections}
            receipts={authorizationReceipts}
            engines={engines}
            observedAt={observedAt}
            evidenceAvailable={
              authorizationEvidenceAvailable && contextAvailable
            }
          />
          <ConnectionSyncEvidenceCenter
            inputs={syncInputs}
            observedAt={observedAt}
            expectedBindingCount={expectedBindingCount}
          />
          <FinancialContinuityCenter
            connections={connections}
            accounts={accounts}
            engines={engines}
            observedAt={observedAt}
            contextAvailable={contextAvailable}
          />
          <SchwabMarketDataReadinessView
            connections={connections}
            engines={engines}
            contextAvailable={contextAvailable}
          />
        </div>
      </details>
      <footer>
        Saved evidence only · no provider refresh · no model rerun · no account
        mutation · no broker order · no live path
      </footer>
    </section>
  );
}
