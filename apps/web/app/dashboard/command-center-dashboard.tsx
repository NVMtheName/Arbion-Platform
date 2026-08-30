"use client";

import { motion } from "motion/react";
import Link from "next/link";

import { ArbionBrand } from "../brand";
import { AppNavigation } from "../app-navigation";
import { LogoutButton } from "./logout-button";
import {
  OwnerAttentionCenter,
  type OwnerAttentionOverview,
} from "./owner-attention-center";
import { formatExactMoney, sumExactMoney } from "../exact-money";

type DashboardUser = {
  email: string;
  display_name: string;
  entitlement: string;
  role: string;
};

export type DashboardMoney = {
  amount: string;
  currency: string;
};

export type DashboardAccountSummary = {
  id: string;
  provider: string;
  displayName: string;
  status: string;
  observedValue?: DashboardMoney;
  cash?: DashboardMoney;
  positionCount?: number;
  availability: "ready" | "partial" | "unavailable";
  asOf?: string;
};

export type DashboardAIEngineSummary = {
  id: string;
  mandateID: string;
  accountName: string;
  provider: string;
  status: string;
  currentState: string;
  executionMode: string;
  modelID?: string;
  lastEvaluatedAt?: string;
  nextRunAt?: string;
  scheduleStatus?: string;
  scheduleAvailable?: boolean;
  journalAvailable?: boolean;
  consecutiveFailures: number;
  lastDecision?: string;
  lastDecisionSymbol?: string;
  lastDecisionAt?: string;
  latestDecisionAIProvider?: string;
  latestDecisionAIModelID?: string;
  latestDecisionAIProfile?: string;
  latestDecisionLatencyMS?: number;
  latestDecisionInputUsage?: number;
  latestDecisionOutputUsage?: number;
  evidenceAvailable?: boolean;
  evidenceStatus?: string;
  evidenceBlockers?: string[];
  oneHourSampleSize?: number;
  twentyFourHourSampleSize?: number;
  minimumSamplePerHorizon?: number;
  evidenceWindowHours?: number;
  minimumEvidenceWindowHours?: number;
};

type DashboardProps = {
  user: DashboardUser;
  connectionCount: number;
  modelConfigured: boolean;
  modelID?: string;
  accounts: DashboardAccountSummary[];
  aiEngines?: DashboardAIEngineSummary[];
  attention?: OwnerAttentionOverview;
  attentionAvailable?: boolean;
};

const enter = {
  hidden: { opacity: 0, y: 16 },
  visible: { opacity: 1, y: 0 },
};

function firstName(user: DashboardUser) {
  const preferred = user.display_name.trim();
  if (preferred) return preferred.split(/\s+/)[0];
  return user.email.split("@")[0];
}

function formatMoney(money?: DashboardMoney) {
  return formatExactMoney(money);
}

function observedPortfolio(accounts: DashboardAccountSummary[]) {
  const values = accounts.filter(
    (
      account,
    ): account is DashboardAccountSummary & {
      observedValue: DashboardMoney;
    } => Boolean(account.observedValue),
  );
  const money = sumExactMoney(values.map((account) => account.observedValue));
  if (!money) return undefined;
  return {
    money,
    reporting: values.length,
  };
}

function providerLabel(provider: string) {
  if (provider === "coinbase") return "Coinbase";
  if (provider === "schwab") return "Charles Schwab";
  return provider;
}

function providerInitial(provider: string) {
  if (provider === "schwab") return "S";
  if (provider === "coinbase") return "C";
  return provider.slice(0, 1).toUpperCase();
}

function availabilityLabel(account: DashboardAccountSummary) {
  if (account.availability === "ready") return "Live";
  if (account.availability === "partial") return "Provider data partial";
  return "Provider refresh unavailable";
}

function readableTime(value?: string) {
  if (!value) return "—";
  const parsed = new Date(value);
  if (Number.isNaN(parsed.valueOf())) return "—";
  return new Intl.DateTimeFormat("en-US", {
    month: "short",
    day: "numeric",
    hour: "numeric",
    minute: "2-digit",
    timeZone: "UTC",
    timeZoneName: "short",
  }).format(parsed);
}

function decisionLabel(engine: DashboardAIEngineSummary) {
  if (engine.journalAvailable === false) return "Decision journal unavailable";
  if (engine.lastDecision === "ALLOW_WOULD_HAVE_SUBMITTED")
    return `Would have submitted${engine.lastDecisionSymbol ? ` · ${engine.lastDecisionSymbol}` : ""}`;
  if (engine.lastDecision === "ALLOW_SIMULATED_FILLED")
    return `Simulated fill${engine.lastDecisionSymbol ? ` · ${engine.lastDecisionSymbol}` : ""}`;
  if (engine.lastDecision === "ALLOW_SIMULATED_REJECTED")
    return `Simulation rejected${engine.lastDecisionSymbol ? ` · ${engine.lastDecisionSymbol}` : ""}`;
  if (engine.lastDecision?.startsWith("DENY_"))
    return `Held by controls${engine.lastDecisionSymbol ? ` · ${engine.lastDecisionSymbol}` : ""}`;
  if (engine.lastDecision === "ABSTAIN") return "Abstained · No action";
  return "Awaiting first decision";
}

function engineHealth(engine: DashboardAIEngineSummary) {
  if (engine.status === "PAUSED") return "Paused by owner";
  if (engine.journalAvailable === false) return "Decision journal unavailable";
  if (engine.scheduleAvailable === false) return "Schedule status unavailable";
  if (engine.consecutiveFailures > 0 || engine.scheduleStatus === "FAILED")
    return "Schedule needs review";
  if (engine.nextRunAt) return "Healthy schedule";
  return "Manual cycles only";
}

function engineNeedsReview(engine: DashboardAIEngineSummary) {
  return (
    engine.journalAvailable === false ||
    engine.scheduleAvailable === false ||
    engine.consecutiveFailures > 0 ||
    engine.scheduleStatus === "FAILED"
  );
}

function engineStatus(engine: DashboardAIEngineSummary) {
  if (engine.status === "PAUSED") return "Paused";
  if (engine.currentState === "AI_MONITORING") return "Monitoring";
  return engine.currentState
    .toLowerCase()
    .split("_")
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(" ");
}

function evidenceStatusLabel(status?: string) {
  if (status === "EVIDENCE_REVIEWABLE") return "Reviewable evidence";
  if (status === "COLLECTING_EVIDENCE") return "Collecting evidence";
  if (status) return status.replaceAll("_", " ");
  return "Evidence unavailable";
}

function evidenceBlockerLabel(value: string) {
  const labels: Record<string, string> = {
    ONE_HOUR_SAMPLE_INCOMPLETE: "Collect more 1-hour outcome marks",
    TWENTY_FOUR_HOUR_SAMPLE_INCOMPLETE: "Collect more 24-hour outcome marks",
    EVIDENCE_WINDOW_INCOMPLETE: "Observe the mandate across a longer window",
    SCHEDULE_NOT_VERIFIED: "Complete a healthy scheduled cycle",
    SCHEDULE_UNHEALTHY: "Resolve the current scheduler failure",
  };
  return labels[value] ?? value.replaceAll("_", " ");
}

function aiProviderLabel(value: string) {
  if (value === "openai") return "OpenAI";
  if (value === "anthropic") return "Claude";
  if (value === "gemini") return "Gemini";
  return value;
}

function latestRouteLabel(engine: DashboardAIEngineSummary) {
  if (
    !engine.latestDecisionAIProvider ||
    !engine.latestDecisionAIModelID ||
    !engine.latestDecisionAIProfile
  )
    return "Unattributed legacy route";
  return `${aiProviderLabel(engine.latestDecisionAIProvider)} · ${engine.latestDecisionAIModelID} · ${engine.latestDecisionAIProfile}`;
}

function latestTelemetryLabel(engine: DashboardAIEngineSummary) {
  const values = [
    engine.latestDecisionLatencyMS,
    engine.latestDecisionInputUsage,
    engine.latestDecisionOutputUsage,
  ];
  if (
    values.some(
      (value) => value === undefined || !Number.isFinite(value) || value < 0,
    )
  )
    return "Telemetry unavailable";
  const integer = new Intl.NumberFormat("en-US", { maximumFractionDigits: 0 });
  return `${integer.format(engine.latestDecisionLatencyMS ?? 0)} ms · ${integer.format(engine.latestDecisionInputUsage ?? 0)} in / ${integer.format(engine.latestDecisionOutputUsage ?? 0)} out`;
}

function evidenceSummary(engine: DashboardAIEngineSummary) {
  if (!engine.evidenceAvailable) {
    return {
      label: "Evidence unavailable",
      detail: "Open the strategy to inspect its immutable Shadow scorecard.",
      completed: 0,
      required: 1,
      blockers: [],
    };
  }

  const minimum = Math.max(engine.minimumSamplePerHorizon ?? 20, 1);
  const oneHour = Math.max(engine.oneHourSampleSize ?? 0, 0);
  const twentyFourHour = Math.max(engine.twentyFourHourSampleSize ?? 0, 0);
  const required = minimum * 2;
  const completed = Math.min(oneHour + twentyFourHour, required);
  const window = Math.max(engine.evidenceWindowHours ?? 0, 0);
  const minimumWindow = Math.max(engine.minimumEvidenceWindowHours ?? 168, 0);

  return {
    label: evidenceStatusLabel(engine.evidenceStatus),
    detail: `${oneHour}/${minimum} one-hour · ${twentyFourHour}/${minimum} 24-hour · ${window}h/${minimumWindow}h window`,
    completed,
    required,
    blockers: engine.evidenceBlockers ?? [],
  };
}

export function CommandCenterDashboard({
  user,
  connectionCount,
  modelConfigured,
  modelID,
  accounts,
  aiEngines = [],
  attention,
  attentionAvailable = true,
}: DashboardProps) {
  const activeAccounts = accounts.filter(
    (account) => account.status === "active",
  );
  const accountCount = activeAccounts.length;
  const portfolio = observedPortfolio(activeAccounts);
  const setupComplete =
    accountCount > 0 && connectionCount > 0 && modelConfigured;
  const nextSetupAction =
    accountCount === 0
      ? {
          href: "/connections#financial-accounts",
          label: "Connect a financial account",
        }
      : connectionCount === 0
        ? {
            href: "/connections#ai-providers",
            label: "Connect an AI provider",
          }
        : !modelConfigured
          ? {
              href: "/connections#model-choice",
              label: "Choose your AI model",
            }
          : { href: "/automations/new", label: "Build a strategy" };

  return (
    <main className="command-dashboard portfolio-dashboard">
      <header className="command-topbar">
        <ArbionBrand className="command-brand" href="/dashboard" priority />
        <AppNavigation className="command-navigation" />
        <div className="command-account-actions">
          {(user.role === "admin" || user.role === "superadmin") && (
            <Link href="/admin">Admin</Link>
          )}
          <LogoutButton />
        </div>
      </header>

      <motion.section
        className="command-welcome portfolio-welcome"
        initial="hidden"
        animate="visible"
        transition={{ staggerChildren: 0.08 }}
      >
        <motion.div variants={enter}>
          <p className="command-kicker">
            <span /> WELCOME, {firstName(user).toUpperCase()}
          </p>
          <h1>Your portfolio. One clear view.</h1>
          <p>
            Real connected accounts, your chosen AI model, and your strategies
            together—without the infrastructure noise.
          </p>
        </motion.div>
        <motion.div className="command-primary-actions" variants={enter}>
          <Link className="command-primary-link" href={nextSetupAction.href}>
            {nextSetupAction.label} <span aria-hidden="true">↗</span>
          </Link>
          <Link className="command-secondary-link" href="/connections">
            Manage connections
          </Link>
        </motion.div>
      </motion.section>

      <OwnerAttentionCenter
        attention={attention}
        available={attentionAvailable}
      />

      {!setupComplete && (
        <motion.section
          className="command-setup-path"
          aria-labelledby="setup-path-title"
          initial={{ opacity: 0, y: 14 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ delay: 0.2 }}
        >
          <header>
            <div>
              <p className="command-kicker">FINISH SETUP</p>
              <h2 id="setup-path-title">Connect. Choose. Build.</h2>
            </div>
            <Link href="/connections">Open connection hub ↗</Link>
          </header>
          <ol>
            <li className={accountCount > 0 ? "is-complete" : ""}>
              <span>1</span>
              <div>
                <strong>Financial account</strong>
                <small>{accountCount > 0 ? "Connected" : "Connect one"}</small>
              </div>
            </li>
            <li className={connectionCount > 0 ? "is-complete" : ""}>
              <span>2</span>
              <div>
                <strong>AI provider</strong>
                <small>
                  {connectionCount > 0 ? "Connected" : "Add your API key"}
                </small>
              </div>
            </li>
            <li className={modelConfigured ? "is-complete" : ""}>
              <span>3</span>
              <div>
                <strong>AI model</strong>
                <small>{modelConfigured ? "Selected" : "Choose one"}</small>
              </div>
            </li>
          </ol>
        </motion.section>
      )}

      <motion.section
        className="ai-engine-cockpit"
        aria-labelledby="ai-engine-cockpit-title"
        initial={{ opacity: 0, y: 18 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ delay: 0.16 }}
      >
        <header>
          <div>
            <p className="command-kicker">AUTONOMOUS ENGINE</p>
            <h2 id="ai-engine-cockpit-title">AI oversight at a glance.</h2>
            <p>
              Current model, schedule health, and immutable non-live reasoning
              across your connected accounts.
            </p>
          </div>
          <Link href="/automations">Open AI strategies ↗</Link>
        </header>

        {aiEngines.length === 0 ? (
          <div className="ai-engine-empty">
            <strong>No AI Engine is monitoring yet.</strong>
            <p>
              Start with a bounded, non-live engine tied to one account and one
              model.
            </p>
            <Link href="/automations/new">Create an AI Engine →</Link>
          </div>
        ) : (
          <div className="ai-engine-grid">
            {aiEngines.map((engine) => {
              const paper = engine.executionMode === "PAPER";
              const evidence = paper ? undefined : evidenceSummary(engine);
              return (
                <article key={engine.id}>
                  <header>
                    <div>
                      <span
                        className={`provider-mark provider-${engine.provider}`}
                      >
                        {providerInitial(engine.provider)}
                      </span>
                      <div>
                        <strong>{engine.accountName}</strong>
                        <small>{providerLabel(engine.provider)}</small>
                      </div>
                    </div>
                    <div className="ai-engine-badges">
                      <span
                        className={
                          engine.status === "ACTIVE"
                            ? "is-monitoring"
                            : "is-paused"
                        }
                      >
                        {engineStatus(engine)}
                      </span>
                      <span className={paper ? "is-paper" : "is-shadow"}>
                        {paper ? "Paper simulation" : "Shadow only"}
                      </span>
                    </div>
                  </header>
                  <dl>
                    <div>
                      <dt>Model</dt>
                      <dd>{engine.modelID ?? "Not recorded"}</dd>
                    </div>
                    <div>
                      <dt>Latest decision</dt>
                      <dd>{decisionLabel(engine)}</dd>
                    </div>
                    <div>
                      <dt>AI route</dt>
                      <dd>{latestRouteLabel(engine)}</dd>
                    </div>
                    <div>
                      <dt>Route telemetry</dt>
                      <dd>{latestTelemetryLabel(engine)}</dd>
                    </div>
                    <div>
                      <dt>Last cycle</dt>
                      <dd>
                        {readableTime(
                          engine.lastDecisionAt ?? engine.lastEvaluatedAt,
                        )}
                      </dd>
                    </div>
                    <div>
                      <dt>Next cycle</dt>
                      <dd>
                        {engine.nextRunAt
                          ? readableTime(engine.nextRunAt)
                          : "Not scheduled"}
                      </dd>
                    </div>
                  </dl>
                  {paper ? (
                    <div
                      className="ai-engine-evidence is-paper"
                      aria-label="Paper simulation boundary"
                    >
                      <div className="ai-engine-evidence-heading">
                        <span>PAPER LEDGER</span>
                        <strong>Simulation only</strong>
                      </div>
                      <small>
                        Isolated simulated cash, positions, and fills. The
                        connected account supplies prices only; no broker order
                        can be sent.
                      </small>
                    </div>
                  ) : (
                    <div
                      className="ai-engine-evidence"
                      aria-label="Shadow evidence progress"
                    >
                      <div className="ai-engine-evidence-heading">
                        <span>Shadow evidence</span>
                        <strong>{evidence?.label}</strong>
                      </div>
                      <progress
                        max={evidence?.required}
                        value={evidence?.completed}
                        aria-label={`${evidence?.completed} of ${evidence?.required} evidence marks`}
                      />
                      <small>{evidence?.detail}</small>
                      {evidence && evidence.blockers.length > 0 && (
                        <ul
                          className="ai-engine-evidence-blockers"
                          aria-label="Shadow evidence blockers"
                        >
                          {evidence.blockers.map((blocker) => (
                            <li key={blocker}>
                              {evidenceBlockerLabel(blocker)}
                            </li>
                          ))}
                        </ul>
                      )}
                    </div>
                  )}
                  <footer>
                    <span
                      className={
                        engineNeedsReview(engine) ? "needs-review" : "healthy"
                      }
                    >
                      <i /> {engineHealth(engine)}
                    </span>
                    <Link href={`/automations/${engine.mandateID}`}>
                      Review journal →
                    </Link>
                  </footer>
                </article>
              );
            })}
          </div>
        )}
        <p className="ai-engine-safety">
          No broker order can be sent. Every displayed decision is non-live and
          remains subject to Arbion&apos;s deterministic controls.
        </p>
      </motion.section>

      <motion.section
        className="portfolio-command"
        aria-labelledby="portfolio-command-title"
        initial={{ opacity: 0, y: 18 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ delay: 0.18 }}
      >
        <header className="portfolio-command-header">
          <div>
            <p className="command-kicker">CONNECTED PORTFOLIO</p>
            <h2 id="portfolio-command-title">What you own, right now.</h2>
          </div>
          <Link href="/accounts">Open full portfolio ↗</Link>
        </header>

        {accountCount === 0 ? (
          <div className="portfolio-command-empty">
            <strong>Your accounts will appear here.</strong>
            <p>
              Connect Schwab or Coinbase with your own developer credentials.
            </p>
            <Link href="/connections#financial-accounts">
              Connect an account →
            </Link>
          </div>
        ) : (
          <>
            <div className="portfolio-command-total">
              <span>Observed portfolio value</span>
              <strong>
                {portfolio ? formatMoney(portfolio.money) : "Unavailable"}
              </strong>
              <small>
                {portfolio
                  ? `${portfolio.reporting} of ${accountCount} connected account${accountCount === 1 ? "" : "s"} reporting`
                  : `${accountCount} connected account${accountCount === 1 ? "" : "s"}`}
              </small>
            </div>
            <div className="portfolio-account-grid">
              {activeAccounts.map((account, index) => (
                <motion.article
                  initial={{ opacity: 0, y: 12 }}
                  animate={{ opacity: 1, y: 0 }}
                  transition={{ delay: 0.24 + index * 0.06 }}
                  key={account.id}
                >
                  <header>
                    <div>
                      <span
                        className={`provider-mark provider-${account.provider}`}
                      >
                        {providerInitial(account.provider)}
                      </span>
                      <div>
                        <strong>{account.displayName}</strong>
                        <small>{providerLabel(account.provider)}</small>
                      </div>
                    </div>
                    <span
                      className={`portfolio-account-status ${account.availability}`}
                    >
                      {availabilityLabel(account)}
                    </span>
                  </header>
                  <div className="portfolio-account-value">
                    <span>Account value</span>
                    <strong>{formatMoney(account.observedValue)}</strong>
                  </div>
                  <dl>
                    <div>
                      <dt>Cash</dt>
                      <dd>{formatMoney(account.cash)}</dd>
                    </div>
                    <div>
                      <dt>Positions</dt>
                      <dd>{account.positionCount ?? "Unavailable"}</dd>
                    </div>
                  </dl>
                  <Link href={`/accounts/${account.id}`}>Open account →</Link>
                </motion.article>
              ))}
            </div>
          </>
        )}
      </motion.section>

      <motion.section
        className="strategy-launchpad"
        initial={{ opacity: 0, y: 18 }}
        whileInView={{ opacity: 1, y: 0 }}
        viewport={{ once: true, amount: 0.3 }}
      >
        <div>
          <p className="command-kicker">STRATEGY LAUNCHPAD</p>
          <h2>Account → AI → strategy.</h2>
          <p>
            Choose the connected account and model, then start with a safe,
            reviewable paper strategy.
          </p>
        </div>
        <dl>
          <div>
            <dt>Accounts</dt>
            <dd>{accountCount || "Not connected"}</dd>
          </div>
          <div>
            <dt>AI model</dt>
            <dd>{modelID || "Not selected"}</dd>
          </div>
          <div>
            <dt>Starting guardrail</dt>
            <dd>Paper · confirm each action</dd>
          </div>
        </dl>
        <Link
          className={
            setupComplete ? "command-primary-link" : "command-secondary-link"
          }
          href={setupComplete ? "/automations/new" : nextSetupAction.href}
        >
          {setupComplete ? "Create a strategy" : nextSetupAction.label}
          <span aria-hidden="true">↗</span>
        </Link>
      </motion.section>

      <footer className="command-footer">
        <span>Your connected accounts remain under your control.</span>
        <span>Live broker execution is not enabled.</span>
      </footer>
    </main>
  );
}
