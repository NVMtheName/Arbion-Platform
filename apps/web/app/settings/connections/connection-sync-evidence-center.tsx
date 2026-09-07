import Link from "next/link";

import type { AccountSyncHistoryResult } from "../../accounts/[id]/account-sync-history";
import type { ConnectionSyncAttemptHistoryResult } from "./connection-sync-attempt-history";

export type ConnectionSyncAccount = {
  id: string;
  provider_connection_id: string;
  provider: string;
  display_name: string;
  status: string;
  last_synced_at?: string | null;
};

export type ConnectionRuntimeBinding = {
  instance_id?: string;
  mandate_id?: string;
  account_id?: string;
  capital_bucket_id?: string;
  strategy_identifier?: string;
  execution_mode?: string;
  current_state?: string;
  status?: string;
};

export type ConnectionSyncEvidenceInput = {
  account: ConnectionSyncAccount;
  syncHistory: AccountSyncHistoryResult;
  attemptHistory: ConnectionSyncAttemptHistoryResult;
  reconciliationPayload?: unknown;
  bindings: ConnectionRuntimeBinding[];
};

type EvidenceState = "CURRENT" | "COLLECTING" | "REVIEW" | "UNAVAILABLE";

export type ConnectionSyncEvidenceProjection = {
  status: "VERIFIED" | "ATTENTION" | "UNAVAILABLE";
  currentCount: number;
  collectingCount: number;
  reviewCount: number;
  unavailableCount: number;
  accounts: Array<{
    accountID: string;
    accountName: string;
    provider: string;
    state: EvidenceState;
    label: string;
    guidance: string;
    latestSyncAt?: string;
    syncSampleCount: number;
    latestPortfolioObservedAt?: string;
    holdingsStatus: "COMPLETE" | "REVIEW" | "UNAVAILABLE";
    holdingCount?: number;
    restrictedInventoryStatus:
      | "AVAILABLE"
      | "PARTIAL"
      | "NOT_APPLICABLE"
      | "UNAVAILABLE";
    restrictedHoldingCount?: number;
    providerEventTimeStatus: "UNAVAILABLE";
    syncFailureHistoryStatus: "AVAILABLE" | "COLLECTING" | "UNAVAILABLE";
    syncAttemptCount: number;
    syncSuccessCount: number;
    syncFailureCount: number;
    recoveredFailureCount: number;
    currentFailureCount: number;
    minimumAttemptDurationMilliseconds?: number;
    medianAttemptDurationMilliseconds?: number;
    maximumAttemptDurationMilliseconds?: number;
    recoveredAttemptCount: number;
    medianRecoveryMilliseconds?: number;
    maximumRecoveryMilliseconds?: number;
    currentFailureAgeMilliseconds?: number;
    firstAttemptAt?: string;
    latestAttemptAt?: string;
    latestAttemptOutcome?: "SAVED" | "FAILED";
    latestFailureAt?: string;
    latestFailureStage?: string;
    latestFailureCode?: string;
    aiEngineCount: number;
    tradeLifecycleCount: number;
    bindings: Array<Required<ConnectionRuntimeBinding>>;
    reconciliationID?: string;
    evidenceHash?: string;
    blocksNewActions?: boolean;
  }>;
};

const uuidPattern =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;
const unsignedDecimalPattern = /^(0|[1-9][0-9]*)(\.[0-9]+)?$/;
const comparisonStatuses = new Set([
  "BASELINE",
  "MATCHED",
  "INCOMPLETE",
  "DRIFT_DETECTED",
]);
const evidenceStatuses = new Set(["READY", "UNAVAILABLE"]);

function record(value: unknown): Record<string, unknown> | undefined {
  return value !== null && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : undefined;
}

function text(value: unknown) {
  return typeof value === "string" ? value : undefined;
}

function timestamp(value: unknown, observedAt: Date) {
  const raw = text(value);
  if (!raw || raw.length > 64) return;
  const parsed = new Date(raw);
  if (Number.isNaN(parsed.valueOf()) || parsed.valueOf() > observedAt.valueOf())
    return;
  return parsed.toISOString();
}

function nonnegativeInteger(value: unknown) {
  return typeof value === "number" && Number.isSafeInteger(value) && value >= 0;
}

function exactNonzero(value: string) {
  return unsignedDecimalPattern.test(value) && /[1-9]/.test(value);
}

function decimalParts(value: string) {
  if (!unsignedDecimalPattern.test(value)) return;
  const [integer, fraction = ""] = value.split(".");
  return { integer, fraction };
}

function exactDecimalSum(left: string, right: string, expected: string) {
  const values = [left, right, expected].map(decimalParts);
  if (values.some((value) => value === undefined)) return false;
  const scale = Math.max(...values.map((value) => value!.fraction.length));
  const scaled = values.map((value) =>
    BigInt(value!.integer + value!.fraction.padEnd(scale, "0")),
  );
  return scaled[0] + scaled[1] === scaled[2];
}

function savedPortfolioEvidence(
  payload: unknown,
  account: ConnectionSyncAccount,
  observedAt: Date,
) {
  const root = record(payload);
  const report = record(root?.reconciliation);
  const positions = report?.positions;
  const id = text(report?.id);
  const financialAccountID = text(report?.financial_account_id);
  const provider = text(report?.provider);
  const comparisonStatus = text(report?.comparison_status);
  const balancesStatus = text(report?.balances_status);
  const positionsStatus = text(report?.positions_status);
  const observedPositionCount = report?.observed_position_count;
  const observed = timestamp(report?.observed_at, observedAt);
  const created = timestamp(report?.created_at, observedAt);
  const evidenceHash = text(report?.evidence_hash);
  if (
    root?.evidence_semantics !== "SAVED_PORTFOLIO_RECONCILIATION" ||
    root?.provider_read_performed !== false ||
    root?.broker_action_available !== false ||
    root?.live_execution_available !== false ||
    !id ||
    !uuidPattern.test(id) ||
    financialAccountID !== account.id ||
    provider !== account.provider ||
    !comparisonStatus ||
    !comparisonStatuses.has(comparisonStatus) ||
    !balancesStatus ||
    !evidenceStatuses.has(balancesStatus) ||
    !positionsStatus ||
    !evidenceStatuses.has(positionsStatus) ||
    !nonnegativeInteger(observedPositionCount) ||
    !observed ||
    !created ||
    Date.parse(created) < Date.parse(observed) ||
    !evidenceHash ||
    !/^[0-9a-f]{64}$/.test(evidenceHash) ||
    typeof report?.blocks_new_actions !== "boolean" ||
    !Array.isArray(positions) ||
    positions.length !== observedPositionCount ||
    positions.length > 1000
  ) {
    return;
  }

  let restrictedCoverage = account.provider === "coinbase";
  let restrictedHoldingCount = 0;
  const positionKeys = new Set<string>();
  for (const rawPosition of positions) {
    const position = record(rawPosition);
    const symbol = text(position?.symbol);
    const instrument = text(position?.instrument_type);
    const direction = text(position?.direction);
    const quantity = text(position?.quantity);
    const available = text(position?.available_quantity);
    const unavailable = text(position?.unavailable_to_trade_quantity);
    const key = `${symbol}:${instrument}:${direction}`;
    if (
      !symbol ||
      !/^[A-Z0-9][A-Z0-9.-]{0,19}$/.test(symbol) ||
      !instrument ||
      !direction ||
      !quantity ||
      !unsignedDecimalPattern.test(quantity) ||
      positionKeys.has(key)
    )
      return;
    positionKeys.add(key);
    if (account.provider === "coinbase") {
      if (!available || !unavailable) {
        restrictedCoverage = false;
      } else if (
        !unsignedDecimalPattern.test(available) ||
        !unsignedDecimalPattern.test(unavailable) ||
        !exactDecimalSum(available, unavailable, quantity)
      ) {
        return;
      } else if (exactNonzero(unavailable)) {
        restrictedHoldingCount += 1;
      }
    }
  }
  return {
    id,
    comparisonStatus,
    balancesStatus,
    positionsStatus,
    observedPositionCount,
    observedAt: observed,
    evidenceHash,
    blocksNewActions: report.blocks_new_actions as boolean,
    restrictedCoverage,
    restrictedHoldingCount,
  };
}

function exactBindings(
  bindings: ConnectionRuntimeBinding[],
  accountID: string,
  globalCounts: Map<string, number>,
) {
  const projected: Array<Required<ConnectionRuntimeBinding>> = [];
  for (const binding of bindings) {
    if (
      !binding.instance_id ||
      !uuidPattern.test(binding.instance_id) ||
      globalCounts.get(binding.instance_id) !== 1 ||
      !binding.mandate_id ||
      !uuidPattern.test(binding.mandate_id) ||
      binding.account_id !== accountID ||
      !binding.capital_bucket_id ||
      !uuidPattern.test(binding.capital_bucket_id) ||
      !binding.strategy_identifier ||
      !["PAPER", "SHADOW"].includes(binding.execution_mode ?? "") ||
      !binding.current_state ||
      binding.status !== "ACTIVE"
    )
      return;
    projected.push(binding as Required<ConnectionRuntimeBinding>);
  }
  return projected.sort((left, right) =>
    `${left.execution_mode}:${left.instance_id}`.localeCompare(
      `${right.execution_mode}:${right.instance_id}`,
    ),
  );
}

function exactMedian(values: number[]) {
  if (values.length === 0) return;
  const sorted = [...values].sort((left, right) => left - right);
  const midpoint = Math.floor(sorted.length / 2);
  return sorted.length % 2 === 1
    ? sorted[midpoint]
    : sorted[midpoint - 1] + (sorted[midpoint] - sorted[midpoint - 1]) / 2;
}

export function projectConnectionSyncEvidence({
  inputs,
  observedAt,
  expectedBindingCount,
}: {
  inputs: ConnectionSyncEvidenceInput[];
  observedAt: string;
  expectedBindingCount: number;
}): ConnectionSyncEvidenceProjection {
  const viewedAt = new Date(observedAt);
  const accountCounts = new Map<string, number>();
  const bindingCounts = new Map<string, number>();
  for (const input of inputs) {
    accountCounts.set(
      input.account.id,
      (accountCounts.get(input.account.id) ?? 0) + 1,
    );
    for (const binding of input.bindings) {
      if (!binding.instance_id) continue;
      bindingCounts.set(
        binding.instance_id,
        (bindingCounts.get(binding.instance_id) ?? 0) + 1,
      );
    }
  }
  const referencedBindingCount = inputs.reduce(
    (count, input) => count + input.bindings.length,
    0,
  );
  const exactFleetBindings =
    Number.isSafeInteger(expectedBindingCount) &&
    expectedBindingCount >= 0 &&
    referencedBindingCount === expectedBindingCount &&
    bindingCounts.size === expectedBindingCount;
  const accounts = inputs
    .map((input) => {
      const account = input.account;
      const bindings = exactBindings(input.bindings, account.id, bindingCounts);
      const portfolio = savedPortfolioEvidence(
        input.reconciliationPayload,
        account,
        viewedAt,
      );
      const latestCheckpoint = input.syncHistory.checkpoints[0];
      const latestSyncAt = latestCheckpoint
        ? timestamp(latestCheckpoint.completedAt, viewedAt)
        : undefined;
      const accountSyncedAt = timestamp(account.last_synced_at, viewedAt);
      const exactAccount = Boolean(
        !Number.isNaN(viewedAt.valueOf()) &&
          accountCounts.get(account.id) === 1 &&
          uuidPattern.test(account.id) &&
          uuidPattern.test(account.provider_connection_id) &&
          account.provider &&
          account.display_name &&
          account.status === "active" &&
          accountSyncedAt &&
          exactFleetBindings &&
          bindings,
      );
      const exactHistory = Boolean(
        input.syncHistory.state !== "UNAVAILABLE" &&
          !input.syncHistory.unauthorized &&
          input.syncHistory.checkpoints.length <= 6 &&
          (!latestCheckpoint ||
            (latestCheckpoint.financialAccountID === account.id &&
              latestCheckpoint.providerConnectionID ===
                account.provider_connection_id &&
              latestCheckpoint.provider === account.provider &&
              latestSyncAt)),
      );
      const attemptHistory = input.attemptHistory;
      const latestAttempt = attemptHistory.attempts[0];
      const newestSavedAttempt = attemptHistory.attempts.find(
        (attempt) => attempt.outcome === "SAVED",
      );
      const exactAttemptHistory = Boolean(
        attemptHistory.state !== "UNAVAILABLE" &&
          !attemptHistory.unauthorized &&
          attemptHistory.attempts.length <= 12 &&
          (!newestSavedAttempt ||
            (latestCheckpoint &&
              newestSavedAttempt.id.toLowerCase() ===
                latestCheckpoint.operationID.toLowerCase())),
      );
      const successfulAttemptTimes = attemptHistory.attempts
        .filter((attempt) => attempt.outcome === "SAVED")
        .map((attempt) => Date.parse(attempt.completedAt));
      const failures = attemptHistory.attempts.filter(
        (attempt) => attempt.outcome === "FAILED",
      );
      const successes = attemptHistory.attempts.filter(
        (attempt) => attempt.outcome === "SAVED",
      );
      const recoveredFailures = failures.filter((attempt) =>
        successfulAttemptTimes.some(
          (completedAt) => completedAt > Date.parse(attempt.completedAt),
        ),
      );
      const currentFailures = failures.length - recoveredFailures.length;
      const latestFailure = failures[0];
      const attemptDurations = attemptHistory.attempts.map(
        (attempt) => attempt.durationMilliseconds,
      );
      const recoveryDurations = failures.flatMap((attempt) => {
        const failureCompletedAt = Date.parse(attempt.completedAt);
        const firstLaterSuccess = successfulAttemptTimes
          .filter((completedAt) => completedAt > failureCompletedAt)
          .sort((left, right) => left - right)[0];
        return firstLaterSuccess === undefined
          ? []
          : [firstLaterSuccess - failureCompletedAt];
      });
      const newestCurrentFailure = failures.find(
        (attempt) =>
          !successfulAttemptTimes.some(
            (completedAt) => completedAt > Date.parse(attempt.completedAt),
          ),
      );

      let state: EvidenceState = "CURRENT";
      let label = "Saved sync and holdings evidence is current";
      let guidance =
        "No owner action is required. Existing Paper and Shadow runtimes remain bound to this account without receiving authority to move assets.";
      if (
        !exactAccount ||
        !exactHistory ||
        !exactAttemptHistory ||
        !portfolio ||
        !bindings
      ) {
        state = "UNAVAILABLE";
        label = "Sync evidence is unavailable";
        guidance =
          "Arbion cannot prove the complete saved account, sync, portfolio, and runtime-binding chain and will not infer the missing facts.";
      } else if (currentFailures > 0) {
        state = "REVIEW";
        label = "The newest account sync failed closed";
        guidance =
          "The failed attempt is preserved without credentials or raw provider output. Existing holdings and runtimes are unchanged; review the exact safe failure stage before the next normal sync.";
      } else if (
        input.syncHistory.state === "FORWARD_COLLECTION_PENDING" ||
        attemptHistory.state === "FORWARD_COLLECTION_PENDING"
      ) {
        state = "COLLECTING";
        label = "Saved sync receipts are collecting forward";
        guidance =
          "The portfolio evidence is preserved, but no post-release account-discovery receipt exists yet. A normal future sync can add one; this view does not contact the provider.";
      } else if (
        portfolio.balancesStatus !== "READY" ||
        portfolio.positionsStatus !== "READY" ||
        portfolio.blocksNewActions
      ) {
        state = "REVIEW";
        label = "Portfolio evidence is safely blocked for review";
        guidance =
          "Review the saved reconciliation before relying on this account for a new non-live action. Existing holdings and runtime records remain unchanged.";
      }

      return {
        accountID: account.id,
        accountName: account.display_name,
        provider: account.provider,
        state,
        label,
        guidance,
        latestSyncAt,
        syncSampleCount: input.syncHistory.checkpoints.length,
        latestPortfolioObservedAt: portfolio?.observedAt,
        holdingsStatus: portfolio
          ? portfolio.positionsStatus === "READY" &&
            portfolio.balancesStatus === "READY"
            ? ("COMPLETE" as const)
            : ("REVIEW" as const)
          : ("UNAVAILABLE" as const),
        holdingCount: portfolio?.observedPositionCount,
        restrictedInventoryStatus:
          account.provider !== "coinbase"
            ? ("NOT_APPLICABLE" as const)
            : portfolio?.restrictedCoverage
              ? ("AVAILABLE" as const)
              : portfolio
                ? ("PARTIAL" as const)
                : ("UNAVAILABLE" as const),
        restrictedHoldingCount: portfolio?.restrictedCoverage
          ? portfolio.restrictedHoldingCount
          : undefined,
        providerEventTimeStatus: "UNAVAILABLE" as const,
        syncFailureHistoryStatus:
          attemptHistory.state === "UNAVAILABLE"
            ? ("UNAVAILABLE" as const)
            : attemptHistory.state === "FORWARD_COLLECTION_PENDING"
              ? ("COLLECTING" as const)
              : ("AVAILABLE" as const),
        syncAttemptCount: attemptHistory.attempts.length,
        syncSuccessCount: successes.length,
        syncFailureCount: failures.length,
        recoveredFailureCount: recoveredFailures.length,
        currentFailureCount: currentFailures,
        minimumAttemptDurationMilliseconds:
          attemptDurations.length > 0
            ? Math.min(...attemptDurations)
            : undefined,
        medianAttemptDurationMilliseconds: exactMedian(attemptDurations),
        maximumAttemptDurationMilliseconds:
          attemptDurations.length > 0
            ? Math.max(...attemptDurations)
            : undefined,
        recoveredAttemptCount: recoveryDurations.length,
        medianRecoveryMilliseconds: exactMedian(recoveryDurations),
        maximumRecoveryMilliseconds:
          recoveryDurations.length > 0
            ? Math.max(...recoveryDurations)
            : undefined,
        currentFailureAgeMilliseconds: newestCurrentFailure
          ? viewedAt.valueOf() - Date.parse(newestCurrentFailure.completedAt)
          : undefined,
        firstAttemptAt:
          attemptHistory.attempts.at(-1)?.completedAt ?? undefined,
        latestAttemptAt: latestAttempt?.completedAt,
        latestAttemptOutcome: latestAttempt?.outcome,
        latestFailureAt: latestFailure?.completedAt,
        latestFailureStage: latestFailure?.failureStage,
        latestFailureCode: latestFailure?.errorCode,
        aiEngineCount:
          bindings?.filter(
            (binding) => binding.strategy_identifier === "ai_shadow",
          ).length ?? 0,
        tradeLifecycleCount:
          bindings?.filter(
            (binding) => binding.strategy_identifier !== "ai_shadow",
          ).length ?? 0,
        bindings: bindings ?? [],
        reconciliationID: portfolio?.id,
        evidenceHash: portfolio?.evidenceHash,
        blocksNewActions: portfolio?.blocksNewActions,
      };
    })
    .sort((left, right) =>
      `${left.provider}:${left.accountName}`.localeCompare(
        `${right.provider}:${right.accountName}`,
      ),
    );
  const unavailableCount = accounts.filter(
    (account) => account.state === "UNAVAILABLE",
  ).length;
  const reviewCount = accounts.filter(
    (account) => account.state === "REVIEW",
  ).length;
  const collectingCount = accounts.filter(
    (account) => account.state === "COLLECTING",
  ).length;
  return {
    status:
      unavailableCount > 0
        ? "UNAVAILABLE"
        : reviewCount > 0 || collectingCount > 0
          ? "ATTENTION"
          : "VERIFIED",
    currentCount: accounts.filter((account) => account.state === "CURRENT")
      .length,
    collectingCount,
    reviewCount,
    unavailableCount,
    accounts,
  };
}

function providerName(provider: string) {
  if (provider === "coinbase") return "Coinbase";
  if (provider === "schwab") return "Charles Schwab";
  return provider;
}

function readableTime(value?: string) {
  if (!value) return "UNAVAILABLE";
  return new Intl.DateTimeFormat("en-US", {
    month: "short",
    day: "numeric",
    year: "numeric",
    hour: "numeric",
    minute: "2-digit",
    second: "2-digit",
    timeZone: "UTC",
    timeZoneName: "short",
  }).format(new Date(value));
}

function readableMilliseconds(value?: number) {
  if (value === undefined) return "UNAVAILABLE";
  return `${value.toLocaleString("en-US", { maximumFractionDigits: 1 })} ms`;
}

export function ConnectionSyncEvidenceCenter({
  inputs,
  observedAt,
  expectedBindingCount,
}: {
  inputs: ConnectionSyncEvidenceInput[];
  observedAt: string;
  expectedBindingCount: number;
}) {
  if (inputs.length === 0) return null;
  const projection = projectConnectionSyncEvidence({
    inputs,
    observedAt,
    expectedBindingCount,
  });
  return (
    <section
      className={`connection-sync-evidence-center is-${projection.status.toLowerCase()}`}
      aria-labelledby="connection-sync-evidence-title"
    >
      <header>
        <div>
          <p className="eyebrow">CONNECTION SYNC EVIDENCE</p>
          <h2 id="connection-sync-evidence-title">
            {projection.status === "VERIFIED"
              ? "Every account has an attributable saved input chain."
              : projection.status === "ATTENTION"
                ? "Some saved account evidence is still collecting or needs review."
                : "Some account evidence is unavailable."}
          </h2>
          <p>
            One read-only view of saved sync receipts, portfolio completeness,
            restricted inventory, and the exact non-live runtimes bound to each
            account.
          </p>
        </div>
        <span>
          {projection.currentCount} current · {projection.collectingCount}{" "}
          collecting · {projection.reviewCount} review ·{" "}
          {projection.unavailableCount} unavailable
        </span>
      </header>
      <ol>
        {projection.accounts.map((account) => {
          const attention = account.state !== "CURRENT";
          return (
            <li
              key={account.accountID}
              className={`is-${account.state.toLowerCase()}`}
            >
              <header>
                <div>
                  <span
                    className={`provider-mark provider-${account.provider}`}
                    aria-hidden="true"
                  >
                    {providerName(account.provider).slice(0, 1)}
                  </span>
                  <div>
                    <strong>{account.accountName}</strong>
                    <small>{providerName(account.provider)}</small>
                  </div>
                </div>
                <span>{account.state.replaceAll("_", " ")}</span>
              </header>
              <h3>{account.label}</h3>
              <p>{account.guidance}</p>
              <dl>
                <div>
                  <dt>Latest saved sync</dt>
                  <dd>{readableTime(account.latestSyncAt)}</dd>
                  <small>
                    {account.syncSampleCount} immutable receipts loaded
                  </small>
                </div>
                <div>
                  <dt>Saved holdings</dt>
                  <dd>{account.holdingsStatus}</dd>
                  <small>
                    {account.holdingCount === undefined
                      ? "Count unavailable"
                      : `${account.holdingCount} exact positions at ${readableTime(account.latestPortfolioObservedAt)}`}
                  </small>
                </div>
                <div>
                  <dt>Restricted / staked inventory</dt>
                  <dd>
                    {account.restrictedInventoryStatus.replaceAll("_", " ")}
                  </dd>
                  <small>
                    {account.restrictedHoldingCount === undefined
                      ? "Exact classification unavailable"
                      : `${account.restrictedHoldingCount} positions include unavailable-to-trade quantity`}
                  </small>
                </div>
                <div>
                  <dt>Runtime bindings</dt>
                  <dd>
                    {account.aiEngineCount} AI · {account.tradeLifecycleCount}{" "}
                    rules
                  </dd>
                  <small>
                    {account.bindings.length} active non-live bindings
                  </small>
                </div>
                <div className="connection-sync-reliability-summary">
                  <dt>Sync attempt reliability</dt>
                  <dd>
                    {account.syncAttemptCount === 0
                      ? "COLLECTING FORWARD"
                      : `${account.syncSuccessCount} of ${account.syncAttemptCount} saved`}
                  </dd>
                  <small>
                    {account.syncAttemptCount === 0
                      ? "No post-release attempt is saved yet"
                      : `${account.currentFailureCount} current · ${account.recoveredFailureCount} recovered failures`}
                  </small>
                </div>
              </dl>
              <details open={attention}>
                <summary>Exact saved evidence and limitations</summary>
                <div className="connection-sync-evidence-details">
                  <p>
                    Provider event time: {account.providerEventTimeStatus}. The
                    saved timestamp is when Arbion observed the provider
                    response, not a provider event timestamp.
                  </p>
                  <p>
                    Sync failure and retry history:{" "}
                    {account.syncFailureHistoryStatus}.{" "}
                    {account.syncAttemptCount} immutable attempts loaded;{" "}
                    {account.syncFailureCount} failed,{" "}
                    {account.recoveredFailureCount} followed by a later saved
                    success, {account.currentFailureCount} current.
                  </p>
                  <p>
                    Bounded completion timing: minimum{" "}
                    {readableMilliseconds(
                      account.minimumAttemptDurationMilliseconds,
                    )}
                    , median{" "}
                    {readableMilliseconds(
                      account.medianAttemptDurationMilliseconds,
                    )}
                    , maximum{" "}
                    {readableMilliseconds(
                      account.maximumAttemptDurationMilliseconds,
                    )}
                    . Sample window: {readableTime(account.firstAttemptAt)} to{" "}
                    {readableTime(account.latestAttemptAt)}.
                  </p>
                  <p>
                    Automatic recovery timing: {account.recoveredAttemptCount}{" "}
                    exact pairs, median{" "}
                    {account.recoveredAttemptCount === 0
                      ? "NOT APPLICABLE"
                      : readableMilliseconds(
                          account.medianRecoveryMilliseconds,
                        )}
                    , maximum{" "}
                    {account.recoveredAttemptCount === 0
                      ? "NOT APPLICABLE"
                      : readableMilliseconds(
                          account.maximumRecoveryMilliseconds,
                        )}
                    . Current fail-closed age:{" "}
                    {account.currentFailureCount === 0
                      ? "NONE CURRENT"
                      : readableMilliseconds(
                          account.currentFailureAgeMilliseconds,
                        )}
                    . Timing proves sequence only, not initiator or cause.
                  </p>
                  {account.latestFailureAt ? (
                    <p>
                      Latest saved failure: {account.latestFailureStage} /{" "}
                      {account.latestFailureCode} at{" "}
                      {readableTime(account.latestFailureAt)}. This
                      classification contains no credentials, raw provider
                      output, or causal claim.
                    </p>
                  ) : null}
                  <p>
                    Staking / earn classification: UNAVAILABLE. Coinbase’s
                    unavailable-to-trade quantity can include staking, rewards,
                    vaults, open-order holds, or another provider restriction.
                  </p>
                  {account.bindings.length > 0 ? (
                    <ul>
                      {account.bindings.map((binding) => (
                        <li key={binding.instance_id}>
                          <strong>
                            {binding.strategy_identifier === "ai_shadow"
                              ? "AI engine"
                              : "Rules lifecycle"}{" "}
                            · {binding.execution_mode}
                          </strong>
                          <span>
                            {binding.current_state} · capital policy{" "}
                            {binding.capital_bucket_id}
                          </span>
                          <Link
                            href={`/automations/${encodeURIComponent(binding.mandate_id)}#runtime-evidence`}
                          >
                            Open immutable runtime evidence →
                          </Link>
                        </li>
                      ))}
                    </ul>
                  ) : (
                    <p>No active non-live runtime is bound to this account.</p>
                  )}
                  <footer>
                    <span>
                      Reconciliation {account.reconciliationID ?? "UNAVAILABLE"}
                    </span>
                    <Link
                      href={`/accounts/${encodeURIComponent(account.accountID)}#account-sync-history-title`}
                    >
                      Open account evidence →
                    </Link>
                  </footer>
                </div>
              </details>
            </li>
          );
        })}
      </ol>
      <footer>
        Saved evidence only · no provider refresh · no model rerun · no account
        mutation · no broker order · no live path
      </footer>
    </section>
  );
}
