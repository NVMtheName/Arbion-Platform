export type ConnectionSyncAttemptAccount = {
  id: string;
  provider_connection_id: string;
  provider: string;
};

export type ConnectionSyncAttempt = {
  id: string;
  providerConnectionID: string;
  provider: string;
  sourceOperation: "PROVIDER_ACCOUNT_DISCOVERY";
  outcome: "SAVED" | "FAILED";
  failureStage?: "CREDENTIAL_ACCESS" | "ACCOUNT_DISCOVERY" | "PERSISTENCE";
  errorCode?: string;
  accountCount?: number;
  observedAt: string;
  completedAt: string;
  createdAt: string;
  durationMilliseconds: number;
};

export type ConnectionSyncAttemptHistoryResult = {
  state: "CURRENT" | "FORWARD_COLLECTION_PENDING" | "UNAVAILABLE";
  unauthorized: boolean;
  attempts: ConnectionSyncAttempt[];
};

const attemptLimit = 12;
const uuidPattern =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;
const failureStages = new Set([
  "CREDENTIAL_ACCESS",
  "ACCOUNT_DISCOVERY",
  "PERSISTENCE",
]);
const errorCodes = new Set([
  "AUTHORIZATION_FAILED",
  "INVALID_CREDENTIAL_FORMAT",
  "AUTHORIZATION_EXPIRED",
  "PROVIDER_UNAVAILABLE",
  "RATE_LIMITED",
  "TIMEOUT",
  "ACCOUNT_NOT_FOUND",
  "PERMISSION_DENIED",
  "INVALID_PROVIDER_RESPONSE",
  "CONNECTION_DISABLED",
  "CREDENTIAL_UNAVAILABLE",
  "INTERNAL_ERROR",
]);

function record(value: unknown): Record<string, unknown> | undefined {
  return value !== null && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : undefined;
}

function exactTimestamp(value: unknown) {
  if (typeof value !== "string" || value.length > 64) return;
  const parsed = new Date(value);
  return Number.isNaN(parsed.valueOf()) ? undefined : parsed;
}

function unavailable(unauthorized = false): ConnectionSyncAttemptHistoryResult {
  return { state: "UNAVAILABLE", unauthorized, attempts: [] };
}

export function projectConnectionSyncAttemptHistory(
  payload: unknown,
  account: ConnectionSyncAttemptAccount,
  viewedAt: Date,
): ConnectionSyncAttemptHistoryResult {
  const root = record(payload);
  const history = record(root?.history);
  const rawAttempts = history?.attempts;
  if (
    !uuidPattern.test(account.id) ||
    !uuidPattern.test(account.provider_connection_id) ||
    !account.provider ||
    Number.isNaN(viewedAt.valueOf()) ||
    root?.history_semantics !==
      "IMMUTABLE_FINANCIAL_CONNECTION_SYNC_ATTEMPTS" ||
    root?.provider_read_performed !== false ||
    root?.broker_action_available !== false ||
    root?.live_execution_available !== false ||
    !Array.isArray(rawAttempts) ||
    rawAttempts.length > attemptLimit
  ) {
    return unavailable();
  }

  const attempts: ConnectionSyncAttempt[] = [];
  const ids = new Set<string>();
  let previousCompletedAt: Date | undefined;
  let previousID = "";
  for (const value of rawAttempts) {
    const item = record(value);
    const id = typeof item?.id === "string" ? item.id : undefined;
    const providerConnectionID =
      typeof item?.provider_connection_id === "string"
        ? item.provider_connection_id
        : undefined;
    const provider =
      typeof item?.provider === "string" ? item.provider : undefined;
    const sourceOperation = item?.source_operation;
    const outcome = item?.outcome;
    const failureStage = item?.failure_stage;
    const errorCode = item?.error_code;
    const accountCount = item?.account_count;
    const observedAt = exactTimestamp(item?.observed_at);
    const completedAt = exactTimestamp(item?.completed_at);
    const createdAt = exactTimestamp(item?.created_at);
    const validSuccess =
      outcome === "SAVED" &&
      failureStage === undefined &&
      errorCode === undefined &&
      typeof accountCount === "number" &&
      Number.isSafeInteger(accountCount) &&
      accountCount > 0;
    const validFailure =
      outcome === "FAILED" &&
      typeof failureStage === "string" &&
      failureStages.has(failureStage) &&
      typeof errorCode === "string" &&
      errorCodes.has(errorCode) &&
      accountCount === undefined;
    if (
      !id ||
      !uuidPattern.test(id) ||
      ids.has(id) ||
      providerConnectionID !== account.provider_connection_id ||
      provider !== account.provider ||
      sourceOperation !== "PROVIDER_ACCOUNT_DISCOVERY" ||
      (!validSuccess && !validFailure) ||
      !observedAt ||
      !completedAt ||
      !createdAt ||
      observedAt.valueOf() > completedAt.valueOf() ||
      completedAt.valueOf() > createdAt.valueOf() ||
      createdAt.valueOf() > viewedAt.valueOf() ||
      (previousCompletedAt !== undefined &&
        (completedAt.valueOf() > previousCompletedAt.valueOf() ||
          (completedAt.valueOf() === previousCompletedAt.valueOf() &&
            id.localeCompare(previousID) >= 0)))
    ) {
      return unavailable();
    }
    ids.add(id);
    previousCompletedAt = completedAt;
    previousID = id;
    attempts.push({
      id,
      providerConnectionID,
      provider,
      sourceOperation: "PROVIDER_ACCOUNT_DISCOVERY",
      outcome: outcome as "SAVED" | "FAILED",
      ...(validFailure
        ? {
            failureStage: failureStage as ConnectionSyncAttempt["failureStage"],
            errorCode: errorCode as string,
          }
        : { accountCount: accountCount as number }),
      observedAt: observedAt.toISOString(),
      completedAt: completedAt.toISOString(),
      createdAt: createdAt.toISOString(),
      durationMilliseconds: completedAt.valueOf() - observedAt.valueOf(),
    });
  }
  return {
    state: attempts.length > 0 ? "CURRENT" : "FORWARD_COLLECTION_PENDING",
    unauthorized: false,
    attempts,
  };
}

export async function loadConnectionSyncAttemptHistory({
  account,
  base,
  headers,
  viewedAt,
}: {
  account: ConnectionSyncAttemptAccount;
  base: string;
  headers: { cookie: string };
  viewedAt: Date;
}): Promise<ConnectionSyncAttemptHistoryResult> {
  try {
    const response = await fetch(
      `${base}/api/accounts/${encodeURIComponent(account.id)}/sync-attempts?limit=${attemptLimit}`,
      { headers, cache: "no-store" },
    );
    if (response.status === 401) return unavailable(true);
    if (!response.ok) return unavailable();
    return projectConnectionSyncAttemptHistory(
      await response.json(),
      account,
      viewedAt,
    );
  } catch {
    return unavailable();
  }
}
