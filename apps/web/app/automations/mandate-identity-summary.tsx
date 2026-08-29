import Link from "next/link";

type Entity = Record<string, unknown> | undefined;

function read(entity: Entity, key: string, legacy: string, fallback = "") {
  return String(entity?.[key] ?? entity?.[legacy] ?? fallback);
}

function humanize(value: string) {
  return value
    .toLowerCase()
    .split("_")
    .filter(Boolean)
    .map((part) => part.slice(0, 1).toUpperCase() + part.slice(1))
    .join(" ");
}

function providerLabel(provider: string) {
  if (provider.toLowerCase() === "schwab") return "Charles Schwab";
  if (provider.toLowerCase() === "coinbase") return "Coinbase";
  return humanize(provider) || "Connected account";
}

function strategyLabel(
  automationType: string,
  strategyIdentifier: string,
  executionMode: string,
) {
  if (automationType === "AI_AUTONOMOUS")
    return executionMode === "PAPER" ? "AI Paper Engine" : "AI Shadow Engine";
  return humanize(strategyIdentifier) || "Configured strategy";
}

function conciseDecimal(value: string) {
  if (!/^-?\d+(\.\d+)?$/.test(value)) return value;
  const [whole, fraction = ""] = value.split(".");
  const trimmedFraction = fraction.replace(/0+$/, "");
  return trimmedFraction ? `${whole}.${trimmedFraction}` : whole;
}

function allocationLabel(bucket: Entity) {
  const type = read(bucket, "allocation_type", "AllocationType");
  const value = read(bucket, "allocation_value", "AllocationValue");
  const currency = read(bucket, "currency", "Currency", "USD");
  if (!value) return "Protected by its saved allocation policy";
  if (type === "FIXED_AMOUNT") {
    const amount = conciseDecimal(value);
    return currency === "USD"
      ? `$${amount} fixed allocation`
      : `${currency} ${amount} fixed allocation`;
  }
  if (type === "PERCENT_OF_AVAILABLE_CASH") {
    return `${value}% of available cash`;
  }
  if (type === "PERCENT_OF_BUYING_POWER") {
    return `${value}% of buying power`;
  }
  return `${value} ${humanize(type).toLowerCase()}`;
}

function reference(id: string) {
  return id && id !== "—" ? id : "Not available";
}

export function MandateIdentitySummary({
  mandateId,
  automationType,
  financialAccountId,
  financialAccount,
  capitalBucketId,
  capitalBucket,
  strategyIdentifier,
  aiModelId,
  autonomyLevel,
  executionMode,
  status,
  currentVersion,
  strategyInstanceId,
}: {
  mandateId: string;
  automationType: string;
  financialAccountId: string;
  financialAccount?: Record<string, unknown>;
  capitalBucketId: string;
  capitalBucket?: Record<string, unknown>;
  strategyIdentifier: string;
  aiModelId: string;
  autonomyLevel: string;
  executionMode: string;
  status: string;
  currentVersion: number;
  strategyInstanceId?: string;
}) {
  const accountName = read(
    financialAccount,
    "display_name",
    "DisplayName",
    "Connected financial account",
  );
  const provider = providerLabel(
    read(financialAccount, "provider", "Provider"),
  );
  const bucketName = read(
    capitalBucket,
    "name",
    "Name",
    "Saved capital policy",
  );

  return (
    <>
      <section
        className="review-grid mandate-identity-summary"
        aria-label="Mandate identity and safeguards"
      >
        <article>
          <strong>Account</strong>
          {financialAccountId && financialAccountId !== "—" ? (
            <Link href={`/accounts/${financialAccountId}`}>{accountName}</Link>
          ) : (
            <span>{accountName}</span>
          )}
          <small>{provider}</small>
        </article>
        <article>
          <strong>Engine</strong>
          <span>
            {strategyLabel(automationType, strategyIdentifier, executionMode)}
          </span>
          <small>One immutable mandate version at a time</small>
        </article>
        <article>
          <strong>Capital guardrail</strong>
          <span>{bucketName}</span>
          <small>{allocationLabel(capitalBucket)}</small>
        </article>
        {automationType === "AI_AUTONOMOUS" && (
          <article>
            <strong>Decision model</strong>
            <span>{aiModelId || "Saved model"}</span>
            <small>Frozen into this mandate version</small>
          </article>
        )}
        <article>
          <strong>Autonomy</strong>
          <span>{humanize(autonomyLevel)}</span>
          <small>Bounded by the saved universe and risk policy</small>
        </article>
        <article>
          <strong>Execution</strong>
          <span>
            {executionMode === "SHADOW"
              ? "Shadow only"
              : executionMode === "PAPER"
                ? "Paper simulation"
                : humanize(executionMode)}
          </span>
          <small>
            {executionMode === "SHADOW"
              ? "No broker order can be sent"
              : executionMode === "PAPER"
                ? "Isolated simulated ledger; no broker order"
                : "Uses the configured non-live adapter"}
          </small>
        </article>
        <article>
          <strong>State</strong>
          <span>{humanize(status)}</span>
          <small>Mandate version {currentVersion}</small>
        </article>
      </section>
      <details className="mandate-technical-references">
        <summary>Technical references</summary>
        <dl>
          <div>
            <dt>Mandate</dt>
            <dd>{reference(mandateId)}</dd>
          </div>
          <div>
            <dt>Account</dt>
            <dd>{reference(financialAccountId)}</dd>
          </div>
          <div>
            <dt>Capital bucket</dt>
            <dd>{reference(capitalBucketId)}</dd>
          </div>
          <div>
            <dt>Strategy instance</dt>
            <dd>{reference(strategyInstanceId ?? "")}</dd>
          </div>
        </dl>
      </details>
    </>
  );
}
