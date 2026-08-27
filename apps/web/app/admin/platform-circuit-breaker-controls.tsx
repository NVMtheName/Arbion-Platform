"use client";

import { FormEvent, useState } from "react";
import { useRouter } from "next/navigation";

export type PlatformCircuitBreaker = {
  id: string;
  scope: "GLOBAL";
  state: "OPEN" | "CLOSED";
  reason: string;
  source: string;
  engaged_at: string;
  released_at?: string;
};

export function PlatformCircuitBreakerControls({
  breaker,
}: {
  breaker?: PlatformCircuitBreaker | null;
}) {
  const router = useRouter();
  const active = breaker?.state === "OPEN";
  const [reason, setReason] = useState("");
  const [mfaCode, setMFACode] = useState("");
  const [confirmed, setConfirmed] = useState(false);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setBusy(true);
    setMessage("");
    const action = active ? "release" : "engage";
    try {
      const response = await fetch(
        `/api/admin/risk/circuit-breaker/${action}`,
        {
          method: "POST",
          headers: { "content-type": "application/json" },
          body: JSON.stringify({
            reason,
            confirm: confirmed,
            ...(active ? { mfa_code: mfaCode } : {}),
          }),
        },
      );
      if (!response.ok) {
        const body = (await response.json().catch(() => null)) as {
          error?: { code?: string };
        } | null;
        setMessage(
          body?.error?.code === "CIRCUIT_BREAKER_CONFLICT"
            ? "The platform-stop state changed. Refresh and review the current state."
            : body?.error?.code === "INVALID_MFA_CODE"
              ? "Use a fresh six-digit authenticator code to release the platform stop."
              : "The platform safety change was not saved. Review the inputs and try again.",
        );
        return;
      }
      setReason("");
      setMFACode("");
      setConfirmed(false);
      setMessage(
        active
          ? "Platform stop released after superadmin review and fresh MFA. New actions may be evaluated again; no broker action was requested."
          : "Platform stop engaged. Arbion will deny every new risk-gated action across every user and account without changing broker positions.",
      );
      router.refresh();
    } catch {
      setMessage(
        "The platform safety service could not be reached. Refresh before attempting another change.",
      );
    } finally {
      setBusy(false);
    }
  }

  return (
    <section
      className={`content-card mandate-controls${active ? " circuit-breaker-active" : ""}`}
      aria-label="Platform-wide emergency stop"
    >
      <p className="eyebrow">PLATFORM-WIDE SAFETY CONTROL</p>
      <h2>{active ? "Platform actions stopped" : "Platform emergency stop"}</h2>
      <p className="status-badge">{active ? "GLOBAL STOP ACTIVE" : "READY"}</p>
      {active ? (
        <>
          <p role="alert">
            The deterministic control engine will deny new risk-gated actions
            for every user, account, and automation. Monitoring and immutable
            evidence may continue. Arbion does not close positions, revoke
            credentials, or send broker instructions.
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
          Superadmins can immediately fail closed every new Arbion action when a
          platform-wide incident is suspected. Narrower owner, account, and
          automation stops remain available. Engaging this control does not
          require MFA; releasing it does.
        </p>
      )}
      <form onSubmit={submit}>
        <label>
          {active
            ? "Why is the platform safe to release?"
            : "Why are you stopping platform actions?"}
          <textarea
            required
            minLength={8}
            maxLength={280}
            value={reason}
            onChange={(event) => setReason(event.target.value)}
            placeholder={
              active
                ? "Describe the platform conditions and evidence reviewed before release."
                : "Describe the security, provider, or operational reason for the stop."
            }
          />
        </label>
        {active && (
          <label>
            Fresh authenticator code
            <input
              required
              inputMode="numeric"
              autoComplete="one-time-code"
              pattern="[0-9]{6}"
              minLength={6}
              maxLength={6}
              value={mfaCode}
              onChange={(event) =>
                setMFACode(event.target.value.replace(/\D/g, "").slice(0, 6))
              }
              placeholder="123456"
            />
          </label>
        )}
        <label className="checkbox-row">
          <input
            type="checkbox"
            required
            checked={confirmed}
            onChange={(event) => setConfirmed(event.target.checked)}
          />
          {active
            ? "I reviewed the incident and understand new actions may be evaluated again after release."
            : "I understand this blocks new risk-gated actions across the entire Arbion platform."}
        </label>
        <button
          className={active ? undefined : "danger"}
          type="submit"
          disabled={busy}
        >
          {busy
            ? active
              ? "Verifying and releasing…"
              : "Stopping platform…"
            : active
              ? "Release Platform Stop"
              : "Stop Platform Actions"}
        </button>
      </form>
      {message && <p role="status">{message}</p>}
    </section>
  );
}
