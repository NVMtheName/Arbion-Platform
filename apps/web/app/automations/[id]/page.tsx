import Link from "next/link";
import { cookies } from "next/headers";
import { redirect } from "next/navigation";
export default async function MandateReview({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  const jar = await cookies();
  const r = await fetch(
    `${process.env.API_BASE_URL ?? "http://localhost:8080"}/api/automations/${id}`,
    { headers: { cookie: jar.toString() }, cache: "no-store" },
  );
  if (r.status === 401) redirect("/login");
  if (!r.ok)
    return (
      <main>
        <Link href="/automations">← Automations</Link>
        <h1>Mandate unavailable</h1>
      </main>
    );
  const { automation: m } = (await r.json()) as {
    automation: Record<string, unknown>;
  };
  const read = (key: string, legacy: string) =>
    String(m[key] ?? m[legacy] ?? "—");
  return (
    <main className="connections-page automation-page">
      <Link href="/automations">← Automations</Link>
      <p className="eyebrow">AUTOMATION MANDATE REVIEW</p>
      <h1>{read("automation_type", "AutomationType")}</h1>
      <section className="review-grid">
        <p>
          <strong>Account</strong>
          {read("financial_account_id", "FinancialAccountID")}
        </p>
        <p>
          <strong>Strategy</strong>
          {read("strategy_identifier", "StrategyIdentifier")}
        </p>
        <p>
          <strong>Capital Bucket</strong>
          {read("capital_bucket_id", "CapitalBucketID")}
        </p>
        <p>
          <strong>Autonomy</strong>
          {read("autonomy_level", "AutonomyLevel")}
        </p>
        <p>
          <strong>Execution</strong>
          {read("execution_mode", "ExecutionMode")}
        </p>
        <p>
          <strong>Status / Version</strong>
          {read("status", "Status")} /{" "}
          {read("current_version", "CurrentVersion")}
        </p>
      </section>
      <p className="security-note">
        Capability checks are conservative: UNKNOWN is never treated as
        supported. A configured or READY Automation Mandate does not itself
        execute trades.
      </p>
    </main>
  );
}
