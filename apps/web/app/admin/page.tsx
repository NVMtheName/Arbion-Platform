import Link from "next/link";
import { cookies } from "next/headers";
import { redirect } from "next/navigation";

import { AppPageHeader } from "../app-page-header";
import {
  PlatformCircuitBreakerControls,
  type PlatformCircuitBreaker,
} from "./platform-circuit-breaker-controls";
import {
  PlatformOperationsReadiness,
  type PlatformOperationsOverview,
} from "./platform-operations-readiness";
type User = { id: string; email: string; role: string; entitlement: string };
export default async function Admin() {
  const c = await cookies();
  const base = process.env.API_BASE_URL ?? "http://localhost:8080";
  const me = await fetch(`${base}/api/admin/me`, {
    headers: { cookie: c.toString() },
    cache: "no-store",
  });
  if (!me.ok) redirect("/dashboard");
  const { user } = (await me.json()) as {
    user: { role: string; entitlement: string };
  };
  const response = await fetch(`${base}/api/admin/users`, {
    headers: { cookie: c.toString() },
    cache: "no-store",
  });
  if (!response.ok) redirect("/dashboard");
  const { users } = (await response.json()) as { users: User[] };
  let platformBreaker: PlatformCircuitBreaker | null | undefined;
  let platformOperations: PlatformOperationsOverview | undefined;
  if (user.role === "superadmin") {
    const [platformResponse, operationsResponse] = await Promise.all([
      fetch(`${base}/api/admin/risk/circuit-breaker`, {
        headers: { cookie: c.toString() },
        cache: "no-store",
      }),
      fetch(base + "/api/admin/operations/readiness", {
        headers: { cookie: c.toString() },
        cache: "no-store",
      }),
    ]);
    platformBreaker = platformResponse.ok
      ? ((
          (await platformResponse.json()) as {
            circuit_breaker?: PlatformCircuitBreaker | null;
          }
        ).circuit_breaker ?? null)
      : undefined;
    platformOperations = operationsResponse.ok
      ? (
          (await operationsResponse.json()) as {
            operations: PlatformOperationsOverview;
          }
        ).operations
      : undefined;
  }
  return (
    <main className="dashboard">
      <AppPageHeader />
      <p className="eyebrow">ARBION ADMIN</p>
      <h1>Arbion Admin</h1>
      <p>System role: {user.role}</p>
      <p>entitlement: {user.entitlement}</p>
      {user.role === "superadmin" &&
        (platformBreaker !== undefined ? (
          <PlatformCircuitBreakerControls breaker={platformBreaker} />
        ) : (
          <section className="content-card" role="status">
            <h2>Platform safety control unavailable</h2>
            <p>
              Arbion could not verify the current platform-stop state, so this
              page will not present a potentially stale engage or release
              action.
            </p>
          </section>
        ))}
      {user.role === "superadmin" &&
        (platformOperations ? (
          <PlatformOperationsReadiness operations={platformOperations} />
        ) : (
          <section className="content-card" role="status">
            <h2>Production operations unavailable</h2>
            <p>
              Arbion could not verify the aggregate production snapshot. Treat
              the control plane as requiring review until this evidence loads.
            </p>
          </section>
        ))}
      <h2>Users</h2>
      <div className="admin-table">
        {users.map((u) => (
          <Link key={u.id} href={`/admin/users/${u.id}`}>
            <span>{u.email}</span>
            <span>{u.role}</span>
            <span>{u.entitlement}</span>
          </Link>
        ))}
      </div>
    </main>
  );
}
