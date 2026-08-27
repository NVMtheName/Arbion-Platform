"use client";

import { FormEvent, useState } from "react";
import { useRouter } from "next/navigation";

export type UserCircuitBreaker = {
  id: string;
  scope: "USER";
  scope_id?: string;
  state: "OPEN" | "CLOSED";
  reason: string;
  source: string;
  engaged_at: string;
  released_at?: string;
};

export function UserCircuitBreakerControls({
  breaker,
}: {
  breaker?: UserCircuitBreaker | null;
}) {
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
    const response = await fetch(`/api/risk/circuit-breaker/${action}`, {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ reason, confirm: confirmed }),
    });
    setBusy(false);
    if (!response.ok) {
      const body = (await response.json().catch(() => null)) as {
        error?: { code?: string };
      } | null;
      setMessage(
        body?.error?.code === "CIRCUIT_BREAKER_CONFLICT"
          ? "The owner-stop state changed. Refresh the page and review the current state."
          : "The owner-stop change was not saved. Review the reason and try again.",
      );
      return;
    }
    setReason("");
    setConfirmed(false);
    setMessage(
      active
        ? "Owner-wide stop released after review. Arbion may evaluate account automations again; no broker action was requested."
        : "Owner-wide stop engaged. Arbion will deny every new action across your connected accounts without changing broker positions.",
    );
    router.refresh();
  }

  return (
    <section
      className={`content-card mandate-controls${active ? " circuit-breaker-active" : ""}`}
      aria-label="Owner-wide emergency stop"
    >
      <p className="eyebrow">OWNER-WIDE SAFETY CONTROL</p>
      <h2>
        {active ? "All Arbion actions stopped" : "Stop all Arbion actions"}
      </h2>
      <p className="status-badge">{active ? "STOP ACTIVE" : "ACTIVE"}</p>
      {active ? (
        <>
          <p role="alert">
            The deterministic control engine will deny new actions across every
            connected account and automation owned by you. Monitoring and
            immutable evidence may continue, but Arbion does not close
            positions, revoke credentials, or send broker instructions.
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
          Use this only when you need every Arbion action to fail closed at
          once. Per-account and per-automation controls remain available for
          narrower isolation. This control never submits an order or changes a
          position.
        </p>
      )}
      <form onSubmit={submit}>
        <label>
          {active
            ? "Why is it safe to release?"
            : "Why are you stopping all actions?"}
          <textarea
            required
            minLength={8}
            maxLength={280}
            value={reason}
            onChange={(event) => setReason(event.target.value)}
            placeholder={
              active
                ? "Describe the accounts and conditions you reviewed before releasing the stop."
                : "Describe the safety or operational reason for stopping all new actions."
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
            ? "I reviewed the cause and understand Arbion may evaluate my account automations again."
            : "I understand this blocks every new Arbion action across my connected accounts."}
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
              ? "Release Owner-Wide Stop"
              : "Stop All Arbion Actions"}
        </button>
      </form>
      {message && <p role="status">{message}</p>}
    </section>
  );
}
