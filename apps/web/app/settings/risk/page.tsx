import { cookies } from "next/headers";
import { redirect } from "next/navigation";

import { AppPageHeader } from "../../app-page-header";
import {
  UserCircuitBreakerControls,
  type UserCircuitBreaker,
} from "./user-circuit-breaker-controls";

export default async function RiskSafetyPage() {
  const jar = await cookies();
  const base = process.env.API_BASE_URL ?? "http://localhost:8080";
  const response = await fetch(`${base}/api/risk/circuit-breaker`, {
    headers: { cookie: jar.toString() },
    cache: "no-store",
  });
  if (response.status === 401) redirect("/login");
  const breaker = response.ok
    ? ((
        (await response.json()) as {
          circuit_breaker?: UserCircuitBreaker | null;
        }
      ).circuit_breaker ?? null)
    : undefined;

  return (
    <main className="dashboard-shell command-content-continuity">
      <AppPageHeader />
      <section className="hero-panel">
        <p className="eyebrow">Safety controls</p>
        <h1>Risk / Control status</h1>
        <p>
          These controls prevent authorization of new automated actions. They
          never close positions or submit a trade.
        </p>
      </section>
      {breaker !== undefined ? (
        <UserCircuitBreakerControls breaker={breaker} />
      ) : (
        <section className="content-card" role="status">
          <h2>Owner-wide safety control unavailable</h2>
          <p>
            Arbion could not verify the current owner-stop state, so this page
            will not present a potentially stale engage or release action.
          </p>
        </section>
      )}
      <section className="content-card">
        <h2>Account and mandate controls</h2>
        <p>
          Account-scoped breakers are available on each connected account, and
          automation-scoped breakers remain on each automation. All applicable
          stops are evaluated before capital and strategy rules.
        </p>
        <p>These controls do not close positions or grant trading authority.</p>
      </section>
    </main>
  );
}
