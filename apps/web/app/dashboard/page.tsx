import { cookies } from "next/headers";
import { redirect } from "next/navigation";
import { LogoutButton } from "./logout-button";
import Link from "next/link";
type User = { email: string; display_name: string; entitlement: string };
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
  return (
    <main className="dashboard">
      <header>
        <p className="eyebrow">ARBION</p>
        <div className="account-actions">
          <span>Account</span>
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
          <p>Disabled</p>
        </article>
      </section>
    </main>
  );
}
