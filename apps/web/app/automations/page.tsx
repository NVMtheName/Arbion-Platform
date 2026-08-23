import Link from "next/link";
import { cookies } from "next/headers";
import { redirect } from "next/navigation";

import { AppPageHeader } from "../app-page-header";
import { asList } from "./response";

export default async function Automations() {
  const jar = await cookies();
  const headers = { cookie: jar.toString() };
  const base = process.env.API_BASE_URL ?? "http://localhost:8080";
  const mandatesResponse = await fetch(`${base}/api/automations`, {
    headers,
    cache: "no-store",
  });
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
  return (
    <main className="connections-page automation-page">
      <AppPageHeader />
      <p className="eyebrow">YOUR PLAYBOOKS</p>
      <h1>Strategies</h1>
      <p className="lede">
        Each strategy brings together one account, one AI model, a trading
        playbook, and a defined budget. Drafts never place trades.
      </p>
      <div className="connection-actions">
        <Link className="button-link" href="/automations/new">
          Create Strategy
        </Link>
      </div>
      <section className="provider-list">
        {mandates.length === 0 && (
          <p>
            No strategies yet. Create a draft when your connections are ready.
          </p>
        )}
        {mandates.map((m) => (
          <article key={String(m.ID ?? m.id)}>
            <p className="eyebrow">{String(m.Status ?? m.status)}</p>
            <h2>
              {String(
                m.StrategyIdentifier ??
                  m.strategy_identifier ??
                  "Trading strategy",
              )}
            </h2>
            <p>
              {String(m.ExecutionMode ?? m.execution_mode)} ·{" "}
              {String(m.AutonomyLevel ?? m.autonomy_level)}
            </p>
            <Link href={`/automations/${String(m.ID ?? m.id)}`}>
              Review strategy
            </Link>
          </article>
        ))}
      </section>
    </main>
  );
}
