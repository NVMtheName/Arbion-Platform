"use client";

import { useEffect, useRef, useState } from "react";
import {
  CommissioningClientError,
  type CommissioningClient,
} from "./commissioning-client";
import {
  type CommissioningReview,
  type CommissioningReceipt,
  type CommissioningConsent,
} from "./commissioning-contract";
import styles from "./owner-execution.module.css";

type Uncertain = "prepare" | "approve" | "revoke" | null;

export function CommissioningWorkspace({
  review,
  client,
}: {
  review: CommissioningReview;
  client: CommissioningClient;
}) {
  const [receipt, setReceipt] = useState<CommissioningReceipt | null>(null);
  const [consent, setConsent] = useState<CommissioningConsent | null>(null);
  const [code, setCode] = useState("");
  const [busy, setBusy] = useState(false);
  const [uncertain, setUncertain] = useState<Uncertain>(null);
  const [message, setMessage] = useState("");
  const [observedAt, setObservedAt] = useState(() => Date.now());
  const locked = useRef(false),
    alive = useRef(true),
    uncertainty = useRef<Uncertain>(null);
  useEffect(() => {
    alive.current = true;
    return () => {
      alive.current = false;
    };
  }, []);
  const expired = Date.parse(review.expires_at) <= observedAt;

  async function action(kind: "read" | "prepare" | "approve" | "revoke") {
    if (
      locked.current ||
      !alive.current ||
      (kind !== "read" && uncertainty.current !== null)
    )
      return;
    const now = Date.now();
    setObservedAt(now);
    const termsExpired = Date.parse(review.expires_at) <= now;
    if (kind === "prepare" && (receipt || termsExpired)) return;
    if (
      kind === "approve" &&
      (!receipt || consent || !/^\d{6}$/.test(code) || termsExpired)
    )
      return;
    if (kind === "revoke" && (!consent || consent.revoked_at !== null)) return;
    locked.current = true;
    setBusy(true);
    setMessage("");
    // Never retain the factor after dispatch, including network failures.
    const submittedCode = code;
    setCode("");
    try {
      if (kind === "prepare") {
        const saved = await client.prepare();
        if (!alive.current) return;
        setReceipt(saved);
        setMessage(
          "Exact setup saved. It does not grant consent or start trading.",
        );
      } else if (kind === "read") {
        const saved = await client.readReceipt();
        if (!alive.current) return;
        setReceipt(saved);
        if (uncertainty.current === "prepare") {
          uncertainty.current = null;
          setUncertain(null);
        }
        try {
          const recorded = await client.readConsent();
          if (!alive.current) return;
          setConsent(recorded);
          if (
            uncertainty.current === "approve" ||
            (uncertainty.current === "revoke" && recorded.revoked_at !== null)
          ) {
            uncertainty.current = null;
            setUncertain(null);
          }
          setMessage(
            "Saved history recovered. This is not proof of current trading authorization.",
          );
        } catch (error) {
          if (
            !(error instanceof CommissioningClientError) ||
            error.code !== "EXECUTION_NOT_FOUND"
          )
            throw error;
          if (alive.current)
            setMessage(
              "No consent receipt was found. An unconfirmed earlier request may still finish; do not repeat it.",
            );
        }
      } else {
        const recorded =
          kind === "approve"
            ? await client.approve(submittedCode)
            : await client.revoke();
        if (!alive.current) return;
        setConsent(recorded);
        setMessage(
          kind === "approve"
            ? "Consent receipt recorded. No execution is started by this screen."
            : "Consent revocation recorded. Existing orders would still require reconciliation.",
        );
      }
    } catch (error) {
      if (!alive.current) return;
      if (kind !== "read") {
        uncertainty.current = kind;
        setUncertain(kind);
      }
      setMessage(
        error instanceof CommissioningClientError
          ? error.message
          : "The outcome is unconfirmed. Read saved status; do not repeat the action.",
      );
    } finally {
      if (alive.current) {
        locked.current = false;
        setBusy(false);
        setCode("");
      }
    }
  }

  return (
    <section className={styles.workspace} aria-label="Coinbase pilot setup">
      <header>
        <h2>Review personal Coinbase pilot</h2>
        <p>
          {review.account_label} · {review.product_id}
        </p>
        <p className={styles.notice}>
          These are exact server-selected terms. Saving them and recording
          consent are separate steps. This screen does not submit orders or
          start trading.
        </p>
      </header>
      <dl className={styles.facts}>
        {[
          ["Allocated cash (USD)", review.allocation_usd],
          ["Maximum order including fees (USD)", review.maximum_order_usd],
          ["Maximum deployed capital (USD)", review.max_capital_deployed_usd],
          ["Maximum single position (USD)", review.max_single_position_usd],
          ["Minimum cash reserve (USD)", review.minimum_cash_reserve_usd],
          ["Maximum trades per day", String(review.max_trades_per_day)],
          ["Evaluation interval (minutes)", String(review.interval_minutes)],
          ["Starts (UTC)", review.effective_from],
          ["Pilot expires (UTC)", review.expires_at],
          ["Pinned AI model", review.model_id],
          ["Objective", review.objective],
        ].map(([label, value]) => (
          <div key={label}>
            <dt>{label}</dt>
            <dd>{value}</dd>
          </div>
        ))}
      </dl>
      <p>
        Spot only, one USD pair, price-bounded orders and one unresolved order
        at a time. Existing holdings are not the pilot’s starting capital. Tests
        and recorded consent do not establish profitability.
      </p>
      {expired && (
        <p role="status">
          These pilot terms have expired. Saved history can still be read.
        </p>
      )}
      {message && <p role="status">{message}</p>}
      {uncertain && (
        <p role="alert">
          An earlier{" "}
          {uncertain === "prepare"
            ? "setup"
            : uncertain === "approve"
              ? "consent"
              : "revocation"}{" "}
          outcome remains unconfirmed. Only read saved status; a missing receipt
          does not permit retry.
        </p>
      )}
      <div className={styles.actions}>
        <button
          disabled={busy || !!receipt || !!uncertain || expired}
          onClick={() => void action("prepare")}
        >
          Save exact pilot setup
        </button>
        <button disabled={busy} onClick={() => void action("read")}>
          Read saved setup and consent
        </button>
      </div>
      {receipt && (
        <section className={styles.saved}>
          <h3>Exact setup saved</h3>
          <p>
            The original terms are immutable. Reloading cannot extend their
            expiry.
          </p>
          {!consent && (
            <form
              onSubmit={(event) => {
                event.preventDefault();
                void action("approve");
              }}
            >
              <label>
                Fresh setup MFA code
                <input
                  autoComplete="off"
                  inputMode="numeric"
                  type="password"
                  maxLength={6}
                  value={code}
                  disabled={busy || !!uncertain || expired}
                  onChange={(event) => setCode(event.target.value)}
                />
              </label>
              <p>
                Use a fresh authenticator code generated after saving this
                setup.
              </p>
              <button
                disabled={
                  busy || !!uncertain || expired || !/^\d{6}$/.test(code)
                }
              >
                Record consent to exact terms
              </button>
            </form>
          )}
        </section>
      )}
      {consent && (
        <section className={styles.saved}>
          <h3>
            {consent.revoked_at
              ? "Consent revoked"
              : "Historical consent receipt"}
          </h3>
          <p>
            This receipt is saved history, not current authorization or proof
            that live trading is running. The engine must still verify current
            controls for every action.
          </p>
          <dl className={styles.facts}>
            <div>
              <dt>Recorded (UTC)</dt>
              <dd>{consent.approved_at}</dd>
            </div>
            <div>
              <dt>Expires (UTC)</dt>
              <dd>{consent.expires_at}</dd>
            </div>
            {consent.revoked_at && (
              <div>
                <dt>Revoked (UTC)</dt>
                <dd>{consent.revoked_at}</dd>
              </div>
            )}
          </dl>
          <button
            disabled={busy || !!uncertain || consent.revoked_at !== null}
            onClick={() => void action("revoke")}
          >
            Revoke recorded consent
          </button>
        </section>
      )}
    </section>
  );
}
