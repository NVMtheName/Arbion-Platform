"use client";

import { FormEvent, useState } from "react";
import { useRouter } from "next/navigation";

type Props = {
  bucketId: string;
  financialAccountId: string;
  name: string;
  allocationType: string;
  allocationValue: string;
  currency: string;
  protectedAmount: string;
  allocationLimit?: string;
  isReserve: boolean;
  status: string;
  hasActiveInstance: boolean;
};

function allocationLabel(type: string) {
  if (type === "PERCENT_OF_AVAILABLE_CASH")
    return "Available cash allocation (%)";
  if (type === "PERCENT_OF_BUYING_POWER") return "Buying power allocation (%)";
  return "Fixed capital allocation (USD)";
}

export function CapitalBucketAllocationControls(props: Props) {
  const router = useRouter();
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const editable =
    Boolean(props.bucketId) && props.status === "ACTIVE" && !props.isReserve;

  async function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    setBusy(true);
    setMessage("");
    const request: Record<string, unknown> = {
      financial_account_id: String(form.get("financial_account_id") ?? ""),
      name: props.name,
      allocation_type: props.allocationType,
      allocation_value: String(form.get("allocation_value") ?? "").trim(),
      currency: props.currency,
      protected_amount: props.protectedAmount,
      is_reserve: props.isReserve,
    };
    if (props.allocationLimit) request.allocation_limit = props.allocationLimit;
    const response = await fetch(`/api/capital-buckets/${props.bucketId}`, {
      method: "PATCH",
      headers: { "content-type": "application/json" },
      body: JSON.stringify(request),
    });
    setBusy(false);
    if (!response.ok) {
      setMessage(
        "The capital allocation was not accepted. Review the value and any account-level allocation limits.",
      );
      return;
    }
    setMessage(
      "Capital guardrail updated. This changed only Arbion's non-live risk limit; no AI cycle or broker order was sent.",
    );
    router.refresh();
  }

  return (
    <section
      className="mandate-controls"
      aria-label="Capital bucket allocation"
    >
      <p className="eyebrow">NON-LIVE CAPITAL GUARDRAIL</p>
      <h2>{props.name || "Capital bucket"}</h2>
      <p>
        This is an Arbion authorization limit, not deposited cash or permission
        to trade. Real provider cash, buying power, and every deterministic risk
        check remain authoritative.
      </p>
      <form onSubmit={save}>
        <input
          type="hidden"
          name="financial_account_id"
          value={props.financialAccountId}
        />
        <label>
          {allocationLabel(props.allocationType)}
          <input
            name="allocation_value"
            inputMode="decimal"
            min="0.0000000001"
            max={props.allocationType === "FIXED_AMOUNT" ? undefined : "100"}
            step="any"
            defaultValue={props.allocationValue}
            required
          />
        </label>
        <button type="submit" disabled={busy || !editable}>
          {busy ? "Updating guardrail…" : "Update Capital Guardrail"}
        </button>
      </form>
      {props.hasActiveInstance && (
        <p className="security-note">
          The active non-live instance will use the revised limit on its next
          guarded evaluation. Its mandate, mode, schedule, and history do not
          change.
        </p>
      )}
      {message && <p role="status">{message}</p>}
    </section>
  );
}
