"use client";

import { FormEvent, useState } from "react";
import { useRouter } from "next/navigation";

import type { PaperPosition } from "./paper-portfolio-summary";

type Props = {
  instanceId: string;
  currentState: string;
  stateVersion: number;
  status: string;
  executionMode: string;
  strategyIdentifier: string;
  openPosition?: PaperPosition;
};

type LifecycleChoice = {
  value: "EXPIRE_WORTHLESS" | "ASSIGNED" | "CALLED_AWAY";
  label: string;
};

function choicesFor(state: string): LifecycleChoice[] {
  if (state === "SHORT_PUT_OPEN") {
    return [
      {
        value: "EXPIRE_WORTHLESS",
        label: "Put expired worthless — close the simulated put",
      },
      {
        value: "ASSIGNED",
        label: "Put assigned — buy the simulated shares at the strike",
      },
    ];
  }
  if (state === "SHORT_CALL_OPEN") {
    return [
      {
        value: "EXPIRE_WORTHLESS",
        label: "Call expired worthless — keep the simulated shares",
      },
      {
        value: "CALLED_AWAY",
        label: "Shares called away — sell the simulated shares at the strike",
      },
    ];
  }
  return [];
}

export function StrategyLifecycleControls(props: Props) {
  const router = useRouter();
  const choices = choicesFor(props.currentState);
  const expectedOptionType =
    props.currentState === "SHORT_PUT_OPEN"
      ? "PUT"
      : props.currentState === "SHORT_CALL_OPEN"
        ? "CALL"
        : "";
  const stateEligible =
    Boolean(props.instanceId) &&
    (props.status === "ACTIVE" || props.status === "PAUSED") &&
    props.executionMode === "PAPER" &&
    props.strategyIdentifier === "wheel" &&
    choices.length > 0;
  const eligible =
    stateEligible &&
    props.openPosition?.is_open === true &&
    props.openPosition.instrument === "OPTION" &&
    props.openPosition.option_type === expectedOptionType;
  const [confirmed, setConfirmed] = useState(false);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!eligible || !confirmed) return;
    const form = new FormData(event.currentTarget);
    setBusy(true);
    setMessage("");
    const response = await fetch(
      `/api/strategy-instances/${props.instanceId}/lifecycle-events`,
      {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({
          event_id: `manual-lifecycle:${crypto.randomUUID()}`,
          event_type: String(form.get("event_type")),
          expected_state_version: props.stateVersion,
          confirm_paper_simulation: true,
        }),
      },
    );
    setBusy(false);
    if (!response.ok) {
      setMessage(
        "The PAPER event was not recorded. Refresh the page and confirm the simulated contract state before trying again.",
      );
      return;
    }
    setMessage(
      "PAPER lifecycle event recorded. The simulated ledger and strategy state were updated; no Schwab order was sent.",
    );
    setConfirmed(false);
    router.refresh();
  }

  if (!stateEligible) return null;

  if (!eligible || !props.openPosition) {
    return (
      <section className="mandate-controls" aria-label="Paper lifecycle event">
        <p className="eyebrow">PAPER LIFECYCLE</p>
        <h2>Simulated contract details unavailable</h2>
        <p className="security-note">
          Event recording is disabled because Arbion could not confirm exactly
          one matching open simulated option from the PAPER ledger.
        </p>
      </section>
    );
  }

  return (
    <section className="mandate-controls" aria-label="Paper lifecycle event">
      <p className="eyebrow">PAPER LIFECYCLE</p>
      <h2>Record what happened to the simulated option</h2>
      <p>
        This updates only Arbion&apos;s PAPER ledger. The open simulated
        contract, strike, quantity, and shares are derived from durable records
        rather than typed into this form. A paused strategy remains paused after
        this lifecycle event.
      </p>
      <div className="paper-contract-facts" aria-label="Open simulated option">
        <p>
          <strong>Contract</strong>
          {props.openPosition.symbol} {props.openPosition.option_type}
        </p>
        <p>
          <strong>Strike</strong>
          {props.openPosition.strike}
        </p>
        <p>
          <strong>Expiration</strong>
          {props.openPosition.expiration}
        </p>
        <p>
          <strong>Quantity</strong>
          {props.openPosition.quantity}
        </p>
      </div>
      <form onSubmit={submit}>
        <label>
          Outcome
          <select name="event_type" disabled={busy}>
            {choices.map((choice) => (
              <option key={choice.value} value={choice.value}>
                {choice.label}
              </option>
            ))}
          </select>
        </label>
        <label>
          <input
            type="checkbox"
            checked={confirmed}
            disabled={busy}
            onChange={(event) => setConfirmed(event.currentTarget.checked)}
          />{" "}
          I confirm this is a PAPER simulation and does not change Schwab.
        </label>
        <button type="submit" disabled={!confirmed || busy}>
          Record PAPER Event
        </button>
      </form>
      <p className="security-note">
        Expiry is accepted only at or after 4:00 p.m. New York time on the
        contract&apos;s expiration date. Assignment and called-away outcomes are
        explicit owner attestations.
      </p>
      {message && <p role="status">{message}</p>}
    </section>
  );
}
