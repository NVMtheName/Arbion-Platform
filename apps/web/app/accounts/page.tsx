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
      <p className="eyebrow">PORTFOLIO</p>
      <h1>Your connected portfolio</h1>
      {data.accounts.length === 0 ? (
        <section className="portfolio-empty-state">
          <h2>Connect your first account</h2>
          <p>
            Add Coinbase or Schwab credentials once, then return here for your
            balances and positions.
          </p>
          <Link className="button-link" href="/connections#financial-accounts">
            Open connection hub
          </Link>
        </section>
      ) : (
        <>
          <p className="lede">
            Every connected account in one place. Open an account for its live
            provider balances, positions, and activity.
          </p>
          <section className="provider-list">
            {data.accounts.map((a) => (
              <article key={a.id}>
                <h2>{a.display_name}</h2>
                <p>
                  {a.provider === "coinbase" ? "Coinbase" : "Charles Schwab"}
                </p>
                <p>Status: {a.status}</p>
                <Link href={`/accounts/${a.id}`}>Open account portfolio</Link>
              </article>
            ))}
          </section>
          <Link
            className="connection-text-link"
            href="/connections#financial-accounts"
          >
            Connect another account →
          </Link>
        </>
      )}
    </main>
  );
}
