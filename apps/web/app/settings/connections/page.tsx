import { cookies } from "next/headers";
import Link from "next/link";
import { redirect } from "next/navigation";
import { AppPageHeader } from "../../app-page-header";
import { ConnectionsManager } from "./connections-manager";
import {
  FinancialContinuityCenter,
  SchwabMarketDataReadinessView,
  type FinancialContinuityEngine,
  type FinancialContinuityRun,
} from "./financial-continuity-center";
import { FinancialManager } from "./financial-manager";

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
type FinancialProvider = {
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
  const activeAIConnections = data.connections.filter(
    (connection) => connection.status === "active" && connection.enabled,
  );
  const readySteps = [
    activeFinancialAccounts.length > 0,
    activeAIConnections.length > 0,
    preference !== null,
  ].filter(Boolean).length;
  const availableFinancialProviders = financial.providers.filter(
    (provider) => provider.availability === "implemented",
  );
  const plannedFinancialProviders = financial.providers.filter(
    (provider) => provider.availability === "planned",
  );
  return (
    <main className="connection-hub">
      <AppPageHeader />
      <section className="connection-hub-hero">
        <div>
          <p className="eyebrow">CONNECTION HUB</p>
          <h1>Connect once. See everything together.</h1>
          <p className="connection-hub-lede">
            Bring your own financial-provider and AI-provider credentials.
            Arbion keeps them encrypted, then gives you one place to choose an
            account, model, and strategy.
          </p>
        </div>
        <div className="connection-progress" aria-label="Setup progress">
          <strong>{readySteps} of 3 essentials ready</strong>
          <span>
            {readySteps === 3
              ? "Your command center is connected."
              : "Finish the highlighted steps to start building."}
          </span>
        </div>
      </section>

      <nav className="connection-steps" aria-label="Connection setup steps">
        <a
          className={activeFinancialAccounts.length > 0 ? "is-complete" : ""}
          href="#financial-accounts"
        >
          <span>1</span>
          <strong>Financial account</strong>
          <small>
            {activeFinancialAccounts.length > 0
              ? `${activeFinancialAccounts.length} connected`
              : "Connect a provider"}
          </small>
        </a>
        <a
          className={activeAIConnections.length > 0 ? "is-complete" : ""}
          href="#ai-providers"
        >
          <span>2</span>
          <strong>AI provider</strong>
          <small>
            {activeAIConnections.length > 0
              ? `${activeAIConnections.length} ready`
              : "Add an API key"}
          </small>
        </a>
        <a className={preference ? "is-complete" : ""} href="#model-choice">
          <span>3</span>
          <strong>Default model</strong>
          <small>{preference ? "Model selected" : "Choose a model"}</small>
        </a>
        <Link
          className={readySteps === 3 ? "is-ready" : ""}
          href="/automations/new"
        >
          <span>4</span>
          <strong>Strategy</strong>
          <small>
            {readySteps === 3 ? "Ready to build" : "Available next"}
          </small>
        </Link>
      </nav>

      <section
        className="connection-hub-section"
        id="financial-accounts"
        aria-labelledby="financial-title"
      >
        <header className="connection-section-heading">
          <div>
            <p className="connection-step-label">STEP 1</p>
            <h2 id="financial-title">Connect a financial account</h2>
          </div>
          <p>
            Use the API credentials or provider authorization you already have.
            Arbion never asks for your brokerage password.
          </p>
        </header>
        <FinancialContinuityCenter
          connections={financialConnections}
          accounts={financialAccounts}
          engines={continuityEngines}
          observedAt={continuityObservedAt.toISOString()}
          contextAvailable={continuityInventoryAvailable}
        />
        <SchwabMarketDataReadinessView
          connections={financialConnections}
          engines={continuityEngines}
          contextAvailable={continuityInventoryAvailable}
        />
        <div className="financial-provider-grid">
          {availableFinancialProviders.map((provider) => (
            <article className="financial-provider-card" key={provider.id}>
              <header>
                <div>
                  <span className={`provider-mark provider-${provider.id}`}>
                    {provider.label.slice(0, 1)}
                  </span>
                  <div>
                    <h3>{provider.label}</h3>
                    <p>
                      {provider.id === "coinbase"
                        ? "Crypto portfolio"
                        : "Brokerage portfolio"}
                    </p>
                  </div>
                </div>
                <span
                  className={`connection-status ${
                    financialConnections.some(
                      (connection) =>
                        connection.provider === provider.id &&
                        connection.status === "active",
                    )
                      ? "is-connected"
                      : financialConnections.some(
                            (connection) =>
                              connection.provider === provider.id &&
                              ["expired", "error"].includes(connection.status),
                          )
                        ? "is-attention"
                        : ""
                  }`}
                >
                  {financialConnections.some(
                    (connection) =>
                      connection.provider === provider.id &&
                      connection.status === "active",
                  )
                    ? "Connected"
                    : financialConnections.some(
                          (connection) =>
                            connection.provider === provider.id &&
                            ["expired", "error"].includes(connection.status),
                        )
                      ? "Reconnect"
                      : "Not connected"}
                </span>
              </header>
              {provider.configured ? (
                <FinancialManager
                  provider={provider}
                  entitled={financial.can_connect_financial_accounts}
                  connections={financialConnections.filter(
                    (connection) => connection.provider === provider.id,
                  )}
                  accounts={financialAccounts}
                />
              ) : (
                <p className="connection-quiet-state">
                  This connection is temporarily unavailable.
                </p>
              )}
            </article>
          ))}
        </div>
        {plannedFinancialProviders.length > 0 && (
          <p className="connection-coming-soon">
            Coming next:{" "}
            {plannedFinancialProviders
              .map((provider) => provider.label)
              .join(", ")}
            . Additional providers will use this same connection flow.
          </p>
        )}
      </section>

      <ConnectionsManager
        initialConnections={data.connections}
        initialPreference={preference}
        providers={data.providers}
        entitled={data.can_use_neural_engine}
      />

      <section className="connection-finish" aria-label="Connection next step">
        <div>
          <p className="connection-step-label">NEXT</p>
          <h2>
            {readySteps === 3
              ? "Your connected command center is ready."
              : "Complete the three essentials above."}
          </h2>
          <p>
            Choose an account and strategy next. Risk limits and approval
            controls stay in the background until they are needed.
          </p>
        </div>
        <div>
          <Link className="button-link" href="/accounts">
            View portfolio
          </Link>
          <Link className="connection-text-link" href="/automations/new">
            Choose a strategy →
          </Link>
        </div>
      </section>
    </main>
  );
}
