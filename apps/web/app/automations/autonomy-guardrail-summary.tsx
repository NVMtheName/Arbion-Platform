import {
  calculatePaperPerformance,
  type PaperMarketSnapshot,
} from "./paper-performance";
import type { PaperPortfolio } from "./paper-portfolio-summary";
import {
  compareExactDecimals,
  formatExactMoney,
  subtractExactDecimals,
} from "../exact-money";

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

function exactString(policy: RiskPolicy, key: string, legacy: string) {
  const value = read(policy, key, legacy);
  return typeof value === "string" && decimal(value) !== null
    ? value
    : undefined;
}

function money(value: string | undefined, currency: string) {
  return formatExactMoney(value ? { amount: value, currency } : undefined, {
    maximumFractionDigits: 4,
  });
}

function within(value: string | undefined, ceiling: string | undefined) {
  if (!value || !ceiling) return false;
  const comparison = compareExactDecimals(value, ceiling);
  return comparison !== undefined && comparison <= 0;
}

function atLeast(value: string | undefined, floor: string | undefined) {
  if (!value || !floor) return false;
  const comparison = compareExactDecimals(value, floor);
  return comparison !== undefined && comparison >= 0;
}

function largestExposure(
  positions: Array<{ symbol: string; marketValue: string }>,
) {
  return positions.reduce<(typeof positions)[number] | undefined>(
    (largest, position) =>
      !largest ||
      compareExactDecimals(position.marketValue, largest.marketValue) === 1
        ? position
        : largest,
    undefined,
  );
}

export function AutonomyGuardrailSummary({
  riskPolicy = {},
  strategyPolicy = {},
  executionMode,
  paperPortfolio,
  paperMarkets = [],
}: {
  riskPolicy?: RiskPolicy;
  strategyPolicy?: RiskPolicy;
  executionMode: string;
  paperPortfolio?: PaperPortfolio;
  paperMarkets?: PaperMarketSnapshot[];
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
  const paperPerformance = paperPortfolio
    ? calculatePaperPerformance(paperPortfolio, paperMarkets)
    : undefined;
  const cashFloor = exactString(
    riskPolicy,
    "minimum_cash_reserve",
    "MinimumCashReserve",
  );
  const exposureCeiling = exactString(
    riskPolicy,
    "max_capital_deployed",
    "MaxCapitalDeployed",
  );
  const symbolCeiling = exactString(
    riskPolicy,
    "max_single_position_amount",
    "MaxSinglePositionAmount",
  );
  const proposalCeiling = exactString(
    strategyPolicy,
    "max_proposal_notional",
    "MaxProposalNotional",
  );
  const largestPosition = largestExposure(paperPerformance?.positions ?? []);
  const paperEvidenceAvailable =
    executionMode === "PAPER" &&
    Boolean(paperPortfolio) &&
    paperPerformance?.status !== "UNAVAILABLE" &&
    Boolean(cashFloor && exposureCeiling && symbolCeiling && proposalCeiling);
  const cashWithin = atLeast(paperPortfolio?.cash, cashFloor);
  const totalWithin = within(
    paperPerformance?.investedExposure ??
      (paperPerformance?.status === "CASH_ONLY" ? "0" : undefined),
    exposureCeiling,
  );
  const symbolWithin = within(
    largestPosition?.marketValue ?? "0",
    symbolCeiling,
  );
  const paperWithin =
    paperEvidenceAvailable && cashWithin && totalWithin && symbolWithin;
  const cashHeadroom =
    paperPortfolio?.cash && cashFloor
      ? subtractExactDecimals(paperPortfolio.cash, cashFloor)
      : undefined;
  const exposureHeadroom =
    exposureCeiling && paperPerformance?.investedExposure
      ? subtractExactDecimals(
          exposureCeiling,
          paperPerformance.investedExposure,
        )
      : paperPerformance?.status === "CASH_ONLY"
        ? exposureCeiling
        : undefined;
  const symbolHeadroom = symbolCeiling
    ? subtractExactDecimals(symbolCeiling, largestPosition?.marketValue ?? "0")
    : undefined;

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
      {executionMode === "PAPER" && (
        <section
          className={`paper-capital-headroom ${paperEvidenceAvailable ? (paperWithin ? "is-within" : "is-breached") : "is-unavailable"}`}
          aria-label="Current Paper capital headroom"
        >
          <header>
            <div>
              <p className="eyebrow">CURRENT PAPER CAPITAL ENVELOPE</p>
              <h3>Exact simulated usage against immutable limits</h3>
            </div>
            <span>
              {!paperEvidenceAvailable
                ? "EVIDENCE UNAVAILABLE"
                : paperWithin
                  ? "WITHIN LIMITS"
                  : "LIMIT BREACH"}
            </span>
          </header>
          {!paperEvidenceAvailable ? (
            <p>
              Arbion is waiting for one complete immutable market snapshot and
              canonical cash, exposure, symbol, and proposal limits. No missing
              value is inferred.
            </p>
          ) : (
            <div className="paper-capital-headroom-grid">
              <article className={cashWithin ? "is-ready" : "is-blocked"}>
                <span>Simulated cash</span>
                <strong>
                  {money(paperPortfolio!.cash, paperPortfolio!.currency)}
                </strong>
                <small>
                  {money(cashHeadroom, paperPortfolio!.currency)} floor headroom
                  · {money(cashFloor, paperPortfolio!.currency)} reserve
                </small>
              </article>
              <article className={totalWithin ? "is-ready" : "is-blocked"}>
                <span>Total invested exposure</span>
                <strong>
                  {money(
                    paperPerformance!.investedExposure ?? "0",
                    paperPortfolio!.currency,
                  )}
                </strong>
                <small>
                  {money(exposureHeadroom, paperPortfolio!.currency)} ceiling
                  headroom · {money(exposureCeiling, paperPortfolio!.currency)}{" "}
                  limit
                </small>
              </article>
              <article className={symbolWithin ? "is-ready" : "is-blocked"}>
                <span>Largest symbol exposure</span>
                <strong>
                  {largestPosition
                    ? `${largestPosition.symbol} · ${money(largestPosition.marketValue, paperPortfolio!.currency)}`
                    : "No open positions"}
                </strong>
                <small>
                  {money(symbolHeadroom, paperPortfolio!.currency)} ceiling
                  headroom · {money(symbolCeiling, paperPortfolio!.currency)}{" "}
                  one-symbol limit
                </small>
              </article>
              <article className="is-ready">
                <span>Next AI proposal ceiling</span>
                <strong>
                  {money(proposalCeiling, paperPortfolio!.currency)}
                </strong>
                <small>
                  The model cannot increase this immutable per-decision limit
                </small>
              </article>
            </div>
          )}
          <p>
            Values come from Arbion&apos;s isolated Paper ledger and the latest
            complete immutable AI market snapshot. They are not broker balances
            or live quotes.
          </p>
        </section>
      )}
      <p className="security-note">
        {executionMode === "PAPER"
          ? "Daily realized-loss enforcement is not enabled for this PAPER mandate. The isolated simulated cash and position ledger, account budget, circuit breaker, allowlist, repeat-action cooldown, and the limits above remain authoritative."
          : "Daily realized-loss enforcement is intentionally unavailable for this SHADOW mandate because Arbion does not infer broker profit and loss. The account budget, circuit breaker, allowlist, repeat-action cooldown, and the limits above remain authoritative."}
      </p>
    </section>
  );
}
