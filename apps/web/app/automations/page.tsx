import Link from "next/link";
import { cookies } from "next/headers";
import { redirect } from "next/navigation";

import { AppPageHeader } from "../app-page-header";
import { asList } from "./response";
import {
  CapitalBucketForm,
  type CapitalAccountOption,
} from "./capital-bucket-form";

export default async function Automations() {
  const jar = await cookies();
  const headers = { cookie: jar.toString() };
  const base = process.env.API_BASE_URL ?? "http://localhost:8080";
  const [mandatesResponse, bucketsResponse, accountsResponse] =
    await Promise.all([
      fetch(`${base}/api/automations`, { headers, cache: "no-store" }),
      fetch(`${base}/api/capital-buckets`, { headers, cache: "no-store" }),
      fetch(`${base}/api/accounts`, { headers, cache: "no-store" }),
    ]);
  if (mandatesResponse.status === 401) redirect("/login");
  const mandates = asList(
    mandatesResponse.ok
      ? (
          (await mandatesResponse.json()) as {
            automations?: Record<string, unknown>[] | null;
          }
        ).automations
      : [],
  );
  const buckets = asList(
    bucketsResponse.ok
      ? (
          (await bucketsResponse.json()) as {
            capital_buckets?: Record<string, unknown>[] | null;
          }
        ).capital_buckets
      : [],
  );
  const accounts = asList(
    accountsResponse.ok
      ? (
          (await accountsResponse.json()) as {
            accounts?: Record<string, unknown>[] | null;
          }
        ).accounts
      : [],
  ).map(
    (account): CapitalAccountOption => ({
      id: String(account.id ?? account.ID ?? ""),
      display_name: String(
        account.display_name ?? account.DisplayName ?? "Financial account",
      ),
      status: String(account.status ?? account.Status ?? ""),
    }),
  );
  return (
    <main className="connections-page automation-page">
      <AppPageHeader />
      <p className="eyebrow">AUTHORIZATION, NOT EXECUTION</p>
      <h1>Automations</h1>
      <p className="security-note">
        A configured or READY Automation Mandate does not execute trades. Live
        execution is not yet enabled by the platform.
      </p>
      <div className="connection-actions">
        <Link className="button-link" href="/automations/new">
          Create Automation
        </Link>
      </div>
      <section className="provider-list">
        {mandates.length === 0 && (
          <p>
            No automations yet. Create a draft when your connections are ready.
          </p>
        )}
        {mandates.map((m) => (
          <article key={String(m.ID ?? m.id)}>
            <p className="eyebrow">{String(m.Status ?? m.status)}</p>
            <h2>{String(m.AutomationType ?? m.automation_type)}</h2>
            <p>
              {String(m.ExecutionMode ?? m.execution_mode)} · Version{" "}
              {String(m.CurrentVersion ?? m.current_version)}
            </p>
            <Link href={`/automations/${String(m.ID ?? m.id)}`}>
              Review mandate
            </Link>
          </article>
        ))}
      </section>
      <section className="allocation-panel">
        <p className="eyebrow">ARBION CAPITAL ALLOCATION</p>
        <h2>Capital Buckets</h2>
        <p>
          Broker buying power is informational. It is not Arbion trading
          authority and is never allocated automatically.
        </p>
        <CapitalBucketForm accounts={accounts} />
        {buckets.length === 0 && (
          <p>
            No capital buckets yet. Connect an account before allocating
            capital.
          </p>
        )}
        {buckets.map((b) => (
          <article key={String(b.ID ?? b.id)}>
            <strong>{String(b.Name ?? b.name)}</strong>
            <span>
              {String(b.AllocationValue ?? b.allocation_value)}{" "}
              {String(b.Currency ?? b.currency)}
              {Boolean(b.IsReserve ?? b.is_reserve)
                ? " · Reserve / Never Touch"
                : ""}
            </span>
          </article>
        ))}
      </section>
    </main>
  );
}
