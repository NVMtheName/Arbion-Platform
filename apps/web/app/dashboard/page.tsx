import { cookies } from "next/headers";
import { redirect } from "next/navigation";
import { ArbionBrand } from "../brand";
import { LogoutButton } from "./logout-button";
import { InsightPanel } from "./insight-panel";
import Link from "next/link";
type User = {
  email: string;
  display_name: string;
  entitlement: string;
  role: string;
};
type Preference = { connection_id: string; model_id: string };
export default async function Dashboard() {
  const cookieStore = await cookies();
  const response = await fetch(
    `${process.env.API_BASE_URL ?? "http://localhost:8080"}/api/auth/me`,
    { headers: { cookie: cookieStore.toString() }, cache: "no-store" },
  );
  if (!response.ok) redirect("/login");
  const { user } = (await response.json()) as { user: User };
  const connectionsResponse = await fetch(
    `${process.env.API_BASE_URL ?? "http://localhost:8080"}/api/connections/ai`,
    { headers: { cookie: cookieStore.toString() }, cache: "no-store" },
  );
  const connectionCount = connectionsResponse.ok
    ? ((await connectionsResponse.json()) as { connections: unknown[] })
        .connections.length
    : 0;
  const accountsResponse = await fetch(
    `${process.env.API_BASE_URL ?? "http://localhost:8080"}/api/accounts`,
    { headers: { cookie: cookieStore.toString() }, cache: "no-store" },
  );
  const accountCount = accountsResponse.ok
    ? ((await accountsResponse.json()) as { accounts: unknown[] }).accounts
        .length
    : 0;
  const preferenceResponse = await fetch(
    `${process.env.API_BASE_URL ?? "http://localhost:8080"}/api/settings/neural-engine`,
    { headers: { cookie: cookieStore.toString() }, cache: "no-store" },
  );
  const preference = preferenceResponse.ok
    ? ((await preferenceResponse.json()) as { preference: Preference | null })
        .preference
    : null;
  return (
    <main className="dashboard">
      <header>
        <ArbionBrand className="dashboard-brand" href="/dashboard" priority />
        <div className="account-actions">
          <Link href="/settings/risk">Risk</Link>
          <Link href="/settings/security">Security</Link>
          {(user.role === "admin" || user.role === "superadmin") && (
            <Link href="/admin">Admin</Link>
          )}
          <LogoutButton />
        </div>
      </header>
      <h1>Welcome, {user.display_name || user.email}</h1>
      <p className="lede">Your secure command center is ready.</p>
      <p className="plan">Plan: {user.entitlement.replaceAll("_", " ")}</p>
      <section className="dashboard-grid">
        <article>
          <span className="icon">✦</span>
          <h2>Neural Engine</h2>
          <p>
            {connectionCount === 0
              ? "Not configured"
              : `${connectionCount} provider${connectionCount === 1 ? "" : "s"} configured`}
          </p>
          <Link href="/settings/connections">Manage providers</Link>
        </article>
        <article>
          <span className="icon">◎</span>
          <h2>Financial Accounts</h2>
          <p>
            {accountCount === 0
              ? "No accounts connected"
              : `${accountCount} connected account${accountCount === 1 ? "" : "s"}`}
          </p>
          <Link href="/accounts">View accounts</Link>
        </article>
        <article>
          <span className="icon">◈</span>
          <h2>Automation</h2>
          <p>Configuration only · execution disabled</p>
          <Link href="/automations">Build automations</Link>
        </article>
        <article>
          <span className="icon">⌁</span>
          <h2>Decision Journal</h2>
          <p>Read-only PAPER and SHADOW decision evidence</p>
          <Link href="/activity">Review activity</Link>
        </article>
        <article>
          <span className="icon">◌</span>
          <h2>Market Command Center</h2>
          <p>Source-stamped equities, crypto, and insider intelligence</p>
          <Link href="/markets">View market intelligence</Link>
        </article>
      </section>
      <InsightPanel
        configured={preference !== null}
        model={preference?.model_id}
      />
    </main>
  );
}
