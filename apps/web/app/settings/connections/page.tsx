import { cookies } from "next/headers";
import Link from "next/link";
import { redirect } from "next/navigation";
import { AppPageHeader } from "../../app-page-header";
import { ConnectionsManager } from "./connections-manager";
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
  last_synced_at?: string;
  authorization_expires_at?: string;
};
export type FinancialAccount = {
  id: string;
  provider_connection_id: string;
  provider: string;
  display_name: string;
  status: string;
  capabilities?: Record<string, "SUPPORTED" | "UNSUPPORTED" | "UNKNOWN">;
};
export type NeuralPreference = {
  connection_id: string;
  model_id: string;
};
export default async function ConnectionsPage() {
  const jar = await cookies();
  const response = await fetch(
    `${process.env.API_BASE_URL ?? "http://localhost:8080"}/api/connections/ai`,
    { headers: { cookie: jar.toString() }, cache: "no-store" },
  );
  if (response.status === 401) redirect("/login");
  if (!response.ok) throw new Error("Unable to load provider connections");
  const data = (await response.json()) as {
    connections: Connection[];
    providers: Provider[];
    can_use_neural_engine: boolean;
  };
  const financialResponse = await fetch(
    `${process.env.API_BASE_URL ?? "http://localhost:8080"}/api/connections/financial/providers`,
    { headers: { cookie: jar.toString() }, cache: "no-store" },
  );
  const financial = financialResponse.ok
    ? ((await financialResponse.json()) as {
        providers: FinancialProvider[];
        can_connect_financial_accounts: boolean;
      })
    : { providers: [], can_connect_financial_accounts: false };
  const [connectionsResponse, accountsResponse, preferenceResponse] =
    await Promise.all([
      fetch(
        `${process.env.API_BASE_URL ?? "http://localhost:8080"}/api/connections/financial`,
        { headers: { cookie: jar.toString() }, cache: "no-store" },
      ),
      fetch(
        `${process.env.API_BASE_URL ?? "http://localhost:8080"}/api/accounts`,
        { headers: { cookie: jar.toString() }, cache: "no-store" },
      ),
      fetch(
        `${process.env.API_BASE_URL ?? "http://localhost:8080"}/api/settings/neural-engine`,
        { headers: { cookie: jar.toString() }, cache: "no-store" },
      ),
    ]);
  const financialConnections = connectionsResponse.ok
    ? (
        (await connectionsResponse.json()) as {
          connections: FinancialConnection[];
        }
      ).connections
    : [];
  const financialAccounts = accountsResponse.ok
    ? ((await accountsResponse.json()) as { accounts: FinancialAccount[] })
        .accounts
    : [];
  const preference = preferenceResponse.ok
    ? (
        (await preferenceResponse.json()) as {
          preference: NeuralPreference | null;
        }
      ).preference
    : null;
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
