"use client";

import { FormEvent, useState } from "react";
import { useRouter } from "next/navigation";

type Props = {
  automationId: string;
  currentVersion: number;
  automationType: string;
  executionMode: string;
  optionsAllowed: boolean;
  capabilityUnverified: boolean;
  attested: boolean;
  hasActiveInstance: boolean;
};

export function PaperOptionsSimulationAttestationControls(props: Props) {
  const router = useRouter();
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const eligible =
    props.automationType === "STRATEGY" &&
    props.executionMode === "PAPER" &&
    props.optionsAllowed &&
    props.capabilityUnverified;

  if (!eligible) return null;

  async function update(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setBusy(true);
    setMessage("");
    const response = await fetch(
      `/api/automations/${props.automationId}/paper-options-simulation-attestation`,
      {
        method: "PATCH",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({
          expected_version: props.currentVersion,
          attested: !props.attested,
        }),
      },
    );
    setBusy(false);
    if (!response.ok) {
      setMessage(
        "The attestation was not saved. Refresh and review the latest mandate version and capability state.",
      );
      return;
    }
    setMessage(
      props.attested
        ? "The PAPER-only attestation was removed in a new DRAFT version. Schwab was not changed."
        : "The PAPER-only attestation was saved in a new DRAFT version. Schwab capability remains UNKNOWN and no broker order was sent.",
    );
    router.refresh();
  }

  return (
    <section
      className="mandate-controls"
      aria-label="PAPER options simulation attestation"
    >
      <p className="eyebrow">PAPER-ONLY OPTIONS SIMULATION</p>
      <h2>
        {props.attested
          ? "Simulation attestation is recorded"
          : "Allow options simulation only"}
      </h2>
      <p>
        Schwab&apos;s options capability is UNKNOWN. This attestation permits
        Arbion to model options inside this PAPER mandate only. It is not proof
        of broker approval, cannot apply to SHADOW or LIVE, and cannot submit a
        broker order.
      </p>
      {props.hasActiveInstance ? (
        <p className="security-note">
          Finish the active or paused non-live simulation before changing this
          immutable mandate setting.
        </p>
      ) : (
        <form onSubmit={update}>
          <label className="checkbox-row">
            <input type="checkbox" required />
            {props.attested
              ? "I confirm that I want to revoke the PAPER-only simulation attestation."
              : "I understand this permits simulation only and does not confirm or change my Schwab options approval."}
          </label>
          <button type="submit" disabled={busy}>
            {busy
              ? "Creating Draft…"
              : props.attested
                ? "Create Draft Without Attestation"
                : "Create Attested PAPER Draft"}
          </button>
        </form>
      )}
      {message && <p role="status">{message}</p>}
    </section>
  );
}
