"use client";

import Link from "next/link";
import { FormEvent, useEffect, useMemo, useState } from "react";

type Item = {
  id: string;
  display_name?: string;
  name?: string;
  status?: string;
  is_reserve?: boolean;
  financial_account_id?: string;
};

type Model = {
  id: string;
  display_name: string;
};

const strategies = [
  {
    id: "wheel",
    name: "Wheel",
    description:
      "Sell cash-secured puts, then covered calls if shares are assigned.",
  },
  {
    id: "covered_call",
    name: "Covered Call",
    description: "Generate option premium from shares you already hold.",
  },
  {
    id: "cash_secured_put",
    name: "Cash-Secured Put",
    description:
      "Target an entry price while keeping the required cash set aside.",
  },
] as const;

function accountItem(account: Record<string, unknown>): Item {
  return {
    id: String(account.id ?? account.ID ?? ""),
    display_name: String(
      account.display_name ?? account.DisplayName ?? "Financial account",
    ),
    status: String(account.status ?? account.Status ?? ""),
  };
}

function bucketItem(bucket: Record<string, unknown>): Item {
  return {
    id: String(bucket.id ?? bucket.ID ?? ""),
    name: String(bucket.name ?? bucket.Name ?? "Trading budget"),
    status: String(bucket.status ?? bucket.Status ?? ""),
    is_reserve: Boolean(bucket.is_reserve ?? bucket.IsReserve),
    financial_account_id: String(
      bucket.financial_account_id ?? bucket.FinancialAccountID ?? "",
    ),
  };
}

export default function AutomationBuilder() {
  const [accounts, setAccounts] = useState<Item[]>([]);
  const [buckets, setBuckets] = useState<Item[]>([]);
  const [ai, setAI] = useState<Item[]>([]);
  const [models, setModels] = useState<Model[]>([]);
  const [accountID, setAccountID] = useState("");
  const [aiID, setAIID] = useState("");
  const [modelID, setModelID] = useState("");
  const [preferredModelID, setPreferredModelID] = useState("");
  const [strategy, setStrategy] = useState("wheel");
  const [bucketID, setBucketID] = useState("");
  const [budgetAmount, setBudgetAmount] = useState("");
  const [mode, setMode] = useState("PAPER");
  const [autonomy, setAutonomy] = useState("CONFIRM_EACH");
  const [marginAllowed, setMarginAllowed] = useState(false);
  const [message, setMessage] = useState("");
  const [budgetMessage, setBudgetMessage] = useState("");
  const [createdAutomationID, setCreatedAutomationID] = useState("");
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState(false);
  const [modelsLoading, setModelsLoading] = useState(false);
  const [budgetBusy, setBudgetBusy] = useState(false);
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    let active = true;
    void Promise.all([
      fetch("/api/accounts").then((response) => response.json()),
      fetch("/api/capital-buckets").then((response) => response.json()),
      fetch("/api/connections/ai").then((response) => response.json()),
      fetch("/api/settings/neural-engine").then((response) => response.json()),
    ])
      .then(([accountPayload, bucketPayload, aiPayload, preferencePayload]) => {
        if (!active) return;
        const accountItems = Array.isArray(accountPayload.accounts)
          ? accountPayload.accounts.map(accountItem)
          : [];
        const bucketItems = Array.isArray(bucketPayload.capital_buckets)
          ? bucketPayload.capital_buckets.map(bucketItem)
          : [];
        const aiItems = Array.isArray(aiPayload.connections)
          ? aiPayload.connections.map(
              (connection: Record<string, unknown>) => ({
                id: String(connection.id ?? connection.ID ?? ""),
                display_name: String(
                  connection.display_name ??
                    connection.DisplayName ??
                    "AI provider",
                ),
                status: String(connection.status ?? connection.Status ?? ""),
              }),
            )
          : [];
        const activeAccounts = accountItems.filter(
          (account: Item) => account.status === "active",
        );
        const activeAI = aiItems.filter(
          (connection: Item) => connection.status === "active",
        );
        const preference = preferencePayload.preference as
          | { connection_id?: string; model_id?: string }
          | null
          | undefined;
        const preferredAI =
          activeAI.find(
            (connection: Item) => connection.id === preference?.connection_id,
          ) ?? activeAI[0];
        setAccounts(accountItems);
        setBuckets(bucketItems);
        setAI(aiItems);
        setAccountID(activeAccounts[0]?.id ?? "");
        setModelsLoading(Boolean(preferredAI));
        setAIID(preferredAI?.id ?? "");
        setPreferredModelID(preference?.model_id ?? "");
      })
      .catch(() => {
        if (active) setLoadError(true);
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, []);

  useEffect(() => {
    if (!aiID) {
      return;
    }
    let active = true;
    void fetch(`/api/connections/ai/${aiID}/models`)
      .then((response) => response.json())
      .then((payload) => {
        if (!active) return;
        const items = Array.isArray(payload.models)
          ? payload.models.map((model: Record<string, unknown>) => ({
              id: String(model.id ?? ""),
              display_name: String(model.display_name ?? model.id ?? "Model"),
            }))
          : [];
        setModels(items);
        setModelID(
          items.find((model: Model) => model.id === preferredModelID)?.id ??
            items[0]?.id ??
            "",
        );
      })
      .catch(() => {
        if (active) {
          setModels([]);
          setModelID("");
        }
      })
      .finally(() => {
        if (active) setModelsLoading(false);
      });
    return () => {
      active = false;
    };
  }, [aiID, preferredModelID]);

  const activeAccounts = accounts.filter(
    (account) => account.status === "active",
  );
  const activeAI = ai.filter((connection) => connection.status === "active");
  const eligibleBuckets = useMemo(
    () =>
      buckets.filter(
        (bucket) =>
          (bucket.status === "" || bucket.status === "ACTIVE") &&
          !bucket.is_reserve &&
          bucket.financial_account_id === accountID,
      ),
    [accountID, buckets],
  );

  const selectedBucketID = eligibleBuckets.some(
    (bucket) => bucket.id === bucketID,
  )
    ? bucketID
    : (eligibleBuckets[0]?.id ?? "");

  async function createBudget() {
    if (!accountID || !budgetAmount) return;
    setBudgetBusy(true);
    setBudgetMessage("");
    const selectedStrategy = strategies.find((item) => item.id === strategy);
    try {
      const response = await fetch("/api/capital-buckets", {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({
          financial_account_id: accountID,
          name: `${selectedStrategy?.name ?? "Strategy"} budget`,
          allocation_type: "FIXED_AMOUNT",
          allocation_value: budgetAmount,
          currency: "USD",
          protected_amount: "0",
          is_reserve: false,
        }),
      });
      if (!response.ok) throw new Error("budget rejected");
      const payload = (await response.json()) as {
        capital_bucket?: Record<string, unknown>;
      };
      if (!payload.capital_bucket) throw new Error("budget missing");
      const item = bucketItem(payload.capital_bucket);
      setBuckets((current) => [...current, item]);
      setBucketID(item.id);
      setBudgetAmount("");
      setBudgetMessage("Trading budget saved for this account.");
    } catch {
      setBudgetMessage(
        "The trading budget could not be saved. Check the amount and try again.",
      );
    } finally {
      setBudgetBusy(false);
    }
  }

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setSubmitting(true);
    setMessage("");
    setCreatedAutomationID("");
    try {
      const response = await fetch("/api/automations", {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({
          financial_account_id: accountID,
          automation_type: "HYBRID",
          strategy_identifier: strategy,
          ai_provider_connection_id: aiID,
          ai_model_id: modelID,
          capital_bucket_id: selectedBucketID,
          autonomy_level: autonomy,
          execution_mode: mode,
          margin_allowed: marginAllowed,
          options_allowed: true,
          strategy_parameters: {},
          risk_parameters: {},
          allowed_universe: { symbols: [], universe_ids: [] },
          prohibited_universe: { symbols: [] },
          schedule_conditions: {},
        }),
      });
      if (!response.ok) throw new Error("strategy rejected");
      const payload = (await response.json()) as {
        automation?: { id?: string; ID?: string };
      };
      setCreatedAutomationID(
        String(payload.automation?.id ?? payload.automation?.ID ?? ""),
      );
      setMessage(
        "Strategy draft created. Review it before enabling any evaluation.",
      );
    } catch {
      setMessage(
        "The strategy draft could not be created. Review the selected account, model, and budget.",
      );
    } finally {
      setSubmitting(false);
    }
  }

  const ready =
    !loading &&
    !loadError &&
    Boolean(accountID && aiID && modelID && selectedBucketID);

  return (
    <form className="strategy-launch" onSubmit={submit}>
      {loading && <p>Loading your connected accounts and models…</p>}
      {loadError && (
        <p className="form-error" role="alert">
          Your connections could not be loaded. Try again shortly.
        </p>
      )}

      {!loading && !loadError && activeAccounts.length === 0 && (
        <section className="strategy-prerequisite">
          <strong>Connect a financial account first</strong>
          <p>Your strategy needs an account whose portfolio it can evaluate.</p>
          <Link href="/connections#financial-accounts">
            Open connection hub →
          </Link>
        </section>
      )}

      {!loading && !loadError && activeAI.length === 0 && (
        <section className="strategy-prerequisite">
          <strong>Connect an AI provider first</strong>
          <p>Add OpenAI, Claude, or Gemini and select a verified model.</p>
          <Link href="/connections#ai-providers">Open connection hub →</Link>
        </section>
      )}

      <section
        className="strategy-launch-section"
        aria-labelledby="engine-title"
      >
        <header>
          <span>01</span>
          <div>
            <h2 id="engine-title">Choose the account and intelligence</h2>
            <p>These can be changed before the strategy is activated.</p>
          </div>
        </header>
        <div className="strategy-launch-grid">
          <label>
            Financial account
            <select
              disabled={activeAccounts.length === 0}
              required
              value={accountID}
              onChange={(event) => setAccountID(event.target.value)}
            >
              {activeAccounts.length === 0 && (
                <option value="">No connected accounts</option>
              )}
              {activeAccounts.map((account) => (
                <option key={account.id} value={account.id}>
                  {account.display_name}
                </option>
              ))}
            </select>
          </label>
          <label>
            AI provider
            <select
              disabled={activeAI.length === 0}
              required
              value={aiID}
              onChange={(event) => {
                setPreferredModelID("");
                setModels([]);
                setModelID("");
                setModelsLoading(Boolean(event.target.value));
                setAIID(event.target.value);
              }}
            >
              {activeAI.length === 0 && (
                <option value="">No connected AI providers</option>
              )}
              {activeAI.map((connection) => (
                <option key={connection.id} value={connection.id}>
                  {connection.display_name}
                </option>
              ))}
            </select>
          </label>
          <label>
            AI model
            <select
              disabled={!aiID || modelsLoading || models.length === 0}
              required
              value={modelID}
              onChange={(event) => setModelID(event.target.value)}
            >
              {models.length === 0 && (
                <option value="">
                  {modelsLoading
                    ? "Loading verified models…"
                    : "No models loaded"}
                </option>
              )}
              {models.map((model) => (
                <option key={model.id} value={model.id}>
                  {model.display_name}
                </option>
              ))}
            </select>
          </label>
        </div>
      </section>

      <fieldset className="strategy-launch-section strategy-picker">
        <legend>
          <span>02</span>
          <span>
            <strong>Choose a trading strategy</strong>
            <small>Start with one clear, repeatable playbook.</small>
          </span>
        </legend>
        <div className="strategy-choice-grid">
          {strategies.map((option) => (
            <label
              className={strategy === option.id ? "is-selected" : ""}
              key={option.id}
            >
              <input
                checked={strategy === option.id}
                name="strategy"
                onChange={() => setStrategy(option.id)}
                type="radio"
                value={option.id}
              />
              <span>
                <strong>{option.name}</strong>
                <small>{option.description}</small>
              </span>
            </label>
          ))}
        </div>
      </fieldset>

      <section
        className="strategy-launch-section"
        aria-labelledby="budget-title"
      >
        <header>
          <span>03</span>
          <div>
            <h2 id="budget-title">Set the trading budget</h2>
            <p>
              This is the most Arbion may allocate to the strategy—not
              permission to spend the rest of the account.
            </p>
          </div>
        </header>
        {eligibleBuckets.length > 0 ? (
          <label className="strategy-budget-select">
            Trading budget
            <select
              required
              value={selectedBucketID}
              onChange={(event) => setBucketID(event.target.value)}
            >
              {eligibleBuckets.map((bucket) => (
                <option key={bucket.id} value={bucket.id}>
                  {bucket.name}
                </option>
              ))}
            </select>
          </label>
        ) : (
          <div className="strategy-budget-create">
            <label>
              Amount available to this strategy (USD)
              <input
                inputMode="decimal"
                min="1"
                onChange={(event) => setBudgetAmount(event.target.value)}
                placeholder="1,000"
                step="any"
                type="number"
                value={budgetAmount}
              />
            </label>
            <button
              disabled={!accountID || !budgetAmount || budgetBusy}
              onClick={createBudget}
              type="button"
            >
              {budgetBusy ? "Saving…" : "Save trading budget"}
            </button>
          </div>
        )}
        {budgetMessage && <p role="status">{budgetMessage}</p>}
      </section>

      <details className="strategy-advanced">
        <summary>Advanced controls</summary>
        <div className="strategy-launch-grid">
          <label>
            Starting mode
            <select
              value={mode}
              onChange={(event) => setMode(event.target.value)}
            >
              <option value="PAPER">Paper portfolio</option>
              <option value="SHADOW">Shadow decisions only</option>
              <option value="BACKTEST">Backtest configuration</option>
            </select>
          </label>
          <label>
            Approval style
            <select
              value={autonomy}
              onChange={(event) => setAutonomy(event.target.value)}
            >
              <option value="CONFIRM_EACH">
                Confirm every proposed action
              </option>
              <option value="SUGGEST">Suggestions only</option>
              <option value="RESEARCH_ONLY">Research only</option>
            </select>
          </label>
          <label className="checkbox-row">
            <input
              checked={marginAllowed}
              onChange={(event) => setMarginAllowed(event.target.checked)}
              type="checkbox"
            />
            Allow the strategy to consider margin
          </label>
        </div>
      </details>

      <footer className="strategy-launch-footer">
        <div>
          <strong>Creates a reviewable draft</strong>
          <span>
            No trade is submitted and no strategy starts automatically.
          </span>
        </div>
        <button disabled={!ready || submitting} type="submit">
          {submitting ? "Creating…" : "Create strategy draft"}
        </button>
      </footer>

      {message && (
        <p
          className={createdAutomationID ? "form-success" : "form-error"}
          role="status"
        >
          {message}{" "}
          {createdAutomationID && (
            <Link href={`/automations/${createdAutomationID}`}>
              Review strategy →
            </Link>
          )}
        </p>
      )}
    </form>
  );
}
