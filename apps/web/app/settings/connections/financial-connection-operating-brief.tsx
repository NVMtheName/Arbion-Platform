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
  provider: string;
  status: "STARTED" | "COMPLETED" | "FAILED";
  connection_id?: string;
  authorization_expires_at?: string;
  current_last_verified_at?: string;
  authorization_expiry_matches_current_connection?: boolean;
  occurred_at: string;
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
      | "COMPLETED"
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

function authorizationReceiptForConnection({
  connection,
  receipts,
  observedAt,
  evidenceAvailable,
}: {
  connection: ReturnType<
    typeof projectFinancialContinuityCenter
  >["connections"][number];
  receipts: FinancialAuthorizationReceipt[];
  observedAt: string;
  evidenceAvailable: boolean;
}) {
  const observed = Date.parse(observedAt);
  const providerReceipts = receipts.filter(
    (receipt) => receipt.provider === connection.provider,
  );
  const receiptTimes = providerReceipts.map((receipt) => receipt.occurred_at);
  if (
    !evidenceAvailable ||
    !Number.isFinite(observed) ||
    providerReceipts.length === 0 ||
    new Set(providerReceipts.map((receipt) => receipt.id)).size !==
      providerReceipts.length ||
    new Set(receiptTimes).size !== receiptTimes.length ||
    providerReceipts.some((receipt) => {
      const occurred = Date.parse(receipt.occurred_at);
      return (
        !receipt.id ||
        !Number.isFinite(occurred) ||
        occurred > observed ||
        !["STARTED", "COMPLETED", "FAILED"].includes(receipt.status)
      );
    })
  ) {
    return {
      status: "UNAVAILABLE" as const,
      label: "Authorization receipt unavailable",
    };
  }
  const sortedReceipts = providerReceipts.toSorted(
    (left, right) =>
      Date.parse(right.occurred_at) - Date.parse(left.occurred_at),
  );
  const latest = sortedReceipts[0];
  if (latest.status === "STARTED") {
    return {
      status: "PENDING" as const,
      label: "Provider callback has not completed",
      occurredAt: latest.occurred_at,
    };
  }
  if (latest.status === "FAILED") {
    return {
      status: "FAILED" as const,
      label: "Latest authorization attempt failed closed",
      occurredAt: latest.occurred_at,
    };
  }
  const completion =
    latest.connection_id === connection.id
      ? latest
      : sortedReceipts.find(
          (receipt) =>
            receipt.status === "COMPLETED" &&
            receipt.connection_id === connection.id,
        );
  const lastVerifiedAt = connection.lastVerifiedAt
    ? Date.parse(connection.lastVerifiedAt)
    : Number.NaN;
  if (
    !completion ||
    !Number.isFinite(lastVerifiedAt) ||
    lastVerifiedAt > observed ||
    lastVerifiedAt < Date.parse(completion.occurred_at) - 5 * 60 * 1000 ||
    completion.current_last_verified_at !== connection.lastVerifiedAt ||
    completion.authorization_expiry_matches_current_connection !== true
  ) {
    return {
      status: "UNAVAILABLE" as const,
      label: "Authorization receipt does not match this connection",
      occurredAt: completion?.occurred_at ?? latest.occurred_at,
    };
  }
  return {
    status: "COMPLETED" as const,
    label: "Latest authorization completed",
    occurredAt: completion.occurred_at,
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
  const syncEvidence = projectConnectionSyncEvidence({
    inputs: syncInputs,
    observedAt,
    expectedBindingCount,
  });
  const projected = continuity.connections.map((connection) => {
    const authorizationReceipt = authorizationReceiptForConnection({
      connection,
      receipts: authorizationReceipts,
      observedAt,
      evidenceAvailable: authorizationEvidenceAvailable,
    });
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
      authorizationReceipt.status === "UNAVAILABLE" ||
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
      authorizationReceipt.status === "PENDING" ||
      authorizationReceipt.status === "FAILED" ||
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
          : authorizationReceipt.status === "PENDING"
            ? "The latest authorization callback is still pending"
            : authorizationReceipt.status === "FAILED"
              ? "The latest authorization attempt failed closed"
              : connection.label;
      guidance =
        currentFailureCount > 0
          ? "Existing holdings and non-live engines are unchanged. Review the saved failure stage; a future normal sync can prove recovery."
          : authorizationReceipt.status === "PENDING"
            ? "Finish the provider callback in the same browser session. Existing holdings and non-live engines remain unchanged while Arbion waits for an exact completion receipt."
            : authorizationReceipt.status === "FAILED"
              ? "Reconnect from the provider card when ready. Existing holdings and non-live engines remain unchanged; Arbion will not infer a renewal from an incomplete attempt."
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
      authorizationReceiptStatus: authorizationReceipt.status,
      authorizationReceiptLabel: authorizationReceipt.label,
      authorizationReceiptAt: authorizationReceipt.occurredAt,
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
