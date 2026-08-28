import Link from "next/link";
import { cookies } from "next/headers";
import { redirect } from "next/navigation";

import { AppPageHeader } from "../app-page-header";
import { asList } from "./response";
import { StrategyFleet, type StrategyFleetItem } from "./strategy-fleet";

type RecordValue = Record<string, unknown>;

function text(record: RecordValue | undefined, key: string, legacy: string) {
  const value = record?.[key] ?? record?.[legacy];
  return typeof value === "string" ? value : undefined;
}

function number(record: RecordValue | undefined, key: string, legacy: string) {
  const value = record?.[key] ?? record?.[legacy];
  return typeof value === "number" && Number.isFinite(value)
    ? value
    : undefined;
}

function flag(record: RecordValue | undefined, key: string, legacy: string) {
  const value = record?.[key] ?? record?.[legacy];
  return typeof value === "boolean" ? value : undefined;
}

function symbols(mandate: RecordValue) {
  const universe = (mandate.allowed_universe ?? mandate.AllowedUniverse) as
    | RecordValue
    | undefined;
  const values = universe?.symbols ?? universe?.Symbols;
  return Array.isArray(values)
    ? values.filter((value): value is string => typeof value === "string")
    : [];
}

function stringList(value: unknown) {
  return Array.isArray(value)
    ? value.filter((item): item is string => typeof item === "string")
    : [];
}

function strategyTitle(mandate: RecordValue) {
  if (text(mandate, "automation_type", "AutomationType") === "AI_AUTONOMOUS")
    return "AI Shadow Engine";
  const identifier = text(mandate, "strategy_identifier", "StrategyIdentifier");
  if (!identifier) return "Trading strategy";
  return identifier
    .toLowerCase()
    .split("_")
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(" ");
}

async function fetchOptional<T>(
  url: string,
  headers: { cookie: string },
): Promise<{ available: boolean; status?: number; payload?: T }> {
  try {
    const response = await fetch(url, { headers, cache: "no-store" });
    if (!response.ok) return { available: false, status: response.status };
    return {
      available: true,
      status: response.status,
      payload: (await response.json()) as T,
    };
  } catch {
    return { available: false };
  }
}

function currentInstance(mandate: RecordValue, instances: RecordValue[]) {
  const mandateID = text(mandate, "id", "ID");
  const version = number(mandate, "current_version", "CurrentVersion");
  const matches = instances.filter(
    (instance) =>
      text(instance, "automation_mandate_id", "AutomationMandateID") ===
      mandateID,
  );
  return (
    matches.find((instance) =>
      ["ACTIVE", "PAUSED"].includes(text(instance, "status", "Status") ?? ""),
    ) ??
    matches.find(
      (instance) =>
        number(instance, "mandate_version", "MandateVersion") === version,
    )
  );
}

async function fleetItem(
  mandate: RecordValue,
  accounts: RecordValue[],
  instances: RecordValue[],
  base: string,
  headers: { cookie: string },
  accountContextAvailable: boolean,
  instanceContextAvailable: boolean,
): Promise<StrategyFleetItem> {
  const id = text(mandate, "id", "ID") ?? "";
  const accountID =
    text(mandate, "financial_account_id", "FinancialAccountID") ?? "";
  const account = accounts.find(
    (candidate) => text(candidate, "id", "ID") === accountID,
  );
  const instance = currentInstance(mandate, instances);
  const instanceID = text(instance, "id", "ID");
  const instanceStatus = text(instance, "status", "Status");
  const automationType =
    text(mandate, "automation_type", "AutomationType") ?? "UNKNOWN";
  const expectsSchedule =
    Boolean(instanceID) && ["ACTIVE", "PAUSED"].includes(instanceStatus ?? "");
  const expectsEvidence =
    Boolean(instanceID) && automationType === "AI_AUTONOMOUS";
  const [scheduleResult, scorecardResult] = await Promise.all([
    expectsSchedule
      ? fetchOptional<{ schedule?: RecordValue }>(
          `${base}/api/strategy-instances/${encodeURIComponent(instanceID ?? "")}/schedule`,
          headers,
        )
      : Promise.resolve({ available: true as const, payload: undefined }),
    expectsEvidence
      ? fetchOptional<{ scorecard?: RecordValue }>(
          `${base}/api/strategy-instances/${encodeURIComponent(instanceID ?? "")}/shadow-scorecard`,
          headers,
        )
      : Promise.resolve({ available: true as const, payload: undefined }),
  ]);
  const schedule = scheduleResult.payload?.schedule;
  const scorecard = scorecardResult.payload?.scorecard;
  const evidenceGate = (scorecard?.evidence_gate ?? scorecard?.EvidenceGate) as
    | RecordValue
    | undefined;
  const evidenceAvailable =
    expectsEvidence && scorecardResult.available && Boolean(evidenceGate);

  return {
    id,
    title: strategyTitle(mandate),
    accountName:
      text(account, "display_name", "DisplayName") ?? "Connected account",
    provider: text(account, "provider", "Provider") ?? "connected_account",
    automationType,
    mandateStatus: text(mandate, "status", "Status") ?? "UNKNOWN",
    autonomyLevel:
      text(mandate, "autonomy_level", "AutonomyLevel") ?? "UNKNOWN",
    executionMode:
      text(mandate, "execution_mode", "ExecutionMode") ?? "UNKNOWN",
    modelID: text(mandate, "ai_model_id", "AIModelID"),
    symbols: symbols(mandate),
    instanceStatus,
    currentState: text(instance, "current_state", "CurrentState"),
    lastEvaluatedAt: text(instance, "last_evaluated_at", "LastEvaluatedAt"),
    scheduleAvailable: expectsSchedule ? scheduleResult.available : undefined,
    scheduleEnabled: flag(schedule, "enabled", "Enabled"),
    scheduleStatus: text(schedule, "last_status", "LastStatus"),
    consecutiveFailures:
      number(schedule, "consecutive_failures", "ConsecutiveFailures") ?? 0,
    nextRunAt: text(schedule, "next_run_at", "NextRunAt"),
    evidenceAvailable: expectsEvidence ? evidenceAvailable : undefined,
    evidenceStatus: text(evidenceGate, "status", "Status"),
    oneHourSampleSize: number(
      evidenceGate,
      "one_hour_sample_size",
      "OneHourSampleSize",
    ),
    twentyFourHourSampleSize: number(
      evidenceGate,
      "twenty_four_hour_sample_size",
      "TwentyFourHourSampleSize",
    ),
    minimumSamplePerHorizon: number(
      evidenceGate,
      "minimum_sample_per_horizon",
      "MinimumSamplePerHorizon",
    ),
    evidenceWindowHours: number(
      evidenceGate,
      "evidence_window_hours",
      "EvidenceWindowHours",
    ),
    minimumEvidenceWindowHours: number(
      evidenceGate,
      "minimum_evidence_window_hours",
      "MinimumEvidenceWindowHours",
    ),
    evidenceScheduleHealthy: flag(
      evidenceGate,
      "schedule_healthy",
      "ScheduleHealthy",
    ),
    evidenceBlockers: stringList(
      evidenceGate?.blockers ?? evidenceGate?.Blockers,
    ),
    currentEvidenceReviewed: flag(
      scorecard,
      "current_evidence_reviewed",
      "CurrentEvidenceReviewed",
    ),
    accountContextAvailable: accountContextAvailable && Boolean(account),
    instanceContextAvailable,
  };
}

export default async function Automations() {
  const jar = await cookies();
  const headers = { cookie: jar.toString() };
  const base = process.env.API_BASE_URL ?? "http://localhost:8080";
  const [mandatesResult, accountsResult, instancesResult] = await Promise.all([
    fetchOptional<{ automations?: RecordValue[] | null }>(
      `${base}/api/automations`,
      headers,
    ),
    fetchOptional<{ accounts?: RecordValue[] | null }>(
      `${base}/api/accounts`,
      headers,
    ),
    fetchOptional<{ strategy_instances?: RecordValue[] | null }>(
      `${base}/api/strategy-instances`,
      headers,
    ),
  ]);
  if (
    mandatesResult.status === 401 ||
    accountsResult.status === 401 ||
    instancesResult.status === 401
  )
    redirect("/login");

  const mandates = asList(mandatesResult.payload?.automations);
  const accounts = asList(accountsResult.payload?.accounts);
  const instances = asList(instancesResult.payload?.strategy_instances);
  const items = mandatesResult.available
    ? await Promise.all(
        mandates.map((mandate) =>
          fleetItem(
            mandate,
            accounts,
            instances,
            base,
            headers,
            accountsResult.available,
            instancesResult.available,
          ),
        ),
      )
    : [];
  const contextWarnings = [
    !accountsResult.available
      ? "Account names and providers could not be refreshed."
      : "",
    !instancesResult.available
      ? "Current engine state could not be refreshed."
      : "",
  ].filter(Boolean);

  return (
    <main className="connections-page automation-page strategy-fleet-page">
      <AppPageHeader />
      <section className="strategy-fleet-hero">
        <div>
          <p className="eyebrow">YOUR AUTONOMY FLEET</p>
          <h1>Every engine. One command surface.</h1>
          <p className="lede">
            See the account, model, coverage, state, and schedule behind every
            strategy—without opening each mandate first.
          </p>
        </div>
        <div className="strategy-fleet-actions">
          <Link className="button-link" href="/automations/new">
            Launch AI Shadow Engine
          </Link>
          <div className="strategy-fleet-secondary-actions">
            <Link href="/capital">Capital budgets →</Link>
            <Link href="/activity">Decision journal →</Link>
          </div>
        </div>
      </section>
      <StrategyFleet
        contextWarnings={contextWarnings}
        inventoryAvailable={mandatesResult.available}
        items={items}
      />
    </main>
  );
}
