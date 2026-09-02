"use client";

import Link from "next/link";

import type { FinancialInputChainProjection } from "../settings/connections/financial-continuity-center";

function providerLabel(provider: string) {
  if (provider === "coinbase") return "Coinbase";
  if (provider === "schwab") return "Charles Schwab";
  return provider || "Provider unavailable";
}

function providerInitial(provider: string) {
  if (provider === "schwab") return "S";
  if (provider === "coinbase") return "C";
  return providerLabel(provider).slice(0, 1).toUpperCase();
}

function readableTime(value: string) {
  const parsed = new Date(value);
  if (Number.isNaN(parsed.valueOf())) return "Unavailable";
  return new Intl.DateTimeFormat("en-US", {
    month: "short",
    day: "numeric",
    hour: "numeric",
    minute: "2-digit",
    timeZone: "UTC",
    timeZoneName: "short",
  }).format(parsed);
}

function stateLabel(
  state: FinancialInputChainProjection["engines"][number]["state"],
) {
  if (state === "CURRENT") return "Current";
  if (state === "WAITING_FOR_ACCOUNT_SYNC") return "Waiting for portfolio";
  if (state === "WAITING_FOR_EVALUATION") return "Waiting for evaluation";
  if (state === "SAFE_BLOCKED") return "Stopped safely";
  return "Evidence unavailable";
}

export function FinancialInputChainSummary({
  projection,
  available,
  scope = "fleet",
}: {
  projection?: FinancialInputChainProjection;
  available: boolean;
  scope?: "fleet" | "account";
}) {
  if (scope === "fleet" && available && projection?.engineCount === 0)
    return null;
  const empty = available && projection?.engineCount === 0;
  const status =
    !available || !projection ? "unavailable" : projection.status.toLowerCase();
  const headline =
    !available || !projection
      ? "Financial input status is unavailable."
      : empty
        ? "No active AI engine uses this account."
        : projection.status === "VERIFIED"
          ? scope === "account"
            ? "This account’s AI input chain is current."
            : "Every AI input chain is current."
          : `${projection.waitingCount + projection.blockedCount + projection.unavailableCount} AI input ${projection.waitingCount + projection.blockedCount + projection.unavailableCount === 1 ? "chain needs" : "chains need"} attention.`;

  return (
    <section
      className={`dashboard-input-chain is-${status}`}
      aria-labelledby="dashboard-input-chain-title"
    >
      <header>
        <div>
          <p className="command-kicker">FINANCIAL INPUT CHAIN</p>
          <h2 id="dashboard-input-chain-title">{headline}</h2>
          <p>
            {empty
              ? "Connect this account to a Paper or Shadow strategy when you are ready."
              : "Saved connection → portfolio → automatic evaluation, kept separate for every Paper and Shadow engine."}
          </p>
        </div>
        <Link href="/connections#financial-input-chain">
          Open connection evidence ↗
        </Link>
      </header>

      {!available || !projection ? (
        <div className="dashboard-input-chain-unavailable" role="status">
          <strong>Arbion will not infer a healthy chain.</strong>
          <p>
            Existing controls remain unchanged while this read-only saved
            evidence view recovers.
          </p>
        </div>
      ) : empty ? (
        <div className="dashboard-input-chain-empty" role="status">
          <strong>No trading activity was started.</strong>
          <p>
            This read-only checkpoint will appear automatically after an active
            Paper or Shadow engine is attached to the account.
          </p>
        </div>
      ) : (
        <ol>
          {projection.engines.map((engine) => {
            const attention = engine.state !== "CURRENT";
            return (
              <li
                className={`is-${engine.state.toLowerCase().replaceAll("_", "-")}`}
                key={engine.instanceID}
              >
                <details open={attention}>
                  <summary>
                    <span
                      className={`provider-mark provider-${engine.provider}`}
                      aria-hidden="true"
                    >
                      {providerInitial(engine.provider)}
                    </span>
                    <span>
                      <strong>{engine.accountName}</strong>
                      <small>
                        {providerLabel(engine.provider)} ·{" "}
                        {engine.executionMode}
                      </small>
                    </span>
                    <span className="dashboard-input-chain-state">
                      {stateLabel(engine.state)}
                    </span>
                    <span className="dashboard-input-chain-next">
                      Next {readableTime(engine.nextRunAt)}
                    </span>
                  </summary>
                  <div>
                    <p>{engine.guidance}</p>
                    <footer>
                      <span>
                        {engine.errorCode
                          ? `Saved stop: ${engine.errorCode}`
                          : engine.label}
                      </span>
                      <Link
                        href={
                          attention
                            ? "/connections#financial-input-chain"
                            : `/automations/${encodeURIComponent(engine.mandateID)}#runtime-evidence`
                        }
                      >
                        {attention
                          ? "Review input chain"
                          : "Open immutable evidence"}{" "}
                        →
                      </Link>
                    </footer>
                  </div>
                </details>
              </li>
            );
          })}
        </ol>
      )}

      <footer>
        Read-only saved evidence. Ordering does not prove provider cause,
        entitlement, decision quality, or live authority.
      </footer>
    </section>
  );
}
