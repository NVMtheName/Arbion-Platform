import Link from "next/link";
import { cookies } from "next/headers";
import { redirect } from "next/navigation";
import { AppPageHeader } from "../app-page-header";
import type { FinancialAccount } from "../settings/connections/page";
export default async function Accounts() {
  const jar = await cookies();
  const r = await fetch(
    `${process.env.API_BASE_URL ?? "http://localhost:8080"}/api/accounts`,
    { headers: { cookie: jar.toString() }, cache: "no-store" },
  );
  if (r.status === 401) redirect("/login");
  const data = r.ok
    ? ((await r.json()) as { accounts: FinancialAccount[] })
    : { accounts: [] };
  return (
    <main className="connections-page">
      <AppPageHeader />
      <p className="eyebrow">READ-ONLY</p>
      <h1>Financial Accounts</h1>
      {data.accounts.length === 0 ? (
        <p>No financial accounts connected.</p>
      ) : (
        <section className="provider-list">
          {data.accounts.map((a) => (
            <article key={a.id}>
              <h2>{a.display_name}</h2>
              <p>Status: {a.status}</p>
              <Link href={`/accounts/${a.id}`}>
                View balances and positions
              </Link>
            </article>
          ))}
        </section>
      )}
    </main>
  );
}
