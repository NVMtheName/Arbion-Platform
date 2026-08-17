"use client";

import Link from "next/link";
import { FormEvent, useState } from "react";

type Insight = {
  summary: string;
  key_points: string[];
  risk_flags: string[];
  limitations: string[];
  requires_current_data: boolean;
  metadata: {
    model: string;
    input_usage?: number;
    output_usage?: number;
  };
};

export function InsightPanel({
  configured,
  model,
}: {
  configured: boolean;
  model?: string;
}) {
  const [prompt, setPrompt] = useState("");
  const [insight, setInsight] = useState<Insight | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const question = prompt.trim();
    if (!question) return;
    setLoading(true);
    setError("");
    setInsight(null);
    try {
      const response = await fetch("/api/neural/insight", {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ prompt: question }),
      });
      const body = (await response.json()) as {
        insight?: Insight;
        error?: { code?: string };
      };
      if (!response.ok || !body.insight) {
        setError(
          body.error?.code === "insight_rate_limited"
            ? "You have reached the hourly insight limit. Try again later."
            : "Arbion Insight is temporarily unavailable. Try again shortly.",
        );
        return;
      }
      setInsight(body.insight);
    } catch {
      setError("Arbion Insight is temporarily unavailable. Try again shortly.");
    } finally {
      setLoading(false);
    }
  }

  return (
    <section className="insight-panel" aria-labelledby="insight-title">
      <p className="eyebrow">READ-ONLY AI</p>
      <h2 id="insight-title">Ask Arbion</h2>
      <p className="insight-disclaimer">
        Educational analysis of the text you provide. Arbion Insight has no
        access to your accounts, portfolio, broker, or live market data and
        cannot place or authorize trades.
      </p>
      {!configured ? (
        <p className="security-note">
          Verify an AI provider and select a model before asking a question.{" "}
          <Link href="/settings/connections">Configure Neural Engine</Link>
        </p>
      ) : (
        <form onSubmit={submit}>
          <label htmlFor="insight-prompt">
            What would you like to understand?
          </label>
          <textarea
            id="insight-prompt"
            value={prompt}
            onChange={(event) => setPrompt(event.target.value)}
            maxLength={2000}
            placeholder="For example: Explain diversification in plain language without using live market data."
            required
          />
          <p className="character-count">{prompt.length} / 2,000</p>
          <button disabled={loading || prompt.trim() === ""} type="submit">
            {loading ? "Analyzing…" : "Generate insight"}
          </button>
          {model && <p className="insight-meta">Selected model: {model}</p>}
        </form>
      )}
      {error && (
        <p className="form-error" role="alert">
          {error}
        </p>
      )}
      {insight && (
        <article className="insight-result" aria-live="polite">
          {insight.requires_current_data && (
            <p className="insight-warning">
              This question needs current data that was not provided. Treat the
              result as general context only.
            </p>
          )}
          <h3>Insight</h3>
          <p>{insight.summary}</p>
          <InsightList title="Key points" items={insight.key_points} />
          <InsightList title="Risks to consider" items={insight.risk_flags} />
          <InsightList title="Limitations" items={insight.limitations} />
          <p className="insight-meta">
            Generated with {insight.metadata.model}
          </p>
        </article>
      )}
    </section>
  );
}

function InsightList({ title, items }: { title: string; items: string[] }) {
  if (items.length === 0) return null;
  return (
    <section>
      <h4>{title}</h4>
      <ul>
        {items.map((item, index) => (
          <li key={title + "-" + index}>{item}</li>
        ))}
      </ul>
    </section>
  );
}
