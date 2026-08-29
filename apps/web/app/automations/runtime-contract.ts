export type RuntimeRecord = Record<string, unknown>;

function value(record: RuntimeRecord | undefined, key: string, legacy: string) {
  return record?.[key] ?? record?.[legacy];
}

function text(record: RuntimeRecord | undefined, key: string, legacy: string) {
  const candidate = value(record, key, legacy);
  return typeof candidate === "string" ? candidate : undefined;
}

function integer(
  record: RuntimeRecord | undefined,
  key: string,
  legacy: string,
) {
  const candidate = value(record, key, legacy);
  return typeof candidate === "number" && Number.isInteger(candidate)
    ? candidate
    : undefined;
}

function flag(record: RuntimeRecord | undefined, key: string, legacy: string) {
  const candidate = value(record, key, legacy);
  return typeof candidate === "boolean" ? candidate : undefined;
}

function record(candidate: unknown) {
  return candidate && typeof candidate === "object" && !Array.isArray(candidate)
    ? (candidate as RuntimeRecord)
    : undefined;
}

function decimalUnits(candidate: string | undefined) {
  if (!candidate || !/^\d+(\.\d{1,10})?$/.test(candidate)) return undefined;
  const [whole, fraction = ""] = candidate.split(".");
  return BigInt(`${whole}${fraction.padEnd(10, "0")}`);
}

function validAIUniverse(universe: RuntimeRecord | undefined) {
  const symbolValue = value(universe, "symbols", "Symbols");
  const symbols = Array.isArray(symbolValue)
    ? symbolValue.filter(
        (symbol): symbol is string => typeof symbol === "string",
      )
    : [];
  const universeIDValue = value(universe, "universe_ids", "UniverseIDs");
  const universeIDsValid =
    universeIDValue === undefined ||
    (Array.isArray(universeIDValue) && universeIDValue.length === 0);
  return {
    symbols,
    valid:
      Array.isArray(symbolValue) &&
      symbols.length === symbolValue.length &&
      symbols.length >= 1 &&
      symbols.length <= 8 &&
      universeIDsValid &&
      new Set(symbols).size === symbols.length &&
      symbols.every(
        (symbol) =>
          symbol === symbol.trim().toUpperCase() &&
          /^[A-Z][A-Z0-9.-]{0,9}$/.test(symbol),
      ),
  };
}

export type PinnedAIRuntimeContract = {
  contextAvailable: boolean;
  bindingValid: boolean;
  configuration?: RuntimeRecord;
  pinnedVersion?: number;
  currentVersion?: number;
  currentStatus?: string;
  newerDraftAvailable: boolean;
  symbols: string[];
  maxProposalNotional?: string;
  maxTradesPerDay?: number;
  legacyDailyActionLimitMissing: boolean;
  scheduleEnabled?: boolean;
  scheduleIntervalMinutes?: number;
  scheduleSession?: string;
};

export function projectPinnedAIRuntime({
  mandate,
  instance,
  versionAvailable,
  version,
}: {
  mandate: RuntimeRecord;
  instance: RuntimeRecord;
  versionAvailable: boolean;
  version?: RuntimeRecord;
}): PinnedAIRuntimeContract {
  const mandateID = text(mandate, "id", "ID");
  const currentVersion = integer(mandate, "current_version", "CurrentVersion");
  const currentStatus = text(mandate, "status", "Status");
  const instanceID = text(instance, "id", "ID");
  const pinnedVersion = integer(instance, "mandate_version", "MandateVersion");
  const snapshot = record(value(version, "snapshot", "Snapshot"));
  const allowedUniverse = record(
    value(snapshot, "allowed_universe", "AllowedUniverse"),
  );
  const universe = validAIUniverse(allowedUniverse);
  const strategyParameters = record(
    value(snapshot, "strategy_parameters", "StrategyParameters"),
  );
  const riskParameters = record(
    value(snapshot, "risk_parameters", "RiskParameters"),
  );
  const scheduleConditions = record(
    value(snapshot, "schedule_conditions", "ScheduleConditions"),
  );
  const maxProposalNotional = text(
    strategyParameters,
    "max_proposal_notional",
    "MaxProposalNotional",
  );
  const objective = text(strategyParameters, "objective", "Objective");
  const proposalUnits = decimalUnits(maxProposalNotional);
  const maxTradesPerDayValue = value(
    riskParameters,
    "max_trades_per_day",
    "MaxTradesPerDay",
  );
  const maxTradesPerDay = integer(
    riskParameters,
    "max_trades_per_day",
    "MaxTradesPerDay",
  );
  const legacyDailyActionLimitMissing = maxTradesPerDayValue == null;
  const scheduleEnabled = flag(scheduleConditions, "enabled", "Enabled");
  const scheduleIntervalMinutes = integer(
    scheduleConditions,
    "interval_minutes",
    "IntervalMinutes",
  );
  const scheduleSession = text(scheduleConditions, "session", "Session");
  const snapshotExecutionMode = text(
    snapshot,
    "execution_mode",
    "ExecutionMode",
  );
  const instanceExecutionMode = text(
    instance,
    "execution_mode",
    "ExecutionMode",
  );
  const nonLiveExecutionMode =
    snapshotExecutionMode === instanceExecutionMode &&
    (snapshotExecutionMode === "PAPER" || snapshotExecutionMode === "SHADOW");
  const scheduleValid =
    scheduleEnabled === false ||
    (scheduleEnabled === true &&
      scheduleIntervalMinutes !== undefined &&
      scheduleIntervalMinutes >= 30 &&
      scheduleIntervalMinutes <= 1440 &&
      ["CONTINUOUS", "US_EQUITIES_REGULAR"].includes(scheduleSession ?? ""));
  const bindingValid = Boolean(
    versionAvailable &&
      version &&
      snapshot &&
      mandateID &&
      instanceID &&
      pinnedVersion &&
      currentVersion &&
      currentVersion >= pinnedVersion &&
      text(version, "id", "ID") &&
      text(version, "mandate_id", "MandateID") === mandateID &&
      integer(version, "version_number", "VersionNumber") === pinnedVersion &&
      text(instance, "automation_mandate_id", "AutomationMandateID") ===
        mandateID &&
      text(instance, "strategy_identifier", "StrategyIdentifier") ===
        "ai_shadow" &&
      text(snapshot, "financial_account_id", "FinancialAccountID") ===
        text(instance, "financial_account_id", "FinancialAccountID") &&
      text(snapshot, "capital_bucket_id", "CapitalBucketID") ===
        text(instance, "capital_bucket_id", "CapitalBucketID") &&
      text(snapshot, "automation_type", "AutomationType") === "AI_AUTONOMOUS" &&
      text(snapshot, "status", "Status") === "READY" &&
      nonLiveExecutionMode &&
      text(snapshot, "autonomy_level", "AutonomyLevel") === "FULL_AUTONOMOUS" &&
      text(snapshot, "ai_provider_connection_id", "AIProviderConnectionID") &&
      text(snapshot, "ai_model_id", "AIModelID") &&
      value(snapshot, "strategy_identifier", "StrategyIdentifier") == null &&
      flag(snapshot, "margin_allowed", "MarginAllowed") === false &&
      flag(snapshot, "options_allowed", "OptionsAllowed") === false &&
      flag(snapshot, "execution_capable", "ExecutionCapable") === false &&
      universe.valid &&
      riskParameters &&
      proposalUnits !== undefined &&
      proposalUnits > BigInt(0) &&
      proposalUnits <= BigInt("10000000000000000000") &&
      objective &&
      objective === objective.trim() &&
      objective.length <= 500 &&
      (legacyDailyActionLimitMissing ||
        (maxTradesPerDay !== undefined &&
          maxTradesPerDay >= 1 &&
          maxTradesPerDay <= 48)) &&
      value(riskParameters, "max_daily_loss", "MaxDailyLoss") == null &&
      scheduleValid,
  );

  return {
    contextAvailable: versionAvailable,
    bindingValid,
    configuration: bindingValid ? snapshot : undefined,
    pinnedVersion,
    currentVersion,
    currentStatus,
    newerDraftAvailable: Boolean(
      bindingValid &&
        currentVersion &&
        pinnedVersion &&
        currentVersion > pinnedVersion &&
        currentStatus === "DRAFT",
    ),
    symbols: bindingValid ? universe.symbols : [],
    maxProposalNotional: bindingValid ? maxProposalNotional : undefined,
    maxTradesPerDay: bindingValid ? maxTradesPerDay : undefined,
    legacyDailyActionLimitMissing:
      bindingValid && legacyDailyActionLimitMissing,
    scheduleEnabled: bindingValid ? scheduleEnabled : undefined,
    scheduleIntervalMinutes: bindingValid ? scheduleIntervalMinutes : undefined,
    scheduleSession: bindingValid ? scheduleSession : undefined,
  };
}

export function scheduleMatchesPinnedAIRuntime({
  contract,
  instanceID,
  mandateID,
  scheduleAvailable,
  envelope,
}: {
  contract: PinnedAIRuntimeContract;
  instanceID: string;
  mandateID: string;
  scheduleAvailable: boolean;
  envelope?: RuntimeRecord;
}) {
  const schedule = record(value(envelope, "schedule", "Schedule"));
  if (
    !contract.bindingValid ||
    !scheduleAvailable ||
    !schedule ||
    flag(envelope, "live_execution_available", "LiveExecutionAvailable") !==
      false
  )
    return false;

  const enabled = flag(schedule, "enabled", "Enabled");
  const projectedInstanceID = text(
    schedule,
    "strategy_instance_id",
    "StrategyInstanceID",
  );
  if (projectedInstanceID !== instanceID) return false;
  if (contract.scheduleEnabled === false) return enabled === false;
  return Boolean(
    contract.scheduleEnabled === true &&
      enabled === true &&
      text(schedule, "mandate_id", "MandateID") === mandateID &&
      integer(schedule, "mandate_version", "MandateVersion") ===
        contract.pinnedVersion &&
      integer(schedule, "interval_minutes", "IntervalMinutes") ===
        contract.scheduleIntervalMinutes &&
      text(schedule, "session", "Session") === contract.scheduleSession,
  );
}
