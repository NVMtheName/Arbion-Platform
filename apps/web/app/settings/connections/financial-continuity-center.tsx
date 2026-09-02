import Link from "next/link";
import { scheduleFailureGuidance } from "../../automations/schedule-failure-guidance";
import type { FinancialAccount, FinancialConnection } from "./page";

export type FinancialContinuityRun = {
  id?: string | null;
  scheduled_for?: string | null;
  completed_at?: string | null;
  next_run_at?: string | null;
  status?: string | null;
  error_code?: string | null;
  duplicate_recovered?: boolean | null;
  consecutive_failures?: number | null;
};

export type FinancialContinuityEngine = {
  mandate_id?: string;
  instance_id?: string;
  connection_id?: string;
  account_id?: string;
  account_name?: string;
  provider?: string;
  execution_mode?: string;
  instance_status?: string;
  current_state?: string;
  schedule_available: boolean;
  schedule_history_available: boolean;
  schedule_status?: string;
  schedule_completed_at?: string;
  schedule_next_run_at?: string;
  schedule_timing_status?: "ON_SCHEDULE" | "OVERDUE" | "UNAVAILABLE";
  consecutive_failures?: number;
  recent_runs: FinancialContinuityRun[];
  market_scope_available?: boolean;
  market_symbols?: string[];
  required_market_quality?: string;
};

export type SchwabMarketDataReadiness = {
  status: "READY" | "ATTENTION" | "UNAVAILABLE";
  engineCount: number;
  readyCount: number;
  attentionCount: number;
  unavailableCount: number;
  engines: Array<{
    mandateID: string;
    instanceID: string;
    accountName: string;
    connectionStatus: string;
    state:
      | "READY"
      | "ENTITLEMENT_REVIEW"
      | "AUTHORIZATION_REVIEW"
      | "AUTOMATIC_RETRY"
      | "OPERATOR_REVIEW"
      | "SESSION_WAIT"
      | "UNAVAILABLE";
    label: string;
    guidance: string;
    symbols: string[];
    requiredQuality: string;
    scheduleStatus: string;
    errorCode?: string;
    consecutiveFailures: number;
    completedAt: string;
    nextRunAt: string;
    authorizationProof: "VERIFIED" | "REVIEW" | "UNAVAILABLE";
    entitlementProof: "PROVEN" | "REVIEW" | "NOT_TESTED" | "UNAVAILABLE";
    quoteProof:
      | "BROKER_REALTIME"
      | "DELAYED"
      | "REALTIME_UNCONFIRMED"
      | "LEGACY_UNRESOLVED"
      | "SESSION_WAIT"
      | "NOT_EVALUATED"
      | "UNAVAILABLE";
    quoteHistoryStatus: "AVAILABLE" | "UNAVAILABLE";
    quoteHistorySampleCount: number;
    quoteHistoryCapped: boolean;
    quoteRecovery?: {
      errorCode: string;
      blockedAt: string;
      recoveredAt: string;
    };
    quoteIncidentStatus: "AVAILABLE" | "UNAVAILABLE";
    quoteSafeWaitCount: number;
    quoteIncidents: Array<{
      id: string;
      state: "OPEN" | "RECOVERED";
      recurringAfterRecovery: boolean;
      startedAt: string;
      latestFailureAt: string;
      recoveredAt?: string;
      failureCount: number;
      safeWaitCount: number;
      errorCodes: string[];
    }>;
    quoteHistory: Array<{
      id: string;
      state:
        | "BROKER_REALTIME"
        | "DELAYED"
        | "REALTIME_UNCONFIRMED"
        | "LEGACY_AMBIGUOUS"
        | "SESSION_WAIT"
        | "NOT_EVALUATED";
      label: string;
      status: string;
      errorCode?: string;
      completedAt: string;
    }>;
  }>;
};

export type FinancialContinuityState =
  | "CURRENT"
  | "EXPIRING_SOON"
  | "EXPIRED"
  | "SCHEDULER_AFFECTED"
  | "RECOVERED"
  | "UNAVAILABLE";

export type FinancialContinuityProjection = {
  status: "VERIFIED" | "ATTENTION" | "UNAVAILABLE";
  connectionCount: number;
  currentCount: number;
  recoveredCount: number;
  attentionCount: number;
  unavailableCount: number;
  engineCount: number;
  connections: Array<{
    id: string;
    provider: string;
    displayName: string;
    connectionStatus?: string;
    state: FinancialContinuityState;
    label: string;
    guidance: string;
    attention: boolean;
    lastVerifiedAt?: string;
    authorizationExpiresAt?: string;
    authorizationRemainingSeconds?: number;
    accountCount: number;
    latestAccountSyncedAt?: string;
    engineCount: number;
    paperEngineCount: number;
    shadowEngineCount: number;
    recoveredAt?: string;
    priorIncidentAt?: string;
    errorCodes: string[];
    schedulerErrorCodes: string[];
    accounts: FinancialAccount[];
    engines: FinancialContinuityEngine[];
  }>;
};

const connectionFailureCodes = new Set(["RECONCILIATION_REFRESH_FAILED"]);
const schwabAutomaticRetryCodes = new Set([
  "PROVIDER",
  "PROVIDER_UNAVAILABLE",
  "RATE_LIMITED",
  "TIMEOUT",
  "MARKET_DATA_STALE",
]);
const schwabQuoteHistoryLimit = 6;

function milliseconds(value?: string | null) {
  if (!value) return;
  const result = Date.parse(value);
  return Number.isNaN(result) ? undefined : result;
}

function sameInstant(left?: string | null, right?: string | null) {
  const leftTime = milliseconds(left);
  const rightTime = milliseconds(right);
  return leftTime !== undefined && leftTime === rightTime;
}

function validRun(run: FinancialContinuityRun) {
  const scheduledAt = milliseconds(run.scheduled_for);
  const completedAt = milliseconds(run.completed_at);
  const nextRunAt = milliseconds(run.next_run_at);
  return Boolean(
    run.id &&
      scheduledAt !== undefined &&
      completedAt !== undefined &&
      nextRunAt !== undefined &&
      completedAt >= scheduledAt &&
      nextRunAt > scheduledAt &&
      run.status &&
      ["SUCCEEDED", "FAILED", "SKIPPED"].includes(run.status) &&
      typeof run.duplicate_recovered === "boolean" &&
      Number.isInteger(run.consecutive_failures) &&
      (run.consecutive_failures ?? -1) >= 0 &&
      ((run.status === "SUCCEEDED" && run.error_code == null) ||
        (["FAILED", "SKIPPED"].includes(run.status) &&
          Boolean(run.error_code))),
  );
}

function schwabQuoteHistory(run: FinancialContinuityRun) {
  const errorCode = run.error_code ?? undefined;
  if (run.status === "SUCCEEDED" && !errorCode) {
    return {
      state: "BROKER_REALTIME" as const,
      label: "Broker real-time check passed",
    };
  }
  if (run.status === "SKIPPED" && errorCode === "OUTSIDE_SESSION") {
    return {
      state: "SESSION_WAIT" as const,
      label: "Market session wait; quote not checked",
    };
  }
  if (errorCode === "MARKET_DATA_DELAYED") {
    return {
      state: "DELAYED" as const,
      label: "Delayed quote rejected",
    };
  }
  if (errorCode === "MARKET_DATA_REALTIME_UNCONFIRMED") {
    return {
      state: "REALTIME_UNCONFIRMED" as const,
      label: "Real-time status missing",
    };
  }
  if (errorCode === "MARKET_DATA_NOT_REALTIME") {
    return {
      state: "LEGACY_AMBIGUOUS" as const,
      label: "Legacy quote-quality rejection",
    };
  }
  return {
    state: "NOT_EVALUATED" as const,
    label: "Quote gate was not proven",
  };
}

function projectSchwabQuoteHistory(runs: FinancialContinuityRun[]) {
  const seen = new Set<string>();
  let previousCompletedAt: number | undefined;
  const exact = runs.every((run) => {
    const completedAt = milliseconds(run.completed_at);
    if (
      !validRun(run) ||
      !run.id ||
      seen.has(run.id) ||
      completedAt === undefined ||
      (previousCompletedAt !== undefined && completedAt >= previousCompletedAt)
    ) {
      return false;
    }
    seen.add(run.id);
    previousCompletedAt = completedAt;
    return true;
  });
  if (!exact || runs.length === 0) {
    return {
      status: "UNAVAILABLE" as const,
      sampleCount: runs.length,
      capped: runs.length > schwabQuoteHistoryLimit,
      rows: [],
    };
  }
  return {
    status: "AVAILABLE" as const,
    sampleCount: runs.length,
    capped: runs.length > schwabQuoteHistoryLimit,
    rows: runs.slice(0, schwabQuoteHistoryLimit).map((run) => ({
      id: run.id!,
      ...schwabQuoteHistory(run),
      status: run.status!,
      errorCode: run.error_code ?? undefined,
      completedAt: run.completed_at!,
    })),
  };
}

function projectSchwabQuoteRecovery(
  history: ReturnType<typeof projectSchwabQuoteHistory>,
  consecutiveFailures?: number,
) {
  if (
    history.status !== "AVAILABLE" ||
    consecutiveFailures !== 0 ||
    history.rows[0]?.state !== "BROKER_REALTIME"
  ) {
    return;
  }
  for (const row of history.rows.slice(1)) {
    if (row.state === "BROKER_REALTIME" || row.state === "NOT_EVALUATED") {
      return;
    }
    if (row.state === "SESSION_WAIT") continue;
    if (!row.errorCode) return;
    return {
      errorCode: row.errorCode,
      blockedAt: row.completedAt,
      recoveredAt: history.rows[0].completedAt,
    };
  }
}

function projectSchwabQuoteIncidents(
  history: ReturnType<typeof projectSchwabQuoteHistory>,
) {
  if (
    history.status !== "AVAILABLE" ||
    history.rows.some((row) => row.state === "NOT_EVALUATED")
  ) {
    return {
      status: "UNAVAILABLE" as const,
      safeWaitCount: 0,
      incidents: [],
    };
  }

  type Incident = {
    id: string;
    state: "OPEN" | "RECOVERED";
    recurringAfterRecovery: boolean;
    startedAt: string;
    latestFailureAt: string;
    recoveredAt?: string;
    failureCount: number;
    safeWaitCount: number;
    errorCodes: string[];
  };
  const incidents: Incident[] = [];
  let current: Incident | undefined;
  let safeWaitCount = 0;
  let recoverySeen = false;

  for (const row of history.rows.toReversed()) {
    if (row.state === "SESSION_WAIT") {
      safeWaitCount += 1;
      if (current) current.safeWaitCount += 1;
      continue;
    }
    if (row.state === "BROKER_REALTIME") {
      if (current) {
        current.state = "RECOVERED";
        current.recoveredAt = row.completedAt;
        incidents.push(current);
        current = undefined;
        recoverySeen = true;
      }
      continue;
    }
    if (!row.errorCode) {
      return {
        status: "UNAVAILABLE" as const,
        safeWaitCount: 0,
        incidents: [],
      };
    }
    if (!current) {
      current = {
        id: row.id,
        state: "OPEN",
        recurringAfterRecovery: recoverySeen,
        startedAt: row.completedAt,
        latestFailureAt: row.completedAt,
        failureCount: 0,
        safeWaitCount: 0,
        errorCodes: [],
      };
    }
    current.latestFailureAt = row.completedAt;
    current.failureCount += 1;
    if (!current.errorCodes.includes(row.errorCode)) {
      current.errorCodes.push(row.errorCode);
    }
  }
  if (current) incidents.push(current);

  return {
    status: "AVAILABLE" as const,
    safeWaitCount,
    incidents: incidents.toReversed(),
  };
}

function schwabReadinessProof(
  connectionStatus: string,
  scheduleStatus?: string,
  errorCode?: string,
) {
  if (connectionStatus !== "active") {
    return {
      authorizationProof: "REVIEW" as const,
      entitlementProof: "NOT_TESTED" as const,
      quoteProof: "NOT_EVALUATED" as const,
    };
  }
  if (scheduleStatus === "SUCCEEDED" && !errorCode) {
    return {
      authorizationProof: "VERIFIED" as const,
      entitlementProof: "PROVEN" as const,
      quoteProof: "BROKER_REALTIME" as const,
    };
  }
  if (scheduleStatus === "SKIPPED" && errorCode === "OUTSIDE_SESSION") {
    return {
      authorizationProof: "VERIFIED" as const,
      entitlementProof: "NOT_TESTED" as const,
      quoteProof: "SESSION_WAIT" as const,
    };
  }
  const quoteProof = {
    MARKET_DATA_DELAYED: "DELAYED" as const,
    MARKET_DATA_REALTIME_UNCONFIRMED: "REALTIME_UNCONFIRMED" as const,
    MARKET_DATA_NOT_REALTIME: "LEGACY_UNRESOLVED" as const,
  }[errorCode ?? ""];
  if (quoteProof) {
    return {
      authorizationProof: "VERIFIED" as const,
      entitlementProof: "REVIEW" as const,
      quoteProof,
    };
  }
  return {
    authorizationProof: "VERIFIED" as const,
    entitlementProof: "NOT_TESTED" as const,
    quoteProof: "NOT_EVALUATED" as const,
  };
}

export function projectFinancialContinuityCenter({
  connections,
  accounts,
  engines,
  observedAt,
}: {
  connections: FinancialConnection[];
  accounts: FinancialAccount[];
  engines: FinancialContinuityEngine[];
  observedAt: string;
}): FinancialContinuityProjection {
  const observedTime = milliseconds(observedAt);
  const accountOwners = new Map<string, Set<string>>();
  const accountOccurrences = new Map<string, number>();
  const engineOwners = new Map<string, Set<string>>();
  const engineOccurrences = new Map<string, number>();
  const connectionOccurrences = new Map<string, number>();
  for (const connection of connections) {
    connectionOccurrences.set(
      connection.id,
      (connectionOccurrences.get(connection.id) ?? 0) + 1,
    );
  }
  for (const account of accounts) {
    const owners = accountOwners.get(account.id) ?? new Set<string>();
    owners.add(account.provider_connection_id);
    accountOwners.set(account.id, owners);
    accountOccurrences.set(
      account.id,
      (accountOccurrences.get(account.id) ?? 0) + 1,
    );
  }
  for (const engine of engines) {
    if (!engine.instance_id || !engine.connection_id) continue;
    const owners = engineOwners.get(engine.instance_id) ?? new Set<string>();
    owners.add(engine.connection_id);
    engineOwners.set(engine.instance_id, owners);
    engineOccurrences.set(
      engine.instance_id,
      (engineOccurrences.get(engine.instance_id) ?? 0) + 1,
    );
  }

  const projected = connections
    .map((connection) => {
      const connectionAccounts = accounts
        .filter((account) => account.provider_connection_id === connection.id)
        .sort((left, right) =>
          left.display_name.localeCompare(right.display_name),
        );
      const connectionEngines = engines
        .filter((engine) => engine.connection_id === connection.id)
        .sort((left, right) =>
          `${left.execution_mode}:${left.instance_id}`.localeCompare(
            `${right.execution_mode}:${right.instance_id}`,
          ),
        );
      const verifiedAt = milliseconds(connection.last_synced_at);
      const expiresAt = milliseconds(connection.authorization_expires_at);
      const accountSyncTimes = connectionAccounts.map((account) =>
        milliseconds(account.last_synced_at),
      );
      const exactAccounts = Boolean(
        observedTime !== undefined &&
          connectionAccounts.length > 0 &&
          connectionAccounts.every((account, index) =>
            Boolean(
              account.id &&
                account.provider === connection.provider &&
                account.provider_connection_id === connection.id &&
                account.status &&
                accountSyncTimes[index] !== undefined &&
                accountSyncTimes[index]! <= observedTime &&
                accountOwners.get(account.id)?.size === 1 &&
                accountOccurrences.get(account.id) === 1,
            ),
          ),
      );
      const exactEngines = connectionEngines.every((engine) => {
        const recentRuns = engine.recent_runs;
        const latest = recentRuns[0];
        const runTimes = recentRuns.map((run) =>
          milliseconds(run.completed_at),
        );
        return Boolean(
          engine.mandate_id &&
            engine.instance_id &&
            engine.account_id &&
            engine.account_name &&
            engine.provider === connection.provider &&
            engine.instance_status === "ACTIVE" &&
            engine.current_state === "AI_MONITORING" &&
            ["PAPER", "SHADOW"].includes(engine.execution_mode ?? "") &&
            engine.schedule_available &&
            engine.schedule_history_available &&
            engine.schedule_timing_status &&
            engine.schedule_timing_status !== "UNAVAILABLE" &&
            Number.isInteger(engine.consecutive_failures) &&
            (engine.consecutive_failures ?? -1) >= 0 &&
            recentRuns.length > 0 &&
            recentRuns.length <= 12 &&
            recentRuns.every(validRun) &&
            new Set(recentRuns.map((run) => run.id)).size ===
              recentRuns.length &&
            runTimes.every(
              (runTime, index) =>
                runTime !== undefined &&
                observedTime !== undefined &&
                runTime <= observedTime &&
                (index === 0 || runTime <= runTimes[index - 1]!),
            ) &&
            latest?.status === engine.schedule_status &&
            latest?.consecutive_failures === engine.consecutive_failures &&
            sameInstant(latest?.completed_at, engine.schedule_completed_at) &&
            sameInstant(latest?.next_run_at, engine.schedule_next_run_at) &&
            engineOwners.get(engine.instance_id)?.size === 1 &&
            engineOccurrences.get(engine.instance_id) === 1 &&
            connectionAccounts.some(
              (account) => account.id === engine.account_id,
            ),
        );
      });
      const exactConnection = Boolean(
        observedTime !== undefined &&
          connection.id &&
          connection.provider &&
          connection.display_name &&
          connection.status &&
          connectionOccurrences.get(connection.id) === 1 &&
          verifiedAt !== undefined &&
          verifiedAt <= observedTime &&
          (connection.authorization_expires_at === undefined ||
            expiresAt !== undefined) &&
          exactAccounts &&
          exactEngines,
      );

      const runs = connectionEngines.flatMap((engine) =>
        engine.recent_runs.map((run) => ({ engine, run })),
      );
      const connectionFailures = runs
        .filter(
          ({ run }) =>
            run.status === "FAILED" &&
            Boolean(
              run.error_code && connectionFailureCodes.has(run.error_code),
            ),
        )
        .sort(
          (left, right) =>
            (milliseconds(left.run.completed_at) ?? 0) -
            (milliseconds(right.run.completed_at) ?? 0),
        );
      const latestFailure = connectionFailures.at(-1);
      const latestFailureTime = milliseconds(latestFailure?.run.completed_at);
      const recoveredRun =
        latestFailureTime === undefined || !latestFailure?.engine.instance_id
          ? undefined
          : runs
              .filter(
                ({ engine, run }) =>
                  engine.instance_id === latestFailure.engine.instance_id &&
                  run.status === "SUCCEEDED" &&
                  (milliseconds(run.completed_at) ?? 0) > latestFailureTime,
              )
              .sort(
                (left, right) =>
                  (milliseconds(left.run.completed_at) ?? 0) -
                  (milliseconds(right.run.completed_at) ?? 0),
              )[0];
      const expired =
        ["expired", "revoked"].includes(connection.status) ||
        (expiresAt !== undefined &&
          observedTime !== undefined &&
          expiresAt <= observedTime);
      const expiringSoon =
        !expired &&
        expiresAt !== undefined &&
        observedTime !== undefined &&
        expiresAt - observedTime <= 24 * 60 * 60 * 1000;
      const schedulerAffected = connectionEngines.some(
        (engine) =>
          engine.schedule_status === "FAILED" ||
          (engine.consecutive_failures ?? 0) > 0 ||
          engine.schedule_timing_status === "OVERDUE",
      );
      const dependencyAffected =
        connection.status !== "active" ||
        connectionAccounts.some((account) => account.status !== "active") ||
        schedulerAffected;
      const schedulerErrorCodes = [
        ...new Set(
          connectionEngines
            .filter((engine) => engine.schedule_status === "FAILED")
            .map((engine) => engine.recent_runs[0]?.error_code)
            .filter((code): code is string => Boolean(code)),
        ),
      ].sort();

      let state: FinancialContinuityState = "CURRENT";
      let label = "Connection current";
      let guidance =
        "No owner action is required. The next guarded non-live cycle remains scheduled.";
      let attention = false;
      if (!exactConnection) {
        state = "UNAVAILABLE";
        label = "Continuity evidence unavailable";
        guidance =
          "Arbion cannot prove the exact connection, account-sync, engine, or scheduler chain and will not infer continuity.";
        attention = true;
      } else if (expired) {
        state = "EXPIRED";
        label = "Authorization expired";
        guidance =
          "Use the existing provider reconnect control. Dependent non-live cycles remain fail-closed until current authorization evidence returns.";
        attention = true;
      } else if (
        schedulerAffected &&
        connection.status === "active" &&
        connectionAccounts.every((account) => account.status === "active")
      ) {
        state = "SCHEDULER_AFFECTED";
        label = "Authorization active; dependent engine paused safely";
        guidance =
          "The provider connection and linked account are active. A saved non-live engine check stopped the latest cycle separately; review its exact reason below. No model or broker action ran after that stop.";
        attention = true;
      } else if (dependencyAffected) {
        state = "SCHEDULER_AFFECTED";
        label = "Connection or dependent engine needs review";
        guidance =
          "Review the exact saved connection and engine evidence. Arbion does not attribute a scheduler issue to this provider without a saved connection-stage code.";
        attention = true;
      } else if (expiringSoon) {
        state = "EXPIRING_SOON";
        label = "Authorization expires within 24 hours";
        guidance =
          "Use the existing renewal control before the exact deadline to avoid a fail-closed interruption.";
        attention = true;
      } else if (latestFailure && recoveredRun) {
        state = "RECOVERED";
        label = "Connection recovered automatically";
        guidance =
          "An exact saved connection-stage failure remains immutable and the same engine later succeeded on schedule. No owner action is required now.";
      }

      return {
        id: connection.id,
        provider: connection.provider,
        displayName: connection.display_name,
        connectionStatus: connection.status,
        state,
        label,
        guidance,
        attention,
        lastVerifiedAt: connection.last_synced_at ?? undefined,
        authorizationExpiresAt:
          connection.authorization_expires_at ?? undefined,
        authorizationRemainingSeconds:
          expiresAt !== undefined && observedTime !== undefined
            ? Math.floor((expiresAt - observedTime) / 1000)
            : undefined,
        accountCount: connectionAccounts.length,
        latestAccountSyncedAt: connectionAccounts
          .map((account) => account.last_synced_at)
          .filter((value): value is string => Boolean(value))
          .sort((left, right) => Date.parse(right) - Date.parse(left))[0],
        engineCount: connectionEngines.length,
        paperEngineCount: connectionEngines.filter(
          (engine) => engine.execution_mode === "PAPER",
        ).length,
        shadowEngineCount: connectionEngines.filter(
          (engine) => engine.execution_mode === "SHADOW",
        ).length,
        recoveredAt: recoveredRun?.run.completed_at ?? undefined,
        priorIncidentAt: latestFailure?.run.completed_at ?? undefined,
        errorCodes: [
          ...new Set(
            connectionFailures
              .map(({ run }) => run.error_code)
              .filter((code): code is string => Boolean(code)),
          ),
        ].sort(),
        schedulerErrorCodes,
        accounts: connectionAccounts,
        engines: connectionEngines,
      };
    })
    .sort((left, right) =>
      `${left.provider}:${left.displayName}`.localeCompare(
        `${right.provider}:${right.displayName}`,
      ),
    );

  const unavailableCount = projected.filter(
    (connection) => connection.state === "UNAVAILABLE",
  ).length;
  const attentionCount = projected.filter(
    (connection) => connection.attention,
  ).length;
  return {
    status:
      unavailableCount > 0
        ? "UNAVAILABLE"
        : attentionCount > 0
          ? "ATTENTION"
          : "VERIFIED",
    connectionCount: projected.length,
    currentCount: projected.filter(
      (connection) => connection.state === "CURRENT",
    ).length,
    recoveredCount: projected.filter(
      (connection) => connection.state === "RECOVERED",
    ).length,
    attentionCount,
    unavailableCount,
    engineCount: projected.reduce(
      (total, connection) => total + connection.engineCount,
      0,
    ),
    connections: projected,
  };
}

export function projectSchwabMarketDataReadiness({
  connections,
  engines,
}: {
  connections: FinancialConnection[];
  engines: FinancialContinuityEngine[];
}): SchwabMarketDataReadiness {
  const schwabConnections = connections.filter(
    (connection) => connection.provider === "schwab",
  );
  const projected = engines
    .filter(
      (engine) =>
        engine.provider === "schwab" && engine.instance_status === "ACTIVE",
    )
    .map((engine) => {
      const connection = schwabConnections.find(
        (candidate) => candidate.id === engine.connection_id,
      );
      const latest = engine.recent_runs[0];
      const symbols = engine.market_symbols ?? [];
      const quoteHistory = projectSchwabQuoteHistory(engine.recent_runs);
      const quoteRecovery = projectSchwabQuoteRecovery(
        quoteHistory,
        engine.consecutive_failures,
      );
      const quoteIncidents = projectSchwabQuoteIncidents(quoteHistory);
      const exact = Boolean(
        connection &&
          engine.mandate_id &&
          engine.instance_id &&
          engine.account_name &&
          engine.execution_mode === "SHADOW" &&
          engine.current_state === "AI_MONITORING" &&
          engine.schedule_available &&
          engine.schedule_history_available &&
          engine.schedule_timing_status &&
          engine.schedule_timing_status !== "UNAVAILABLE" &&
          engine.market_scope_available === true &&
          symbols.length > 0 &&
          new Set(symbols).size === symbols.length &&
          symbols.every((symbol) => /^[A-Z][A-Z0-9.\-]{0,14}$/.test(symbol)) &&
          engine.required_market_quality === "BROKER_REALTIME" &&
          Number.isInteger(engine.consecutive_failures) &&
          (engine.consecutive_failures ?? -1) >= 0 &&
          latest &&
          validRun(latest) &&
          latest.status === engine.schedule_status &&
          latest.consecutive_failures === engine.consecutive_failures &&
          sameInstant(latest.completed_at, engine.schedule_completed_at) &&
          sameInstant(latest.next_run_at, engine.schedule_next_run_at),
      );
      if (!exact) {
        return {
          mandateID: engine.mandate_id ?? "",
          instanceID: engine.instance_id ?? "",
          accountName: engine.account_name ?? "Schwab account",
          connectionStatus: connection?.status ?? "unavailable",
          state: "UNAVAILABLE" as const,
          label: "Readiness evidence unavailable",
          guidance:
            "Arbion cannot prove the exact connection, configured symbol scope, required quote quality, and newest scheduler result as one saved chain. No readiness is inferred.",
          symbols,
          requiredQuality: engine.required_market_quality ?? "BROKER_REALTIME",
          scheduleStatus: engine.schedule_status ?? "UNAVAILABLE",
          errorCode: latest?.error_code ?? undefined,
          consecutiveFailures: engine.consecutive_failures ?? 0,
          completedAt: engine.schedule_completed_at ?? "",
          nextRunAt: engine.schedule_next_run_at ?? "",
          authorizationProof: "UNAVAILABLE" as const,
          entitlementProof: "UNAVAILABLE" as const,
          quoteProof: "UNAVAILABLE" as const,
          quoteHistoryStatus: quoteHistory.status,
          quoteHistorySampleCount: quoteHistory.sampleCount,
          quoteHistoryCapped: quoteHistory.capped,
          quoteRecovery: undefined,
          quoteIncidentStatus: quoteIncidents.status,
          quoteSafeWaitCount: quoteIncidents.safeWaitCount,
          quoteIncidents: quoteIncidents.incidents,
          quoteHistory: quoteHistory.rows,
        };
      }

      const code = latest?.error_code ?? undefined;
      const readinessProof = schwabReadinessProof(
        connection!.status,
        engine.schedule_status,
        code,
      );
      let state: SchwabMarketDataReadiness["engines"][number]["state"] =
        "UNAVAILABLE";
      let label = "Readiness evidence unavailable";
      let guidance =
        "Review the exact saved scheduler evidence. Arbion will not infer market-data readiness.";
      if (connection!.status !== "active") {
        state = "AUTHORIZATION_REVIEW";
        label = "Authorization needs review";
        guidance =
          "Schwab authorization is not active. Use the existing reconnect control; no market-data readiness is inferred until the connection is active again.";
      } else if (engine.schedule_status === "SUCCEEDED" && !code) {
        state = "READY";
        label = "Broker real-time check passed";
        guidance =
          "The newest saved cycle passed the strict Schwab real-time quote gate. No owner action is required; the next guarded cycle remains automatic.";
      } else if (
        engine.schedule_status === "SKIPPED" &&
        code === "OUTSIDE_SESSION"
      ) {
        state = "SESSION_WAIT";
        label = "Waiting for market session";
        guidance =
          "The connection is active. Arbion is waiting for the next supported U.S. equities session before checking Schwab quote quality again.";
      } else if (code === "MARKET_DATA_DELAYED") {
        state = "ENTITLEMENT_REVIEW";
        label = "Connected; provider quote is delayed";
        guidance = `Schwab authorization is active, but Schwab explicitly marked the newest saved ${symbols.join(" / ")} quote as delayed. Review real-time market-data access; reconnecting alone does not change quote entitlement. Arbion will check again automatically at the next guarded cycle.`;
      } else if (code === "MARKET_DATA_REALTIME_UNCONFIRMED") {
        state = "ENTITLEMENT_REVIEW";
        label = "Connected; quote quality unconfirmed";
        guidance = `Schwab authorization is active, but the newest saved ${symbols.join(" / ")} quote omitted a real-time status. Review the Schwab app's market-data product and entitlement. Arbion will check again automatically at the next guarded cycle without guessing.`;
      } else if (code === "MARKET_DATA_NOT_REALTIME") {
        state = "ENTITLEMENT_REVIEW";
        label = "Connected; real-time quote not confirmed";
        guidance = `Schwab authorization is active, but the newest saved ${symbols.join(" / ")} quote did not explicitly report real-time quality. Review the Schwab account or app's market-data entitlement; reconnecting alone does not prove quote entitlement. Arbion will check again automatically at the next guarded cycle.`;
      } else if (
        code === "AUTHORIZATION_FAILED" ||
        code === "AUTHORIZATION_EXPIRED"
      ) {
        state = "AUTHORIZATION_REVIEW";
        label = "Authorization needs review";
        guidance = scheduleFailureGuidance(code, "schwab").message;
      } else if (code && schwabAutomaticRetryCodes.has(code)) {
        state = "AUTOMATIC_RETRY";
        label = "Provider read will retry automatically";
        guidance = scheduleFailureGuidance(code, "schwab").message;
      } else if (code === "MARKET_DATA_INVALID") {
        state = "OPERATOR_REVIEW";
        label = "Quote contract needs operator review";
        guidance = scheduleFailureGuidance(code, "schwab").message;
      }
      return {
        mandateID: engine.mandate_id!,
        instanceID: engine.instance_id!,
        accountName: engine.account_name!,
        connectionStatus: connection!.status,
        state,
        label,
        guidance,
        symbols,
        requiredQuality: engine.required_market_quality!,
        scheduleStatus: engine.schedule_status!,
        errorCode: code,
        consecutiveFailures: engine.consecutive_failures!,
        completedAt: engine.schedule_completed_at!,
        nextRunAt: engine.schedule_next_run_at!,
        ...readinessProof,
        quoteHistoryStatus: quoteHistory.status,
        quoteHistorySampleCount: quoteHistory.sampleCount,
        quoteHistoryCapped: quoteHistory.capped,
        quoteRecovery,
        quoteIncidentStatus: quoteIncidents.status,
        quoteSafeWaitCount: quoteIncidents.safeWaitCount,
        quoteIncidents: quoteIncidents.incidents,
        quoteHistory: quoteHistory.rows,
      };
    });
  const unavailableCount = projected.filter(
    (engine) => engine.state === "UNAVAILABLE",
  ).length;
  const readyCount = projected.filter((engine) =>
    ["READY", "SESSION_WAIT"].includes(engine.state),
  ).length;
  const attentionCount = projected.length - unavailableCount - readyCount;
  return {
    status:
      projected.length === 0 || unavailableCount > 0
        ? "UNAVAILABLE"
        : attentionCount > 0
          ? "ATTENTION"
          : "READY",
    engineCount: projected.length,
    readyCount,
    attentionCount,
    unavailableCount,
    engines: projected,
  };
}

function providerName(provider: string) {
  if (provider === "schwab") return "Charles Schwab";
  if (provider === "coinbase") return "Coinbase";
  return provider;
}

function timestamp(value?: string | null) {
  if (!value || milliseconds(value) === undefined) return "Unavailable";
  return new Intl.DateTimeFormat("en-US", {
    dateStyle: "medium",
    timeStyle: "short",
    timeZone: "UTC",
  }).format(new Date(value));
}

function duration(seconds?: number): string {
  if (seconds === undefined) return "No saved fixed expiry";
  if (seconds === 0) return "Expired now";
  if (seconds < 0) return `Expired ${duration(Math.abs(seconds))} ago`;
  const days = Math.floor(seconds / 86400);
  const hours = Math.floor((seconds % 86400) / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  return [days ? `${days}d` : "", hours ? `${hours}h` : "", `${minutes}m`]
    .filter(Boolean)
    .join(" ");
}

export function SchwabMarketDataReadinessView({
  connections,
  engines,
  contextAvailable = true,
}: {
  connections: FinancialConnection[];
  engines: FinancialContinuityEngine[];
  contextAvailable?: boolean;
}) {
  const hasSchwab = connections.some(
    (connection) => connection.provider === "schwab",
  );
  if (!hasSchwab) return null;
  const readiness = projectSchwabMarketDataReadiness({
    connections,
    engines,
  });
  const unavailable = !contextAvailable || readiness.status === "UNAVAILABLE";
  const attention = unavailable || readiness.status === "ATTENTION";
  const headline = unavailable
    ? "Schwab market-data readiness is unavailable."
    : readiness.status === "ATTENTION"
      ? "Schwab is connected, but its quote gate needs review."
      : "Schwab market-data readiness is on course.";
  return (
    <section
      className={`schwab-market-readiness${attention ? " needs-review" : ""}`}
      id="schwab-market-readiness"
      aria-label={headline}
    >
      <header>
        <div>
          <p className="eyebrow">SCHWAB MARKET DATA READINESS</p>
          <h2>{headline}</h2>
          <p>
            Connection authorization and real-time quote entitlement are two
            separate checks. Arbion requires Schwab to explicitly identify the
            saved quote as broker real-time before the AI model can evaluate it.
          </p>
        </div>
        <span>{unavailable ? "REVIEW EVIDENCE" : readiness.status}</span>
      </header>
      {!contextAvailable || readiness.engineCount === 0 ? (
        <div className="schwab-market-readiness-unavailable">
          <strong>Exact engine readiness could not be proven</strong>
          <p>
            The complete connection, configured market scope, required quote
            quality, and immutable scheduler result were not available together.
            No entitlement or market-data readiness is inferred.
          </p>
        </div>
      ) : (
        <ol>
          {readiness.engines.map((engine) => {
            const engineAttention = !["READY", "SESSION_WAIT"].includes(
              engine.state,
            );
            const entitlementAttention = engine.state === "ENTITLEMENT_REVIEW";
            return (
              <li
                className={`is-${engine.state.toLowerCase().replaceAll("_", "-")}`}
                key={engine.instanceID}
              >
                <header>
                  <div>
                    <small>{engine.accountName}</small>
                    <strong>{engine.label}</strong>
                  </div>
                  <span>{engineAttention ? "OWNER REVIEW" : "ON COURSE"}</span>
                </header>
                <p>{engine.guidance}</p>
                <ol
                  className="schwab-readiness-path"
                  aria-label="Schwab readiness path"
                >
                  <li>
                    <small>1 · Connection authorization</small>
                    <strong>{engine.authorizationProof}</strong>
                    <span>Saved connection: {engine.connectionStatus}</span>
                  </li>
                  <li>
                    <small>2 · Market-data access</small>
                    <strong>{engine.entitlementProof}</strong>
                    <span>
                      Proven only when the saved quote passes BROKER_REALTIME
                    </span>
                  </li>
                  <li>
                    <small>3 · Saved quote proof</small>
                    <strong>{engine.quoteProof}</strong>
                    <span>
                      {engine.scheduleStatus}
                      {engine.errorCode ? ` · ${engine.errorCode}` : ""}
                      {` · ${timestamp(engine.completedAt)} UTC`}
                    </span>
                  </li>
                </ol>
                {engine.quoteRecovery ? (
                  <aside
                    className="schwab-quote-recovery"
                    aria-label={`Automatic Schwab quote recovery for ${engine.accountName}`}
                  >
                    <div>
                      <strong>Automatic recovery proven</strong>
                      <span>
                        The saved quote gate moved from
                        {` ${engine.quoteRecovery.errorCode}`} at
                        {` ${timestamp(engine.quoteRecovery.blockedAt)} UTC`} to
                        BROKER_REALTIME at
                        {` ${timestamp(engine.quoteRecovery.recoveredAt)} UTC`}.
                      </span>
                    </div>
                    <small>
                      Failure streak reset to 0 · no manual cycle required
                    </small>
                  </aside>
                ) : null}
                <dl>
                  <div>
                    <dt>Configured market scope</dt>
                    <dd>{engine.symbols.join(" · ")}</dd>
                    <small>Exact pinned mandate universe</small>
                  </div>
                  <div>
                    <dt>Failure streak</dt>
                    <dd>{engine.consecutiveFailures}</dd>
                    <small>Preserved immutable scheduler evidence</small>
                  </div>
                  <div>
                    <dt>Next automatic check</dt>
                    <dd>{timestamp(engine.nextRunAt)} UTC</dd>
                    <small>No manual refresh or cycle required</small>
                  </div>
                </dl>
                <details open={engineAttention}>
                  <summary>
                    <span>
                      <strong>Why connected can still stop safely</strong>
                      <small>
                        Authorization, entitlement, and execution boundary
                      </small>
                    </span>
                    <span>
                      {engineAttention ? "Review steps" : "Verified boundary"}
                    </span>
                  </summary>
                  <ol>
                    <li>
                      <strong>1. Authorization</strong>
                      <span>
                        Arbion can authenticate to the saved Schwab connection.
                      </span>
                    </li>
                    <li>
                      <strong>2. Quote entitlement</strong>
                      <span>
                        Schwab must mark the {engine.symbols.join(" / ")} quote
                        as {engine.requiredQuality}. Reconnecting does not prove
                        this separate provider/account capability.
                      </span>
                    </li>
                    <li>
                      <strong>3. Fail-closed evaluation</strong>
                      <span>
                        If quote quality is not explicit, Arbion stops before
                        the AI model, risk proposal, or any broker path.
                      </span>
                    </li>
                  </ol>
                </details>
                {entitlementAttention ? (
                  <aside
                    className="schwab-entitlement-review"
                    aria-label={`What to verify in Schwab for ${engine.accountName}`}
                  >
                    <header>
                      <div>
                        <strong>What to verify in Schwab</strong>
                        <small>
                          Account access—not another Arbion reconnect
                        </small>
                      </div>
                      <span>OWNER CHECKLIST</span>
                    </header>
                    <ol>
                      <li>
                        <strong>Developer app</strong>
                        <span>
                          Confirm your Schwab app is approved for Trader API –
                          Individual production market data.
                        </span>
                      </li>
                      <li>
                        <strong>Linked brokerage account</strong>
                        <span>
                          Ask Schwab to confirm this account is eligible for
                          real-time U.S. equity quotes and that any required
                          market-data agreements are current.
                        </span>
                      </li>
                      <li>
                        <strong>Safe support evidence</strong>
                        <span>
                          If both appear active, give Schwab the symbol
                          {` ${engine.symbols.join(" / ")}`}, the saved code
                          {engine.errorCode ? ` ${engine.errorCode}` : ""}, and
                          the check time {timestamp(engine.completedAt)} UTC.
                          Never send a client secret or refresh token.
                        </span>
                      </li>
                    </ol>
                    <footer>
                      <a
                        href="https://developer.schwab.com/dashboard/apps"
                        target="_blank"
                        rel="noreferrer"
                      >
                        Open Schwab developer apps ↗
                      </a>
                      <span>
                        Return here afterward. Arbion will test the saved quote
                        again automatically—no manual cycle is needed.
                      </span>
                    </footer>
                  </aside>
                ) : null}
                <details
                  open={
                    engineAttention ||
                    engine.quoteHistoryStatus === "UNAVAILABLE"
                  }
                >
                  <summary>
                    <span>
                      <strong>Recent automatic quote checks</strong>
                      <small>
                        {engine.quoteHistoryStatus === "AVAILABLE"
                          ? `${engine.quoteHistorySampleCount} immutable scheduler results${engine.quoteHistoryCapped ? ` · newest ${schwabQuoteHistoryLimit} shown` : ""}`
                          : "Complete ordered history could not be proven"}
                      </small>
                    </span>
                    <span>
                      {engine.quoteHistoryStatus === "AVAILABLE"
                        ? "Saved history"
                        : "Review evidence"}
                    </span>
                  </summary>
                  {engine.quoteHistoryStatus === "AVAILABLE" ? (
                    <>
                      <ol className="schwab-quote-history">
                        {engine.quoteHistory.map((run) => (
                          <li
                            className={`is-${run.state.toLowerCase().replaceAll("_", "-")}`}
                            key={run.id}
                          >
                            <strong>{run.label}</strong>
                            <span>
                              {run.status}
                              {run.errorCode ? ` · ${run.errorCode}` : ""}
                              {` · ${timestamp(run.completedAt)} UTC`}
                            </span>
                          </li>
                        ))}
                      </ol>
                      <section
                        className="schwab-quote-incidents"
                        aria-label={`Schwab market-data incident timeline for ${engine.accountName}`}
                      >
                        <header>
                          <div>
                            <strong>Market-data incident timeline</strong>
                            <small>
                              Exact events in the newest saved scheduler window
                            </small>
                          </div>
                          <span>
                            {engine.quoteIncidentStatus === "AVAILABLE"
                              ? `${engine.quoteIncidents.length} incident${engine.quoteIncidents.length === 1 ? "" : "s"}`
                              : "UNAVAILABLE"}
                          </span>
                        </header>
                        {engine.quoteIncidentStatus === "AVAILABLE" ? (
                          engine.quoteIncidents.length > 0 ? (
                            <ol>
                              {engine.quoteIncidents.map((incident) => (
                                <li
                                  className={`is-${incident.state.toLowerCase()}`}
                                  key={incident.id}
                                >
                                  <div>
                                    <strong>
                                      {incident.state === "OPEN"
                                        ? "Open quote-quality incident"
                                        : "Recovered automatically"}
                                    </strong>
                                    {incident.recurringAfterRecovery ? (
                                      <small>Recurring after recovery</small>
                                    ) : null}
                                  </div>
                                  <span>
                                    {incident.failureCount} failed check
                                    {incident.failureCount === 1 ? "" : "s"}
                                    {` · ${incident.errorCodes.join(" · ")}`}
                                    {` · visible since ${timestamp(incident.startedAt)} UTC`}
                                    {` · latest ${timestamp(incident.latestFailureAt)} UTC`}
                                    {incident.recoveredAt
                                      ? ` · recovered ${timestamp(incident.recoveredAt)} UTC`
                                      : " · awaiting the next automatic check"}
                                  </span>
                                </li>
                              ))}
                            </ol>
                          ) : (
                            <p>
                              No quote-quality incident appears in this saved
                              window.
                            </p>
                          )
                        ) : (
                          <p>
                            The incident chain is unavailable because a saved
                            result did not evaluate the Schwab quote gate. No
                            entitlement state or recovery is inferred.
                          </p>
                        )}
                        <footer>
                          <span>
                            {engine.quoteSafeWaitCount} exact OUTSIDE_SESSION
                            {engine.quoteSafeWaitCount === 1
                              ? " wait"
                              : " waits"}{" "}
                            kept separate
                          </span>
                          <span>
                            {engine.quoteHistoryCapped
                              ? `Bounded view · newest ${schwabQuoteHistoryLimit} checks`
                              : "Complete loaded history"}
                          </span>
                        </footer>
                      </section>
                    </>
                  ) : (
                    <p className="schwab-quote-history-unavailable">
                      A saved row is missing, duplicated, malformed, or out of
                      order. Arbion does not infer quote quality from an
                      incomplete chain; open the immutable scheduler evidence.
                    </p>
                  )}
                </details>
                <footer>
                  <Link
                    href={`/automations/${encodeURIComponent(engine.mandateID)}#runtime-evidence`}
                  >
                    Open immutable scheduler evidence →
                  </Link>
                  <span>
                    Read-only · automatic next check · no broker or live path
                  </span>
                </footer>
              </li>
            );
          })}
        </ol>
      )}
    </section>
  );
}

export function FinancialContinuityCenter({
  connections,
  accounts,
  engines,
  observedAt,
  contextAvailable = true,
}: {
  connections: FinancialConnection[];
  accounts: FinancialAccount[];
  engines: FinancialContinuityEngine[];
  observedAt: string;
  contextAvailable?: boolean;
}) {
  if (!contextAvailable)
    return (
      <section
        className="financial-continuity-center is-unavailable"
        aria-label="Financial connection continuity is unavailable."
      >
        <header>
          <div>
            <p className="eyebrow">CONNECTION CONTINUITY</p>
            <h2>Financial connection continuity is unavailable.</h2>
            <p>
              Arbion could not load the complete connection, account, and engine
              inventory and will not infer dependency safety.
            </p>
          </div>
          <span>REVIEW EVIDENCE</span>
        </header>
        <footer>
          Existing provider controls remain explicit · no model rerun · no
          broker order · no live path
        </footer>
      </section>
    );
  const center = projectFinancialContinuityCenter({
    connections,
    accounts,
    engines,
    observedAt,
  });
  if (center.connectionCount === 0) return null;
  const headline =
    center.status === "UNAVAILABLE"
      ? "Some connection continuity evidence is unavailable."
      : center.status === "ATTENTION"
        ? `${center.attentionCount} financial ${center.attentionCount === 1 ? "connection needs" : "connections need"} review.`
        : "Every connected provider is on course.";
  return (
    <section
      className={`financial-continuity-center is-${center.status.toLowerCase()}`}
      aria-label={headline}
    >
      <header>
        <div>
          <p className="eyebrow">CONNECTION CONTINUITY</p>
          <h2>{headline}</h2>
          <p>
            See which non-live engines depend on each provider and what happened
            after a saved reconnect interruption—without refreshing a provider
            or rerunning a model.
          </p>
        </div>
        <span>
          {center.connectionCount} connections · {center.engineCount} guarded
          engines
        </span>
      </header>
      <ol>
        {center.connections.map((connection) => (
          <li key={connection.id}>
            <details
              className={`is-${connection.state.toLowerCase().replaceAll("_", "-")}`}
              open={connection.attention}
            >
              <summary>
                <span>
                  <span
                    className={`provider-mark provider-${connection.provider}`}
                    aria-hidden="true"
                  >
                    {providerName(connection.provider).slice(0, 1)}
                  </span>
                  <span>
                    <strong>{providerName(connection.provider)}</strong>
                    <small>{connection.label}</small>
                  </span>
                </span>
                <span>
                  {connection.attention ? "OWNER REVIEW" : "ON COURSE"}
                </span>
              </summary>
              <div>
                <p>{connection.guidance}</p>
                <dl>
                  <div>
                    <dt>Connection</dt>
                    <dd>{connection.connectionStatus ?? "Unavailable"}</dd>
                    <code>{connection.id}</code>
                  </div>
                  <div>
                    <dt>Authorization</dt>
                    <dd>
                      {duration(connection.authorizationRemainingSeconds)}
                    </dd>
                    <small>
                      {connection.authorizationExpiresAt
                        ? `Expires ${timestamp(connection.authorizationExpiresAt)} UTC`
                        : "No fixed expiry saved for this credential type"}
                    </small>
                  </div>
                  <div>
                    <dt>Provider verified</dt>
                    <dd>{timestamp(connection.lastVerifiedAt)} UTC</dd>
                    <small>
                      Account sync {timestamp(connection.latestAccountSyncedAt)}{" "}
                      UTC
                    </small>
                  </div>
                  <div>
                    <dt>Dependency boundary</dt>
                    <dd>
                      {connection.engineCount} engines ·{" "}
                      {connection.accountCount} accounts
                    </dd>
                    <small>
                      {connection.paperEngineCount} Paper ·{" "}
                      {connection.shadowEngineCount} Shadow ·{" "}
                      {connection.engineCount > 1 ? "shared" : "isolated"}
                    </small>
                  </div>
                  <div>
                    <dt>Reconnect evidence</dt>
                    <dd>
                      {connection.errorCodes.length > 0
                        ? connection.errorCodes.join(" · ")
                        : "No saved connection-stage failure"}
                    </dd>
                    <small>
                      {connection.priorIncidentAt && connection.recoveredAt
                        ? `Failed ${timestamp(connection.priorIncidentAt)} UTC · recovered ${timestamp(connection.recoveredAt)} UTC`
                        : "No recovery pairing required"}
                    </small>
                  </div>
                  <div>
                    <dt>Engine readiness</dt>
                    <dd>
                      {connection.schedulerErrorCodes.length > 0
                        ? connection.schedulerErrorCodes.join(" · ")
                        : "No current engine failure"}
                    </dd>
                    <small>
                      Connection authorization and non-live market-data
                      readiness are checked separately.
                    </small>
                  </div>
                </dl>
                <section
                  aria-label={`${providerName(connection.provider)} dependent engines`}
                >
                  <h3>Dependent non-live engines</h3>
                  {connection.engines.length === 0 ? (
                    <p>
                      No active Paper or Shadow engine depends on this
                      connection.
                    </p>
                  ) : (
                    <ul>
                      {connection.engines.map((engine) => (
                        <li key={engine.instance_id}>
                          {(() => {
                            const currentCode =
                              engine.schedule_status === "FAILED"
                                ? engine.recent_runs[0]?.error_code
                                : undefined;
                            const recovery = currentCode
                              ? scheduleFailureGuidance(
                                  currentCode,
                                  engine.provider,
                                )
                              : undefined;
                            return (
                              <>
                                <div>
                                  <strong>
                                    AI{" "}
                                    {engine.execution_mode === "PAPER"
                                      ? "Paper"
                                      : "Shadow"}
                                  </strong>
                                  <span>{engine.execution_mode}</span>
                                </div>
                                <p>
                                  {engine.account_name} ·{" "}
                                  {engine.schedule_status} ·{" "}
                                  {engine.consecutive_failures} failures · next{" "}
                                  {timestamp(engine.schedule_next_run_at)} UTC
                                </p>
                                {currentCode && recovery && (
                                  <p className="financial-continuity-engine-guidance">
                                    <strong>Saved reason: {currentCode}</strong>{" "}
                                    · {recovery.message}
                                  </p>
                                )}
                                <code>{engine.account_id}</code>
                                <Link
                                  href={`/automations/${encodeURIComponent(engine.mandate_id ?? "")}#runtime-evidence`}
                                >
                                  Open immutable engine evidence →
                                </Link>
                              </>
                            );
                          })()}
                        </li>
                      ))}
                    </ul>
                  )}
                </section>
              </div>
            </details>
          </li>
        ))}
      </ol>
      <footer>
        Reconnecting one provider does not rewrite another provider&apos;s
        account, immutable Paper ledger, capital claim, or strategy identity.
        Broker positions remain provider-owned; this view proves only saved
        Arbion evidence. No model rerun · no manual cycle · no broker order · no
        live path
      </footer>
    </section>
  );
}
