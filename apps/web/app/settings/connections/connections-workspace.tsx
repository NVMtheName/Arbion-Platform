import type { ReactNode } from "react";
import Link from "next/link";
import { AppPageHeader } from "../../app-page-header";
import { ConnectionsManager } from "./connections-manager";
import { FinancialManager } from "./financial-manager";
import type {
  Connection,
  Provider,
  FinancialProvider,
  FinancialConnection,
  FinancialAccount,
  NeuralPreference,
} from "./page";

type ConnectionsWorkspaceProps = {
  connections: Connection[];
  providers: Provider[];
  aiEntitled: boolean;
  preference: NeuralPreference | null;
  preferenceAvailable: boolean;
  financialProviders: FinancialProvider[];
  financialProvidersAvailable: boolean;
  financialEntitled: boolean;
  financialConnections: FinancialConnection[];
  financialAccounts: FinancialAccount[];
  inventoryAvailable: boolean;
  operatingEvidence: ReactNode;
};

export function ConnectionsWorkspace({
  connections,
  providers,
  aiEntitled,
  preference,
  preferenceAvailable,
  financialProviders,
  financialProvidersAvailable,
  financialEntitled,
  financialConnections,
  financialAccounts,
  inventoryAvailable,
  operatingEvidence,
}: ConnectionsWorkspaceProps) {
  const activeConnectionIDs = new Set(
    financialConnections
      .filter((item) => item.status === "active")
      .map((item) => item.id),
  );
  const activeFinancialAccounts = inventoryAvailable
    ? financialAccounts.filter(
        (account) =>
          account.status === "active" &&
          activeConnectionIDs.has(account.provider_connection_id),
      )
    : [];
  const activeAIConnections = connections.filter(
    (item) => item.status === "active" && item.enabled,
  );
  const readySteps = [
    activeFinancialAccounts.length > 0,
    activeAIConnections.length > 0,
    preferenceAvailable && preference !== null,
  ].filter(Boolean).length;
  const availableFinancialProviders = financialProviders.filter(
    (provider) =>
      financialProvidersAvailable && provider.availability === "implemented",
  );
  const plannedFinancialProviders = financialProviders.filter(
    (provider) => provider.availability === "planned",
  );
  return (
    <main className="connection-hub connections-workspace command-content-continuity">
      <AppPageHeader contentHeadingId="connections-page-title" />
      <section className="connection-hub-hero">
        <div>
          <p className="eyebrow">YOUR WORKSPACE</p>
          <h1 id="connections-page-title">Connections</h1>
          <p className="connection-hub-lede">
            Link your financial accounts, add an AI key, and choose your model.
          </p>
        </div>
        <div className="connection-progress" aria-label="Setup progress">
          <strong>{readySteps} of 3 essentials ready</strong>
          <span>
            {readySteps === 3
              ? "Account, AI provider, and default model selected."
              : "Complete your account, AI provider, and model setup."}
          </span>
          <a className="connection-health-link" href="#connection-health">
            Review connection health
          </a>
        </div>
      </section>

      {(!inventoryAvailable ||
        !preferenceAvailable ||
        financialConnections.some((connection) =>
          ["expired", "error"].includes(connection.status),
        )) && (
        <p className="connection-attention-banner" role="status">
          {!inventoryAvailable || !preferenceAvailable
            ? "Some saved setup information is unavailable. No account or model has been changed."
            : "A financial connection needs attention. Review its authorization below."}
          <a href="#connection-health">Review saved connection health</a>
        </p>
      )}
      <nav className="connection-steps" aria-label="Connection setup steps">
        <a
          className={activeFinancialAccounts.length > 0 ? "is-complete" : ""}
          href="#financial-accounts"
        >
          <span>1</span>
          <strong>Financial account</strong>
          <small>
            {!inventoryAvailable
              ? "Status unavailable"
              : activeFinancialAccounts.length > 0
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
        <a
          className={preferenceAvailable && preference ? "is-complete" : ""}
          href="#model-choice"
        >
          <span>3</span>
          <strong>Default model</strong>
          <small>
            {!preferenceAvailable
              ? "Status unavailable"
              : preference
                ? "Model selected"
                : "Choose a model"}
          </small>
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
        {!financialProvidersAvailable && (
          <p className="connection-quiet-state" role="status">
            Financial providers are unavailable right now. Your existing
            connections have not been changed.
          </p>
        )}
        <div className="financial-provider-grid">
          {availableFinancialProviders.map((provider) => (
            <article
              className="financial-provider-card"
              id={`financial-provider-${provider.id}`}
              aria-labelledby={`financial-provider-title-${provider.id}`}
              key={provider.id}
            >
              <header>
                <div>
                  <span
                    aria-hidden="true"
                    className={`provider-mark provider-${provider.id}`}
                  >
                    {provider.label.slice(0, 1)}
                  </span>
                  <div>
                    <h3 id={`financial-provider-title-${provider.id}`}>
                      {provider.label}
                    </h3>
                    <p>
                      {provider.id === "coinbase"
                        ? "Crypto portfolio"
                        : "Brokerage portfolio"}
                    </p>
                  </div>
                </div>
                <span
                  className={`connection-status ${
                    !inventoryAvailable
                      ? "is-attention"
                      : financialConnections.some(
                            (connection) =>
                              connection.provider === provider.id &&
                              connection.status === "active",
                          )
                        ? "is-connected"
                        : financialConnections.some(
                              (connection) =>
                                connection.provider === provider.id &&
                                ["expired", "error"].includes(
                                  connection.status,
                                ),
                            )
                          ? "is-attention"
                          : ""
                  }`}
                >
                  {!inventoryAvailable
                    ? "Unavailable"
                    : financialConnections.some(
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
              {provider.configured && inventoryAvailable ? (
                <FinancialManager
                  provider={provider}
                  entitled={financialEntitled}
                  connections={financialConnections.filter(
                    (connection) => connection.provider === provider.id,
                  )}
                  accounts={financialAccounts}
                />
              ) : (
                <p className="connection-quiet-state">
                  {!inventoryAvailable
                    ? "Connection inventory is unavailable. Existing accounts have not been changed; connection controls will return when the saved inventory is available."
                    : "This connection is temporarily unavailable."}
                </p>
              )}
            </article>
          ))}
        </div>
        {plannedFinancialProviders.length > 0 && (
          <p className="connection-coming-soon">
            Planned — not yet available:{" "}
            {plannedFinancialProviders
              .map((provider) => provider.label)
              .join(", ")}
            .
          </p>
        )}
      </section>

      <ConnectionsManager
        initialConnections={connections}
        initialPreference={preferenceAvailable ? preference : null}
        providers={providers}
        entitled={aiEntitled}
      />

      <section className="connection-finish" aria-label="Connection next step">
        <div>
          <p className="connection-step-label">NEXT</p>
          <h2>
            {readySteps === 3
              ? "Choose your next strategy."
              : "Finish setup at your own pace."}
          </h2>
          <p>
            Connected accounts and a selected model do not enable trading. Your
            strategy’s mode, risk limits, and execution controls still apply.
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
      <section
        id="connection-health"
        className="connection-review-section"
        aria-label="Saved connection health"
      >
        <header className="connection-review-heading">
          <p className="connection-step-label">SAVED EVIDENCE</p>
          <h2>Connection health</h2>
        </header>
        {financialConnections.length === 0 && (
          <p className="connection-quiet-state">
            {inventoryAvailable
              ? "Connect a financial account to see its saved authorization, sync, and strategy-continuity evidence here."
              : "Saved connection health is unavailable while the connection inventory cannot be loaded. This is not evidence that your accounts were disconnected."}
          </p>
        )}
        {operatingEvidence}
      </section>
    </main>
  );
}
