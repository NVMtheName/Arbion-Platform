"use client";

import { useLayoutEffect, useRef, useState, type FormEvent } from "react";

import { compareExactDecimals } from "../exact-money";
import type { OwnerExecutionClient } from "./client";
import {
  parsePrepareCommand,
  type OwnerOrder,
  type OwnerPreflight,
  type PrepareCommand,
} from "./contract";
import styles from "./owner-execution.module.css";

type Props = {
  productID: string;
  accountLabel: string;
  client: OwnerExecutionClient;
};

const descriptions: Record<OwnerOrder["state"], string> = {
  PREPARED: "Prepared, not submitted. Review the exact terms before approval.",
  APPROVED:
    "Approval recorded. Current checks are still required before sending.",
  SUBMISSION_UNKNOWN:
    "Submission is unresolved. Do not send again. Recover the original order.",
  REJECTED_HELD:
    "Submission was rejected. Capital remains held for operator review; do not resend.",
  BROKER_ACKNOWLEDGED: "Order acknowledged. This does not prove a fill.",
  PARTIALLY_FILLED: "Verified fills are recorded. The order is not yet final.",
  AWAITING_ACCOUNT_SETTLEMENT:
    "Final order history is verified. Cash and position accounting is still required.",
  SETTLED:
    "Final order history and exact cash and position accounting are recorded.",
  NOT_SENT:
    "The sender was never entered. Original reservations were released; this attempt cannot be reused.",
  UNAVAILABLE:
    "Saved evidence is unavailable or inconsistent. Operator review is required.",
};
const amounts = [
  "base_size",
  "limit_price",
  "fee_allowance_usd",
  "maximum_debit_usd",
] as const;
const labels = [
  "Base quantity",
  "Limit price (USD)",
  "Fee allowance (USD)",
  "Maximum debit (USD)",
];
const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

function sameTerms(
  order: OwnerOrder,
  other: Pick<PrepareCommand, "side" | (typeof amounts)[number]>,
) {
  return (
    order.side === other.side &&
    amounts.every((key) => compareExactDecimals(order[key], other[key]) === 0)
  );
}

// Deliberately unmounted: no page, navigation entry, proxy or runtime switch.
// The future trusted host supplies display scope and the reviewed command client.
// Neither these props nor a saved status grant execution authority.
export function OwnerExecutionWorkspace({
  productID,
  accountLabel,
  client,
}: Props) {
  const [binding] = useState({ productID, accountLabel, client });
  const scopeMatches =
    productID === binding.productID &&
    accountLabel === binding.accountLabel &&
    client === binding.client;
  const [scopeInvalid, setScopeInvalid] = useState(false);
  // Permanent for this mounted instance, including A -> B -> A changes.
  if (!scopeMatches && !scopeInvalid) setScopeInvalid(true);
  const lifecycle = useRef({ generation: 0, alive: false, invalid: false });
  useLayoutEffect(() => {
    const life = lifecycle.current;
    life.generation++;
    life.alive = true;
    if (!scopeMatches) life.invalid = true;
    return () => {
      life.alive = false;
      life.generation++;
    };
  }, [productID, accountLabel, client, scopeMatches]);

  const busyRef = useRef(false);
  const [sent, setSent] = useState<ReadonlySet<string>>(new Set());
  const [canceled, setCanceled] = useState<ReadonlySet<string>>(new Set());
  const preparation = useRef<PrepareCommand | null>(null);
  const [busy, setBusy] = useState(false);
  const [order, setOrder] = useState<OwnerOrder | null>(null);
  const [observedAt, setObservedAt] = useState(0);
  const [draft, setDraft] = useState<Omit<PrepareCommand, "request_key">>({
    side: "BUY",
    base_size: "",
    limit_price: "",
    fee_allowance_usd: "",
    maximum_debit_usd: "",
  });
  const [draftLocked, setDraftLocked] = useState(false);
  const [loadStarted, setLoadStarted] = useState(false);
  const [lookupID, setLookupID] = useState("");
  const [mfa, setMFA] = useState("");
  const [evidence, setEvidence] = useState<OwnerPreflight | null>(null);
  const [sendConfirmed, setSendConfirmed] = useState(false);
  const [cancelConfirmed, setCancelConfirmed] = useState(false);
  const [stale, setStale] = useState(false);
  const [error, setError] = useState("");

  function accept(next: OwnerOrder, previous: OwnerOrder | null) {
    if (
      next.product_id !== productID ||
      (previous &&
        (next.id !== previous.id ||
          next.request_digest !== previous.request_digest ||
          !sameTerms(next, previous)))
    ) {
      throw new Error("Response binding mismatch");
    }
    return next;
  }

  async function run(
    action: () => Promise<OwnerOrder>,
    previous: OwnerOrder | null = order,
    completed?: (next: OwnerOrder) => void,
  ) {
    if (
      busyRef.current ||
      scopeInvalid ||
      !scopeMatches ||
      lifecycle.current.invalid ||
      !lifecycle.current.alive
    )
      return;
    busyRef.current = true;
    const version = lifecycle.current.generation;
    const active = () =>
      lifecycle.current.alive &&
      !lifecycle.current.invalid &&
      lifecycle.current.generation === version;
    setBusy(true);
    setError("");
    setEvidence(null);
    setSendConfirmed(false);
    setCancelConfirmed(false);
    setMFA("");
    try {
      const next = accept(await action(), previous);
      if (!active()) return;
      setOrder(next);
      setObservedAt(Date.now());
      setLookupID(next.id);
      setStale(false);
      completed?.(next);
    } catch {
      if (!active()) return;
      setStale(true);
      setError(
        "The result could not be verified. Read saved status before further action. Do not repeat a send or cancellation.",
      );
    } finally {
      if (active()) {
        busyRef.current = false;
        setBusy(false);
      }
    }
  }

  async function prepare(event: FormEvent) {
    event.preventDefault();
    if (busyRef.current || order || loadStarted) return;
    if (!preparation.current) {
      let requestKey: string;
      try {
        requestKey = crypto.randomUUID();
      } catch {
        setError(
          "Secure request identity is unavailable. No request was sent.",
        );
        return;
      }
      try {
        preparation.current = parsePrepareCommand({
          ...draft,
          request_key: requestKey,
        });
      } catch {
        setError(
          "Enter valid exact decimal limits. Quantity and price must be positive; a buy must have a positive maximum debit. No request was sent.",
        );
        return;
      }
    }
    setDraftLocked(true);
    const frozen = preparation.current;
    await run(async () => {
      const next = await client.prepare(frozen);
      if (!sameTerms(next, frozen))
        throw new Error("Preparation terms changed");
      return next;
    }, null);
  }

  function load(event: FormEvent) {
    event.preventDefault();
    if (
      busyRef.current ||
      !uuid.test(lookupID) ||
      (order && lookupID !== order.id)
    )
      return;
    setLoadStarted(true);
    void run(async () => {
      const next = await client.get(lookupID);
      if (next.id !== lookupID) throw new Error("Saved identity mismatch");
      return next;
    });
  }

  const available = !!order && !busy && !stale && order.state !== "UNAVAILABLE";
  const approvedAtSnapshot =
    !!order &&
    order.state === "APPROVED" &&
    order.approval_status === "RECORDED" &&
    !!order.approval_expires_at &&
    Date.parse(order.approval_expires_at) > observedAt &&
    !order.account_blocked &&
    !sent.has(order.id);
  const canApprove =
    available &&
    order.state === "PREPARED" &&
    order.approval_status === "NONE" &&
    !order.account_blocked &&
    !sent.has(order.id);
  const canPreflight = available && approvedAtSnapshot;
  const canSend =
    canPreflight &&
    !!evidence &&
    evidence.order.id === order.id &&
    evidence.order.request_digest === order.request_digest;
  const working =
    available &&
    (order.state === "BROKER_ACKNOWLEDGED" ||
      order.state === "PARTIALLY_FILLED");
  const canCancel =
    working && order.cancellation_status === "NONE" && !canceled.has(order.id);

  function approvedNow() {
    if (
      !order?.approval_expires_at ||
      Date.parse(order.approval_expires_at) <= Date.now()
    ) {
      setEvidence(null);
      setSendConfirmed(false);
      setStale(true);
      setError(
        "Approval has expired. Read saved status; this order cannot be re-approved.",
      );
      return false;
    }
    return approvedAtSnapshot;
  }

  function approve(event: FormEvent) {
    event.preventDefault();
    if (!canApprove || busyRef.current || !/^\d{6}$/.test(mfa)) return;
    const command = { expected_digest: order.request_digest, mfa_code: mfa };
    void run(() => client.approve(order.id, command));
  }

  function preflight() {
    if (!canPreflight || busyRef.current || !approvedNow()) return;
    let captured: OwnerPreflight | null = null;
    void run(
      async () => {
        captured = await client.preflight(order.id);
        return captured.order;
      },
      order,
      (next) => {
        if (
          captured &&
          next.state === "APPROVED" &&
          next.approval_status === "RECORDED"
        )
          setEvidence(captured);
      },
    );
  }

  function send() {
    if (!canSend || !sendConfirmed || busyRef.current || !approvedNow()) return;
    setSent(new Set([...sent, order.id])); // run's synchronous busy fence covers duplicate events.
    void run(() =>
      client.send(order.id, { evidence_id: evidence.evidence_id }),
    );
  }

  function cancel() {
    if (!canCancel || !cancelConfirmed || busyRef.current) return;
    setCanceled(new Set([...canceled, order.id]));
    void run(() => client.cancel(order.id));
  }

  if (
    scopeInvalid ||
    !scopeMatches ||
    !/^[A-Z][A-Z0-9]{0,15}-USD$/.test(productID) ||
    productID === "USD-USD" ||
    !accountLabel.trim()
  ) {
    return (
      <section className={styles.workspace}>
        <p role="alert">
          Execution workspace scope is unavailable or changed. Reopen through
          the authenticated owner view.
        </p>
      </section>
    );
  }

  return (
    <section
      className={styles.workspace}
      aria-label="Personal Coinbase execution"
      aria-busy={busy}
    >
      <header>
        <p className="eyebrow">PERSONAL COINBASE · OWNER CONFIRMED</p>
        <h2>One order. Exact limits.</h2>
        <p>
          {accountLabel} · {productID} · price-bounded immediate-or-cancel
        </p>
        <p className={styles.notice}>
          This interface is not enabled in production. Live runtime review and
          explicit activation are still required. No automatic trading.
        </p>
      </header>
      <form onSubmit={load} className={styles.lookup}>
        <label>
          Saved order ID
          <input
            value={lookupID}
            onChange={(e) => setLookupID(e.target.value)}
            readOnly={!!order}
            disabled={busy}
            autoComplete="off"
            spellCheck={false}
          />
        </label>
        <button
          disabled={
            busy || !uuid.test(lookupID) || (!!order && lookupID !== order.id)
          }
        >
          Read saved status
        </button>
        <p>
          On return, enter the original saved ID. This reads records only; it
          does not contact Coinbase or submit an order.
        </p>
      </form>
      {error && (
        <p role="alert" className={styles.notice}>
          {error}
        </p>
      )}
      {busy && <p role="status">Request in progress. No automatic retry.</p>}
      {!order && !loadStarted && (
        <form onSubmit={prepare}>
          <h3>Prepare exact terms</h3>
          <p>
            Preparation does not submit an order. The account and pair are fixed
            by server configuration, not this form.
          </p>
          <fieldset disabled={busy || draftLocked} className={styles.fields}>
            <legend>Order limits</legend>
            <label>
              Side
              <select
                value={draft.side}
                onChange={(e) =>
                  setDraft({ ...draft, side: e.target.value as "BUY" | "SELL" })
                }
              >
                <option value="BUY">Buy</option>
                <option value="SELL">Sell</option>
              </select>
            </label>
            {amounts.map((key, i) => (
              <label key={key}>
                {labels[i]}
                <input
                  required
                  inputMode="decimal"
                  autoComplete="off"
                  value={draft[key]}
                  maxLength={80}
                  pattern="(0|[1-9][0-9]*)(\.[0-9]+)?"
                  onChange={(e) =>
                    setDraft({ ...draft, [key]: e.target.value })
                  }
                />
              </label>
            ))}
          </fieldset>
          <button disabled={busy}>
            {draftLocked ? "Recover preparation" : "Prepare order"}
          </button>
          {draftLocked && (
            <p>
              Terms and request key are frozen. Recovery repeats only this
              preparation with the same identity, never a broker submission.
            </p>
          )}
        </form>
      )}
      {order && (
        <>
          <section aria-label="Saved order" className={styles.saved}>
            <h3>{order.state.replaceAll("_", " ")}</h3>
            <p role="status">{descriptions[order.state]}</p>
            {stale && (
              <p className={styles.notice}>
                Last verified snapshot only. Refresh saved status before further
                commands.
              </p>
            )}
            {order.account_blocked && (
              <p role="alert">
                Account blocked for reconciliation review. No new submission is
                allowed.
              </p>
            )}
            <dl className={styles.facts}>
              <div>
                <dt>Pair / side</dt>
                <dd>
                  {order.product_id} / {order.side}
                </dd>
              </div>
              {amounts.map((key, i) => (
                <div key={key}>
                  <dt>{labels[i]}</dt>
                  <dd>{order[key] || "Unavailable"}</dd>
                </div>
              ))}
              <div>
                <dt>Approval</dt>
                <dd>{order.approval_status}</dd>
              </div>
              {order.approval_expires_at && (
                <div>
                  <dt>Approval expires (UTC)</dt>
                  <dd>
                    <time dateTime={order.approval_expires_at}>
                      {order.approval_expires_at}
                    </time>
                  </dd>
                </div>
              )}
              <div>
                <dt>Order slot held</dt>
                <dd>{order.account_held ? "Yes" : "No"}</dd>
              </div>
              <div>
                <dt>Capital held</dt>
                <dd>{order.capital_held ? "Yes" : "No"}</dd>
              </div>
            </dl>
            <p>
              Saved status is not permission to trade. The server rechecks
              authorization, risk, balances and evidence at each financial
              boundary.
            </p>
          </section>
          {canApprove && (
            <form onSubmit={approve} className={styles.actions}>
              <label>
                Execution MFA code
                <input
                  value={mfa}
                  onChange={(e) => setMFA(e.target.value)}
                  inputMode="numeric"
                  autoComplete="one-time-code"
                  maxLength={6}
                  pattern="[0-9]{6}"
                  required
                />
              </label>
              <button disabled={!/^\d{6}$/.test(mfa)}>
                Approve exact terms
              </button>
              <p>
                This confirmation applies only to the immutable terms above, not
                future orders or unattended trading.
              </p>
            </form>
          )}
          {order.approval_status === "RECORDED" && (
            <button
              disabled={!available}
              onClick={() => void run(() => client.revoke(order.id))}
            >
              Revoke approval
            </button>
          )}
          {(order.approval_status === "EXPIRED" ||
            order.approval_status === "REVOKED") && (
            <p>
              Approval cannot be renewed on this order. Existing attempts still
              require recovery or reconciliation.
            </p>
          )}
          {canPreflight && (
            <div className={styles.actions}>
              <button onClick={preflight}>Check current preflight</button>
              <p>
                Checks provider facts and saves evidence. It does not send the
                order.
              </p>
            </div>
          )}
          {canSend && (
            <div className={styles.actions}>
              <label className={styles.confirm}>
                <input
                  type="checkbox"
                  checked={sendConfirmed}
                  onChange={(e) => setSendConfirmed(e.target.checked)}
                />
                I confirm one price-bounded order using the exact terms above.
              </label>
              <button disabled={!sendConfirmed} onClick={send}>
                Send order once
              </button>
              <p>
                This command can use real funds when the reviewed runtime is
                activated. Acknowledgement is not a fill; an error is not
                permission to retry.
              </p>
            </div>
          )}
          {available &&
            (order.state === "SUBMISSION_UNKNOWN" ||
              (sent.has(order.id) &&
                (order.state === "APPROVED" ||
                  order.state === "PREPARED"))) && (
              <button onClick={() => void run(() => client.recover(order.id))}>
                Recover original submission
              </button>
            )}
          {working && (
            <div className={styles.actions}>
              <button
                onClick={() => void run(() => client.reconcile(order.id))}
              >
                Reconcile fills and status
              </button>
              {canCancel && (
                <>
                  <label className={styles.confirm}>
                    <input
                      type="checkbox"
                      checked={cancelConfirmed}
                      onChange={(e) => setCancelConfirmed(e.target.checked)}
                    />
                    Request cancellation of only this original order.
                  </label>
                  <button disabled={!cancelConfirmed} onClick={cancel}>
                    Request cancellation once
                  </button>
                </>
              )}
            </div>
          )}
          {order.cancellation_status !== "NONE" && (
            <p>
              Cancellation: {order.cancellation_status}. This is not proof of
              final order status or released capital.
            </p>
          )}
          {available && order.state === "AWAITING_ACCOUNT_SETTLEMENT" && (
            <button onClick={() => void run(() => client.settle(order.id))}>
              Reconcile cash and position
            </button>
          )}
          <details className={styles.details}>
            <summary>Exact saved evidence</summary>
            <dl className={styles.facts}>
              <div>
                <dt>Order ID</dt>
                <dd>{order.id}</dd>
              </div>
              <div>
                <dt>Request digest</dt>
                <dd>{order.request_digest || "Unavailable"}</dd>
              </div>
              <div>
                <dt>Created (UTC)</dt>
                <dd>
                  <time dateTime={order.created_at}>{order.created_at}</time>
                </dd>
              </div>
              <div>
                <dt>Verified fill count</dt>
                <dd>{order.fill_count}</dd>
              </div>
              <div>
                <dt>Base filled</dt>
                <dd>{order.base_filled || "Unavailable"}</dd>
              </div>
              <div>
                <dt>Gross value (USD)</dt>
                <dd>{order.gross_usd || "Unavailable"}</dd>
              </div>
              <div>
                <dt>Fees (USD)</dt>
                <dd>{order.fee_usd || "Unavailable"}</dd>
              </div>
              <div>
                <dt>Final order status</dt>
                <dd>{order.terminal_status || "Not recorded"}</dd>
              </div>
              {order.state === "SETTLED" && order.accounting && (
                <>
                  <div>
                    <dt>Opening cash (USD)</dt>
                    <dd>{order.accounting.opening_cash_usd}</dd>
                  </div>
                  <div>
                    <dt>Closing cash (USD)</dt>
                    <dd>{order.accounting.closing_cash_usd}</dd>
                  </div>
                  <div>
                    <dt>Opening base</dt>
                    <dd>{order.accounting.opening_base}</dd>
                  </div>
                  <div>
                    <dt>Closing base</dt>
                    <dd>{order.accounting.closing_base}</dd>
                  </div>
                  <div>
                    <dt>Accounting recorded (UTC)</dt>
                    <dd>
                      <time dateTime={order.accounting.recorded_at}>
                        {order.accounting.recorded_at}
                      </time>
                    </dd>
                  </div>
                </>
              )}
            </dl>
          </details>
        </>
      )}
    </section>
  );
}
