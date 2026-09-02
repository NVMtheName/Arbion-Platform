export type SyncHistoryAccount = {
  id: string;
  provider_connection_id: string;
  provider: string;
  display_name: string;
};

export type AccountSyncCheckpoint = {
  id: string;
  operationID: string;
  financialAccountID: string;
  providerConnectionID: string;
  provider: string;
  sourceOperation: "PROVIDER_ACCOUNT_DISCOVERY";
  outcome: "SAVED";
  accountCount: number;
  observedAt: string;
  completedAt: string;
  createdAt: string;
  durationMilliseconds: number;
};

export type AccountSyncHistoryResult = {
  state: "CURRENT" | "FORWARD_COLLECTION_PENDING" | "UNAVAILABLE";
  unauthorized: boolean;
  checkpoints: AccountSyncCheckpoint[];
  nextCursor?: string;
};

const checkpointPageLimit = 6;
const uuidPattern =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;

function record(value: unknown): Record<string, unknown> | undefined {
  return value !== null && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : undefined;
}

function stringValue(value: unknown) {
  return typeof value === "string" ? value : undefined;
}

function exactTimestamp(value: unknown) {
  if (typeof value !== "string" || value.length > 64) return undefined;
  const parsed = new Date(value);
  return Number.isNaN(parsed.valueOf()) ? undefined : parsed;
}

function unavailable(): AccountSyncHistoryResult {
  return { state: "UNAVAILABLE", unauthorized: false, checkpoints: [] };
}

export function projectAccountSyncHistory(
  payload: unknown,
  account: SyncHistoryAccount,
  viewedAt: Date,
): AccountSyncHistoryResult {
  if (
    !uuidPattern.test(account.id) ||
    !uuidPattern.test(account.provider_connection_id) ||
    !account.provider ||
    Number.isNaN(viewedAt.valueOf())
  ) {
    return unavailable();
  }
  const root = record(payload);
  const history = record(root?.history);
  const rawCheckpoints = history?.checkpoints;
  const nextCursor = stringValue(history?.next_cursor);
  if (
    root?.history_semantics !==
      "IMMUTABLE_FINANCIAL_ACCOUNT_SYNC_CHECKPOINTS" ||
    root?.provider_read_performed !== false ||
    root?.broker_action_available !== false ||
    root?.live_execution_available !== false ||
    !Array.isArray(rawCheckpoints) ||
    rawCheckpoints.length > checkpointPageLimit ||
    (nextCursor !== undefined && !uuidPattern.test(nextCursor))
  ) {
    return unavailable();
  }

  const checkpoints: AccountSyncCheckpoint[] = [];
  const checkpointIDs = new Set<string>();
  const operationIDs = new Set<string>();
  let previousCompletedAt: Date | undefined;
  let previousID = "";
  for (const value of rawCheckpoints) {
    const item = record(value);
    const id = stringValue(item?.id);
    const operationID = stringValue(item?.operation_id);
    const financialAccountID = stringValue(item?.financial_account_id);
    const providerConnectionID = stringValue(item?.provider_connection_id);
    const provider = stringValue(item?.provider);
    const sourceOperation = stringValue(item?.source_operation);
    const outcome = stringValue(item?.outcome);
    const accountCount = item?.account_count;
    const observedAt = exactTimestamp(item?.observed_at);
    const completedAt = exactTimestamp(item?.completed_at);
    const createdAt = exactTimestamp(item?.created_at);
    if (
      !id ||
      !uuidPattern.test(id) ||
      !operationID ||
      !uuidPattern.test(operationID) ||
      checkpointIDs.has(id) ||
      operationIDs.has(operationID) ||
      financialAccountID !== account.id ||
      providerConnectionID !== account.provider_connection_id ||
      provider !== account.provider ||
      sourceOperation !== "PROVIDER_ACCOUNT_DISCOVERY" ||
      outcome !== "SAVED" ||
      typeof accountCount !== "number" ||
      !Number.isSafeInteger(accountCount) ||
      accountCount < 1 ||
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
    checkpointIDs.add(id);
    operationIDs.add(operationID);
    previousCompletedAt = completedAt;
    previousID = id;
    checkpoints.push({
      id,
      operationID,
      financialAccountID,
      providerConnectionID,
      provider,
      sourceOperation: "PROVIDER_ACCOUNT_DISCOVERY",
      outcome: "SAVED",
      accountCount,
      observedAt: observedAt.toISOString(),
      completedAt: completedAt.toISOString(),
      createdAt: createdAt.toISOString(),
      durationMilliseconds: completedAt.valueOf() - observedAt.valueOf(),
    });
  }
  if (
    (checkpoints.length === 0 && nextCursor !== undefined) ||
    (nextCursor !== undefined &&
      checkpoints.at(-1)?.id.toLowerCase() !== nextCursor.toLowerCase())
  ) {
    return unavailable();
  }
  return {
    state: checkpoints.length > 0 ? "CURRENT" : "FORWARD_COLLECTION_PENDING",
    unauthorized: false,
    checkpoints,
    ...(nextCursor ? { nextCursor } : {}),
  };
}

export async function loadAccountSyncHistory({
  account,
  base,
  headers,
  viewedAt,
  cursor,
}: {
  account: SyncHistoryAccount;
  base: string;
  headers: { cookie: string };
  viewedAt: Date;
  cursor?: string;
}): Promise<AccountSyncHistoryResult> {
  const query = new URLSearchParams({ limit: String(checkpointPageLimit) });
  if (cursor) query.set("cursor", cursor);
  try {
    const response = await fetch(
      base +
        "/api/accounts/" +
        encodeURIComponent(account.id) +
        "/sync-checkpoints?" +
        query.toString(),
      { headers, cache: "no-store" },
    );
    if (response.status === 401) {
      return { state: "UNAVAILABLE", unauthorized: true, checkpoints: [] };
    }
    if (!response.ok) return unavailable();
    return projectAccountSyncHistory(await response.json(), account, viewedAt);
  } catch {
    return unavailable();
  }
}
