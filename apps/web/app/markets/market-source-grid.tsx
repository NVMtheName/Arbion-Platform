"use client";

import { useCallback, useEffect, useMemo, useState } from "react";

export type CapabilityStatus = {
  capability: string;
  enabled: boolean;
  state: "NOT_CONFIGURED" | "AWAITING_OBSERVATION" | "VERIFIED" | "DEGRADED";
  last_attempt_at?: string;
  last_success_at?: string;
  consecutive_failures: number;
  failure_category?: string;
};

export type MarketSource = {
  id: string;
  label: string;
  role: string;
  feed: string;
  quality: string;
  capabilities: string[];
  capability_status?: CapabilityStatus[];
  enabled: boolean;
  healthy: boolean;
};

type SourcesResponse = {
  sources?: MarketSource[];
  status_generated_at?: string;
  status_semantics?: string;
  provider_errors_exposed?: boolean;
  live_execution_available?: boolean;
};

function readable(value: string) {
  return value.replaceAll("_", " ").toLowerCase();
}

function timestamp(value: string | undefined) {
  if (!value) return "";
  const parsed = new Date(value);
  if (Number.isNaN(parsed.valueOf())) return value;
  return new Intl.DateTimeFormat("en-US", {
    dateStyle: "medium",
    timeStyle: "medium",
    timeZone: "UTC",
  }).format(parsed);
}

function statuses(source: MarketSource): CapabilityStatus[] {
  if (source.capability_status?.length) return source.capability_status;
  return source.capabilities.map((capability) => ({
    capability,
    enabled: source.enabled,
    state: source.enabled
      ? source.healthy
        ? "VERIFIED"
        : "AWAITING_OBSERVATION"
      : "NOT_CONFIGURED",
    consecutive_failures: 0,
  }));
}

function sourceStatus(source: MarketSource) {
  if (!source.enabled) return "Not configured";
  if (statuses(source).some((status) => status.state === "DEGRADED")) {
    return "Degraded";
  }
  if (source.healthy) return "Available";
  return "Awaiting data";
}

function sourceStatusClass(source: MarketSource) {
  if (!source.enabled) return "disabled";
  if (statuses(source).some((status) => status.state === "DEGRADED")) {
    return "degraded";
  }
  return source.healthy ? "available" : "awaiting";
}

function capabilityLabel(status: CapabilityStatus) {
  switch (status.state) {
    case "VERIFIED":
      return "Verified";
    case "DEGRADED":
      return "Degraded";
    case "AWAITING_OBSERVATION":
      return "Awaiting observation";
    default:
      return "Not configured";
  }
}

function capabilityDetail(status: CapabilityStatus) {
  if (status.state === "VERIFIED" && status.last_success_at) {
    return `Last provider success ${timestamp(status.last_success_at)} UTC`;
  }
  if (status.state === "DEGRADED") {
    const category = status.failure_category
      ? readable(status.failure_category)
      : "provider failure";
    const attempted = status.last_attempt_at
      ? ` · attempted ${timestamp(status.last_attempt_at)} UTC`
      : "";
    return `${category}${attempted}`;
  }
  if (status.state === "AWAITING_OBSERVATION") {
    return "Configured; no provider attempt has completed in this API process.";
  }
  return "No production adapter is enabled for this capability.";
}

export function MarketSourceGrid({
  sources: initialSources,
  statusGeneratedAt: initialGeneratedAt,
}: {
  sources: MarketSource[];
  statusGeneratedAt?: string;
}) {
  const [sources, setSources] = useState(initialSources);
  const [statusGeneratedAt, setStatusGeneratedAt] = useState(
    initialGeneratedAt ?? "",
  );
  const [refreshing, setRefreshing] = useState(false);
  const [refreshError, setRefreshError] = useState("");

  const refresh = useCallback(async () => {
    setRefreshing(true);
    setRefreshError("");
    try {
      const response = await fetch("/api/markets/sources", {
        cache: "no-store",
      });
      const body = (await response.json()) as SourcesResponse;
      if (
        !response.ok ||
        !Array.isArray(body.sources) ||
        body.live_execution_available !== false ||
        body.provider_errors_exposed !== false
      ) {
        setRefreshError(
          "Source verification is temporarily unavailable. Existing status remains time-labeled.",
        );
        return;
      }
      setSources(body.sources);
      setStatusGeneratedAt(body.status_generated_at ?? "");
    } catch {
      setRefreshError(
        "Source verification is temporarily unavailable. Existing status remains time-labeled.",
      );
    } finally {
      setRefreshing(false);
    }
  }, []);

  useEffect(() => {
    const interval = window.setInterval(() => void refresh(), 30_000);
    return () => window.clearInterval(interval);
  }, [refresh]);

  const summary = useMemo(() => {
    const capabilityStatuses = sources.flatMap(statuses);
    return {
      verified: capabilityStatuses.filter(
        (status) => status.state === "VERIFIED",
      ).length,
      degraded: capabilityStatuses.filter(
        (status) => status.state === "DEGRADED",
      ).length,
      awaiting: capabilityStatuses.filter(
        (status) => status.state === "AWAITING_OBSERVATION",
      ).length,
    };
  }, [sources]);

  return (
    <div className="market-source-monitor">
      <header className="market-source-monitor-header">
        <div>
          <p className="eyebrow">RUNTIME VERIFICATION</p>
          <h3>Every feed earns its own status.</h3>
          <p>
            One healthy Coinbase request cannot mask a failure in another public
            feed. Status resets with the API process and cache hits do not
            invent a new provider success.
          </p>
        </div>
        <button
          className="secondary"
          disabled={refreshing}
          onClick={() => void refresh()}
          type="button"
        >
          {refreshing ? "Refreshing…" : "Refresh source status"}
        </button>
      </header>

      <div className="market-source-totals" aria-label="Capability status">
        <span>
          <strong>{summary.verified}</strong> verified
        </span>
        <span>
          <strong>{summary.awaiting}</strong> awaiting observation
        </span>
        <span className={summary.degraded > 0 ? "degraded" : ""}>
          <strong>{summary.degraded}</strong> degraded
        </span>
        {statusGeneratedAt && (
          <time dateTime={statusGeneratedAt}>
            Status generated {timestamp(statusGeneratedAt)} UTC
          </time>
        )}
      </div>

      {refreshError && (
        <p className="unavailable" role="status">
          {refreshError}
        </p>
      )}

      <section className="market-source-grid" aria-label="Market data sources">
        {sources.map((source) => (
          <article className="market-source-card" key={source.id}>
            <header>
              <div>
                <p className="market-source-role">{readable(source.role)}</p>
                <h3>{source.label}</h3>
              </div>
              <span
                className={`market-source-status ${sourceStatusClass(source)}`}
              >
                {sourceStatus(source)}
              </span>
            </header>
            <dl>
              <div>
                <dt>Feed</dt>
                <dd>{readable(source.feed)}</dd>
              </div>
              <div>
                <dt>Quality</dt>
                <dd>{readable(source.quality)}</dd>
              </div>
            </dl>
            <ul
              className="market-capability-statuses"
              aria-label={`${source.label} capability verification`}
            >
              {statuses(source).map((status) => (
                <li key={status.capability}>
                  <div>
                    <strong>{readable(status.capability)}</strong>
                    <span className={status.state.toLowerCase()}>
                      {capabilityLabel(status)}
                    </span>
                  </div>
                  <small>{capabilityDetail(status)}</small>
                  {status.consecutive_failures > 0 && (
                    <small>
                      {status.consecutive_failures} consecutive provider
                      {status.consecutive_failures === 1
                        ? " failure"
                        : " failures"}
                    </small>
                  )}
                </li>
              ))}
            </ul>
            <footer>
              Runtime verification is operational evidence, not a quote,
              consolidated market, or execution guarantee.
            </footer>
          </article>
        ))}
      </section>
    </div>
  );
}
