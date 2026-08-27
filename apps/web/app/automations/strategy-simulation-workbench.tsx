"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";

type RawRecord = Record<string, unknown>;

type ComparisonField = {
  label: string;
  previous: string;
  current: string;
  changed: boolean;
};

function read(entry: RawRecord | undefined, key: string, legacy = key) {
  if (!entry) return undefined;
  return entry[key] ?? entry[legacy];
}

function text(entry: RawRecord | undefined, key: string, legacy = key) {
  const value = read(entry, key, legacy);
  return typeof value === "string" ? value : undefined;
}

function number(entry: RawRecord | undefined, key: string, legacy = key) {
  const value = read(entry, key, legacy);
  if (typeof value === "number" && Number.isFinite(value)) return value;
  if (typeof value === "string" && value.trim() !== "") {
    const parsed = Number(value);
    return Number.isFinite(parsed) ? parsed : undefined;
  }
  return undefined;
}

function object(entry: RawRecord | undefined, key: string, legacy = key) {
  const value = read(entry, key, legacy);
  return value && typeof value === "object" && !Array.isArray(value)
    ? (value as RawRecord)
    : {};
}

function records(value: unknown) {
  return Array.isArray(value)
    ? value.filter(
        (item): item is RawRecord =>
          Boolean(item) && typeof item === "object" && !Array.isArray(item),
      )
    : [];
}

function strings(value: unknown) {
  return Array.isArray(value)
    ? value.filter((item): item is string => typeof item === "string")
    : [];
}

function label(value: string | undefined) {
  if (!value) return "Unavailable";
  return value
    .toLowerCase()
    .split("_")
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(" ");
}

function decimal(value: number | undefined, maximumFractionDigits = 2) {
  if (value === undefined || !Number.isFinite(value)) return "Unavailable";
  return new Intl.NumberFormat("en-US", {
    maximumFractionDigits,
  }).format(value);
}

function dollars(value: number | undefined) {
  if (value === undefined || !Number.isFinite(value)) return "Unavailable";
  return new Intl.NumberFormat("en-US", {
    style: "currency",
    currency: "USD",
    maximumFractionDigits: 2,
  }).format(value);
}

function timestamp(value: unknown) {
  if (typeof value !== "string") return "Unavailable";
  const parsed = new Date(value);
  if (Number.isNaN(parsed.valueOf())) return "Unavailable";
  return new Intl.DateTimeFormat("en-US", {
    dateStyle: "medium",
    timeStyle: "short",
    timeZone: "UTC",
  }).format(parsed);
}

function snapshot(version: RawRecord | undefined) {
  const value = read(version, "snapshot", "Snapshot");
  return value && typeof value === "object" && !Array.isArray(value)
    ? (value as RawRecord)
    : {};
}

function versionNumber(version: RawRecord | undefined) {
  return number(version, "version_number", "VersionNumber") ?? 0;
}

function versionCreatedAt(version: RawRecord | undefined) {
  return read(version, "created_at", "CreatedAt");
}

function versionSource(version: RawRecord | undefined) {
  return text(version, "source", "Source") ?? "UNKNOWN";
}

function strategyParameters(value: RawRecord) {
  return object(value, "strategy_parameters");
}

function riskParameters(value: RawRecord) {
  return object(value, "risk_parameters");
}

function scheduleConditions(value: RawRecord) {
  return object(value, "schedule_conditions");
}

function symbols(value: RawRecord) {
  return strings(read(object(value, "allowed_universe"), "symbols", "Symbols"));
}

function proposalCeiling(value: RawRecord) {
  return number(strategyParameters(value), "max_proposal_notional");
}

function tradesPerDay(value: RawRecord) {
  return number(riskParameters(value), "max_trades_per_day") ?? 1;
}

function scheduleLabel(value: RawRecord) {
  const schedule = scheduleConditions(value);
  const enabled = Boolean(read(schedule, "enabled", "Enabled"));
  if (!enabled) return "Disabled";
  const interval = number(schedule, "interval_minutes", "IntervalMinutes");
  const session = text(schedule, "session", "Session");
  return `${interval ? `Every ${interval} minutes` : "Enabled"} · ${label(session)}`;
}

function compareSnapshots(previous: RawRecord, current: RawRecord) {
  const field = (
    fieldLabel: string,
    previousValue: string,
    currentValue: string,
  ): ComparisonField => ({
    label: fieldLabel,
    previous: previousValue,
    current: currentValue,
    changed: previousValue !== currentValue,
  });
  const previousStrategy = strategyParameters(previous);
  const currentStrategy = strategyParameters(current);
  return [
    field(
      "Mandate status",
      label(text(previous, "status")),
      label(text(current, "status")),
    ),
    field(
      "AI model",
      text(previous, "ai_model_id") ?? "Unavailable",
      text(current, "ai_model_id") ?? "Unavailable",
    ),
    field(
      "Autonomy",
      label(text(previous, "autonomy_level")),
      label(text(current, "autonomy_level")),
    ),
    field(
      "Execution mode",
      label(text(previous, "execution_mode")),
      label(text(current, "execution_mode")),
    ),
    field(
      "Allowed symbols",
      symbols(previous).join(", ") || "None",
      symbols(current).join(", ") || "None",
    ),
    field(
      "Objective",
      text(previousStrategy, "objective") ?? "Unavailable",
      text(currentStrategy, "objective") ?? "Unavailable",
    ),
    field(
      "Per-decision ceiling",
      dollars(proposalCeiling(previous)),
      dollars(proposalCeiling(current)),
    ),
    field(
      "Maximum decisions per day",
      decimal(tradesPerDay(previous), 0),
      decimal(tradesPerDay(current), 0),
    ),
    field("Schedule", scheduleLabel(previous), scheduleLabel(current)),
    field(
      "Capital bucket assignment",
      text(previous, "capital_bucket_id") ?? "Unavailable",
      text(current, "capital_bucket_id") ?? "Unavailable",
    ),
  ];
}

function bucketCapacity(bucket: RawRecord | undefined) {
  if (!bucket) return undefined;
  const allocationType = text(bucket, "allocation_type", "AllocationType");
  const allocationValue = number(bucket, "allocation_value", "AllocationValue");
  const allocationLimit = number(bucket, "allocation_limit", "AllocationLimit");
  const protectedAmount =
    number(bucket, "protected_amount", "ProtectedAmount") ?? 0;
  if (allocationType === "FIXED_AMOUNT" && allocationValue !== undefined) {
    return Math.max(
      0,
      Math.min(allocationValue, allocationLimit ?? allocationValue) -
        protectedAmount,
    );
  }
  if (allocationLimit !== undefined) {
    return Math.max(0, allocationLimit - protectedAmount);
  }
  return undefined;
}

function marketAnchor(decisions: RawRecord[], selectedSymbol: string) {
  for (const decision of decisions) {
    if (read(decision, "source", "Source") !== "AI") continue;
    const rationale = object(
      decision,
      "structured_rationale",
      "StructuredRationale",
    );
    const input = object(rationale, "input_evidence");
    const markets = records(input.markets);
    const market = markets.find(
      (candidate) => text(candidate, "symbol") === selectedSymbol,
    );
    if (!market) continue;
    const price = ["mark", "last", "bid", "ask"]
      .map((key) => number(market, key))
      .find((candidate) => candidate !== undefined && candidate > 0);
    if (price === undefined) continue;
    return {
      price,
      feed: label(text(market, "feed")),
      quality: label(text(market, "quality")),
      observedAt: read(market, "observed_at"),
    };
  }
  return undefined;
}

export function StrategySimulationWorkbench({
  automationId,
  versions,
  currentVersion,
  capitalBucket,
  decisions = [],
  status = "DRAFT",
  hasActiveInstance = false,
}: {
  automationId?: string;
  versions: RawRecord[];
  currentVersion: number;
  capitalBucket?: RawRecord;
  decisions?: RawRecord[];
  status?: string;
  hasActiveInstance?: boolean;
}) {
  const router = useRouter();
  const orderedVersions = [...versions].sort(
    (left, right) => versionNumber(right) - versionNumber(left),
  );
  const current =
    orderedVersions.find(
      (version) => versionNumber(version) === currentVersion,
    ) ?? orderedVersions[0];
  const currentSnapshot = snapshot(current);
  const historicalVersions = orderedVersions.filter(
    (version) => versionNumber(version) !== versionNumber(current),
  );
  const defaultBaseline = historicalVersions[0] ?? current;
  const [baselineVersion, setBaselineVersion] = useState(
    versionNumber(defaultBaseline),
  );
  const baseline =
    orderedVersions.find(
      (version) => versionNumber(version) === baselineVersion,
    ) ?? defaultBaseline;
  const baselineSnapshot = snapshot(baseline);
  const currentSymbols = symbols(currentSnapshot);
  const currentCeiling = proposalCeiling(currentSnapshot);
  const currentTrades = tradesPerDay(currentSnapshot);
  const [scenarioCeiling, setScenarioCeiling] = useState(
    currentCeiling === undefined ? "" : String(currentCeiling),
  );
  const [scenarioTrades, setScenarioTrades] = useState(String(currentTrades));
  const [scenarioSymbol, setScenarioSymbol] = useState(currentSymbols[0] ?? "");
  const [reviewOpen, setReviewOpen] = useState(false);
  const [confirmed, setConfirmed] = useState(false);
  const [busy, setBusy] = useState(false);
  const [draftMessage, setDraftMessage] = useState("");

  const parsedCeiling = Number(scenarioCeiling);
  const parsedTrades = Number(scenarioTrades);
  const scenarioValid =
    Number.isFinite(parsedCeiling) &&
    parsedCeiling > 0 &&
    Number.isInteger(parsedTrades) &&
    parsedTrades >= 1 &&
    parsedTrades <= 48;
  const dailyResearchNotional = scenarioValid
    ? parsedCeiling * parsedTrades
    : undefined;
  const capacity = bucketCapacity(capitalBucket);
  const pressure =
    dailyResearchNotional !== undefined &&
    capacity !== undefined &&
    capacity > 0
      ? (dailyResearchNotional / capacity) * 100
      : undefined;
  const anchor = marketAnchor(decisions, scenarioSymbol);
  const hypotheticalUnits =
    scenarioValid && anchor ? parsedCeiling / anchor.price : undefined;
  const comparisons = compareSnapshots(baselineSnapshot, currentSnapshot);
  const changedFields = comparisons.filter((item) => item.changed);
  const scenarioChanges = [
    currentCeiling !== undefined && parsedCeiling !== currentCeiling,
    parsedTrades !== currentTrades,
    scenarioSymbol !== (currentSymbols[0] ?? ""),
  ].filter(Boolean).length;
  const draftChanges = [
    currentCeiling !== undefined && parsedCeiling !== currentCeiling,
    parsedTrades !== currentTrades,
  ].filter(Boolean).length;

  const resetScenario = () => {
    setScenarioCeiling(
      currentCeiling === undefined ? "" : String(currentCeiling),
    );
    setScenarioTrades(String(currentTrades));
    setScenarioSymbol(currentSymbols[0] ?? "");
    setReviewOpen(false);
    setConfirmed(false);
    setDraftMessage("");
  };

  const createDraft = async () => {
    if (
      !automationId ||
      !scenarioValid ||
      draftChanges === 0 ||
      !confirmed ||
      busy
    ) {
      return;
    }
    setBusy(true);
    setDraftMessage("");
    const response = await fetch(
      `/api/automations/${automationId}/ai-shadow-scenario-draft`,
      {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({
          expected_version: currentVersion,
          max_proposal_notional: scenarioCeiling.trim(),
          max_trades_per_day: parsedTrades,
          confirm: true,
        }),
      },
    );
    setBusy(false);
    if (!response.ok) {
      const body = (await response.json().catch(() => null)) as {
        error?: { code?: string };
      } | null;
      setDraftMessage(
        body?.error?.code === "VERSION_CONFLICT"
          ? "The mandate changed while you were reviewing it. Refresh and compare the new current version before creating a draft."
          : "The scenario draft was not accepted. Review the ceiling, daily limit, mandate status, and confirmation.",
      );
      return;
    }
    setReviewOpen(false);
    setConfirmed(false);
    setDraftMessage(
      `Version ${currentVersion + 1} was created as an immutable DRAFT. It is not READY, initialized, scheduled, approved, or executable.`,
    );
    router.refresh();
  };

  return (
    <section
      className="strategy-simulation-workbench"
      aria-label="Strategy Simulation Workbench"
    >
      <header>
        <div>
          <p className="eyebrow">STRATEGY SIMULATION WORKBENCH</p>
          <h2>Test the mandate before changing the mandate.</h2>
          <p>
            Compare immutable versions and explore a local capital scenario
            against the latest recorded market evidence. Scenario edits remain
            local unless you deliberately send the two reviewed limits into a
            new DRAFT. No model, market, or broker call occurs.
          </p>
        </div>
        <span>LOCAL WHAT-IF · NON-EXECUTING</span>
      </header>

      {!current ? (
        <div className="strategy-simulation-empty">
          <strong>No immutable mandate version is available.</strong>
          <p>The workbench will appear after the first mandate is created.</p>
        </div>
      ) : (
        <>
          <section className="strategy-version-comparison">
            <header>
              <div>
                <p className="eyebrow">VERSION COMPARISON</p>
                <h3>
                  Version {versionNumber(baseline)} → Version{" "}
                  {versionNumber(current)}
                </h3>
              </div>
              <label>
                Compare from
                <select
                  aria-label="Comparison mandate version"
                  onChange={(event) =>
                    setBaselineVersion(Number(event.target.value))
                  }
                  value={versionNumber(baseline)}
                >
                  {orderedVersions.map((version) => (
                    <option
                      key={versionNumber(version)}
                      value={versionNumber(version)}
                    >
                      Version {versionNumber(version)} ·{" "}
                      {label(versionSource(version))}
                    </option>
                  ))}
                </select>
              </label>
            </header>
            <div className="strategy-version-meta">
              <span>
                Baseline recorded {timestamp(versionCreatedAt(baseline))} UTC
              </span>
              <span>
                Current recorded {timestamp(versionCreatedAt(current))} UTC
              </span>
              <strong>
                {changedFields.length} changed field
                {changedFields.length === 1 ? "" : "s"}
              </strong>
            </div>
            <div className="strategy-version-table" role="table">
              <div role="row">
                <span role="columnheader">Policy field</span>
                <span role="columnheader">
                  Version {versionNumber(baseline)}
                </span>
                <span role="columnheader">
                  Version {versionNumber(current)}
                </span>
              </div>
              {comparisons.map((item) => (
                <div
                  className={item.changed ? "is-changed" : ""}
                  key={item.label}
                  role="row"
                >
                  <strong role="cell">{item.label}</strong>
                  <span role="cell">{item.previous}</span>
                  <span role="cell">{item.current}</span>
                </div>
              ))}
            </div>
          </section>

          <section className="strategy-scenario-builder">
            <header>
              <div>
                <p className="eyebrow">LOCAL CONFIGURATION SCENARIO</p>
                <h3>Pressure-test the current envelope</h3>
              </div>
              <button onClick={resetScenario} type="button">
                Reset to version {versionNumber(current)}
              </button>
            </header>
            <div className="strategy-scenario-controls">
              <label>
                Per-decision ceiling
                <span>
                  <b>$</b>
                  <input
                    aria-describedby="scenario-ceiling-help"
                    aria-label="Scenario per-decision ceiling"
                    inputMode="decimal"
                    min="0.01"
                    onChange={(event) => setScenarioCeiling(event.target.value)}
                    step="0.01"
                    type="number"
                    value={scenarioCeiling}
                  />
                </span>
                <small id="scenario-ceiling-help">
                  Local only. The immutable mandate remains unchanged.
                </small>
              </label>
              <label>
                Maximum decisions per day
                <input
                  aria-label="Scenario maximum decisions per day"
                  max="48"
                  min="1"
                  onChange={(event) => setScenarioTrades(event.target.value)}
                  step="1"
                  type="number"
                  value={scenarioTrades}
                />
                <small>One through forty-eight for bounded comparison.</small>
              </label>
              <label>
                Replay anchor symbol
                <select
                  aria-label="Scenario replay anchor symbol"
                  disabled={currentSymbols.length === 0}
                  onChange={(event) => setScenarioSymbol(event.target.value)}
                  value={scenarioSymbol}
                >
                  {currentSymbols.length === 0 ? (
                    <option value="">No allowlisted symbol</option>
                  ) : (
                    currentSymbols.map((symbol) => (
                      <option key={symbol} value={symbol}>
                        {symbol}
                      </option>
                    ))
                  )}
                </select>
                <small>Uses stored evidence; it never requests a quote.</small>
              </label>
            </div>

            {!scenarioValid && (
              <p className="strategy-scenario-invalid" role="alert">
                Enter a positive proposal ceiling and a whole-number daily limit
                from 1 to 48.
              </p>
            )}

            <div className="strategy-scenario-results">
              <article>
                <span>PER-DECISION ENVELOPE</span>
                <strong>
                  {dollars(scenarioValid ? parsedCeiling : undefined)}
                </strong>
                <small>
                  {scenarioChanges} local change
                  {scenarioChanges === 1 ? "" : "s"} from version{" "}
                  {versionNumber(current)}
                </small>
              </article>
              <article>
                <span>DAILY RESEARCH NOTIONAL</span>
                <strong>{dollars(dailyResearchNotional)}</strong>
                <small>
                  Ceiling × decision count; not expected trading volume
                </small>
              </article>
              <article>
                <span>CAPITAL POLICY CAPACITY</span>
                <strong>
                  {capacity === undefined
                    ? "Provider-relative"
                    : dollars(capacity)}
                </strong>
                <small>
                  {text(capitalBucket, "name", "Name") ??
                    "Current capital bucket"}
                </small>
              </article>
              <article>
                <span>RECORDED-PRICE UNITS</span>
                <strong>
                  {hypotheticalUnits === undefined
                    ? "Unavailable"
                    : `${decimal(hypotheticalUnits, 8)} ${scenarioSymbol}`}
                </strong>
                <small>
                  {anchor
                    ? `${dollars(anchor.price)} · ${anchor.feed}`
                    : "No matching immutable market anchor"}
                </small>
              </article>
            </div>

            <section className="strategy-capital-pressure">
              <header>
                <div>
                  <span>CONFIGURATION PRESSURE</span>
                  <strong>
                    {pressure === undefined
                      ? "Cannot calculate an absolute ratio"
                      : `${decimal(pressure, 1)}% of current capacity`}
                  </strong>
                </div>
                <span>
                  {pressure === undefined
                    ? "PROVIDER FACTS REQUIRED"
                    : pressure <= 100
                      ? "WITHIN STATIC CAPACITY"
                      : "EXCEEDS STATIC CAPACITY"}
                </span>
              </header>
              <div aria-hidden="true">
                <i
                  style={{
                    width: `${Math.max(0, Math.min(pressure ?? 0, 100))}%`,
                  }}
                />
              </div>
              <p>
                This ratio is a configuration comparison only. It does not
                account for future prices, fees, slippage, holdings, cash,
                reservations, reconciliation, or deterministic risk checks.
              </p>
            </section>

            <div className="strategy-market-anchor">
              <div>
                <span>REPLAY ANCHOR</span>
                <strong>{scenarioSymbol || "No symbol"}</strong>
              </div>
              <p>
                {anchor
                  ? `${dollars(anchor.price)} observed ${timestamp(anchor.observedAt)} UTC · ${anchor.feed} · ${anchor.quality}`
                  : "No immutable market observation for this symbol is available. Arbion does not substitute a live or estimated price."}
              </p>
            </div>

            {automationId && (
              <section
                className="strategy-draft-handoff"
                aria-label="Scenario draft review"
              >
                <header>
                  <div>
                    <p className="eyebrow">EXPLICIT DRAFT HANDOFF</p>
                    <h3>Carry only the reviewed limits forward</h3>
                    <p>
                      This action preserves the account, capital bucket, model,
                      objective, symbols, schedule, autonomy, and SHADOW mode.
                    </p>
                  </div>
                  <button
                    disabled={
                      !scenarioValid ||
                      draftChanges === 0 ||
                      status === "ARCHIVED"
                    }
                    onClick={() => {
                      setReviewOpen(true);
                      setConfirmed(false);
                      setDraftMessage("");
                    }}
                    type="button"
                  >
                    Review as new DRAFT
                  </button>
                </header>

                {reviewOpen && (
                  <div className="strategy-draft-review">
                    <div role="table" aria-label="Scenario draft changes">
                      <div role="row">
                        <span role="columnheader">Reviewed limit</span>
                        <span role="columnheader">
                          Version {currentVersion}
                        </span>
                        <span role="columnheader">New DRAFT</span>
                      </div>
                      <div role="row">
                        <strong role="cell">Per-decision ceiling</strong>
                        <span role="cell">{dollars(currentCeiling)}</span>
                        <span role="cell">{dollars(parsedCeiling)}</span>
                      </div>
                      <div role="row">
                        <strong role="cell">Maximum decisions per day</strong>
                        <span role="cell">{currentTrades}</span>
                        <span role="cell">{parsedTrades}</span>
                      </div>
                    </div>
                    {hasActiveInstance && (
                      <p className="security-note">
                        The active non-live instance remains pinned to its
                        reviewed immutable version and continues on that exact
                        schedule. This DRAFT cannot change its limits or model;
                        finish the old instance before marking and initializing
                        a reviewed replacement.
                      </p>
                    )}
                    <label>
                      <input
                        checked={confirmed}
                        onChange={(event) => setConfirmed(event.target.checked)}
                        type="checkbox"
                      />
                      I reviewed both changes and understand this creates only
                      an immutable DRAFT with no execution authority.
                    </label>
                    <div>
                      <button
                        disabled={!confirmed || busy}
                        onClick={createDraft}
                        type="button"
                      >
                        {busy ? "Creating immutable DRAFT…" : "Create DRAFT"}
                      </button>
                      <button
                        disabled={busy}
                        onClick={() => {
                          setReviewOpen(false);
                          setConfirmed(false);
                        }}
                        type="button"
                      >
                        Keep it local
                      </button>
                    </div>
                  </div>
                )}
                {draftMessage && <p role="status">{draftMessage}</p>}
              </section>
            )}
          </section>
        </>
      )}

      <footer>
        <strong>SIMULATION DOES NOT CREATE AUTHORITY</strong>
        <span>
          A local scenario has no authority. Its explicit handoff can create
          only a DRAFT; it cannot mark a mandate READY, initialize an instance,
          clear a breaker, pass risk, or create an order.
        </span>
      </footer>
    </section>
  );
}
