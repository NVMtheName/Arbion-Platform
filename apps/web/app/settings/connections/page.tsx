import { cookies } from "next/headers";
import { redirect } from "next/navigation";
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
  last_synced_at?: string;
};
export type FinancialAccount = {
  id: string;
  provider_connection_id: string;
  provider: string;
  display_name: string;
  status: string;
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
  const [connectionsResponse, accountsResponse] = await Promise.all([
    fetch(
      `${process.env.API_BASE_URL ?? "http://localhost:8080"}/api/connections/financial`,
      { headers: { cookie: jar.toString() }, cache: "no-store" },
    ),
    fetch(
      `${process.env.API_BASE_URL ?? "http://localhost:8080"}/api/accounts`,
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
  return (
    <>
      <ConnectionsManager
        initialConnections={data.connections}
        providers={data.providers}
        entitled={data.can_use_neural_engine}
      />
      <section
        className="financial-connections"
        aria-labelledby="financial-title"
      >
        <p className="eyebrow">READ-ONLY</p>
        <h2 id="financial-title">Financial Accounts</h2>
        <p className="security-note">
          Arbion uses provider authorization. Never enter your brokerage
          password into Arbion. Buying power is information, not trading
          permission.
        </p>
        <div className="provider-list">
          {financial.providers.map((provider) => (
            <article key={provider.id}>
              <h3>{provider.label}</h3>
              {provider.availability === "implemented" &&
              provider.configured ? (
                <>
                  <p>Secure read-only account connection</p>
                  <FinancialManager
                    provider={provider}
                    entitled={financial.can_connect_financial_accounts}
                    connections={financialConnections.filter(
                      (c) => c.provider === provider.id,
                    )}
                    accounts={financialAccounts}
                  />
                </>
              ) : provider.availability === "implemented" ? (
                <p className="unavailable">Not configured</p>
              ) : (
                <p className="unavailable">Coming soon</p>
              )}
            </article>
          ))}
        </div>
      </section>
    </>
  );
}
