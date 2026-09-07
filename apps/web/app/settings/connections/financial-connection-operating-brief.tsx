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
