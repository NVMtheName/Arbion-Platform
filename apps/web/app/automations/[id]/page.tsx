import { cookies } from "next/headers";
import { redirect } from "next/navigation";

import { AppPageHeader } from "../../app-page-header";
import { MandateControls } from "../mandate-controls";
import { StrategyAutonomyControls } from "../strategy-autonomy-controls";
import { PaperOptionsSimulationAttestationControls } from "../paper-options-simulation-attestation-controls";
import {
  StrategyEvaluationControls,
  type StrategyParameters,
} from "../strategy-evaluation-controls";
import {
  StrategyScheduleControls,
  type StrategyScheduleConditions,
  type StrategyScheduleStatus,
} from "../strategy-schedule-controls";
import { StrategyLifecycleControls } from "../strategy-lifecycle-controls";
import { StrategyInstanceControls } from "../strategy-instance-controls";
import {
  PaperPortfolioSummary,
  type PaperPortfolio,
} from "../paper-portfolio-summary";
import {
  AutomationCircuitBreakerControls,
  type AutomationCircuitBreaker,
} from "../automation-circuit-breaker-controls";
import {
  AIShadowEvaluationControls,
  type AIShadowParameters,
} from "../ai-shadow-evaluation-controls";
import { AIDecisionJournal } from "../ai-decision-journal";
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
        <AppPageHeader backHref="/automations" backLabel="Automations" />
        <h1>Mandate unavailable</h1>
      </main>
    );
  const automationResponse = (await r.json()) as {
    automation: Record<string, unknown>;
    email_delivery_available?: boolean;
  };
  const m = automationResponse.automation;
  const [instancesResponse, breakerResponse, accountsResponse] =
    await Promise.all([
      fetch(`${api}/api/strategy-instances`, {
        headers: { cookie: jar.toString() },
        cache: "no-store",
      }),
      fetch(`${api}/api/automations/${id}/circuit-breaker`, {
        headers: { cookie: jar.toString() },
        cache: "no-store",
      }),
      fetch(`${api}/api/accounts`, {
        headers: { cookie: jar.toString() },
        cache: "no-store",
      }),
    ]);
  const instances = instancesResponse.ok
    ? ((
        (await instancesResponse.json()) as {
          strategy_instances: Record<string, unknown>[];
        }
      ).strategy_instances ?? [])
    : [];
  const breaker = breakerResponse.ok
    ? (
        (await breakerResponse.json()) as {
          circuit_breaker?: AutomationCircuitBreaker | null;
        }
      ).circuit_breaker
    : null;
  const accounts = accountsResponse.ok
    ? ((
        (await accountsResponse.json()) as {
          accounts?: Record<string, unknown>[];
        }
      ).accounts ?? [])
    : [];
  const currentVersion = Number(m.current_version ?? m.CurrentVersion ?? 0);
  const mandateInstances = instances.filter(
    (item) =>
      String(item.AutomationMandateID ?? item.automation_mandate_id) === id,
  );
  const instance =
    mandateInstances.find((item) => {
      const status = String(item.Status ?? item.status ?? "");
      return status === "ACTIVE" || status === "PAUSED";
    }) ??
    mandateInstances.find(
      (item) =>
        Number(item.MandateVersion ?? item.mandate_version ?? 0) ===
        currentVersion,
    );
  const instanceID = instance ? String(instance.ID ?? instance.id ?? "") : "";
  const hasActiveInstance = mandateInstances.some((item) => {
    const status = String(item.Status ?? item.status ?? "");
    return status === "ACTIVE" || status === "PAUSED";
  });
  const [
    history,
    decisions,
    executions,
    outcomes,
    scheduleResponse,
    portfolioResponse,
  ] = instanceID
    ? await Promise.all(
        [
          "history",
          "decisions",
          "executions",
          "shadow-outcomes",
          "schedule",
          "paper-portfolio",
        ].map(async (suffix) => {
          const response = await fetch(
            `${api}/api/strategy-instances/${instanceID}/${suffix}`,
            { headers: { cookie: jar.toString() }, cache: "no-store" },
          );
          return response.ok
            ? ((await response.json()) as Record<string, unknown>)
            : {};
        }),
      )
    : [{}, {}, {}, {}, {}, {}];
  const paperPortfolio = portfolioResponse.paper_portfolio as
    | PaperPortfolio
    | undefined;
  const openPaperOptions = (paperPortfolio?.positions ?? []).filter(
    (position) => position.is_open && position.instrument === "OPTION",
  );
  const openPaperOption =
    openPaperOptions.length === 1 ? openPaperOptions[0] : undefined;
  const read = (key: string, legacy: string) =>
    String(m[key] ?? m[legacy] ?? "—");
  const flag = (key: string, legacy: string) =>
    Boolean(m[key] ?? m[legacy] ?? false);
  const count = (value: unknown) => (Array.isArray(value) ? value.length : 0);
  const automationType = read("automation_type", "AutomationType");
  const financialAccountID = read("financial_account_id", "FinancialAccountID");
  const financialAccount = accounts.find(
    (item) => String(item.id ?? item.ID ?? "") === financialAccountID,
  );
  const financialProvider = String(
    financialAccount?.provider ?? financialAccount?.Provider ?? "",
  );
  const allowedUniverse = (m.allowed_universe ?? m.AllowedUniverse ?? {}) as {
    symbols?: string[];
    Symbols?: string[];
  };
  return (
    <main className="connections-page automation-page">
      <AppPageHeader backHref="/automations" backLabel="Automations" />
      <p className="eyebrow">AUTOMATION MANDATE REVIEW</p>
      <h1>
        {automationType === "AI_AUTONOMOUS"
          ? "AI Shadow Engine"
          : automationType}
      </h1>
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
        Capability checks are conservative: UNKNOWN is not treated as broker
        support. A separately confirmed attestation may permit PAPER-only
        simulation, but never SHADOW, LIVE, or broker execution.
      </p>
      <AutomationCircuitBreakerControls automationId={id} breaker={breaker} />
      <MandateControls
        automationId={id}
        currentVersion={currentVersion}
        status={read("status", "Status")}
        automationType={read("automation_type", "AutomationType")}
        executionMode={read("execution_mode", "ExecutionMode")}
        strategyIdentifier={read("strategy_identifier", "StrategyIdentifier")}
        instanceExists={Boolean(instance)}
      />
      {automationType === "STRATEGY" && (
        <StrategyAutonomyControls
          automationId={id}
          currentVersion={currentVersion}
          automationType={read("automation_type", "AutomationType")}
          autonomyLevel={read("autonomy_level", "AutonomyLevel")}
          executionMode={read("execution_mode", "ExecutionMode")}
          hasActiveInstance={hasActiveInstance}
        />
      )}
      {automationType === "STRATEGY" && (
        <PaperOptionsSimulationAttestationControls
          automationId={id}
          currentVersion={currentVersion}
          automationType={read("automation_type", "AutomationType")}
          executionMode={read("execution_mode", "ExecutionMode")}
          optionsAllowed={flag("options_allowed", "OptionsAllowed")}
          capabilityUnverified={flag(
            "capability_unverified",
            "CapabilityUnverified",
          )}
          attested={flag(
            "paper_options_simulation_attested",
            "PaperOptionsSimulationAttested",
          )}
          hasActiveInstance={hasActiveInstance}
        />
      )}
      {automationType === "STRATEGY" && (
        <>
          <StrategyEvaluationControls
            automationId={id}
            currentVersion={currentVersion}
            status={read("status", "Status")}
            executionMode={read("execution_mode", "ExecutionMode")}
            strategyIdentifier={read(
              "strategy_identifier",
              "StrategyIdentifier",
            )}
            instanceId={instanceID}
            instanceStatus={String(instance?.Status ?? instance?.status ?? "")}
            strategyParameters={
              (m.strategy_parameters ??
                m.StrategyParameters ??
                {}) as StrategyParameters
            }
          />
          <StrategyScheduleControls
            automationId={id}
            currentVersion={currentVersion}
            automationType={read("automation_type", "AutomationType")}
            autonomyLevel={read("autonomy_level", "AutonomyLevel")}
            executionMode={read("execution_mode", "ExecutionMode")}
            instanceId={instanceID}
            schedulerEnabled={Boolean(scheduleResponse.scheduler_enabled)}
            emailDeliveryAvailable={Boolean(
              scheduleResponse.email_delivery_available ??
                automationResponse.email_delivery_available,
            )}
            financialProvider={financialProvider}
            conditions={
              (m.schedule_conditions ??
                m.ScheduleConditions ??
                {}) as StrategyScheduleConditions
            }
            runtime={
              scheduleResponse.schedule as unknown as
                | StrategyScheduleStatus
                | undefined
            }
          />
          {instance && (
            <>
              <StrategyLifecycleControls
                instanceId={instanceID}
                currentState={String(
                  instance.CurrentState ?? instance.current_state ?? "",
                )}
                stateVersion={Number(
                  instance.StateVersion ?? instance.state_version ?? 0,
                )}
                status={String(instance.Status ?? instance.status ?? "")}
                executionMode={read("execution_mode", "ExecutionMode")}
                strategyIdentifier={read(
                  "strategy_identifier",
                  "StrategyIdentifier",
                )}
                openPosition={openPaperOption}
              />
              <StrategyInstanceControls
                instanceId={instanceID}
                status={String(instance.Status ?? instance.status ?? "")}
                stateVersion={Number(
                  instance.StateVersion ?? instance.state_version ?? 0,
                )}
              />
            </>
          )}
        </>
      )}
      {automationType === "AI_AUTONOMOUS" && (
        <>
          <AIShadowEvaluationControls
            status={read("status", "Status")}
            instanceId={instanceID}
            instanceStatus={String(instance?.Status ?? instance?.status ?? "")}
            parameters={
              (m.strategy_parameters ??
                m.StrategyParameters ??
                {}) as AIShadowParameters
            }
            symbols={allowedUniverse.symbols ?? allowedUniverse.Symbols ?? []}
          />
          <StrategyScheduleControls
            automationId={id}
            currentVersion={currentVersion}
            automationType={automationType}
            autonomyLevel={read("autonomy_level", "AutonomyLevel")}
            executionMode={read("execution_mode", "ExecutionMode")}
            instanceId={instanceID}
            schedulerEnabled={Boolean(scheduleResponse.scheduler_enabled)}
            emailDeliveryAvailable={Boolean(
              scheduleResponse.email_delivery_available ??
                automationResponse.email_delivery_available,
            )}
            financialProvider={financialProvider}
            conditions={
              (m.schedule_conditions ??
                m.ScheduleConditions ??
                {}) as StrategyScheduleConditions
            }
            runtime={
              scheduleResponse.schedule as unknown as
                | StrategyScheduleStatus
                | undefined
            }
          />
          {instance && (
            <>
              <StrategyInstanceControls
                instanceId={instanceID}
                status={String(instance.Status ?? instance.status ?? "")}
                stateVersion={Number(
                  instance.StateVersion ?? instance.state_version ?? 0,
                )}
              />
              <AIDecisionJournal
                decisions={
                  Array.isArray(decisions.decisions)
                    ? (decisions.decisions as Record<string, unknown>[])
                    : []
                }
                outcomes={
                  Array.isArray(outcomes.outcomes)
                    ? (outcomes.outcomes as Record<string, unknown>[])
                    : []
                }
              />
            </>
          )}
        </>
      )}
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
      </section>
      {instance && (
        <PaperPortfolioSummary
          portfolio={paperPortfolio}
          executionMode={read("execution_mode", "ExecutionMode")}
        />
      )}
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
            {count(history.transitions)} state transition(s) ·{" "}
            {count(decisions.decisions)} decision(s) ·{" "}
            {count(executions.executions)} execution record(s)
          </p>
          <p>
            Evaluations are manual unless this exact mandate version has an
            enabled guarded schedule.
          </p>
        </section>
      )}
    </main>
  );
}
