import Link from "next/link";

import {
  CapitalBucketForm,
  type CapitalAccountOption,
} from "../automations/capital-bucket-form";

export type CapitalAccount = CapitalAccountOption & {
  provider: string;
  currency: string;
};

export type CapitalBucketView = {
  id: string;
  financialAccountID: string;
  name: string;
  allocationType: string;
  allocationValue: string;
  currency: string;
  protectedAmount: string;
  allocationLimit?: string;
  isReserve: boolean;
  status: string;
};

export type CapitalReservationView = {
  id: string;
  strategyInstanceID: string;
  financialAccountID: string;
  capitalBucketID: string;
  executionMode: string;
  reservationAmount?: string;
  currency: string;
  reservationBasis: string;
  accountAllocationLimit?: string;
  status: string;
  reservedAt: string;
  releasedAt?: string;
};

export type CapitalStrategyView = {
  id: string;
  automationMandateID: string;
  status: string;
  currentState: string;
};

type Props = {
  accounts: CapitalAccount[];
  buckets: CapitalBucketView[];
  reservations: CapitalReservationView[];
  strategies: CapitalStrategyView[];
  inventoryAvailable?: boolean;
};

const decimalScale = BigInt("10000000000");

function decimalUnits(value: string | undefined) {
  if (!value || !/^\d+(\.\d{1,10})?$/.test(value)) return undefined;
  const [whole, fraction = ""] = value.split(".");
  return BigInt(whole) * decimalScale + BigInt(fraction.padEnd(10, "0"));
}

function unitsDecimal(value: bigint) {
  const sign = value < BigInt(0) ? "-" : "";
  const absolute = value < BigInt(0) ? -value : value;
  const whole = absolute / decimalScale;
  const fraction = String(absolute % decimalScale)
    .padStart(10, "0")
    .replace(/0+$/, "");
  return `${sign}${whole}${fraction ? `.${fraction}` : ""}`;
}

function exactSum(values: Array<string | undefined>) {
  const parsed = values.map(decimalUnits);
  if (parsed.some((value) => value === undefined)) return undefined;
  const units = parsed.reduce<bigint>(
    (total, value) => total + (value ?? BigInt(0)),
    BigInt(0),
  );
  return unitsDecimal(units);
}

function exactDifference(left: string, right: string) {
  const leftUnits = decimalUnits(left);
  const rightUnits = decimalUnits(right);
  if (leftUnits === undefined || rightUnits === undefined) return undefined;
  return unitsDecimal(leftUnits - rightUnits);
}

function conciseDecimal(value: string) {
  if (!/^-?\d+(\.\d+)?$/.test(value)) return value;
  const [whole, fraction = ""] = value.split(".");
  const compact = fraction.replace(/0+$/, "");
  return compact ? `${whole}.${compact}` : whole;
}

function money(currency: string, value: string | undefined) {
  if (!value) return "Unavailable";
  const compact = conciseDecimal(value);
  const [signedWhole, fraction] = compact.split(".");
  const negative = signedWhole.startsWith("-");
  const whole = negative ? signedWhole.slice(1) : signedWhole;
  const grouped = whole.replace(/\B(?=(\d{3})+(?!\d))/g, ",");
  const amount = `${negative ? "-" : ""}${grouped}${fraction ? `.${fraction}` : ""}`;
  return currency.trim() === "USD"
    ? `$${amount}`
    : `${currency.trim()} ${amount}`;
}

function allocationLabel(bucket: CapitalBucketView) {
  if (bucket.allocationType === "PERCENT_OF_AVAILABLE_CASH")
    return `${conciseDecimal(bucket.allocationValue)}% of available cash`;
  if (bucket.allocationType === "PERCENT_OF_BUYING_POWER")
    return `${conciseDecimal(bucket.allocationValue)}% of buying power`;
  return money(bucket.currency, bucket.allocationValue);
}

function reservationBasisLabel(value: string) {
  if (value === "PAPER_STARTING_CASH") return "Paper starting cash";
  if (value === "BUCKET_FIXED_CAPACITY") return "Fixed budget capacity";
  if (value === "BUCKET_ABSOLUTE_LIMIT") return "Percentage budget cap";
  return "Legacy policy evidence";
}

function accountTitle(provider: string) {
  if (provider === "coinbase") return "Coinbase";
  if (provider === "schwab") return "Charles Schwab";
  return provider || "Connected account";
}

function BudgetCard({
  bucket,
  reservation,
  strategy,
}: {
  bucket: CapitalBucketView;
  reservation?: CapitalReservationView;
  strategy?: CapitalStrategyView;
}) {
  const protectedValue = decimalUnits(bucket.protectedAmount) ?? BigInt(0);
  const isProtected = protectedValue > BigInt(0) || bucket.isReserve;
  const active = reservation?.status === "ACTIVE";
  const paper = active && reservation?.executionMode === "PAPER";
  const claimLabel = bucket.isReserve
    ? "Protected reserve"
    : active
      ? paper
        ? "Used by an active Paper simulation"
        : "Reserved by an active strategy"
      : bucket.status === "ACTIVE"
        ? "Available for a strategy draft"
        : "Archived policy";

  return (
    <article
      className={`capital-budget-card${active ? " is-reserved" : ""}${bucket.isReserve ? " is-reserve" : ""}`}
    >
      <header>
        <div>
          <p className="eyebrow">
            {bucket.isReserve ? "NEVER DEPLOY" : "TRADING BUDGET"}
          </p>
          <h3>{bucket.name}</h3>
        </div>
        <span>{bucket.status}</span>
      </header>
      <div className="capital-budget-amount">
        <strong>{allocationLabel(bucket)}</strong>
        <span>{claimLabel}</span>
      </div>
      <dl>
        <div>
          <dt>Protected</dt>
          <dd>{money(bucket.currency, bucket.protectedAmount)}</dd>
        </div>
        <div>
          <dt>Account ceiling</dt>
          <dd>
            {bucket.isReserve
              ? "Never attachable"
              : paper
                ? "Simulation-only"
                : bucket.allocationLimit
                  ? money(bucket.currency, bucket.allocationLimit)
                  : "Exclusive when active"}
          </dd>
        </div>
        <div>
          <dt>Strategy claim</dt>
          <dd>
            {active
              ? money(reservation.currency, reservation.reservationAmount)
              : "None"}
          </dd>
        </div>
      </dl>
      {active && reservation && (
        <div className="capital-reservation-callout">
          <span>
            {reservation.executionMode} · {strategy?.status ?? "ACTIVE"}
          </span>
          <strong>{reservationBasisLabel(reservation.reservationBasis)}</strong>
          <small>
            Pausing retains this reservation. Finishing the strategy releases it
            without deleting the evidence.
          </small>
          {strategy?.automationMandateID && (
            <Link href={`/automations/${strategy.automationMandateID}`}>
              Open {bucket.name} strategy →
            </Link>
          )}
        </div>
      )}
      {!active && bucket.status === "ACTIVE" && !bucket.isReserve && (
        <footer>
          <span>
            {isProtected
              ? "Protected capital remains outside the claim."
              : "No capital is currently claimed."}
          </span>
          <Link href="/automations/new">Use in a non-live draft →</Link>
        </footer>
      )}
    </article>
  );
}

export function CapitalBudgetCenter({
  accounts,
  buckets,
  reservations,
  strategies,
  inventoryAvailable = true,
}: Props) {
  const strategyByID = new Map(strategies.map((item) => [item.id, item]));
  const activeReservationByBucket = new Map(
    reservations
      .filter((item) => item.status === "ACTIVE")
      .map((item) => [item.capitalBucketID, item]),
  );
  const activeAccounts = accounts.filter(
    (account) => account.status === "active",
  );
  const activeBuckets = buckets.filter((bucket) => bucket.status === "ACTIVE");
  const archivedBuckets = buckets.filter(
    (bucket) => bucket.status !== "ACTIVE",
  );
  const activeReservations = reservations.filter(
    (reservation) => reservation.status === "ACTIVE",
  );

  if (!inventoryAvailable) {
    return (
      <section className="capital-center-unavailable" role="alert">
        <h2>Capital safeguards could not be refreshed.</h2>
        <p>
          Arbion is not showing partial totals. Existing strategy reservations
          remain enforced in the database and no broker action is available.
        </p>
      </section>
    );
  }

  return (
    <>
      <section className="capital-center-summary" aria-label="Capital summary">
        <article>
          <span>Connected accounts</span>
          <strong>{activeAccounts.length}</strong>
          <small>separate policy boundaries</small>
        </article>
        <article>
          <span>Active budgets</span>
          <strong>{activeBuckets.length}</strong>
          <small>owner-defined limits</small>
        </article>
        <article>
          <span>Strategy reservations</span>
          <strong>{activeReservations.length}</strong>
          <small>Paper or Shadow only</small>
        </article>
        <article>
          <span>Broker authority</span>
          <strong>None</strong>
          <small>policy ledger only</small>
        </article>
      </section>

      {activeAccounts.length === 0 ? (
        <section className="capital-center-empty">
          <h2>Connect an account before assigning capital.</h2>
          <p>
            Arbion creates a separate policy boundary for every connected
            account. It never moves or locks provider funds.
          </p>
          <Link className="button-link" href="/connections#financial-accounts">
            Connect an account
          </Link>
        </section>
      ) : (
        <div className="capital-account-list">
          {activeAccounts.map((account) => {
            const accountBuckets = activeBuckets.filter(
              (bucket) => bucket.financialAccountID === account.id,
            );
            const accountReservations = activeReservations.filter(
              (reservation) => reservation.financialAccountID === account.id,
            );
            const shadowReservations = accountReservations.filter(
              (reservation) => reservation.executionMode === "SHADOW",
            );
            const paperReservations = accountReservations.filter(
              (reservation) => reservation.executionMode === "PAPER",
            );
            const currencyMismatch =
              account.currency.length !== 3 ||
              accountBuckets.some(
                (bucket) => bucket.currency !== account.currency,
              ) ||
              accountReservations.some(
                (reservation) => reservation.currency !== account.currency,
              );
            const fixedDefined = exactSum(
              accountBuckets
                .filter((bucket) => bucket.allocationType === "FIXED_AMOUNT")
                .map((bucket) => bucket.allocationValue),
            );
            const shadowReserved = exactSum(
              shadowReservations.map(
                (reservation) => reservation.reservationAmount,
              ),
            );
            const paperStartingCash = exactSum(
              paperReservations.map(
                (reservation) => reservation.reservationAmount,
              ),
            );
            const limits = shadowReservations.map(
              (reservation) => reservation.accountAllocationLimit,
            );
            const invalidAggregate =
              fixedDefined === undefined ||
              shadowReserved === undefined ||
              paperStartingCash === undefined;
            const sharedCeiling =
              !currencyMismatch &&
              !invalidAggregate &&
              limits.length > 0 &&
              limits.every(
                (limit) =>
                  limit &&
                  decimalUnits(limit) !== undefined &&
                  limit === limits[0],
              )
                ? limits[0]
                : undefined;
            const headroom =
              sharedCeiling && shadowReserved
                ? exactDifference(sharedCeiling, shadowReserved)
                : undefined;
            const policyState = currencyMismatch
              ? "Currency inventory mismatch"
              : invalidAggregate
                ? "Capital inventory invalid"
                : accountReservations.length === 0
                  ? "No active claims"
                  : sharedCeiling
                    ? "Shared Shadow ceiling active"
                    : shadowReservations.length > 0
                      ? "Exclusive Shadow claim"
                      : "Paper simulation active";

            return (
              <section className="capital-account" key={account.id}>
                <header>
                  <div className="capital-account-identity">
                    <span
                      className={`provider-mark provider-${account.provider}`}
                    >
                      {account.provider === "coinbase" ? "C" : "S"}
                    </span>
                    <div>
                      <p className="eyebrow">
                        {accountTitle(account.provider)}
                      </p>
                      <h2>{account.display_name}</h2>
                    </div>
                  </div>
                  <span className={sharedCeiling ? "is-shareable" : ""}>
                    {policyState}
                  </span>
                </header>
                <div className="capital-account-metrics">
                  <div>
                    <span>Fixed budgets defined</span>
                    <strong>
                      {currencyMismatch || invalidAggregate
                        ? "Unavailable"
                        : money(account.currency, fixedDefined)}
                    </strong>
                  </div>
                  <div>
                    <span>Shadow capital reserved</span>
                    <strong>
                      {currencyMismatch || invalidAggregate
                        ? "Unavailable"
                        : money(account.currency, shadowReserved)}
                    </strong>
                  </div>
                  <div>
                    <span>Paper starting cash</span>
                    <strong>
                      {currencyMismatch || invalidAggregate
                        ? "Unavailable"
                        : money(account.currency, paperStartingCash)}
                    </strong>
                  </div>
                  <div>
                    <span>Shared Shadow ceiling</span>
                    <strong>
                      {sharedCeiling
                        ? money(account.currency, sharedCeiling)
                        : "Not configured"}
                    </strong>
                  </div>
                  <div>
                    <span>Unreserved Shadow headroom</span>
                    <strong>
                      {headroom === undefined
                        ? "Not applicable"
                        : money(account.currency, headroom)}
                    </strong>
                  </div>
                </div>
                {currencyMismatch && (
                  <p className="capital-account-policy-note is-warning">
                    Arbion found policies in more than one currency and will not
                    combine them into an account total. Each policy remains
                    visible below with its recorded currency.
                  </p>
                )}
                {!currencyMismatch && invalidAggregate && (
                  <p className="capital-account-policy-note is-warning">
                    Arbion found incomplete exact-decimal policy evidence and
                    will not calculate account totals. The durable records
                    remain visible below for review.
                  </p>
                )}
                {!currencyMismatch &&
                  !invalidAggregate &&
                  shadowReservations.length > 0 &&
                  !sharedCeiling && (
                    <p className="capital-account-policy-note">
                      Shadow authority stays exclusive until its running
                      strategy finishes. Isolated Paper simulations may run in
                      parallel because they cannot consume broker cash or send
                      an order.
                    </p>
                  )}
                {!currencyMismatch &&
                  !invalidAggregate &&
                  paperReservations.length > 0 && (
                    <p className="capital-account-policy-note">
                      Paper starting cash is simulated and excluded from Shadow
                      reserved totals and shared headroom.
                    </p>
                  )}
                {accountBuckets.length > 0 ? (
                  <div className="capital-budget-grid">
                    {accountBuckets.map((bucket) => {
                      const reservation = activeReservationByBucket.get(
                        bucket.id,
                      );
                      return (
                        <BudgetCard
                          bucket={bucket}
                          key={bucket.id}
                          reservation={reservation}
                          strategy={
                            reservation
                              ? strategyByID.get(reservation.strategyInstanceID)
                              : undefined
                          }
                        />
                      );
                    })}
                  </div>
                ) : (
                  <div className="capital-account-empty">
                    <strong>
                      No budget has been defined for this account.
                    </strong>
                    <span>Create one below before building a strategy.</span>
                  </div>
                )}
              </section>
            );
          })}
        </div>
      )}

      {activeAccounts.length > 0 && (
        <section className="capital-create-panel">
          <header>
            <div>
              <p className="eyebrow">NEW POLICY BOUNDARY</p>
              <h2>Create a trading budget</h2>
            </div>
            <p>
              Use a shared account ceiling only when you intentionally want
              compatible non-live strategies to divide one exact total.
            </p>
          </header>
          <CapitalBucketForm accounts={accounts} />
        </section>
      )}

      {archivedBuckets.length > 0 && (
        <details className="capital-archive">
          <summary>{archivedBuckets.length} archived capital policies</summary>
          <ul>
            {archivedBuckets.map((bucket) => (
              <li key={bucket.id}>
                <span>{bucket.name}</span>
                <strong>{allocationLabel(bucket)}</strong>
              </li>
            ))}
          </ul>
        </details>
      )}

      <section
        className="capital-center-boundary"
        aria-label="Execution boundary"
      >
        <strong>Arbion policy ledger—not broker cash</strong>
        <p>
          Shadow reservations prevent overlapping account authority. Paper
          starting cash remains inside its isolated simulated ledger. Neither
          transfers, freezes, or earmarks funds at Coinbase or Schwab, and
          neither grants order or live-execution permission.
        </p>
      </section>
    </>
  );
}
