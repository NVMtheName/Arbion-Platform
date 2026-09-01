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
