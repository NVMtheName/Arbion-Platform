"use client";

import { FormEvent, useState } from "react";
import { useRouter } from "next/navigation";

export type CapitalAccountOption = {
  id: string;
  display_name: string;
  status: string;
};

export function CapitalBucketForm({
  accounts,
}: {
  accounts: CapitalAccountOption[];
}) {
  const router = useRouter();
  const [allocationType, setAllocationType] = useState("FIXED_AMOUNT");
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const activeAccounts = accounts.filter(
    (account) => account.status === "active",
  );

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = event.currentTarget;
    const data = new FormData(form);
    const isReserve = data.get("reserve") === "on";
    setBusy(true);
    setMessage("");
    const response = await fetch("/api/capital-buckets", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({
        financial_account_id: data.get("account"),
        name: data.get("name"),
        allocation_type: allocationType,
        allocation_value: data.get("allocation"),
        currency: "USD",
        protected_amount: data.get("protected_amount"),
        is_reserve: isReserve,
      }),
    });
    setBusy(false);
    if (!response.ok) {
      setMessage(
        "The capital bucket could not be created. Review the allocation and try again.",
      );
      return;
    }
    form.reset();
    setAllocationType("FIXED_AMOUNT");
    setMessage(
      isReserve
        ? "Protected reserve created. It cannot be attached to an automation."
        : "Capital bucket created. It is now available to automation drafts.",
    );
    router.refresh();
  }

  if (activeAccounts.length === 0) {
    return (
      <p className="security-note">
        Connect and sync an active financial account before allocating capital.
      </p>
    );
  }

  return (
    <form className="capital-bucket-form" onSubmit={submit}>
      <label>
        Account
        <select name="account" required>
          {activeAccounts.map((account) => (
            <option key={account.id} value={account.id}>
              {account.display_name}
            </option>
          ))}
        </select>
      </label>
      <label>
        Bucket name
        <input
          name="name"
          maxLength={100}
          required
          placeholder="Paper strategy"
        />
      </label>
      <label>
        Allocation method
        <select
          name="allocation_type"
          value={allocationType}
          onChange={(event) => setAllocationType(event.target.value)}
        >
          <option value="FIXED_AMOUNT">Fixed dollar amount</option>
          <option value="PERCENT_OF_AVAILABLE_CASH">
            Percent of available cash
          </option>
          <option value="PERCENT_OF_BUYING_POWER">
            Percent of buying power
          </option>
        </select>
      </label>
      <label>
        {allocationType === "FIXED_AMOUNT"
          ? "Allocation (USD)"
          : "Allocation (%)"}
        <input
          name="allocation"
          inputMode="decimal"
          min="0.0000000001"
          max={allocationType === "FIXED_AMOUNT" ? undefined : "100"}
          step="any"
          required
        />
      </label>
      <label>
        Protected amount (USD)
        <input
          name="protected_amount"
          inputMode="decimal"
          min="0"
          step="any"
          defaultValue="0"
          required
        />
      </label>
      <label className="checkbox-row">
        <input type="checkbox" name="reserve" />
        Reserve / never attach to an automation
      </label>
      <p className="security-note">
        This is an Arbion authorization limit, not permission to trade. It never
        changes your Schwab account.
      </p>
      <button type="submit" disabled={busy}>
        {busy ? "Creating…" : "Create Capital Bucket"}
      </button>
      {message && <p role="status">{message}</p>}
    </form>
  );
}
