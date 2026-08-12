import { cookies } from "next/headers";
import { redirect } from "next/navigation";
import { LogoutButton } from "./logout-button";
type User = { email: string; display_name: string };
export default async function Dashboard() {
  const cookieStore = await cookies();
  const response = await fetch(
    `${process.env.API_BASE_URL ?? "http://localhost:8080"}/api/auth/me`,
    { headers: { cookie: cookieStore.toString() }, cache: "no-store" },
  );
  if (!response.ok) redirect("/login");
  const { user } = (await response.json()) as { user: User };
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
      <section className="dashboard-grid">
        <article>
          <span className="icon">✦</span>
          <h2>Neural Engine</h2>
          <p>Not configured</p>
        </article>
        <article>
          <span className="icon">◎</span>
          <h2>Trading Accounts</h2>
          <p>No accounts connected</p>
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
