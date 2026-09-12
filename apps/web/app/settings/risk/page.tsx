import { cookies } from "next/headers";
import { redirect } from "next/navigation";

import { RiskSettingsWorkspace } from "./risk-settings-workspace";
import type { UserCircuitBreaker } from "./user-circuit-breaker-controls";

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

  return <RiskSettingsWorkspace breaker={breaker} />;
}
