import { cookies } from "next/headers";
import { redirect } from "next/navigation";

import {
  CommandCenterDashboard,
  type DashboardAccountSummary,
  type DashboardAIEngineSummary,
  type DashboardMoney,
} from "./command-center-dashboard";
import type { OwnerAttentionOverview } from "./owner-attention-center";

type User = {
  email: string;
  display_name: string;
  entitlement: string;
  role: string;
};

type Preference = { connection_id: string; model_id: string };

type Account = {
  id: string;
  provider: string;
  display_name: string;
  status: string;
  last_synced_at?: string;
};

type Balances = {
  cash?: DashboardMoney;
  available_cash?: DashboardMoney;
  account_value?: DashboardMoney;
  equity?: DashboardMoney;
};

type CryptoPortfolio = {
  portfolio_state?: "READY" | "PARTIAL";
  balance_state?: "READY" | "UNAVAILABLE";
  holdings_state?: "READY" | "UNAVAILABLE";
  observed_value?: DashboardMoney;
  balances?: {
    cash?: DashboardMoney;
    available_cash?: DashboardMoney;
  };
  total_positions?: number;
  pricing_state?: "READY" | "PARTIAL" | "UNAVAILABLE";
  pricing_as_of?: string;
};

type AutomationRecord = Record<string, unknown>;
type StrategyInstanceRecord = Record<string, unknown>;

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

async function fetchDashboardJSON<T>(
  url: string,
  headers: { cookie: string },
): Promise<{ available: boolean; payload?: T }> {
  try {
    const response = await fetch(url, { headers, cache: "no-store" });
    if (!response.ok) return { available: false };
    return { available: true, payload: (await response.json()) as T };
  } catch {
    return { available: false };
  }
}

async function aiEngineSummaries(
  instances: StrategyInstanceRecord[],
  automations: AutomationRecord[],
  accounts: Account[],
  base: string,
  headers: { cookie: string },
): Promise<DashboardAIEngineSummary[]> {
  const engines = instances.filter(
    (instance) =>
      recordString(instance, "strategy_identifier", "StrategyIdentifier") ===
        "ai_shadow" &&
      recordString(instance, "execution_mode", "ExecutionMode") === "SHADOW" &&
      ["ACTIVE", "PAUSED"].includes(
        recordString(instance, "status", "Status") ?? "",
      ),
  );

  return Promise.all(
    engines.map(async (instance) => {
      const id = recordString(instance, "id", "ID") ?? "";
      const mandateID =
        recordString(
          instance,
          "automation_mandate_id",
          "AutomationMandateID",
        ) ?? "";
      const accountID =
        recordString(instance, "financial_account_id", "FinancialAccountID") ??
        "";
      const mandate = automations.find(
        (item) => recordString(item, "id", "ID") === mandateID,
      );
      const account = accounts.find((item) => item.id === accountID);
      const [scheduleResult, decisionsResult] = await Promise.all([
        fetchDashboardJSON<{
          schedule?: Record<string, unknown>;
        }>(
          `${base}/api/strategy-instances/${encodeURIComponent(id)}/schedule`,
          headers,
        ),
        fetchDashboardJSON<{
          decisions?: Record<string, unknown>[];
        }>(
          `${base}/api/strategy-instances/${encodeURIComponent(id)}/decisions`,
          headers,
        ),
      ]);
      const schedule = scheduleResult.payload?.schedule;
      const lastDecision = decisionsResult.payload?.decisions?.find(
        (decision) => recordString(decision, "source", "Source") === "AI",
      );

      return {
        id,
        mandateID,
        accountName: account?.display_name ?? "Connected account",
        provider: account?.provider ?? "connected_account",
        status: recordString(instance, "status", "Status") ?? "UNKNOWN",
        currentState:
          recordString(instance, "current_state", "CurrentState") ?? "UNKNOWN",
        executionMode:
          recordString(instance, "execution_mode", "ExecutionMode") ?? "SHADOW",
        modelID: recordString(mandate, "ai_model_id", "AIModelID"),
        lastEvaluatedAt: recordString(
          instance,
          "last_evaluated_at",
          "LastEvaluatedAt",
        ),
        nextRunAt: recordString(schedule, "next_run_at", "NextRunAt"),
        scheduleStatus: recordString(schedule, "last_status", "LastStatus"),
        scheduleAvailable: scheduleResult.available,
        journalAvailable: decisionsResult.available,
        consecutiveFailures:
          recordNumber(
            schedule,
            "consecutive_failures",
            "ConsecutiveFailures",
          ) ?? 0,
        lastDecision: recordString(
          lastDecision,
          "decision_type",
          "DecisionType",
        ),
        lastDecisionSymbol: recordString(lastDecision, "symbol", "Symbol"),
        lastDecisionAt: recordString(lastDecision, "created_at", "CreatedAt"),
      };
    }),
  );
}

async function accountSummary(
  account: Account,
  base: string,
  headers: { cookie: string },
): Promise<DashboardAccountSummary> {
  const id = encodeURIComponent(account.id);
  const shared = {
    id: account.id,
    provider: account.provider,
    displayName: account.display_name,
    status: account.status,
    asOf: account.last_synced_at,
  };

  try {
    if (account.provider === "coinbase") {
      const response = await fetch(
        `${base}/api/accounts/${id}/portfolio/crypto`,
        {
          headers,
          cache: "no-store",
        },
      );
      if (!response.ok) {
        return { ...shared, availability: "unavailable" };
      }
      const portfolio = (
        (await response.json()) as { portfolio?: CryptoPortfolio }
      ).portfolio;
      if (!portfolio) return { ...shared, availability: "unavailable" };
      return {
        ...shared,
        observedValue: portfolio.observed_value,
        cash: portfolio.balances?.cash ?? portfolio.balances?.available_cash,
        positionCount:
          portfolio.holdings_state === "READY"
            ? portfolio.total_positions
            : undefined,
        availability:
          portfolio.portfolio_state === "PARTIAL" ||
          portfolio.pricing_state !== "READY"
            ? "partial"
            : "ready",
        asOf: portfolio.pricing_as_of ?? account.last_synced_at,
      };
    }

    const [balancesResponse, positionsResponse] = await Promise.all([
      fetch(`${base}/api/accounts/${id}/balances`, {
        headers,
        cache: "no-store",
      }),
      fetch(`${base}/api/accounts/${id}/positions`, {
        headers,
        cache: "no-store",
      }),
    ]);
    const balances = balancesResponse.ok
      ? ((await balancesResponse.json()) as { balances?: Balances }).balances
      : undefined;
    const positions = positionsResponse.ok
      ? ((await positionsResponse.json()) as { positions?: unknown[] })
          .positions
      : undefined;
    return {
      ...shared,
      observedValue: balances?.account_value ?? balances?.equity,
      cash: balances?.cash ?? balances?.available_cash,
      positionCount: positions?.length,
      availability:
        balancesResponse.ok && positionsResponse.ok
          ? "ready"
          : balancesResponse.ok || positionsResponse.ok
            ? "partial"
            : "unavailable",
    };
  } catch {
    return { ...shared, availability: "unavailable" };
  }
}

export default async function Dashboard() {
  const cookieStore = await cookies();
  const api = process.env.API_BASE_URL ?? "http://localhost:8080";
  const headers = { cookie: cookieStore.toString() };
  const response = await fetch(`${api}/api/auth/me`, {
    headers,
    cache: "no-store",
  });
  if (!response.ok) redirect("/login");
  const { user } = (await response.json()) as { user: User };

  const [
    connectionsResponse,
    accountsResponse,
    preferenceResponse,
    automationsResponse,
    instancesResponse,
    attentionResponse,
  ] = await Promise.all([
    fetch(`${api}/api/connections/ai`, { headers, cache: "no-store" }),
    fetch(`${api}/api/accounts`, { headers, cache: "no-store" }),
    fetch(`${api}/api/settings/neural-engine`, {
      headers,
      cache: "no-store",
    }),
    fetch(`${api}/api/automations`, { headers, cache: "no-store" }),
    fetch(`${api}/api/strategy-instances`, {
      headers,
      cache: "no-store",
    }),
    fetch(`${api}/api/owner/attention`, {
      headers,
      cache: "no-store",
    }).catch(() => null),
  ]);

  const connections = connectionsResponse.ok
    ? ((
        (await connectionsResponse.json()) as {
          connections?: { status?: string }[] | null;
        }
      ).connections ?? [])
    : [];
  const accounts = accountsResponse.ok
    ? ((
        (await accountsResponse.json()) as {
          accounts?: Account[] | null;
        }
      ).accounts ?? [])
    : [];
  const preference = preferenceResponse.ok
    ? (
        (await preferenceResponse.json()) as {
          preference: Preference | null;
        }
      ).preference
    : null;
  const automations = automationsResponse.ok
    ? ((
        (await automationsResponse.json()) as {
          automations?: AutomationRecord[] | null;
        }
      ).automations ?? [])
    : [];
  const instances = instancesResponse.ok
    ? ((
        (await instancesResponse.json()) as {
          strategy_instances?: StrategyInstanceRecord[] | null;
        }
      ).strategy_instances ?? [])
    : [];
  const attention = attentionResponse?.ok
    ? (
        (await attentionResponse.json()) as {
          attention: OwnerAttentionOverview;
        }
      ).attention
    : undefined;
  const activeAccounts = accounts.filter(
    (account) => account.status === "active",
  );
  const [accountSummaries, engines] = await Promise.all([
    Promise.all(
      activeAccounts.map((account) => accountSummary(account, api, headers)),
    ),
    aiEngineSummaries(instances, automations, accounts, api, headers),
  ]);

  return (
    <CommandCenterDashboard
      accounts={accountSummaries}
      aiEngines={engines}
      attention={attention}
      attentionAvailable={attentionResponse?.ok === true}
      connectionCount={
        connections.filter((connection) => connection.status === "active")
          .length
      }
      modelConfigured={preference !== null}
      modelID={preference?.model_id}
      user={user}
    />
  );
}
