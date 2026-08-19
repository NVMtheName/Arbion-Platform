"use client";

import { FormEvent, useState } from "react";
import { useRouter } from "next/navigation";

export type AutomationCircuitBreaker = {
  id: string;
  scope: "AUTOMATION";
  scope_id?: string;
  state: "OPEN" | "CLOSED";
  reason: string;
  source: string;
  engaged_at: string;
  released_at?: string;
};

type Props = {
  automationId: string;
  breaker?: AutomationCircuitBreaker | null;
};

export function AutomationCircuitBreakerControls({
  automationId,
  breaker,
}: Props) {
  const router = useRouter();
  const active = breaker?.state === "OPEN";
  const [reason, setReason] = useState("");
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setBusy(true);
    setMessage("");
    const action = active ? "release" : "engage";
    const response = await fetch(
      `/api/automations/${automationId}/circuit-breaker/${action}`,
      {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ reason, confirm: true }),
      },
    );
    setBusy(false);
    if (!response.ok) {
      const body = (await response.json().catch(() => null)) as {
        error?: { code?: string };
      } | null;
      setMessage(
        body?.error?.code === "CIRCUIT_BREAKER_CONFLICT"
          ? "The emergency-stop state changed. Refresh the page and review the current state."
          : "The emergency-stop change was not saved. Review the reason and try again.",
      );
      return;
    }
    setReason("");
    setMessage(
      active
        ? "Emergency stop released after your review. Arbion may evaluate this automation again, but no Schwab order was sent."
        : "Emergency stop engaged. Arbion will deny new actions for this automation, and no Schwab change was made.",
    );
    router.refresh();
  }

  return (
    <section
      className={`mandate-controls${active ? " circuit-breaker-active" : ""}`}
      aria-label="Automation emergency stop"
    >
      <p className="eyebrow">OWNER SAFETY CONTROL</p>
      <h2>{active ? "Emergency stop active" : "Emergency stop"}</h2>
      {active ? (
        <>
          <p role="alert">
            Arbion&apos;s control engine will deny new actions for this
            automation. Existing PAPER state and evidence are preserved, and
            nothing is sent to Schwab.
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
          Use this if you need Arbion to fail closed immediately for this
          automation. It blocks new risk approvals without deleting history,
          closing simulated positions, or changing the connected account.
        </p>
      )}
      <form onSubmit={submit}>
        <label>
          {active ? "Why is it safe to release?" : "Why are you stopping it?"}
          <textarea
            required
            minLength={8}
            maxLength={280}
            value={reason}
            onChange={(event) => setReason(event.target.value)}
            placeholder={
              active
                ? "Describe what you reviewed and why the stop can be released."
                : "Describe the safety or operational reason for stopping this automation."
            }
          />
        </label>
        <label className="checkbox-row">
          <input type="checkbox" required />
          {active
            ? "I reviewed the cause and understand this automation may be evaluated again."
            : "I understand this immediately blocks new actions for this automation."}
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
              ? "Release Emergency Stop"
              : "Engage Emergency Stop"}
        </button>
      </form>
      {message && <p role="status">{message}</p>}
    </section>
  );
}
