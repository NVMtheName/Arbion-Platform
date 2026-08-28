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
import { AIShadowParameterControls } from "../ai-shadow-parameter-controls";
import { AIDecisionJournal } from "../ai-decision-journal";
import { AIShadowScorecard } from "../ai-shadow-scorecard";
import { MandateIdentitySummary } from "../mandate-identity-summary";
import { AutonomyGuardrailSummary } from "../autonomy-guardrail-summary";
import { CapitalBucketAllocationControls } from "../capital-bucket-allocation-controls";
import { AutonomyReadinessControlPlane } from "../autonomy-readiness-control-plane";
import { AutonomyEvidenceReport } from "../autonomy-evidence-report";
import { DecisionReplayLab } from "../decision-replay-lab";
import { StrategySimulationWorkbench } from "../strategy-simulation-workbench";
import {
  ScheduleRunHistory,
  type ScheduleRunRecord,
} from "../schedule-run-history";

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
  const read = (key: string, legacy: string) =>
    String(m[key] ?? m[legacy] ?? "—");
  const financialAccountID = read("financial_account_id", "FinancialAccountID");
  const [
    instancesResponse,
    breakerResponse,
    accountsResponse,
    bucketsResponse,
    financialConnectionsResponse,
    aiConnectionsResponse,
    versionsResponse,
  ] = await Promise.all([
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
    fetch(`${api}/api/capital-buckets`, {
      headers: { cookie: jar.toString() },
      cache: "no-store",
    }),
    fetch(`${api}/api/connections/financial`, {
      headers: { cookie: jar.toString() },
      cache: "no-store",
    }),
    fetch(`${api}/api/connections/ai`, {
      headers: { cookie: jar.toString() },
      cache: "no-store",
    }),
    fetch(`${api}/api/automations/${id}/versions`, {
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
  const capitalBuckets = bucketsResponse.ok
    ? ((
        (await bucketsResponse.json()) as {
          capital_buckets?: Record<string, unknown>[];
        }
      ).capital_buckets ?? [])
    : [];
  const financialConnections = financialConnectionsResponse.ok
    ? ((
        (await financialConnectionsResponse.json()) as {
          connections?: Record<string, unknown>[];
        }
      ).connections ?? [])
    : [];
  const aiConnections = aiConnectionsResponse.ok
    ? ((
        (await aiConnectionsResponse.json()) as {
          connections?: Record<string, unknown>[];
        }
      ).connections ?? [])
    : [];
  const mandateVersions = versionsResponse.ok
    ? ((
        (await versionsResponse.json()) as {
          versions?: Record<string, unknown>[];
        }
      ).versions ?? [])
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
    scorecardResponse,
    scheduleResponse,
    scheduleRunsResponse,
    portfolioResponse,
  ] = instanceID
    ? await Promise.all(
        [
          "history",
          "decisions",
          "executions",
          "shadow-outcomes",
          "shadow-scorecard",
          "schedule",
          "schedule-runs?limit=12",
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
    : [{}, {}, {}, {}, {}, {}, {}, {}];
  const paperPortfolio = portfolioResponse.paper_portfolio as
    | PaperPortfolio
    | undefined;
  const openPaperOptions = (paperPortfolio?.positions ?? []).filter(
    (position) => position.is_open && position.instrument === "OPTION",
  );
  const openPaperOption =
    openPaperOptions.length === 1 ? openPaperOptions[0] : undefined;
  const flag = (key: string, legacy: string) =>
    Boolean(m[key] ?? m[legacy] ?? false);
  const count = (value: unknown) => (Array.isArray(value) ? value.length : 0);
  const automationType = read("automation_type", "AutomationType");
  const financialAccount = accounts.find(
    (item) => String(item.id ?? item.ID ?? "") === financialAccountID,
  );
  const financialProvider = String(
    financialAccount?.provider ?? financialAccount?.Provider ?? "",
  );
  const financialConnectionID = String(
    financialAccount?.provider_connection_id ??
      financialAccount?.ProviderConnectionID ??
      "",
  );
  const financialConnection = financialConnections.find(
    (item) => String(item.id ?? item.ID ?? "") === financialConnectionID,
  );
  const aiProviderConnectionID = read(
    "ai_provider_connection_id",
    "AIProviderConnectionID",
  );
  const aiConnection = aiConnections.find(
    (item) => String(item.id ?? item.ID ?? "") === aiProviderConnectionID,
  );
  const capitalBucketID = read("capital_bucket_id", "CapitalBucketID");
  const capitalBucket = capitalBuckets.find(
    (item) => String(item.id ?? item.ID ?? "") === capitalBucketID,
  );
  const allowedUniverse = (m.allowed_universe ?? m.AllowedUniverse ?? {}) as {
    symbols?: string[];
    Symbols?: string[];
  };
  const reconciliationResponse =
    financialAccountID && financialAccountID !== "—"
      ? await fetch(
          `${api}/api/accounts/${encodeURIComponent(financialAccountID)}/reconciliations/latest`,
          { headers: { cookie: jar.toString() }, cache: "no-store" },
        )
      : null;
  const reconciliation = reconciliationResponse?.ok
    ? (
        (await reconciliationResponse.json()) as {
          reconciliation?: Record<string, unknown>;
        }
      ).reconciliation
    : undefined;
  const scorecard = scorecardResponse.scorecard as
    | Record<string, unknown>
    | undefined;
  const evidenceGate = (scorecard?.evidence_gate ?? scorecard?.EvidenceGate) as
    | Record<string, unknown>
    | undefined;
  const observedAt = new Date().toISOString();
  return (
    <main className="connections-page automation-page">
      <AppPageHeader backHref="/automations" backLabel="Automations" />
      <p className="eyebrow">AUTOMATION MANDATE REVIEW</p>
      <h1>
        {automationType === "AI_AUTONOMOUS"
          ? "AI Shadow Engine"
          : automationType}
      </h1>
      <MandateIdentitySummary
        mandateId={id}
        automationType={automationType}
        financialAccountId={financialAccountID}
        financialAccount={financialAccount}
        capitalBucketId={capitalBucketID}
        capitalBucket={capitalBucket}
        strategyIdentifier={read("strategy_identifier", "StrategyIdentifier")}
        aiModelId={read("ai_model_id", "AIModelID")}
        autonomyLevel={read("autonomy_level", "AutonomyLevel")}
        executionMode={read("execution_mode", "ExecutionMode")}
        status={read("status", "Status")}
        currentVersion={currentVersion}
        strategyInstanceId={instanceID}
      />
      <p className="security-note">
        Capability checks are conservative: UNKNOWN is not treated as broker
        support. A separately confirmed attestation may permit PAPER-only
        simulation, but never SHADOW, LIVE, or broker execution.
      </p>
      {automationType === "AI_AUTONOMOUS" && (
        <AutonomyReadinessControlPlane
          provider={financialProvider}
          modelID={read("ai_model_id", "AIModelID")}
          mandateStatus={read("status", "Status")}
          currentVersion={currentVersion}
          automationType={automationType}
          autonomyLevel={read("autonomy_level", "AutonomyLevel")}
          executionMode={read("execution_mode", "ExecutionMode")}
          financialAccount={financialAccount}
          financialConnection={financialConnection}
          aiConnection={aiConnection}
          capitalBucket={capitalBucket}
          instance={instance}
          schedule={scheduleResponse.schedule as Record<string, unknown>}
          evidenceGate={evidenceGate}
          reconciliation={reconciliation}
          automationBreaker={breaker as unknown as Record<string, unknown>}
          schedulerEnabled={Boolean(scheduleResponse.scheduler_enabled)}
          observedAt={observedAt}
        />
      )}
      {automationType === "AI_AUTONOMOUS" && (
        <AutonomyEvidenceReport
          generatedAt={observedAt}
          mandateId={id}
          currentVersion={currentVersion}
          mandateStatus={read("status", "Status")}
          automationType={automationType}
          autonomyLevel={read("autonomy_level", "AutonomyLevel")}
          executionMode={read("execution_mode", "ExecutionMode")}
          financialAccountId={financialAccountID}
          financialProvider={financialProvider}
          financialAccount={financialAccount}
          financialConnection={financialConnection}
          aiConnection={aiConnection}
          modelId={read("ai_model_id", "AIModelID")}
          capitalBucketId={capitalBucketID}
          capitalBucket={capitalBucket}
          instance={instance}
          schedule={scheduleResponse.schedule as Record<string, unknown>}
          scheduleRuns={scheduleRunsResponse.runs}
          schedulerEnabled={Boolean(scheduleResponse.scheduler_enabled)}
          scorecard={scorecard}
          evidenceGate={evidenceGate}
          reconciliation={reconciliation}
          automationBreaker={breaker as unknown as Record<string, unknown>}
          breakerObserved={breakerResponse.ok}
        />
      )}
      <AutomationCircuitBreakerControls automationId={id} breaker={breaker} />
      {capitalBucket && (
        <CapitalBucketAllocationControls
          bucketId={capitalBucketID}
          financialAccountId={financialAccountID}
          name={String(
            capitalBucket.name ?? capitalBucket.Name ?? "Capital bucket",
          )}
          allocationType={String(
            capitalBucket.allocation_type ??
              capitalBucket.AllocationType ??
              "FIXED_AMOUNT",
          )}
          allocationValue={String(
            capitalBucket.allocation_value ??
              capitalBucket.AllocationValue ??
              "",
          )}
          currency={String(
            capitalBucket.currency ?? capitalBucket.Currency ?? "USD",
          )}
          protectedAmount={String(
            capitalBucket.protected_amount ??
              capitalBucket.ProtectedAmount ??
              "0",
          )}
          allocationLimit={
            (capitalBucket.allocation_limit ??
              capitalBucket.AllocationLimit) as string | undefined
          }
          isReserve={Boolean(
            capitalBucket.is_reserve ?? capitalBucket.IsReserve,
          )}
          status={String(capitalBucket.status ?? capitalBucket.Status ?? "")}
          hasActiveInstance={hasActiveInstance}
        />
      )}
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
            <ScheduleRunHistory
              instanceId={instanceID}
              initialRuns={
                Array.isArray(scheduleRunsResponse.runs)
                  ? (scheduleRunsResponse.runs as ScheduleRunRecord[])
                  : []
              }
              initialCursor={String(scheduleRunsResponse.next_cursor ?? "")}
            />
          )}
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
          <AutonomyGuardrailSummary
            riskPolicy={
              (m.risk_parameters ?? m.Risk ?? {}) as Record<string, unknown>
            }
          />
          <AIShadowParameterControls
            automationId={id}
            currentVersion={currentVersion}
            status={read("status", "Status")}
            hasActiveInstance={hasActiveInstance}
            parameters={
              (m.strategy_parameters ??
                m.StrategyParameters ??
                {}) as AIShadowParameters
            }
          />
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
            <ScheduleRunHistory
              instanceId={instanceID}
              initialRuns={
                Array.isArray(scheduleRunsResponse.runs)
                  ? (scheduleRunsResponse.runs as ScheduleRunRecord[])
                  : []
              }
              initialCursor={String(scheduleRunsResponse.next_cursor ?? "")}
            />
          )}
          {instance && (
            <>
              <StrategyInstanceControls
                instanceId={instanceID}
                status={String(instance.Status ?? instance.status ?? "")}
                stateVersion={Number(
                  instance.StateVersion ?? instance.state_version ?? 0,
                )}
              />
              <AIShadowScorecard
                scorecard={
                  scorecardResponse.scorecard as
                    | Record<string, unknown>
                    | undefined
                }
              />
              <StrategySimulationWorkbench
                automationId={id}
                versions={mandateVersions}
                currentVersion={currentVersion}
                capitalBucket={capitalBucket}
                status={read("status", "Status")}
                hasActiveInstance={hasActiveInstance}
                decisions={
                  Array.isArray(decisions.decisions)
                    ? (decisions.decisions as Record<string, unknown>[])
                    : []
                }
              />
              <DecisionReplayLab
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
