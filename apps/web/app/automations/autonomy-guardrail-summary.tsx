type RiskPolicy = Record<string, unknown>;

function read(policy: RiskPolicy, key: string, legacy: string) {
  return policy[key] ?? policy[legacy];
}

function decimal(value: unknown) {
  if (typeof value !== "string" || !/^\d+(\.\d+)?$/.test(value)) return null;
  const [whole, fraction = ""] = value.split(".");
  const trimmed = fraction.replace(/0+$/, "");
  return trimmed ? `${whole}.${trimmed}` : whole;
}

function dollars(value: unknown, fallback: string) {
  const amount = decimal(value);
  return amount === null ? fallback : `$${amount}`;
}

export function AutonomyGuardrailSummary({
  riskPolicy = {},
  executionMode,
}: {
  riskPolicy?: RiskPolicy;
  executionMode: string;
}) {
  const actions = Number(
    read(riskPolicy, "max_trades_per_day", "MaxTradesPerDay") ?? 0,
  );
  const concentration = decimal(
    read(
      riskPolicy,
      "max_single_position_percentage",
      "MaxSinglePositionPercentage",
    ),
  );

  return (
    <section
      className="autonomy-guardrail-summary"
      aria-label="Autonomy guardrails"
    >
      <header>
        <div>
          <p className="eyebrow">DETERMINISTIC CONTROL ENVELOPE</p>
          <h2>Limits the model cannot change</h2>
        </div>
        <span>SERVER ENFORCED</span>
      </header>
      <p>
        Every proposal is checked against this immutable mandate version after
        the AI responds and before{" "}
        {executionMode === "PAPER"
          ? "a simulated result or immutable abstention"
          : "Shadow evidence"}{" "}
        can be recorded.
      </p>
      <dl>
        <div>
          <dt>Daily action ceiling</dt>
          <dd>{actions > 0 ? actions : "Legacy mandate"}</dd>
          <small>Per mandate · UTC day · abstentions excluded</small>
        </div>
        <div>
          <dt>Minimum cash reserve</dt>
          <dd>
            {dollars(
              read(riskPolicy, "minimum_cash_reserve", "MinimumCashReserve"),
              "Bucket policy",
            )}
          </dd>
          <small>Checked against current available cash</small>
        </div>
        <div>
          <dt>Total exposure ceiling</dt>
          <dd>
            {dollars(
              read(riskPolicy, "max_capital_deployed", "MaxCapitalDeployed"),
              "Bucket policy",
            )}
          </dd>
          <small>
            {executionMode === "PAPER"
              ? "Simulated portfolio exposure after the proposal"
              : "Connected-account exposure after the proposal"}
          </small>
        </div>
        <div>
          <dt>One-symbol exposure</dt>
          <dd>
            {dollars(
              read(
                riskPolicy,
                "max_single_position_amount",
                "MaxSinglePositionAmount",
              ),
              "No extra cap",
            )}
          </dd>
          <small>
            {concentration === null
              ? "No additional concentration ceiling"
              : `${concentration}% concentration ceiling`}
          </small>
        </div>
      </dl>
      <p className="security-note">
        {executionMode === "PAPER"
          ? "Daily realized-loss enforcement is not enabled for this PAPER mandate. The isolated simulated cash and position ledger, account budget, circuit breaker, allowlist, repeat-action cooldown, and the limits above remain authoritative."
          : "Daily realized-loss enforcement is intentionally unavailable for this SHADOW mandate because Arbion does not infer broker profit and loss. The account budget, circuit breaker, allowlist, repeat-action cooldown, and the limits above remain authoritative."}
      </p>
    </section>
  );
}
