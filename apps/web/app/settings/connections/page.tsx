import { cookies } from "next/headers";
import { redirect } from "next/navigation";
import { loadAccountSyncHistory } from "../../accounts/[id]/account-sync-history";
import { loadConnectionSyncAttemptHistory } from "./connection-sync-attempt-history";
import type { ConnectionRuntimeBinding } from "./connection-sync-evidence-center";
import { ConnectionsWorkspace } from "./connections-workspace";
import {
  FinancialConnectionOperatingWorkspace,
  type FinancialAuthorizationReceipt,
} from "./financial-connection-operating-brief";
import {
  type FinancialContinuityEngine,
  type FinancialContinuityRun,
} from "./financial-continuity-center";

export type Connection = {
  id: string;
  provider: string;
  provider_label: string;
  display_name: string;
  status: string;
  enabled: boolean;
  credential_hint: string;
  runtime_protected: boolean;
  removal_protected: boolean;
  protected_mandate_count: number;
  active_strategy_count: number;
  retained_automation_count: number;
  default_model_selected: boolean;
  last_verified_at?: string;
};
export type Provider = {
  id: string;
  label: string;
  credential_types: string[];
  capabilities: string[];
};
export type FinancialProvider = {
  id: string;
  label: string;
  auth_type: string;
  availability: "implemented" | "planned";
  configured: boolean;
};
export type FinancialConnection = {
  id: string;
  provider: string;
  display_name: string;
  status: string;
  runtime_protected: boolean;
  protected_mandate_count: number;
  active_strategy_count: number;
  last_synced_at?: string | null;
  authorization_expires_at?: string | null;
};
export type FinancialAccount = {
  id: string;
  provider_connection_id: string;
  provider: string;
  display_name: string;
  status: string;
  last_synced_at?: string | null;
  capabilities?: Record<string, "SUPPORTED" | "UNSUPPORTED" | "UNKNOWN">;
};
export type NeuralPreference = {
  connection_id: string;
  model_id: string;
};

type StrategyInstance = {
  id?: string;
  ID?: string;
  automation_mandate_id?: string;
  AutomationMandateID?: string;
  financial_account_id?: string;
  FinancialAccountID?: string;
  capital_bucket_id?: string;
  CapitalBucketID?: string;
  strategy_identifier?: string;
  StrategyIdentifier?: string;
  execution_mode?: string;
  ExecutionMode?: string;
  current_state?: string;
  CurrentState?: string;
  status?: string;
  Status?: string;
  mandate_version?: number;
  MandateVersion?: number;
};

type StrategySchedule = {
  last_status?: string;
  last_completed_at?: string;
  next_run_at?: string;
  consecutive_failures?: number;
};

type StrategyVersion = Record<string, unknown>;

function record(value: unknown): Record<string, unknown> | undefined {
  return value !== null && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : undefined;
}

function pinnedMarketSymbols(value: StrategyVersion | undefined) {
  const snapshot = record(value?.snapshot ?? value?.Snapshot);
  const universe = record(
    snapshot?.allowed_universe ?? snapshot?.AllowedUniverse,
  );
  const raw = universe?.symbols ?? universe?.Symbols;
  if (!Array.isArray(raw)) return [];
  const symbols = raw.filter(
    (symbol): symbol is string =>
      typeof symbol === "string" && /^[A-Z][A-Z0-9.\-]{0,14}$/.test(symbol),
  );
  return new Set(symbols).size === symbols.length ? symbols : [];
}

function normalizedStrategyInstance(
  instance: StrategyInstance,
): StrategyInstance {
  return {
    ...instance,
    id: instance.id ?? instance.ID,
    automation_mandate_id:
      instance.automation_mandate_id ?? instance.AutomationMandateID,
    financial_account_id:
      instance.financial_account_id ?? instance.FinancialAccountID,
    capital_bucket_id: instance.capital_bucket_id ?? instance.CapitalBucketID,
    strategy_identifier:
      instance.strategy_identifier ?? instance.StrategyIdentifier,
    execution_mode: instance.execution_mode ?? instance.ExecutionMode,
    current_state: instance.current_state ?? instance.CurrentState,
    status: instance.status ?? instance.Status,
    mandate_version: instance.mandate_version ?? instance.MandateVersion,
  };
}

function continuityScheduleTiming(nextRunAt: string | undefined, now: Date) {
  if (!nextRunAt || Number.isNaN(now.valueOf())) return "UNAVAILABLE" as const;
  const nextRun = new Date(nextRunAt);
  if (Number.isNaN(nextRun.valueOf())) return "UNAVAILABLE" as const;
  return nextRun.valueOf() < now.valueOf() - 5 * 60 * 1000
    ? ("OVERDUE" as const)
    : ("ON_SCHEDULE" as const);
}

async function optionalJSON<T>(url: string, cookie: string) {
  try {
    const response = await fetch(url, {
      headers: { cookie },
      cache: "no-store",
    });
    if (!response.ok)
      return {
        available: false as const,
        status: response.status,
        payload: undefined,
      };
    return {
      available: true as const,
      status: response.status,
      payload: (await response.json()) as T,
    };
  } catch {
    return {
      available: false as const,
      status: undefined,
      payload: undefined,
    };
  }
}
export default async function ConnectionsPage() {
  const jar = await cookies();
  const cookie = jar.toString();
  const base = process.env.API_BASE_URL ?? "http://localhost:8080";
  const response = await fetch(`${base}/api/connections/ai`, {
    headers: { cookie },
    cache: "no-store",
  });
  if (response.status === 401) redirect("/login");
  if (!response.ok) throw new Error("Unable to load provider connections");
  const data = (await response.json()) as {
    connections: Connection[];
    providers: Provider[];
    can_use_neural_engine: boolean;
  };
  const financialResponse = await fetch(
    `${base}/api/connections/financial/providers`,
    { headers: { cookie }, cache: "no-store" },
  );
  const financial = financialResponse.ok
    ? ((await financialResponse.json()) as {
        providers: FinancialProvider[];
        can_connect_financial_accounts: boolean;
      })
    : { providers: [], can_connect_financial_accounts: false };
  const [
    connectionsResponse,
    accountsResponse,
    preferenceResponse,
    instancesResponse,
  ] = await Promise.all([
    fetch(`${base}/api/connections/financial`, {
      headers: { cookie },
      cache: "no-store",
    }),
    fetch(`${base}/api/accounts`, { headers: { cookie }, cache: "no-store" }),
    fetch(`${base}/api/settings/neural-engine`, {
      headers: { cookie },
      cache: "no-store",
    }),
    fetch(`${base}/api/strategy-instances`, {
      headers: { cookie },
      cache: "no-store",
    }),
  ]);
  if (
    [
      connectionsResponse,
      accountsResponse,
      preferenceResponse,
      instancesResponse,
    ].some((result) => result.status === 401)
  )
    redirect("/login");
  const financialConnectionsPayload = connectionsResponse.ok
    ? ((await connectionsResponse.json()) as {
        connections?: FinancialConnection[];
      })
    : undefined;
  const financialAccountsPayload = accountsResponse.ok
    ? ((await accountsResponse.json()) as { accounts?: FinancialAccount[] })
    : undefined;
  const strategyInstancesPayload = instancesResponse.ok
    ? ((await instancesResponse.json()) as {
        strategy_instances?: StrategyInstance[];
      })
    : undefined;
  const financialConnections = Array.isArray(
    financialConnectionsPayload?.connections,
  )
    ? financialConnectionsPayload.connections
    : [];
  const financialAccounts = Array.isArray(financialAccountsPayload?.accounts)
    ? financialAccountsPayload.accounts
    : [];
  const authorizationReceiptResult = await optionalJSON<{
    receipts?: FinancialAuthorizationReceipt[];
    evidence_semantics?: string;
    timestamp_precision?: string;
    attempt_pairing_semantics?: string;
    receipt_limit?: number;
    provider_contact_performed?: boolean;
    reconnect_performed?: boolean;
    credentials_exposed?: boolean;
    account_mutation_available?: boolean;
    broker_action_available?: boolean;
    live_execution_available?: boolean;
  }>(`${base}/api/connections/financial/authorization-receipts`, cookie);
  if (authorizationReceiptResult.status === 401) redirect("/login");
  const authorizationReceiptPayload = authorizationReceiptResult.payload;
  const authorizationEvidenceAvailable = Boolean(
    authorizationReceiptResult.available &&
      authorizationReceiptPayload?.evidence_semantics ===
        "CREDENTIAL_FREE_FINANCIAL_AUTHORIZATION_RECEIPTS" &&
      authorizationReceiptPayload.timestamp_precision ===
        "MILLISECOND_CANONICAL" &&
      authorizationReceiptPayload.attempt_pairing_semantics ===
        "EXACT_ATTEMPT_ID_CONNECTION_BOUND" &&
      authorizationReceiptPayload.receipt_limit === 20 &&
      authorizationReceiptPayload.provider_contact_performed === false &&
      authorizationReceiptPayload.reconnect_performed === false &&
      authorizationReceiptPayload.credentials_exposed === false &&
      authorizationReceiptPayload.account_mutation_available === false &&
      authorizationReceiptPayload.broker_action_available === false &&
      authorizationReceiptPayload.live_execution_available === false &&
      Array.isArray(authorizationReceiptPayload.receipts),
  );
  const authorizationReceipts = authorizationEvidenceAvailable
    ? (authorizationReceiptPayload?.receipts ?? [])
    : [];
  const rawStrategyInstances = Array.isArray(
    strategyInstancesPayload?.strategy_instances,
  )
    ? strategyInstancesPayload.strategy_instances
    : [];
  const strategyInstances = rawStrategyInstances.map(
    normalizedStrategyInstance,
  );
  const continuityInventoryAvailable = Boolean(
    connectionsResponse.ok &&
      accountsResponse.ok &&
      instancesResponse.ok &&
      Array.isArray(financialConnectionsPayload?.connections) &&
      Array.isArray(financialAccountsPayload?.accounts) &&
      Array.isArray(strategyInstancesPayload?.strategy_instances),
  );
  const preference = preferenceResponse.ok
    ? (
        (await preferenceResponse.json()) as {
          preference: NeuralPreference | null;
        }
      ).preference
    : null;
  const continuityObservedAt = new Date();
  const continuityEngines = await Promise.all(
    strategyInstances
      .filter(
        (instance) =>
          instance.strategy_identifier === "ai_shadow" &&
          instance.status === "ACTIVE",
      )
      .map(async (instance): Promise<FinancialContinuityEngine> => {
        const account = financialAccounts.find(
          (candidate) => candidate.id === instance.financial_account_id,
        );
        const instanceID = instance.id ?? "";
        const [scheduleResult, historyResult, versionResult] =
          await Promise.all([
            instanceID
              ? optionalJSON<{ schedule?: StrategySchedule }>(
                  `${base}/api/strategy-instances/${encodeURIComponent(instanceID)}/schedule`,
                  cookie,
                )
              : Promise.resolve({
                  available: false as const,
                  status: undefined,
                  payload: undefined,
                }),
            instanceID
              ? optionalJSON<{
                  runs?: FinancialContinuityRun[];
                  history_semantics?: string;
                  broker_action_available?: boolean;
                  live_execution_available?: boolean;
                }>(
                  `${base}/api/strategy-instances/${encodeURIComponent(instanceID)}/schedule-runs?limit=12`,
                  cookie,
                )
              : Promise.resolve({
                  available: false as const,
                  status: undefined,
                  payload: undefined,
                }),
            instance.automation_mandate_id && instance.mandate_version
              ? optionalJSON<{ version?: StrategyVersion }>(
                  `${base}/api/automations/${encodeURIComponent(instance.automation_mandate_id)}/versions/${instance.mandate_version}`,
                  cookie,
                )
              : Promise.resolve({
                  available: false as const,
                  status: undefined,
                  payload: undefined,
                }),
          ]);
        if (
          scheduleResult.status === 401 ||
          historyResult.status === 401 ||
          versionResult.status === 401
        )
          redirect("/login");
        const schedule = scheduleResult.payload?.schedule;
        const marketSymbols = pinnedMarketSymbols(
          versionResult.payload?.version,
        );
        const recentRuns = Array.isArray(historyResult.payload?.runs)
          ? historyResult.payload.runs
          : [];
        return {
          mandate_id: instance.automation_mandate_id,
          instance_id: instance.id,
          connection_id: account?.provider_connection_id,
          account_id: account?.id,
          capital_bucket_id: instance.capital_bucket_id,
          account_name: account?.display_name,
          provider: account?.provider,
          execution_mode: instance.execution_mode,
          instance_status: instance.status,
          current_state: instance.current_state,
          schedule_available: scheduleResult.available,
          schedule_history_available: Boolean(
            historyResult.available &&
              historyResult.payload?.history_semantics ===
                "IMMUTABLE_NONLIVE_SCHEDULER_EVIDENCE" &&
              historyResult.payload?.broker_action_available === false &&
              historyResult.payload?.live_execution_available === false,
          ),
          schedule_status: schedule?.last_status,
          schedule_completed_at: schedule?.last_completed_at,
          schedule_next_run_at: schedule?.next_run_at,
          schedule_timing_status: continuityScheduleTiming(
            schedule?.next_run_at,
            continuityObservedAt,
          ),
          consecutive_failures: schedule?.consecutive_failures,
          recent_runs: recentRuns,
          market_scope_available: Boolean(
            versionResult.available && marketSymbols.length > 0,
          ),
          market_symbols: marketSymbols,
          required_market_quality:
            account?.provider === "schwab"
              ? "BROKER_REALTIME"
              : account?.provider === "coinbase"
                ? "REAL_TIME_SINGLE_VENUE"
                : undefined,
        };
      }),
  );
  const activeFinancialConnectionIDs = new Set(
    financialConnections
      .filter((connection) => connection.status === "active")
      .map((connection) => connection.id),
  );
  const activeFinancialAccounts = financialAccounts.filter(
    (account) =>
      account.status === "active" &&
      activeFinancialConnectionIDs.has(account.provider_connection_id),
  );
  const runtimeBindings: ConnectionRuntimeBinding[] = strategyInstances
    .filter((instance) => instance.status === "ACTIVE")
    .map((instance) => ({
      instance_id: instance.id,
      mandate_id: instance.automation_mandate_id,
      account_id: instance.financial_account_id,
      capital_bucket_id: instance.capital_bucket_id,
      strategy_identifier: instance.strategy_identifier,
      execution_mode: instance.execution_mode,
      current_state: instance.current_state,
      status: instance.status,
    }));
  const syncEvidenceInputs = await Promise.all(
    activeFinancialAccounts.map(async (account) => {
      const [syncHistory, attemptHistory, reconciliationResult] =
        await Promise.all([
          loadAccountSyncHistory({
            account,
            base,
            headers: { cookie },
            viewedAt: continuityObservedAt,
          }),
          loadConnectionSyncAttemptHistory({
            account,
            base,
            headers: { cookie },
            viewedAt: continuityObservedAt,
          }),
          optionalJSON<unknown>(
            `${base}/api/accounts/${encodeURIComponent(account.id)}/reconciliations/latest`,
            cookie,
          ),
        ]);
      if (
        syncHistory.unauthorized ||
        attemptHistory.unauthorized ||
        reconciliationResult.status === 401
      )
        redirect("/login");
      return {
        account,
        syncHistory,
        attemptHistory,
        reconciliationPayload: reconciliationResult.available
          ? reconciliationResult.payload
          : undefined,
        bindings: runtimeBindings.filter(
          (binding) => binding.account_id === account.id,
        ),
      };
    }),
  );
  return (
    <ConnectionsWorkspace
      connections={data.connections}
      providers={data.providers}
      aiEntitled={data.can_use_neural_engine}
      preference={preference}
      preferenceAvailable={preferenceResponse.ok}
      financialProviders={financial.providers}
      financialProvidersAvailable={financialResponse.ok}
      financialEntitled={financial.can_connect_financial_accounts}
      financialConnections={financialConnections}
      financialAccounts={financialAccounts}
      inventoryAvailable={Boolean(
        connectionsResponse.ok &&
          accountsResponse.ok &&
          Array.isArray(financialConnectionsPayload?.connections) &&
          Array.isArray(financialAccountsPayload?.accounts),
      )}
      operatingEvidence={
        <FinancialConnectionOperatingWorkspace
          connections={financialConnections}
          accounts={financialAccounts}
          engines={continuityEngines}
          syncInputs={syncEvidenceInputs}
          observedAt={continuityObservedAt.toISOString()}
          contextAvailable={continuityInventoryAvailable}
          expectedBindingCount={runtimeBindings.length}
          authorizationReceipts={authorizationReceipts}
          authorizationEvidenceAvailable={authorizationEvidenceAvailable}
        />
      }
    />
  );
}
