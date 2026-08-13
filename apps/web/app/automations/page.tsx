import Link from "next/link";
import { cookies } from "next/headers";
import { redirect } from "next/navigation";

export default async function Automations() {
  const jar = await cookies();
  const headers = { cookie: jar.toString() };
  const base = process.env.API_BASE_URL ?? "http://localhost:8080";
  const [mandatesResponse, bucketsResponse] = await Promise.all([
    fetch(`${base}/api/automations`, { headers, cache: "no-store" }),
    fetch(`${base}/api/capital-buckets`, { headers, cache: "no-store" }),
  ]);
  if (mandatesResponse.status === 401) redirect("/login");
  const mandates = mandatesResponse.ok
    ? (
        (await mandatesResponse.json()) as {
          automations: Record<string, unknown>[];
        }
      ).automations
    : [];
  const buckets = bucketsResponse.ok
    ? (
        (await bucketsResponse.json()) as {
          capital_buckets: Record<string, unknown>[];
        }
      ).capital_buckets
    : [];
  return (
    <main className="connections-page automation-page">
      <Link href="/dashboard">← Dashboard</Link>
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
