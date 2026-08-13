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
  const api = process.env.API_BASE_URL ?? "http://localhost:8080";
  const r = await fetch(`${api}/api/automations/${id}`, {
    headers: { cookie: jar.toString() },
    cache: "no-store",
  });
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
  const instancesResponse = await fetch(`${api}/api/strategy-instances`, {
    headers: { cookie: jar.toString() },
    cache: "no-store",
  });
  const instances = instancesResponse.ok
    ? ((
        (await instancesResponse.json()) as {
          strategy_instances: Record<string, unknown>[];
        }
      ).strategy_instances ?? [])
    : [];
  const instance = instances.find(
    (item) =>
      String(item.AutomationMandateID ?? item.automation_mandate_id) === id,
  );
  const instanceID = instance ? String(instance.ID ?? instance.id ?? "") : "";
  const [history, decisions, executions] = instanceID
    ? await Promise.all(
        ["history", "decisions", "executions"].map(async (suffix) => {
          const response = await fetch(
            `${api}/api/strategy-instances/${instanceID}/${suffix}`,
            { headers: { cookie: jar.toString() }, cache: "no-store" },
          );
          return response.ok
            ? ((await response.json()) as Record<string, unknown[]>)
            : {};
        }),
      )
    : [{}, {}, {}];
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
      <section className="review-grid" aria-label="Non-live strategy">
        <div>
          <p className="eyebrow">STRATEGY INSTANCE</p>
          <h2>
            {instance
              ? String(instance.CurrentState ?? instance.current_state)
              : "NOT INITIALIZED"}
          </h2>
          <p>
            <strong>Mode</strong>
            {instance
              ? String(instance.ExecutionMode ?? instance.execution_mode)
              : read("execution_mode", "ExecutionMode")}
          </p>
          <p>
            <strong>State version</strong>
            {instance
              ? String(instance.StateVersion ?? instance.state_version)
              : "—"}
          </p>
        </div>
        <div>
          <p className="eyebrow">PAPER PORTFOLIO</p>
          <h2>SIMULATED</h2>
          <p>Paper balances and positions are kept separate from Schwab.</p>
          <p>Paper execution is simulation, not a broker fill.</p>
        </div>
      </section>
      {read("execution_mode", "ExecutionMode") === "SHADOW" && (
        <p className="security-note">
          <strong>SHADOW — NO REAL ORDER WAS SENT.</strong> Decisions record
          only what Arbion would have attempted.
        </p>
      )}
      {instance && (
        <section>
          <p className="eyebrow">STRATEGY HISTORY</p>
          <h2>Durable non-live activity</h2>
          <p>
            {(history.transitions ?? []).length} state transition(s) ·{" "}
            {(decisions.decisions ?? []).length} decision(s) ·{" "}
            {(executions.executions ?? []).length} execution record(s)
          </p>
          <p>
            Evaluation is manual-only. No background strategy worker is running.
          </p>
        </section>
      )}
    </main>
  );
}
