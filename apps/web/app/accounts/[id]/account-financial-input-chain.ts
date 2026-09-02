import {
  type FinancialContinuityEngine,
  type FinancialContinuityRun,
  type FinancialInputChainProjection,
  projectFinancialInputChains,
} from "../../settings/connections/financial-continuity-center";
import type {
  FinancialAccount,
  FinancialConnection,
} from "../../settings/connections/page";

type StrategyInstance = Record<string, unknown>;
type StrategySchedule = Record<string, unknown>;

type OptionalJSON<T> = {
  available: boolean;
  status?: number;
  payload?: T;
};

export type AccountFinancialInputChainResult = {
  available: boolean;
  unauthorized: boolean;
  projection?: FinancialInputChainProjection;
};

function recordString(
  record: Record<string, unknown> | undefined,
  key: string,
  legacy: string,
) {
  const value = record?.[key] ?? record?.[legacy];
  return typeof value === "string" ? value : undefined;
}

function recordNumber(
  record: Record<string, unknown> | undefined,
  key: string,
  legacy: string,
) {
  const value = record?.[key] ?? record?.[legacy];
  return typeof value === "number" && Number.isFinite(value)
    ? value
    : undefined;
}

async function optionalJSON<T>(
  url: string,
  headers: { cookie: string },
): Promise<OptionalJSON<T>> {
  try {
    const response = await fetch(url, { headers, cache: "no-store" });
    if (!response.ok) {
      return { available: false, status: response.status };
    }
    return {
      available: true,
      status: response.status,
      payload: (await response.json()) as T,
    };
  } catch {
    return { available: false };
  }
}

function scheduleTiming(nextRunAt: string | undefined, observedAt: Date) {
  if (!nextRunAt || Number.isNaN(observedAt.valueOf())) {
    return "UNAVAILABLE" as const;
  }
  const nextRun = new Date(nextRunAt);
  if (Number.isNaN(nextRun.valueOf())) return "UNAVAILABLE" as const;
  return nextRun.valueOf() < observedAt.valueOf() - 5 * 60 * 1000
    ? ("OVERDUE" as const)
    : ("ON_SCHEDULE" as const);
}

export async function loadAccountFinancialInputChain({
  accountID,
  base,
  headers,
  observedAt,
}: {
  accountID: string;
  base: string;
  headers: { cookie: string };
  observedAt: Date;
}): Promise<AccountFinancialInputChainResult> {
  const [connectionsResult, accountsResult, instancesResult] =
    await Promise.all([
      optionalJSON<{ connections?: FinancialConnection[] }>(
        `${base}/api/connections/financial`,
        headers,
      ),
      optionalJSON<{ accounts?: FinancialAccount[] }>(
        `${base}/api/accounts`,
        headers,
      ),
      optionalJSON<{ strategy_instances?: StrategyInstance[] }>(
        `${base}/api/strategy-instances`,
        headers,
      ),
    ]);
  const unauthorized = [
    connectionsResult,
    accountsResult,
    instancesResult,
  ].some((result) => result.status === 401);
  if (unauthorized) return { available: false, unauthorized: true };

  const connections = connectionsResult.payload?.connections;
  const accounts = accountsResult.payload?.accounts;
  const instances = instancesResult.payload?.strategy_instances;
  if (
    !connectionsResult.available ||
    !accountsResult.available ||
    !instancesResult.available ||
    !Array.isArray(connections) ||
    !Array.isArray(accounts) ||
    !Array.isArray(instances) ||
    !accounts.some((account) => account.id === accountID)
  ) {
    return { available: false, unauthorized: false };
  }

  const relevantInstances = instances.filter((instance) => {
    const mode = recordString(instance, "execution_mode", "ExecutionMode");
    return (
      recordString(instance, "financial_account_id", "FinancialAccountID") ===
        accountID &&
      recordString(instance, "strategy_identifier", "StrategyIdentifier") ===
        "ai_shadow" &&
      recordString(instance, "status", "Status") === "ACTIVE" &&
      (mode === "PAPER" || mode === "SHADOW")
    );
  });

  let nestedUnauthorized = false;
  const engines = await Promise.all(
    relevantInstances.map(
      async (instance): Promise<FinancialContinuityEngine> => {
        const instanceID = recordString(instance, "id", "ID") ?? "";
        const [scheduleResult, historyResult] = await Promise.all([
          instanceID
            ? optionalJSON<{ schedule?: StrategySchedule }>(
                `${base}/api/strategy-instances/${encodeURIComponent(instanceID)}/schedule`,
                headers,
              )
            : Promise.resolve<OptionalJSON<{ schedule?: StrategySchedule }>>({
                available: false,
              }),
          instanceID
            ? optionalJSON<{
                runs?: FinancialContinuityRun[];
                history_semantics?: string;
                broker_action_available?: boolean;
                live_execution_available?: boolean;
              }>(
                `${base}/api/strategy-instances/${encodeURIComponent(instanceID)}/schedule-runs?limit=12`,
                headers,
              )
            : Promise.resolve<
                OptionalJSON<{
                  runs?: FinancialContinuityRun[];
                  history_semantics?: string;
                  broker_action_available?: boolean;
                  live_execution_available?: boolean;
                }>
              >({ available: false }),
        ]);
        if (scheduleResult.status === 401 || historyResult.status === 401) {
          nestedUnauthorized = true;
        }
        const account = accounts.find(
          (candidate) => candidate.id === accountID,
        );
        const schedule = scheduleResult.payload?.schedule;
        const nextRunAt = recordString(schedule, "next_run_at", "NextRunAt");
        return {
          mandate_id: recordString(
            instance,
            "automation_mandate_id",
            "AutomationMandateID",
          ),
          instance_id: instanceID,
          connection_id: account?.provider_connection_id,
          account_id: account?.id,
          account_name: account?.display_name,
          provider: account?.provider,
          execution_mode: recordString(
            instance,
            "execution_mode",
            "ExecutionMode",
          ),
          instance_status: recordString(instance, "status", "Status"),
          current_state: recordString(
            instance,
            "current_state",
            "CurrentState",
          ),
          schedule_available: scheduleResult.available,
          schedule_history_available: Boolean(
            historyResult.available &&
              historyResult.payload?.history_semantics ===
                "IMMUTABLE_NONLIVE_SCHEDULER_EVIDENCE" &&
              historyResult.payload?.broker_action_available === false &&
              historyResult.payload?.live_execution_available === false,
          ),
          schedule_status: recordString(schedule, "last_status", "LastStatus"),
          schedule_completed_at: recordString(
            schedule,
            "last_completed_at",
            "LastCompletedAt",
          ),
          schedule_next_run_at: nextRunAt,
          schedule_timing_status: scheduleTiming(nextRunAt, observedAt),
          consecutive_failures: recordNumber(
            schedule,
            "consecutive_failures",
            "ConsecutiveFailures",
          ),
          recent_runs: Array.isArray(historyResult.payload?.runs)
            ? historyResult.payload.runs
            : [],
        };
      },
    ),
  );
  if (nestedUnauthorized) {
    return { available: false, unauthorized: true };
  }

  return {
    available: true,
    unauthorized: false,
    projection: projectFinancialInputChains({
      connections,
      accounts,
      engines,
      observedAt: observedAt.toISOString(),
    }),
  };
}
