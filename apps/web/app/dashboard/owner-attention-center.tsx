import Link from "next/link";

export type OwnerAttentionItem = {
  id: string;
  code: string;
  severity: "ATTENTION" | "STOPPED";
  resource_type: string;
  resource_id?: string;
  occurred_at: string;
  count: number;
};

export type OwnerAttentionOverview = {
  generated_at: string;
  status: "CLEAR" | "ATTENTION" | "STOPPED";
  items: OwnerAttentionItem[];
  total: number;
  attention_count: number;
  stopped_count: number;
  live_execution_available: false;
  broker_action_requested: false;
};

type AttentionPresentation = {
  title: string;
  detail: string;
  linkLabel: string;
  href: (item: OwnerAttentionItem) => string;
};

const connectionHref = () => "/connections";
const schwabReadinessHref = () => "/connections#schwab-market-readiness";
const riskHref = () => "/settings/risk";
const accountHref = (item: OwnerAttentionItem) =>
  item.resource_id ? `/accounts/${item.resource_id}` : "/accounts";
const accountReconciliationHref = (item: OwnerAttentionItem) =>
  item.resource_id
    ? `/accounts/${item.resource_id}#reconciliation-resolution-title`
    : "/accounts";
const automationHref = (item: OwnerAttentionItem) =>
  item.resource_id ? `/automations/${item.resource_id}` : "/automations";

const presentations: Record<string, AttentionPresentation> = {
  SCHWAB_MARKET_DATA_ATTENTION: {
    title: "Schwab market data needs review",
    detail:
      "Schwab authorization is active, but the latest saved quote did not prove broker-realtime quality. The AI cycle stopped before the model.",
    linkLabel: "Review Schwab readiness",
    href: schwabReadinessHref,
  },
  SCHEDULE_FAILURE: {
    title: "Scheduled evaluation needs attention",
    detail:
      "A non-live automation cycle did not complete. Review its schedule before relying on the next evaluation.",
    linkLabel: "Review automation",
    href: automationHref,
  },
  PORTFOLIO_DRIFT_REVIEW_REQUIRED: {
    title: "Portfolio inventory changed",
    detail:
      "New autonomous actions are blocked until you review the immutable account reconciliation.",
    linkLabel: "Review account",
    href: accountReconciliationHref,
  },
  PORTFOLIO_EVIDENCE_REQUIRED: {
    title: "Portfolio evidence needs review",
    detail:
      "The latest provider snapshot is incomplete. New autonomous actions remain blocked for this account.",
    linkLabel: "Review account",
    href: accountReconciliationHref,
  },
  AI_CONNECTION_ATTENTION: {
    title: "AI connection needs attention",
    detail:
      "An AI connection is expired, revoked, or unavailable. Stored credentials are never shown here.",
    linkLabel: "Review connections",
    href: connectionHref,
  },
  FINANCIAL_CONNECTION_ATTENTION: {
    title: "Financial connection needs attention",
    detail:
      "A financial connection is expired, revoked, or unavailable. Existing positions are not changed.",
    linkLabel: "Review connections",
    href: connectionHref,
  },
  FINANCIAL_ACCOUNT_UNAVAILABLE: {
    title: "Account refresh is unavailable",
    detail:
      "Arbion cannot currently confirm this account's latest read-only portfolio state.",
    linkLabel: "Review account",
    href: accountHref,
  },
  GLOBAL_SAFETY_STOP: {
    title: "Platform safety stop is active",
    detail:
      "A global deterministic stop is open. Autonomous activity remains blocked across Arbion.",
    linkLabel: "Review risk controls",
    href: riskHref,
  },
  OWNER_SAFETY_STOP: {
    title: "Owner safety stop is active",
    detail:
      "Your deterministic owner stop is open. Autonomous activity remains blocked.",
    linkLabel: "Review risk controls",
    href: riskHref,
  },
  ACCOUNT_SAFETY_STOP: {
    title: "Account safety stop is active",
    detail:
      "A deterministic stop is blocking autonomous activity for one account.",
    linkLabel: "Review risk controls",
    href: riskHref,
  },
  AUTOMATION_SAFETY_STOP: {
    title: "Automation safety stop is active",
    detail:
      "A deterministic stop is blocking this automation. No broker action is requested.",
    linkLabel: "Review automation",
    href: automationHref,
  },
};

const unknownPresentation: AttentionPresentation = {
  title: "A monitored control needs attention",
  detail:
    "Arbion detected an active condition but is withholding unsupported detail. Review risk controls.",
  linkLabel: "Review risk controls",
  href: riskHref,
};

function readableTime(value: string) {
  const parsed = new Date(value);
  if (Number.isNaN(parsed.valueOf())) return "Time unavailable";
  return new Intl.DateTimeFormat("en-US", {
    month: "short",
    day: "numeric",
    hour: "numeric",
    minute: "2-digit",
    timeZone: "UTC",
    timeZoneName: "short",
  }).format(parsed);
}

function statusLabel(attention?: OwnerAttentionOverview) {
  if (!attention) return "Status unavailable";
  if (attention.status === "STOPPED")
    return `${attention.stopped_count} safety stop${attention.stopped_count === 1 ? "" : "s"}`;
  if (attention.status === "ATTENTION")
    return `${attention.attention_count} item${attention.attention_count === 1 ? "" : "s"} to review`;
  return "All monitored controls clear";
}

export function OwnerAttentionCenter({
  attention,
  available,
}: {
  attention?: OwnerAttentionOverview;
  available: boolean;
}) {
  const status = !available
    ? "unavailable"
    : (attention?.status.toLowerCase() ?? "unavailable");

  return (
    <section
      className={`owner-attention-center is-${status}`}
      id="attention-center"
      aria-labelledby="owner-attention-title"
    >
      <header>
        <div>
          <p className="command-kicker">
            <span /> OWNER ATTENTION
          </p>
          <h2 id="owner-attention-title">What needs you now.</h2>
          <p>
            One credential-free view of active connection, portfolio,
            automation, and safety conditions.
          </p>
        </div>
        <span className="owner-attention-status">
          <i aria-hidden="true" /> {statusLabel(attention)}
        </span>
      </header>

      {!available || !attention ? (
        <div className="owner-attention-state is-unavailable" role="status">
          <strong>Attention status is unavailable.</strong>
          <p>
            Arbion is not inferring a healthy state. Your existing controls
            remain unchanged while this read-only view recovers.
          </p>
        </div>
      ) : attention.items.length === 0 ? (
        <div className="owner-attention-state is-clear" role="status">
          <strong>No active issues.</strong>
          <p>
            Arbion&apos;s monitored connection, portfolio, automation, and
            safety controls are clear.
          </p>
        </div>
      ) : (
        <div className="owner-attention-list">
          {attention.items.map((item) => {
            const presentation =
              presentations[item.code] ?? unknownPresentation;
            return (
              <article
                className={`is-${item.severity.toLowerCase()}`}
                key={item.id}
              >
                <span className="owner-attention-icon" aria-hidden="true">
                  {item.severity === "STOPPED" ? "■" : "◆"}
                </span>
                <div>
                  <header>
                    <strong>{presentation.title}</strong>
                    <span>{readableTime(item.occurred_at)}</span>
                  </header>
                  <p>{presentation.detail}</p>
                  {item.code === "SCHEDULE_FAILURE" && item.count > 1 && (
                    <small>{item.count} consecutive failed cycles</small>
                  )}
                </div>
                <Link href={presentation.href(item)}>
                  {presentation.linkLabel} <span aria-hidden="true">→</span>
                </Link>
              </article>
            );
          })}
        </div>
      )}

      <footer>
        Read-only status. No credentials, provider payloads, holdings, or broker
        actions are exposed or requested.
      </footer>
    </section>
  );
}
