type Entity = Record<string, unknown> | null | undefined;

type CheckState =
  | "READY"
  | "BLOCKED"
  | "ACTION_REQUIRED"
  | "NOT_APPLICABLE"
  | "UNAVAILABLE";

export type InitializationReadinessCheck = {
  code: string;
  label: string;
  state: CheckState;
  blocking: boolean;
  detail: string;
};

export type StrategyInitializationAssessment = {
  status:
    | "READY_TO_INITIALIZE"
    | "BLOCKED"
    | "CURRENT_STATE_UNAVAILABLE"
    | "ALREADY_INITIALIZED"
    | "VERSION_ALREADY_USED";
  eligible: boolean;
  paperStartingCashLimit?: string;
  checks: InitializationReadinessCheck[];
};

export type StrategyInitializationInput = {
  automationType: string;
  mandateStatus: string;
  currentVersion: number;
  autonomyLevel: string;
  executionMode: string;
  strategyIdentifier: string;
  modelID: string;
  financialProvider: string;
  financialAccount?: Entity;
  financialConnection?: Entity;
  aiConnection?: Entity;
  capitalBucket?: Entity;
  instance?: Entity;
  activeReservations: Record<string, unknown>[];
  scheduleConditions?: Entity;
  reconciliation?: Entity;
  observedAt: string;
  inventoryAvailable: boolean;
  reconciliationAvailable: boolean;
};

const scale = BigInt("10000000000");
const deterministicStrategies = new Set([
  "wheel",
  "covered_call",
  "cash_secured_put",
]);
const modelProviders: Record<string, string> = {
  "gpt-5.6-luna": "openai",
  "gpt-5.6-terra": "openai",
  "gpt-5.6-sol": "openai",
  "claude-haiku-4-5-20251001": "anthropic",
  "claude-sonnet-5": "anthropic",
  "claude-opus-5": "anthropic",
  "gemini-3.5-flash": "gemini",
  "gemini-3.6-flash": "gemini",
  "gemini-3.7-flash": "gemini",
};

function text(entity: Entity, key: string, legacy: string) {
  const value = entity?.[key] ?? entity?.[legacy];
  return typeof value === "string" ? value : "";
}

function flag(entity: Entity, key: string, legacy: string) {
  const value = entity?.[key] ?? entity?.[legacy];
  return typeof value === "boolean" ? value : false;
}

function integer(entity: Entity, key: string, legacy: string) {
  const value = entity?.[key] ?? entity?.[legacy];
  return typeof value === "number" && Number.isInteger(value) ? value : 0;
}

function decimalUnits(value: string | undefined) {
  if (!value || !/^\d+(\.\d{1,10})?$/.test(value)) return undefined;
  const [whole, fraction = ""] = value.split(".");
  return BigInt(whole) * scale + BigInt(fraction.padEnd(10, "0"));
}

function unitsDecimal(value: bigint) {
  const whole = value / scale;
  const fraction = String(value % scale)
    .padStart(10, "0")
    .replace(/0+$/, "");
  return `${whole}${fraction ? `.${fraction}` : ""}`;
}

function conciseDecimal(value: string) {
  if (!/^\d+(\.\d+)?$/.test(value)) return value;
  const [whole, fraction = ""] = value.split(".");
  const compact = fraction.replace(/0+$/, "");
  return `${whole.replace(/\B(?=(\d{3})+(?!\d))/g, ",")}${compact ? `.${compact}` : ""}`;
}

function money(currency: string, amount: string) {
  const concise = conciseDecimal(amount);
  return currency === "USD" ? `$${concise}` : `${currency} ${concise}`;
}

function readinessCheck(
  code: string,
  label: string,
  state: CheckState,
  blocking: boolean,
  detail: string,
): InitializationReadinessCheck {
  return { code, label, state, blocking, detail };
}

type CapitalClaim = {
  capacity: bigint;
  currency: string;
  shadowAmount?: bigint;
  sharedCeiling?: bigint;
};

function capitalClaim(
  bucket: Entity,
  executionMode: string,
): CapitalClaim | undefined {
  if (
    !bucket ||
    text(bucket, "status", "Status") !== "ACTIVE" ||
    flag(bucket, "is_reserve", "IsReserve")
  ) {
    return undefined;
  }
  const currency = text(bucket, "currency", "Currency");
  const allocationType = text(bucket, "allocation_type", "AllocationType");
  const allocation = decimalUnits(
    text(bucket, "allocation_value", "AllocationValue"),
  );
  const protectedAmount = decimalUnits(
    text(bucket, "protected_amount", "ProtectedAmount"),
  );
  const limitText = text(bucket, "allocation_limit", "AllocationLimit");
  const limit = limitText ? decimalUnits(limitText) : undefined;
  if (
    currency.length !== 3 ||
    allocation === undefined ||
    allocation <= BigInt(0) ||
    protectedAmount === undefined ||
    (limitText && (limit === undefined || limit <= BigInt(0)))
  ) {
    return undefined;
  }

  let grossCapacity: bigint;
  let sharedCeiling: bigint | undefined;
  if (allocationType === "FIXED_AMOUNT") {
    grossCapacity =
      limit !== undefined && limit < allocation ? limit : allocation;
    sharedCeiling = limit;
  } else if (
    allocationType === "PERCENT_OF_AVAILABLE_CASH" ||
    allocationType === "PERCENT_OF_BUYING_POWER"
  ) {
    if (limit === undefined) return undefined;
    grossCapacity = limit;
  } else {
    return undefined;
  }

  const capacity = grossCapacity - protectedAmount;
  if (capacity <= BigInt(0)) return undefined;
  return {
    capacity,
    currency,
    shadowAmount: executionMode === "SHADOW" ? capacity : undefined,
    sharedCeiling,
  };
}

function reconciliationFresh(reconciliation: Entity, observedAt: string) {
  const timestamp = text(reconciliation, "observed_at", "ObservedAt");
  const observed = new Date(timestamp);
  const now = new Date(observedAt);
  if (
    Number.isNaN(observed.valueOf()) ||
    Number.isNaN(now.valueOf()) ||
    observed > now
  ) {
    return false;
  }
  return now.valueOf() - observed.valueOf() <= 24 * 60 * 60 * 1000;
}

export function assessStrategyInitialization(
  input: StrategyInitializationInput,
): StrategyInitializationAssessment {
  const checks: InitializationReadinessCheck[] = [];
  const instanceID = text(input.instance, "id", "ID");
  const instanceStatus = text(input.instance, "status", "Status");
  const activeInstance =
    Boolean(instanceID) &&
    (instanceStatus === "ACTIVE" || instanceStatus === "PAUSED");
  const versionAlreadyUsed = Boolean(instanceID) && !activeInstance;

  checks.push(
    readinessCheck(
      "CURRENT_INVENTORY",
      "Current safety inventory",
      input.inventoryAvailable ? "READY" : "UNAVAILABLE",
      true,
      input.inventoryAvailable
        ? "Account, connection, capital, reservation, and strategy records were refreshed together."
        : "Arbion could not refresh every required record and will not infer initialization readiness from partial data.",
    ),
  );

  checks.push(
    readinessCheck(
      "MANDATE_VERSION",
      "Immutable mandate version",
      input.mandateStatus === "READY" && input.currentVersion > 0
        ? "READY"
        : "BLOCKED",
      true,
      input.mandateStatus === "READY" && input.currentVersion > 0
        ? `Version ${input.currentVersion} is READY and will be pinned to the new instance.`
        : `Version ${input.currentVersion || "—"} is ${input.mandateStatus || "UNKNOWN"}. Review it and mark that exact version READY first.`,
    ),
  );

  const deterministicReady =
    input.automationType === "STRATEGY" &&
    deterministicStrategies.has(input.strategyIdentifier) &&
    (input.executionMode === "PAPER" || input.executionMode === "SHADOW");
  const aiConfigurationReady =
    input.automationType === "AI_AUTONOMOUS" &&
    input.autonomyLevel === "FULL_AUTONOMOUS" &&
    input.executionMode === "SHADOW" &&
    Boolean(modelProviders[input.modelID]);
  const configurationReady = deterministicReady || aiConfigurationReady;
  checks.push(
    readinessCheck(
      "NONLIVE_CONFIGURATION",
      "Non-live configuration",
      configurationReady ? "READY" : "BLOCKED",
      true,
      configurationReady
        ? `${input.automationType === "AI_AUTONOMOUS" ? "AI autonomous" : input.strategyIdentifier} · ${input.autonomyLevel} · ${input.executionMode}.`
        : "Only implemented deterministic PAPER/SHADOW strategies or FULL_AUTONOMOUS AI SHADOW can initialize.",
    ),
  );

  const accountStatus = text(input.financialAccount, "status", "Status");
  const connectionStatus = text(input.financialConnection, "status", "Status");
  const accountReady =
    Boolean(text(input.financialAccount, "id", "ID")) &&
    Boolean(text(input.financialConnection, "id", "ID")) &&
    accountStatus === "active" &&
    connectionStatus === "active";
  checks.push(
    readinessCheck(
      "FINANCIAL_CONNECTION",
      "Financial account connection",
      accountReady ? "READY" : "BLOCKED",
      true,
      accountReady
        ? `${input.financialProvider || "Financial provider"} account and credential connection are active.`
        : `Connection ${connectionStatus || "unavailable"} · account ${accountStatus || "unavailable"}. Reconnect or resync before initialization.`,
    ),
  );

  if (input.automationType === "AI_AUTONOMOUS") {
    const aiStatus = text(input.aiConnection, "status", "Status");
    const aiProvider = text(input.aiConnection, "provider", "Provider");
    const expectedProvider = modelProviders[input.modelID];
    const aiReady =
      Boolean(text(input.aiConnection, "id", "ID")) &&
      aiStatus === "active" &&
      flag(input.aiConnection, "enabled", "Enabled") &&
      Boolean(expectedProvider) &&
      aiProvider === expectedProvider;
    checks.push(
      readinessCheck(
        "AI_ROUTE",
        "AI decision route",
        aiReady ? "READY" : "BLOCKED",
        true,
        aiReady
          ? `${aiProvider} · ${input.modelID} · active and enabled.`
          : "The saved model must match an active, enabled AI-provider connection.",
      ),
    );
  } else {
    checks.push(
      readinessCheck(
        "AI_ROUTE",
        "AI decision route",
        "NOT_APPLICABLE",
        false,
        "This deterministic strategy does not call an AI model.",
      ),
    );
  }

  const claim = capitalClaim(input.capitalBucket, input.executionMode);
  const bucketName =
    text(input.capitalBucket, "name", "Name") || "Selected budget";
  checks.push(
    readinessCheck(
      "CAPITAL_POLICY",
      "Exact capital policy",
      claim ? "READY" : "BLOCKED",
      true,
      claim
        ? `${bucketName} provides ${money(claim.currency, unitsDecimal(claim.capacity))} after protected capital.`
        : "Use an active, non-reserve budget with positive exact capacity. Percentage-based Shadow budgets require an absolute cap.",
    ),
  );

  let paperStartingCashLimit =
    claim && input.executionMode === "PAPER"
      ? unitsDecimal(claim.capacity)
      : undefined;
  const accountID = text(input.financialAccount, "id", "ID");
  const bucketID = text(input.capitalBucket, "id", "ID");
  const activeReservations = input.activeReservations.filter(
    (reservation) =>
      text(reservation, "status", "Status") === "ACTIVE" &&
      text(reservation, "financial_account_id", "FinancialAccountID") ===
        accountID,
  );
  let capitalAvailability = readinessCheck(
    "ACCOUNT_CAPACITY",
    "Account reservation capacity",
    "BLOCKED",
    true,
    "The current account reservation inventory is not compatible with this budget.",
  );
  if (activeInstance) {
    capitalAvailability = readinessCheck(
      "ACCOUNT_CAPACITY",
      "Account reservation capacity",
      "READY",
      false,
      "This mandate already owns its durable non-live reservation.",
    );
  } else if (!input.inventoryAvailable || !claim || !accountID || !bucketID) {
    capitalAvailability = readinessCheck(
      "ACCOUNT_CAPACITY",
      "Account reservation capacity",
      "UNAVAILABLE",
      true,
      "Arbion will not calculate headroom from incomplete or malformed inventory.",
    );
  } else if (
    activeReservations.some(
      (reservation) =>
        text(reservation, "capital_bucket_id", "CapitalBucketID") === bucketID,
    )
  ) {
    capitalAvailability = readinessCheck(
      "ACCOUNT_CAPACITY",
      "Account reservation capacity",
      "BLOCKED",
      true,
      "The selected budget is already claimed by another active or paused strategy.",
    );
  } else if (activeReservations.length === 0) {
    capitalAvailability = readinessCheck(
      "ACCOUNT_CAPACITY",
      "Account reservation capacity",
      "READY",
      false,
      input.executionMode === "PAPER"
        ? `No active claim overlaps this account. Starting simulated cash may be up to ${money(claim.currency, unitsDecimal(claim.capacity))}.`
        : `No active claim overlaps the exact ${money(claim.currency, unitsDecimal(claim.shadowAmount ?? claim.capacity))} Shadow reservation.`,
    );
  } else if (claim.sharedCeiling === undefined) {
    capitalAvailability = readinessCheck(
      "ACCOUNT_CAPACITY",
      "Account reservation capacity",
      "BLOCKED",
      true,
      "Another strategy has an active account claim. This budget has no compatible shared account ceiling.",
    );
  } else {
    let reserved = BigInt(0);
    let compatible = true;
    for (const reservation of activeReservations) {
      const amount = decimalUnits(
        text(reservation, "reservation_amount", "ReservationAmount"),
      );
      const ceiling = decimalUnits(
        text(reservation, "account_allocation_limit", "AccountAllocationLimit"),
      );
      const currency = text(reservation, "currency", "Currency");
      if (
        amount === undefined ||
        ceiling === undefined ||
        ceiling !== claim.sharedCeiling ||
        currency !== claim.currency
      ) {
        compatible = false;
        break;
      }
      reserved += amount;
    }
    const headroom = claim.sharedCeiling - reserved;
    if (!compatible || headroom <= BigInt(0)) {
      capitalAvailability = readinessCheck(
        "ACCOUNT_CAPACITY",
        "Account reservation capacity",
        "BLOCKED",
        true,
        "Active account claims are exclusive, inconsistent, or have exhausted their exact shared ceiling.",
      );
    } else if (input.executionMode === "PAPER") {
      const maximum = headroom < claim.capacity ? headroom : claim.capacity;
      paperStartingCashLimit = unitsDecimal(maximum);
      capitalAvailability = readinessCheck(
        "ACCOUNT_CAPACITY",
        "Account reservation capacity",
        "READY",
        false,
        `Compatible shared headroom is ${money(claim.currency, unitsDecimal(headroom))}; this PAPER strategy may start with up to ${money(claim.currency, paperStartingCashLimit)}.`,
      );
    } else if ((claim.shadowAmount ?? claim.capacity) <= headroom) {
      capitalAvailability = readinessCheck(
        "ACCOUNT_CAPACITY",
        "Account reservation capacity",
        "READY",
        false,
        `${money(claim.currency, unitsDecimal(claim.shadowAmount ?? claim.capacity))} fits within ${money(claim.currency, unitsDecimal(headroom))} of exact shared headroom.`,
      );
    } else {
      capitalAvailability = readinessCheck(
        "ACCOUNT_CAPACITY",
        "Account reservation capacity",
        "BLOCKED",
        true,
        `This Shadow claim exceeds the account's ${money(claim.currency, unitsDecimal(headroom))} of remaining shared headroom.`,
      );
    }
  }
  checks.push(capitalAvailability);

  const scheduleEnabled = flag(input.scheduleConditions, "enabled", "Enabled");
  const interval = integer(
    input.scheduleConditions,
    "interval_minutes",
    "IntervalMinutes",
  );
  const session = text(input.scheduleConditions, "session", "Session");
  const expectedSession =
    input.automationType === "AI_AUTONOMOUS" &&
    input.financialProvider === "coinbase"
      ? "CONTINUOUS"
      : "US_EQUITIES_REGULAR";
  const autonomous =
    input.autonomyLevel === "FULL_AUTONOMOUS" ||
    input.autonomyLevel === "STRATEGY_AUTONOMOUS";
  const scheduleReady =
    scheduleEnabled &&
    interval >= 30 &&
    interval <= 1440 &&
    session === expectedSession;
  checks.push(
    readinessCheck(
      "AUTOMATIC_SCHEDULE",
      "Automatic evaluation schedule",
      !autonomous
        ? "NOT_APPLICABLE"
        : scheduleReady
          ? "READY"
          : "ACTION_REQUIRED",
      false,
      !autonomous
        ? "This mandate is configured for owner-invoked non-live evaluation."
        : scheduleReady
          ? `Every ${interval} minutes · ${session}. Browser presence is not required.`
          : `Configure a 30–1440 minute ${expectedSession} schedule before expecting autonomous cycles.`,
    ),
  );

  if (input.automationType === "AI_AUTONOMOUS") {
    const reconciliationReady =
      Boolean(input.reconciliation) &&
      text(input.reconciliation, "comparison_status", "ComparisonStatus") ===
        "MATCHED" &&
      text(input.reconciliation, "balances_status", "BalancesStatus") ===
        "READY" &&
      text(input.reconciliation, "positions_status", "PositionsStatus") ===
        "READY" &&
      text(input.reconciliation, "autonomy_signal", "AutonomySignal") ===
        "CLEAR" &&
      flag(
        input.reconciliation,
        "autonomy_enforcement_active",
        "AutonomyEnforcementActive",
      ) &&
      !flag(input.reconciliation, "blocks_new_actions", "BlocksNewActions") &&
      reconciliationFresh(input.reconciliation, input.observedAt);
    checks.push(
      readinessCheck(
        "BROKER_RECONCILIATION",
        "Broker reconciliation",
        reconciliationReady
          ? "READY"
          : input.reconciliationAvailable
            ? "ACTION_REQUIRED"
            : "UNAVAILABLE",
        false,
        reconciliationReady
          ? "The latest provider snapshot is complete, matched, clear, and less than 24 hours old."
          : input.reconciliationAvailable
            ? "Initialization may proceed, but every AI proposal remains held until a complete, fresh, matching provider snapshot is available."
            : "Reconciliation status could not be refreshed. Initialization does not bypass the evaluation-time reconciliation gate.",
      ),
    );
  } else {
    checks.push(
      readinessCheck(
        "BROKER_RECONCILIATION",
        "Broker reconciliation",
        "NOT_APPLICABLE",
        false,
        "The deterministic non-live adapter applies its own provider and risk checks at evaluation time.",
      ),
    );
  }

  checks.push(
    readinessCheck(
      "EXECUTION_BOUNDARY",
      "Execution boundary",
      input.executionMode === "PAPER" || input.executionMode === "SHADOW"
        ? "READY"
        : "BLOCKED",
      true,
      input.executionMode === "PAPER"
        ? "PAPER can only mutate simulated portfolio state. Broker order submission is unavailable."
        : input.executionMode === "SHADOW"
          ? "SHADOW records what Arbion would have attempted. Broker order submission is unavailable."
          : "LIVE and BACKTEST configurations cannot initialize in this release.",
    ),
  );

  checks.push(
    readinessCheck(
      "INSTANCE_IDENTITY",
      "Strategy instance identity",
      !instanceID || activeInstance ? "READY" : "BLOCKED",
      Boolean(instanceID),
      !instanceID
        ? "No instance exists for this mandate version; initialization can create one durable non-live identity."
        : activeInstance
          ? `${instanceStatus} instance ${text(input.instance, "execution_mode", "ExecutionMode") || input.executionMode} is already pinned to immutable mandate evidence.`
          : `This version already produced a ${instanceStatus || "historical"} instance. Create and review a new version before initializing again.`,
    ),
  );

  if (activeInstance) {
    return {
      status: "ALREADY_INITIALIZED",
      eligible: false,
      paperStartingCashLimit,
      checks,
    };
  }
  if (versionAlreadyUsed) {
    return {
      status: "VERSION_ALREADY_USED",
      eligible: false,
      paperStartingCashLimit,
      checks,
    };
  }
  if (!input.inventoryAvailable) {
    return {
      status: "CURRENT_STATE_UNAVAILABLE",
      eligible: false,
      paperStartingCashLimit,
      checks,
    };
  }
  const blocked = checks.some(
    (check) => check.blocking && check.state !== "READY",
  );
  return {
    status: blocked ? "BLOCKED" : "READY_TO_INITIALIZE",
    eligible: !blocked,
    paperStartingCashLimit,
    checks,
  };
}

function stateLabel(state: CheckState) {
  if (state === "READY") return "Verified";
  if (state === "BLOCKED") return "Blocked";
  if (state === "ACTION_REQUIRED") return "Next step";
  if (state === "NOT_APPLICABLE") return "Not needed";
  return "Unavailable";
}

function statusLabel(status: StrategyInitializationAssessment["status"]) {
  if (status === "READY_TO_INITIALIZE") return "READY TO INITIALIZE";
  if (status === "ALREADY_INITIALIZED") return "ALREADY INITIALIZED";
  if (status === "VERSION_ALREADY_USED") return "NEW VERSION REQUIRED";
  if (status === "CURRENT_STATE_UNAVAILABLE") return "REFRESH REQUIRED";
  return "BLOCKED";
}

export function StrategyInitializationReadiness({
  assessment,
}: {
  assessment: StrategyInitializationAssessment;
}) {
  const verified = assessment.checks.filter(
    (check) => check.state === "READY" || check.state === "NOT_APPLICABLE",
  ).length;
  const followUps = assessment.checks.filter(
    (check) => check.state === "ACTION_REQUIRED",
  ).length;

  return (
    <section
      className="strategy-initialization-readiness"
      aria-labelledby="strategy-initialization-readiness-heading"
    >
      <header>
        <div>
          <p className="eyebrow">STRATEGY READINESS CHECK</p>
          <h2 id="strategy-initialization-readiness-heading">
            Know every blocker before initialization.
          </h2>
          <p>
            Arbion checks the exact saved version, connected services, capital
            claim, and non-live boundary before creating a strategy instance.
          </p>
        </div>
        <div
          className={`strategy-initialization-score is-${assessment.status.toLowerCase()}`}
          aria-label="Initialization status"
        >
          <span>{statusLabel(assessment.status)}</span>
          <strong>
            {verified}/{assessment.checks.length}
          </strong>
          <small>
            {followUps > 0
              ? `${followUps} non-blocking next ${followUps === 1 ? "step" : "steps"}`
              : "current checks complete"}
          </small>
        </div>
      </header>

      <div className="strategy-initialization-grid">
        {assessment.checks.map((check) => (
          <article
            className={`is-${check.state.toLowerCase().replace("_", "-")}`}
            key={check.code}
          >
            <span>{stateLabel(check.state)}</span>
            <h3>{check.label}</h3>
            <p>{check.detail}</p>
          </article>
        ))}
      </div>

      <footer>
        <strong>Final admission is atomic</strong>
        <p>
          This is a read-only snapshot. Initialization rechecks the exact
          mandate and capital inventory inside the database transaction. It
          creates no broker hold, order preview, approval, or live authority.
        </p>
      </footer>
    </section>
  );
}
