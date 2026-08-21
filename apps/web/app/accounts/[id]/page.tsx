import { cookies } from "next/headers";
import { notFound, redirect } from "next/navigation";

import { AppPageHeader } from "../../app-page-header";
type Money = { amount: string; currency: string };
type Account = {
  id: string;
  provider: string;
  display_name: string;
  account_type: string;
  status: string;
};
type Balances = {
  account_value?: Money;
  cash?: Money;
  available_cash?: Money;
  buying_power?: Money;
};
type Position = {
  symbol: string;
  instrument_type: string;
  quantity: string;
  direction: string;
  market_value?: Money;
  cost_basis?: Money;
};
const show = (m?: Money) =>
  m
    ? new Intl.NumberFormat("en-US", {
        style: "currency",
        currency: m.currency,
      }).format(Number(m.amount))
    : "Unavailable";
export default async function AccountPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  const jar = await cookies();
  const base = process.env.API_BASE_URL ?? "http://localhost:8080";
  const headers = { cookie: jar.toString() };
  const [ar, br, pr] = await Promise.all([
    fetch(`${base}/api/accounts/${id}`, { headers, cache: "no-store" }),
    fetch(`${base}/api/accounts/${id}/balances`, {
      headers,
      cache: "no-store",
    }),
    fetch(`${base}/api/accounts/${id}/positions`, {
      headers,
      cache: "no-store",
    }),
  ]);
  if (ar.status === 401) redirect("/login");
  if (ar.status === 404) notFound();
  if (!ar.ok) throw new Error("Unable to load account");
  const account = ((await ar.json()) as { account: Account }).account;
  const balances = br.ok
    ? ((await br.json()) as { balances: Balances }).balances
    : {};
  const positions = pr.ok
    ? ((await pr.json()) as { positions: Position[] }).positions
    : [];
  const providerLabel =
    account.provider === "coinbase" ? "COINBASE" : "CHARLES SCHWAB";
  return (
    <main className="connections-page">
      <AppPageHeader backHref="/accounts" backLabel="Accounts" />
      <p className="eyebrow">{providerLabel} · CONNECTED ACCOUNT</p>
      <h1>{account.display_name}</h1>
      <section className="dashboard-grid">
        <article>
          <h2>Account Value</h2>
          <p>{show(balances.account_value)}</p>
        </article>
        <article>
          <h2>Cash</h2>
          <p>{show(balances.cash)}</p>
        </article>
        <article>
          <h2>Buying Power</h2>
          <p>{show(balances.buying_power)}</p>
        </article>
      </section>
      <h2>Positions</h2>
      {positions.length === 0 ? (
        <p>No positions reported.</p>
      ) : (
        <section className="provider-list">
          {positions.map((p, i) => (
            <article key={`${p.symbol}-${i}`}>
              <h3>{p.symbol}</h3>
              <p>
                {p.quantity} · {p.direction} · {p.instrument_type}
              </p>
              <p>Market value: {show(p.market_value)}</p>
              {p.cost_basis && <p>Average basis: {show(p.cost_basis)}</p>}
            </article>
          ))}
        </section>
      )}
      <p className="security-note">
        Connected-account data is informational. This page cannot place orders
        or move assets.
      </p>
    </main>
  );
}
