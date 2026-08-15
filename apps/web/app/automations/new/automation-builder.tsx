"use client";
import Link from "next/link";
import { FormEvent, useEffect, useState } from "react";

type Item = {
  id: string;
  display_name?: string;
  name?: string;
  status?: string;
};
export default function AutomationBuilder() {
  const [accounts, setAccounts] = useState<Item[]>([]);
  const [buckets, setBuckets] = useState<Item[]>([]);
  const [ai, setAI] = useState<Item[]>([]);
  const [type, setType] = useState("STRATEGY");
  const [message, setMessage] = useState("");
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState(false);
  useEffect(() => {
    void Promise.all([
      fetch("/api/accounts").then((r) => r.json()),
      fetch("/api/capital-buckets").then((r) => r.json()),
      fetch("/api/connections/ai").then((r) => r.json()),
    ])
      .then(([a, b, c]) => {
        setAccounts(Array.isArray(a.accounts) ? a.accounts : []);
        setBuckets(Array.isArray(b.capital_buckets) ? b.capital_buckets : []);
        setAI(Array.isArray(c.connections) ? c.connections : []);
      })
      .catch(() => setLoadError(true))
      .finally(() => setLoading(false));
  }, []);
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const data = new FormData(event.currentTarget);
    const aiRequired = type !== "STRATEGY";
    const strategyRequired = type !== "AI_AUTONOMOUS";
    const body = {
      financial_account_id: data.get("account"),
      automation_type: type,
      strategy_identifier: strategyRequired ? data.get("strategy") : null,
      ai_provider_connection_id: aiRequired ? data.get("ai") : null,
      ai_model_id: aiRequired ? data.get("model") : null,
      capital_bucket_id: data.get("bucket"),
      autonomy_level: data.get("autonomy"),
      execution_mode: data.get("mode"),
      margin_allowed: data.get("margin") === "on",
      options_allowed: data.get("options") === "on",
      strategy_parameters: {},
      risk_parameters: {},
      allowed_universe: { symbols: [], universe_ids: [] },
      prohibited_universe: { symbols: [] },
      schedule_conditions: {},
    };
    const response = await fetch("/api/automations", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify(body),
    });
    setMessage(
      response.ok
        ? "Draft saved. Review it from Automations."
        : "The draft could not be saved. Check entitlement and configuration.",
    );
  }
  const activeAI = ai.filter((x) => x.status === "active");
  const aiRequired = type !== "STRATEGY";
  const ready =
    !loading &&
    !loadError &&
    accounts.length > 0 &&
    buckets.length > 0 &&
    (!aiRequired || activeAI.length > 0);
  return (
    <form className="builder" onSubmit={submit}>
      {loading && <p>Loading connections…</p>}
      {loadError && (
        <p className="form-error" role="alert">
          Connections could not be loaded. Try again later.
        </p>
      )}
      {!loading && !loadError && accounts.length === 0 && (
        <p className="security-note">
          Connect and sync a financial account before creating an automation.{" "}
          <Link href="/settings/connections">Manage connections</Link>
        </p>
      )}
      {!loading &&
        !loadError &&
        accounts.length > 0 &&
        buckets.length === 0 && (
          <p className="security-note">
            Create a capital bucket for the connected account before saving an
            automation draft.
          </p>
        )}
      <label>
        Account
        <select name="account" required disabled={accounts.length === 0}>
          {accounts.length === 0 && <option>No connected accounts</option>}
          {accounts.map((a) => (
            <option key={a.id} value={a.id}>
              {a.display_name}
            </option>
          ))}
        </select>
      </label>
      <fieldset>
        <legend>Automation Type</legend>
        {["AI_AUTONOMOUS", "STRATEGY", "HYBRID"].map((v) => (
          <label key={v}>
            <input
              type="radio"
              name="type"
              checked={type === v}
              onChange={() => setType(v)}
            />
            {v.replaceAll("_", " ")}
          </label>
        ))}
      </fieldset>
      {type !== "AI_AUTONOMOUS" && (
        <label>
          Strategy (configuration only)
          <select name="strategy">
            <option value="wheel">Wheel</option>
            <option value="covered_call">Covered Call</option>
            <option value="cash_secured_put">Cash-Secured Put</option>
            <option value="collar">Collar</option>
          </select>
        </label>
      )}
      {type !== "STRATEGY" && (
        <>
          <label>
            AI Provider
            <select name="ai" disabled={activeAI.length === 0}>
              {activeAI.length === 0 && <option>No active AI providers</option>}
              {activeAI.map((x) => (
                <option key={x.id} value={x.id}>
                  {x.display_name}
                </option>
              ))}
            </select>
          </label>
          <label>
            Verified model ID
            <input name="model" required />
          </label>
        </>
      )}
      <label>
        Capital Bucket
        <select name="bucket" required disabled={buckets.length === 0}>
          {buckets.length === 0 && <option>No capital buckets</option>}
          {buckets.map((b) => (
            <option key={b.id} value={b.id}>
              {b.name}
            </option>
          ))}
        </select>
      </label>
      <label>
        Autonomy
        <select name="autonomy">
          <option>RESEARCH_ONLY</option>
          <option>SUGGEST</option>
          <option>CONFIRM_EACH</option>
          <option>STRATEGY_AUTONOMOUS</option>
          <option>FULL_AUTONOMOUS</option>
        </select>
      </label>
      <label>
        Execution Mode
        <select name="mode">
          <option>BACKTEST</option>
          <option>PAPER</option>
          <option>SHADOW</option>
          <option>LIVE</option>
        </select>
      </label>
      <label>
        <input type="checkbox" name="margin" /> Margin allowed
      </label>
      <label>
        <input type="checkbox" name="options" /> Options allowed
      </label>
      <p className="security-note">
        Live is a stored preference only. READY does not execute anything, and
        the platform has no execution path.
      </p>
      <button type="submit" disabled={!ready}>
        Save Draft
      </button>
      {message && <p>{message}</p>}
    </form>
  );
}
