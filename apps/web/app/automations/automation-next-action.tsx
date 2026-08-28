import Link from "next/link";

import type { StrategyInitializationAssessment } from "./strategy-initialization-readiness";

type Entity = Record<string, unknown> | null | undefined;

export type AutomationNextActionInput = {
  automationId: string;
  automationType: string;
  autonomyLevel: string;
  executionMode: string;
  assessment: StrategyInitializationAssessment;
  instance?: Entity;
  schedule?: Entity;
  scheduleConfigured: boolean;
  schedulerEnabled: boolean;
};

export type AutomationNextAction = {
  tone: "READY" | "RUNNING" | "ATTENTION" | "PAUSED";
  eyebrow: string;
  title: string;
  detail: string;
  href: string;
  actionLabel: string;
};

function text(entity: Entity, key: string, legacy: string) {
  const value = entity?.[key] ?? entity?.[legacy];
  return typeof value === "string" ? value : "";
}

function integer(entity: Entity, key: string, legacy: string) {
  const value = entity?.[key] ?? entity?.[legacy];
  return typeof value === "number" && Number.isInteger(value) ? value : 0;
}

function readableTime(value: string) {
  const parsed = new Date(value);
  if (Number.isNaN(parsed.valueOf())) return "at its next eligible time";
  return new Intl.DateTimeFormat("en-US", {
    month: "short",
    day: "numeric",
    hour: "numeric",
    minute: "2-digit",
    timeZone: "UTC",
    timeZoneName: "short",
  }).format(parsed);
}

function blockedAction(input: AutomationNextActionInput): AutomationNextAction {
  const blocker = input.assessment.checks.find(
    (check) => check.blocking && check.state !== "READY",
  );
  const common = {
    tone: "ATTENTION" as const,
    eyebrow: "NEXT BLOCKER TO CLEAR",
  };
  switch (blocker?.code) {
    case "CURRENT_INVENTORY":
      return {
        ...common,
        title: "Refresh the current safety inventory",
        detail:
          "Arbion is refusing to infer readiness from a partial account, connection, capital, or strategy snapshot.",
        href: `/automations/${input.automationId}`,
        actionLabel: "Refresh this automation",
      };
    case "MANDATE_VERSION":
      return {
        ...common,
        title: "Review and mark this version Ready",
        detail:
          "Ready pins the exact saved configuration for non-live use. It does not initialize an engine or send an order.",
        href: "#mandate-lifecycle-controls",
        actionLabel: "Go to lifecycle controls",
      };
    case "FINANCIAL_CONNECTION":
      return {
        ...common,
        title: "Restore the financial connection",
        detail:
          "The selected account and its credential connection must both be active before Arbion can create the non-live instance.",
        href: "/connections",
        actionLabel: "Open account connections",
      };
    case "AI_ROUTE":
      return {
        ...common,
        title: "Restore the AI model connection",
        detail:
          "The saved model must match an active, enabled provider connection before the Shadow Engine can initialize.",
        href: "/settings/connections#ai-connections",
        actionLabel: "Open AI connections",
      };
    case "CAPITAL_POLICY":
    case "ACCOUNT_CAPACITY":
      return {
        ...common,
        title: "Resolve the capital claim",
        detail: blocker.detail,
        href: "/capital",
        actionLabel: "Open Capital Center",
      };
    case "INSTANCE_IDENTITY":
      return {
        ...common,
        title: "Create a new immutable version",
        detail:
          "This mandate version already produced a historical instance. Change a bounded control, review the new draft, and mark that version Ready.",
        href: "#configuration-controls",
        actionLabel: "Open version controls",
      };
    case "EXECUTION_BOUNDARY":
      return {
        ...common,
        title: "Choose a supported non-live mode",
        detail:
          "Only PAPER or SHADOW can initialize. LIVE and BACKTEST remain unavailable in this release.",
        href: "/automations/new",
        actionLabel: "Create a non-live draft",
      };
    default:
      return {
        ...common,
        title: "Review the saved non-live configuration",
        detail:
          blocker?.detail ??
          "The current mandate does not yet satisfy every initialization control.",
        href: "#strategy-readiness-check",
        actionLabel: "Review readiness checks",
      };
  }
}

export function selectAutomationNextAction(
  input: AutomationNextActionInput,
): AutomationNextAction {
  const instanceID = text(input.instance, "id", "ID");
  const instanceStatus = text(input.instance, "status", "Status");
  const currentState = text(input.instance, "current_state", "CurrentState");
  const scheduleStatus = text(input.schedule, "last_status", "LastStatus");
  const failures = integer(
    input.schedule,
    "consecutive_failures",
    "ConsecutiveFailures",
  );
  const nextRun = text(input.schedule, "next_run_at", "NextRunAt");
  const autonomous =
    input.autonomyLevel === "FULL_AUTONOMOUS" ||
    input.autonomyLevel === "STRATEGY_AUTONOMOUS";

  if (instanceID) {
    if (instanceStatus === "ERROR" || currentState === "ERROR") {
      return {
        tone: "ATTENTION",
        eyebrow: "ENGINE REVIEW REQUIRED",
        title: "Review the latest immutable runtime evidence",
        detail:
          "The non-live instance is in an error state. No broker order was sent, and Arbion will not continue it automatically.",
        href: "#runtime-evidence",
        actionLabel: "Open runtime evidence",
      };
    }
    if (instanceStatus === "PAUSED") {
      return {
        tone: "PAUSED",
        eyebrow: "PAUSED BY OWNER",
        title: "Resume only when you are ready",
        detail:
          "The strategy retains its Arbion capital claim, but manual and scheduled evaluations are stopped. Resuming remains non-live.",
        href: "#strategy-instance-controls",
        actionLabel: "Open resume controls",
      };
    }
    if (instanceStatus === "ACTIVE") {
      if (scheduleStatus === "FAILED" || failures > 0) {
        return {
          tone: "ATTENTION",
          eyebrow: "SCHEDULE NEEDS REVIEW",
          title: "Inspect the failed non-live cycle",
          detail: `${failures} consecutive ${failures === 1 ? "failure" : "failures"}. Arbion failed closed and sent no broker order.`,
          href: "#schedule-controls",
          actionLabel: "Review schedule health",
        };
      }
      if (
        autonomous &&
        input.scheduleConfigured &&
        input.schedulerEnabled &&
        nextRun
      ) {
        return {
          tone: "RUNNING",
          eyebrow: "AUTONOMY IS RUNNING",
          title: "Let the next guarded cycle run",
          detail: `Next eligible evaluation ${readableTime(nextRun)}. The browser may close; the server-side engine remains PAPER/SHADOW only.`,
          href:
            input.automationType === "AI_AUTONOMOUS"
              ? "#decision-journal"
              : "#schedule-controls",
          actionLabel:
            input.automationType === "AI_AUTONOMOUS"
              ? "View decision journal"
              : "View schedule status",
        };
      }
      if (autonomous) {
        return {
          tone: "READY",
          eyebrow: "ENGINE ACTIVE · SCHEDULE OFF",
          title: "Configure guarded automatic cycles",
          detail:
            "The instance is active, but it will not evaluate autonomously until a valid non-live schedule is saved and pinned.",
          href: "#schedule-controls",
          actionLabel: "Open schedule controls",
        };
      }
      return {
        tone: "READY",
        eyebrow: "INSTANCE ACTIVE",
        title: "Run a non-live evaluation when ready",
        detail:
          "The instance is ready for an owner-invoked PAPER or SHADOW cycle through the deterministic risk gate.",
        href: "#manual-evaluation",
        actionLabel: "Open evaluation controls",
      };
    }
    return {
      tone: "ATTENTION",
      eyebrow: "NEW VERSION REQUIRED",
      title: "Create and review the next version",
      detail:
        "This immutable version already produced a completed instance and cannot be reused for another initialization.",
      href: "#configuration-controls",
      actionLabel: "Open version controls",
    };
  }

  if (input.assessment.eligible) {
    return {
      tone: "READY",
      eyebrow: "READY FOR NON-LIVE INITIALIZATION",
      title:
        input.automationType === "AI_AUTONOMOUS"
          ? "Initialize the AI Shadow Engine"
          : `Initialize the ${input.executionMode} strategy`,
      detail:
        "The current exact checks pass. Initialization creates a durable non-live identity and capital claim, never a broker order.",
      href: "#mandate-lifecycle-controls",
      actionLabel: "Go to initialize",
    };
  }

  return blockedAction(input);
}

export function AutomationNextActionPanel({
  action,
}: {
  action: AutomationNextAction;
}) {
  return (
    <section
      className={`automation-next-action is-${action.tone.toLowerCase()}`}
      aria-label={action.title}
    >
      <div className="automation-next-action-signal" aria-hidden="true">
        <span />
      </div>
      <div>
        <p className="eyebrow">{action.eyebrow}</p>
        <h2>{action.title}</h2>
        <p>{action.detail}</p>
      </div>
      <Link href={action.href}>{action.actionLabel} →</Link>
      <footer>Guidance only · no provider call · no order authority</footer>
    </section>
  );
}
