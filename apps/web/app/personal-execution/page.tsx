import { cookies } from "next/headers";
import { redirect } from "next/navigation";
import { AppPageHeader } from "../app-page-header";
import {
  readOwnerExecutionContext,
  type OwnerExecutionContext,
} from "./context";
import { OwnerExecutionHost } from "./owner-host";
import { CommissioningHost } from "./commissioning-host";
import { readCommissioningContext } from "./commissioning-client";
import type { CommissioningContext } from "./commissioning-contract";

export const dynamic = "force-dynamic";

export default async function PersonalExecutionPage() {
  const jar = await cookies();
  let context: OwnerExecutionContext = { available: false };
  let response: Response | undefined;
  try {
    response = await fetch(
      `${process.env.API_BASE_URL ?? "http://localhost:8080"}/api/personal-execution/context`,
      {
        headers: { cookie: jar.toString(), Accept: "application/json" },
        cache: "no-store",
        redirect: "error",
        signal: AbortSignal.timeout(5_000),
      },
    );
    context = await readOwnerExecutionContext(response);
  } catch {
    // Fail closed without reflecting credentials, upstream bodies or errors.
  }
  if (response?.status === 401) redirect("/login");
  let commissioning: CommissioningContext = { available: false };
  let setup: Response | undefined;
  try {
    setup = await fetch(
      `${process.env.API_BASE_URL ?? "http://localhost:8080"}/api/personal-execution/commissioning/context`,
      {
        headers: { cookie: jar.toString(), Accept: "application/json" },
        cache: "no-store",
        redirect: "error",
        signal: AbortSignal.timeout(5_000),
      },
    );
    commissioning = await readCommissioningContext(setup);
  } catch {
    // No commissioning scope is guessed from the connected account or URL.
  }
  if (setup?.status === 401) redirect("/login");
  return (
    <main className="app-shell">
      <AppPageHeader showConnectionHealth={false} />
      <h1>Personal Coinbase execution</h1>
      <CommissioningHost context={commissioning} />
      <OwnerExecutionHost context={context} />
    </main>
  );
}
