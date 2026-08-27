"use client";

import { FormEvent, useState } from "react";
import { useRouter } from "next/navigation";

export type AccountCircuitBreaker = {
  id: string;
  scope: "ACCOUNT";
  scope_id?: string;
  state: "OPEN" | "CLOSED";
  reason: string;
  source: string;
  engaged_at: string;
  released_at?: string;
};

type Props = {
  accountId: string;
  accountName: string;
  breaker?: AccountCircuitBreaker | null;
};

export function AccountCircuitBreakerControls({
  accountId,
  accountName,
  breaker,
}: Props) {
  const router = useRouter();
  const active = breaker?.state === "OPEN";
  const [reason, setReason] = useState("");
  const [confirmed, setConfirmed] = useState(false);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setBusy(true);
    setMessage("");
    const action = active ? "release" : "engage";
    const response = await fetch(
      `/api/accounts/${accountId}/circuit-breaker/${action}`,
      {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ reason, confirm: confirmed }),
      },
    );
    setBusy(false);
    if (!response.ok) {
      const body = (await response.json().catch(() => null)) as {
        error?: { code?: string };
      } | null;
      setMessage(
        body?.error?.code === "CIRCUIT_BREAKER_CONFLICT"
          ? "The account-stop state changed. Refresh the page and review the current state."
          : "The account-stop change was not saved. Review the reason and try again.",
      );
      return;
    }
    setReason("");
    setConfirmed(false);
    setMessage(
      active
        ? `Account stop released for ${accountName}. Arbion may evaluate its automations again; no broker action was requested.`
        : `Account stop engaged for ${accountName}. Arbion will deny new actions across this account without changing broker positions.`,
    );
    router.refresh();
  }

  return (
    <section
      className={`mandate-controls${active ? " circuit-breaker-active" : ""}`}
      aria-label="Account emergency stop"
    >
      <p className="eyebrow">ACCOUNT SAFETY ISOLATION</p>
      <h2>{active ? "Account stop active" : "Account emergency stop"}</h2>
      {active ? (
        <>
          <p role="alert">
            Arbion&apos;s deterministic control engine will deny every new
            action tied to this account. Monitoring and immutable evidence may
            continue, but positions, credentials, and the broker account are not
            changed.
          </p>
          <dl className="circuit-breaker-facts">
            <div>
              <dt>Engaged</dt>
              <dd>
                <time dateTime={breaker.engaged_at}>
                  {new Date(breaker.engaged_at).toUTCString()}
                </time>
              </dd>
            </div>
            <div>
              <dt>Reason</dt>
              <dd>{breaker.reason}</dd>
            </div>
          </dl>
        </>
      ) : (
        <p>
          Use this to fail closed across every Arbion automation attached to
          this account. Other connected accounts keep operating independently,
          and no broker order, cancellation, transfer, or position change is
          requested.
        </p>
      )}
      <form onSubmit={submit}>
        <label>
          {active
            ? "Why is it safe to release?"
            : "Why are you stopping this account?"}
          <textarea
            required
            minLength={8}
            maxLength={280}
            value={reason}
            onChange={(event) => setReason(event.target.value)}
            placeholder={
              active
                ? "Describe what you reviewed and why this account can resume."
                : "Describe the safety, connection, or operational reason for the stop."
            }
          />
        </label>
        <label className="checkbox-row">
          <input
            type="checkbox"
            required
            checked={confirmed}
            onChange={(event) => setConfirmed(event.target.checked)}
          />
          {active
            ? "I reviewed the cause and understand this account's automations may be evaluated again."
            : "I understand this blocks new Arbion actions for this account only."}
        </label>
        <button
          className={active ? undefined : "danger"}
          type="submit"
          disabled={busy}
        >
          {busy
            ? active
              ? "Releasing…"
              : "Stopping…"
            : active
              ? "Release Account Stop"
              : "Engage Account Stop"}
        </button>
      </form>
      {message && <p role="status">{message}</p>}
    </section>
  );
}
