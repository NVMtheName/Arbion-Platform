"use client";

import Link from "next/link";
import { FormEvent, useEffect, useMemo, useState } from "react";

type Item = {
  id: string;
  display_name?: string;
  name?: string;
  provider?: string;
  status?: string;
  is_reserve?: boolean;
  financial_account_id?: string;
};
type Model = { id: string; display_name: string };

const shadowModels = new Set(["gpt-5.6-luna", "gpt-5.6-terra", "gpt-5.6-sol"]);

function accountItem(value: Record<string, unknown>): Item {
  return {
    id: String(value.id ?? value.ID ?? ""),
    display_name: String(
      value.display_name ?? value.DisplayName ?? "Financial account",
    ),
    provider: String(value.provider ?? value.Provider ?? ""),
    status: String(value.status ?? value.Status ?? ""),
  };
}
function bucketItem(value: Record<string, unknown>): Item {
  return {
    id: String(value.id ?? value.ID ?? ""),
    name: String(value.name ?? value.Name ?? "Trading budget"),
    status: String(value.status ?? value.Status ?? ""),
    is_reserve: Boolean(value.is_reserve ?? value.IsReserve),
    financial_account_id: String(
      value.financial_account_id ?? value.FinancialAccountID ?? "",
    ),
  };
}

export default function AutomationBuilder() {
  const [accounts, setAccounts] = useState<Item[]>([]);
  const [buckets, setBuckets] = useState<Item[]>([]);
  const [connections, setConnections] = useState<Item[]>([]);
  const [models, setModels] = useState<Model[]>([]);
  const [accountID, setAccountID] = useState("");
  const [connectionID, setConnectionID] = useState("");
  const [modelID, setModelID] = useState("");
  const [preferredModelID, setPreferredModelID] = useState("");
  const [bucketID, setBucketID] = useState("");
  const [budgetAmount, setBudgetAmount] = useState("");
  const [objective, setObjective] = useState(
    "Look for cautious, risk-aware opportunities while preserving capital.",
  );
  const [symbols, setSymbols] = useState("");
  const [maxNotional, setMaxNotional] = useState("1");
  const [scheduled, setScheduled] = useState(false);
  const [intervalMinutes, setIntervalMinutes] = useState(60);
  const [loading, setLoading] = useState(true);
  const [modelsLoading, setModelsLoading] = useState(false);
  const [busy, setBusy] = useState(false);
  const [loadError, setLoadError] = useState(false);
  const [message, setMessage] = useState("");
  const [createdID, setCreatedID] = useState("");

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
                provider: String(
                  connection.provider ?? connection.Provider ?? "",
                ),
                status: String(connection.status ?? connection.Status ?? ""),
              }),
            )
          : [];
        const activeAccounts = accountItems.filter(
          (account: Item) => account.status === "active",
        );
        const activeAI = aiItems.filter(
          (connection: Item) =>
            connection.status === "active" && connection.provider === "openai",
        );
        const preference = preferencePayload.preference as
          | { connection_id?: string; model_id?: string }
          | null
          | undefined;
        const selectedAI =
          activeAI.find(
            (item: Item) => item.id === preference?.connection_id,
          ) ?? activeAI[0];
        setAccounts(accountItems);
        setBuckets(bucketItems);
        setConnections(aiItems);
        setAccountID(activeAccounts[0]?.id ?? "");
        setConnectionID(selectedAI?.id ?? "");
        setPreferredModelID(preference?.model_id ?? "");
        setModelsLoading(Boolean(selectedAI));
      })
      .catch(() => active && setLoadError(true))
      .finally(() => active && setLoading(false));
    return () => {
      active = false;
    };
  }, []);

  useEffect(() => {
    if (!connectionID) return;
    let active = true;
    void fetch(`/api/connections/ai/${connectionID}/models`)
      .then((response) => response.json())
      .then((payload) => {
        if (!active) return;
        const items: Model[] = Array.isArray(payload.models)
          ? payload.models
              .map((model: Record<string, unknown>) => ({
                id: String(model.id ?? ""),
                display_name: String(
                  model.display_name ?? model.id ?? "Arbion model",
                ),
              }))
              .filter((model: Model) => shadowModels.has(model.id))
          : [];
        setModels(items);
        setModelID(
          items.find((model) => model.id === preferredModelID)?.id ??
            items.find((model) => model.id === "gpt-5.6-sol")?.id ??
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
      .finally(() => active && setModelsLoading(false));
    return () => {
      active = false;
    };
  }, [connectionID, preferredModelID]);

  const activeAccounts = accounts.filter((item) => item.status === "active");
  const activeAI = connections.filter(
    (item) => item.status === "active" && item.provider === "openai",
  );
  const account = activeAccounts.find((item) => item.id === accountID);
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
    setBusy(true);
    setMessage("");
    try {
      const response = await fetch("/api/capital-buckets", {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({
          financial_account_id: accountID,
          name: "AI Shadow budget",
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
      setMessage("AI Shadow budget saved for this account.");
    } catch {
      setMessage(
        "The budget could not be saved. Check the amount and try again.",
      );
    } finally {
      setBusy(false);
    }
  }

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const allowedSymbols = symbols
      .split(",")
      .map((symbol) => symbol.trim().toUpperCase())
      .filter(Boolean);
    setBusy(true);
    setMessage("");
    setCreatedID("");
    try {
      const response = await fetch("/api/automations", {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({
          financial_account_id: accountID,
          automation_type: "AI_AUTONOMOUS",
          strategy_identifier: null,
          ai_provider_connection_id: connectionID,
          ai_model_id: modelID,
          capital_bucket_id: selectedBucketID,
          autonomy_level: "FULL_AUTONOMOUS",
          execution_mode: "SHADOW",
          margin_allowed: false,
          options_allowed: false,
          strategy_parameters: {
            objective,
            max_proposal_notional: maxNotional,
          },
          risk_parameters: {},
          allowed_universe: { symbols: allowedSymbols, universe_ids: [] },
          prohibited_universe: { symbols: [] },
          schedule_conditions: scheduled
            ? {
                enabled: true,
                interval_minutes: intervalMinutes,
                session:
                  account?.provider === "coinbase"
                    ? "CONTINUOUS"
                    : "US_EQUITIES_REGULAR",
                notifications: {
                  evaluation_completed: true,
                  first_failure: true,
                },
              }
            : { enabled: false },
        }),
      });
      if (!response.ok) {
        const rejected = (await response.json().catch(() => null)) as {
          error?: { message?: string };
        } | null;
        throw new Error(
          rejected?.error?.message ??
            "The AI Shadow draft could not be created. Review the account, model, symbols, and budget.",
        );
      }
      const payload = (await response.json()) as {
        automation?: { id?: string; ID?: string };
      };
      const id = String(payload.automation?.id ?? payload.automation?.ID ?? "");
      setCreatedID(id);
      setMessage(
        "AI Shadow draft created. Review the immutable controls, then mark it ready. No broker order was sent.",
      );
    } catch (error) {
      setMessage(
        error instanceof Error
          ? error.message
          : "The AI Shadow draft could not be created. Review the account, model, symbols, and budget.",
      );
    } finally {
      setBusy(false);
    }
  }

  const ready =
    !loading &&
    !loadError &&
    Boolean(
      accountID &&
        connectionID &&
        modelID &&
        selectedBucketID &&
        objective.trim() &&
        symbols.trim() &&
        Number(maxNotional) > 0,
    );

  return (
    <form className="strategy-launch" onSubmit={submit}>
      <section className="strategy-prerequisite ai-shadow-banner">
        <p className="eyebrow">ARBION AUTONOMOUS ENGINE</p>
        <strong>One intelligence layer. Every connected account.</strong>
        <p>
          Build the same bounded AI decision loop for Coinbase or Schwab. This
          milestone watches real portfolios and real market data, then records
          shadow decisions without sending orders.
        </p>
      </section>
      {loading && <p>Loading your command center connections…</p>}
      {loadError && (
        <p className="form-error">Connections could not be loaded.</p>
      )}
      {!loading && activeAccounts.length === 0 && (
        <section className="strategy-prerequisite">
          <strong>Connect Coinbase or Schwab first</strong>
          <Link href="/connections">Open connection hub →</Link>
        </section>
      )}
      {!loading && activeAI.length === 0 && (
        <section className="strategy-prerequisite">
          <strong>Connect and verify OpenAI first</strong>
          <Link href="/connections#ai-providers">Open AI connections →</Link>
        </section>
      )}

      <section className="strategy-launch-section">
        <header>
          <span>01</span>
          <div>
            <h2>Choose the live account context</h2>
            <p>
              Each account gets an isolated mandate, budget, journal, and kill
              switch.
            </p>
          </div>
        </header>
        <div className="strategy-launch-grid">
          <label>
            Financial account
            <select
              required
              value={accountID}
              onChange={(event) => {
                setAccountID(event.target.value);
                setBucketID("");
                setSymbols("");
              }}
            >
              <option value="">Choose an account</option>
              {activeAccounts.map((item) => (
                <option key={item.id} value={item.id}>
                  {item.display_name} · {item.provider}
                </option>
              ))}
            </select>
          </label>
          <label>
            Arbion intelligence
            <select
              required
              value={connectionID}
              onChange={(event) => {
                setModelsLoading(true);
                setConnectionID(event.target.value);
              }}
            >
              {activeAI.map((item) => (
                <option key={item.id} value={item.id}>
                  {item.display_name}
                </option>
              ))}
            </select>
          </label>
          <label>
            Decision model
            <select
              required
              disabled={modelsLoading || models.length === 0}
              value={modelID}
              onChange={(event) => setModelID(event.target.value)}
            >
              <option value="">
                {modelsLoading ? "Loading models…" : "Choose a model"}
              </option>
              {models.map((model) => (
                <option key={model.id} value={model.id}>
                  {model.display_name}
                </option>
              ))}
            </select>
          </label>
        </div>
      </section>

      <section className="strategy-launch-section">
        <header>
          <span>02</span>
          <div>
            <h2>Define the AI mandate</h2>
            <p>
              The model may choose only from these symbols and may never exceed
              the per-decision ceiling.
            </p>
          </div>
        </header>
        <div className="strategy-launch-grid">
          <label className="strategy-objective">
            Trading objective
            <textarea
              required
              maxLength={500}
              value={objective}
              onChange={(event) => setObjective(event.target.value)}
            />
          </label>
          <label>
            Allowed symbols
            <input
              required
              placeholder={
                account?.provider === "coinbase"
                  ? "BTC, ETH, XRP"
                  : "SPY, QQQ, AAPL"
              }
              value={symbols}
              onChange={(event) => setSymbols(event.target.value)}
            />
            <span className="field-hint">Up to 8, comma separated.</span>
          </label>
          <label>
            Maximum per shadow proposal (USD)
            <input
              required
              inputMode="decimal"
              min="0.01"
              step="any"
              type="number"
              value={maxNotional}
              onChange={(event) => setMaxNotional(event.target.value)}
            />
          </label>
        </div>
      </section>

      <section className="strategy-launch-section">
        <header>
          <span>03</span>
          <div>
            <h2>Bind capital and monitoring</h2>
            <p>
              The account budget is a hard boundary, not permission to use the
              rest of the portfolio.
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
              New AI Shadow budget (USD)
              <input
                inputMode="decimal"
                min="1"
                step="any"
                type="number"
                value={budgetAmount}
                onChange={(event) => setBudgetAmount(event.target.value)}
              />
            </label>
            <button
              disabled={busy || !budgetAmount}
              onClick={createBudget}
              type="button"
            >
              Save isolated budget
            </button>
          </div>
        )}
        <div className="strategy-launch-grid">
          <label className="checkbox-row">
            <input
              type="checkbox"
              checked={scheduled}
              onChange={(event) => setScheduled(event.target.checked)}
            />
            Run guarded evaluations automatically
          </label>
          {scheduled && (
            <label>
              Evaluation interval
              <select
                value={intervalMinutes}
                onChange={(event) =>
                  setIntervalMinutes(Number(event.target.value))
                }
              >
                <option value={30}>Every 30 minutes</option>
                <option value={60}>Every hour</option>
                <option value={240}>Every 4 hours</option>
                <option value={1440}>Once a day</option>
              </select>
            </label>
          )}
        </div>
      </section>

      <footer className="strategy-launch-footer">
        <div>
          <strong>Shadow-only launch</strong>
          <span>
            Real data in. Decision journal out. No Coinbase or Schwab order
            route exists.
          </span>
        </div>
        <button disabled={!ready || busy} type="submit">
          {busy ? "Creating…" : "Create AI Shadow draft"}
        </button>
      </footer>
      {message && (
        <p className={createdID ? "form-success" : "form-error"} role="status">
          {message}{" "}
          {createdID && (
            <Link href={`/automations/${createdID}`}>Review AI engine →</Link>
          )}
        </p>
      )}
    </form>
  );
}
