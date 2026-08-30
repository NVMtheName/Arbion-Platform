"use client";

import { FormEvent, useState } from "react";
import { useRouter } from "next/navigation";

import { compareExactDecimals } from "../exact-money";

export type CapitalAccountOption = {
  id: string;
  display_name: string;
  status: string;
  currency?: string;
};

function positiveDecimal(value: string) {
  return compareExactDecimals(value, "0") === 1;
}

function nonNegativeDecimal(value: string) {
  const comparison = compareExactDecimals(value, "0");
  return comparison !== undefined && comparison >= 0;
}

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
    (account) =>
      account.status === "active" &&
      (account.currency === undefined ||
        account.currency.toUpperCase() === "USD"),
  );

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = event.currentTarget;
    const data = new FormData(form);
    const isReserve = data.get("reserve") === "on";
    const allocation = String(data.get("allocation") ?? "").trim();
    const protectedAmount = String(data.get("protected_amount") ?? "").trim();
    const allocationLimit = String(data.get("allocation_limit") ?? "").trim();
    const allocationMaximum = compareExactDecimals(allocation, "100");
    const allocationValid =
      positiveDecimal(allocation) &&
      (allocationType === "FIXED_AMOUNT" ||
        (allocationMaximum !== undefined && allocationMaximum <= 0));
    const limitValid =
      allocationType === "FIXED_AMOUNT"
        ? !allocationLimit || positiveDecimal(allocationLimit)
        : positiveDecimal(allocationLimit);
    if (
      !allocationValid ||
      !nonNegativeDecimal(protectedAmount) ||
      !limitValid
    ) {
      setMessage(
        "Use canonical decimal amounts. Allocation and any ceiling must be positive; protected capital cannot be negative.",
      );
      return;
    }
    setBusy(true);
    setMessage("");
    const response = await fetch("/api/capital-buckets", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({
        financial_account_id: data.get("account"),
        name: data.get("name"),
        allocation_type: allocationType,
        allocation_value: allocation,
        currency: "USD",
        protected_amount: protectedAmount,
        ...(allocationLimit ? { allocation_limit: allocationLimit } : {}),
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
      <label>
        {allocationType === "FIXED_AMOUNT"
          ? "Shared account ceiling (USD, optional)"
          : "Absolute strategy cap (USD)"}
        <input
          name="allocation_limit"
          inputMode="decimal"
          min="0.0000000001"
          step="any"
          required={allocationType !== "FIXED_AMOUNT"}
          placeholder={
            allocationType === "FIXED_AMOUNT"
              ? "Leave blank for exclusive use"
              : "Required for a strategy"
          }
        />
        <span className="field-hint">
          {allocationType === "FIXED_AMOUNT"
            ? "Use the same exact ceiling on compatible budgets only when you intend multiple non-live strategies to share one account total."
            : "Percentage budgets need an exact dollar cap before a Paper or Shadow strategy can reserve them."}
        </span>
      </label>
      <label className="checkbox-row">
        <input type="checkbox" name="reserve" />
        Reserve / never attach to an automation
      </label>
      <p className="security-note">
        This is an Arbion authorization limit, not permission to trade. It never
        moves or locks funds at your financial provider.
      </p>
      <button type="submit" disabled={busy}>
        {busy ? "Creating…" : "Create Capital Bucket"}
      </button>
      {message && <p role="status">{message}</p>}
    </form>
  );
}
