"use client";

import { motion } from "motion/react";
import Link from "next/link";
import type { ReactNode } from "react";

import { ArbionBrand } from "../brand";
import type { JournalEntry } from "../activity/journal-list";
import type { MarketSource } from "../markets/market-source-grid";
import { LogoutButton } from "./logout-button";

type DashboardUser = {
  email: string;
  display_name: string;
  entitlement: string;
  role: string;
};

type DashboardProps = {
  user: DashboardUser;
  connectionCount: number;
  accountCount: number;
  sources: MarketSource[];
  journalEntries: JournalEntry[];
  asOf: string;
  children: ReactNode;
};

type HeatmapDay = {
  date: string;
  label: string;
  count: number;
};

const enter = {
  hidden: { opacity: 0, y: 16 },
  visible: { opacity: 1, y: 0 },
};

function readable(value: string) {
  return value.replaceAll("_", " ").toLowerCase();
}

function buildHeatmap(entries: JournalEntry[], asOf: string): HeatmapDay[] {
  const anchor = new Date(asOf);
  anchor.setUTCHours(0, 0, 0, 0);
  const counts = new Map<string, number>();
  for (const entry of entries) {
    const parsed = new Date(entry.created_at);
    if (Number.isNaN(parsed.valueOf())) continue;
    const key = parsed.toISOString().slice(0, 10);
    counts.set(key, (counts.get(key) ?? 0) + 1);
  }
  return Array.from({ length: 35 }, (_, index) => {
    const date = new Date(anchor);
    date.setUTCDate(anchor.getUTCDate() - 34 + index);
    const key = date.toISOString().slice(0, 10);
    return {
      date: key,
      label: new Intl.DateTimeFormat("en-US", {
        month: "short",
        day: "numeric",
        timeZone: "UTC",
      }).format(date),
      count: counts.get(key) ?? 0,
    };
  });
}

function sourceState(source: MarketSource) {
  if (!source.enabled) return { label: "Standby", className: "standby" };
  if (!source.healthy) return { label: "Degraded", className: "degraded" };
  return { label: "Ready", className: "ready" };
}

function firstName(user: DashboardUser) {
  const preferred = user.display_name.trim();
  if (preferred) return preferred.split(/\s+/)[0];
  return user.email.split("@")[0];
}

export function CommandCenterDashboard({
  user,
  connectionCount,
  accountCount,
  sources,
  journalEntries,
  asOf,
  children,
}: DashboardProps) {
  const availableSources = sources.filter(
    (source) => source.enabled && source.healthy,
  ).length;
  const heatmap = buildHeatmap(journalEntries, asOf);
  const denied = journalEntries.filter(
    (entry) => entry.risk_decision === "DENY",
  ).length;
  const shadow = journalEntries.filter(
    (entry) => entry.execution_mode === "SHADOW",
  ).length;

  return (
    <main className="command-dashboard">
      <header className="command-topbar">
        <ArbionBrand className="command-brand" href="/dashboard" priority />
        <nav aria-label="Command center navigation">
          <Link className="is-current" href="/dashboard">
            Overview
          </Link>
          <Link href="/markets">Markets</Link>
          <Link href="/accounts">Portfolio</Link>
          <Link href="/automations">Automations</Link>
          <Link href="/activity">Journal</Link>
        </nav>
        <div className="command-account-actions">
          <Link href="/settings/security" aria-label="Security settings">
            Security
          </Link>
          {(user.role === "admin" || user.role === "superadmin") && (
            <Link href="/admin">Admin</Link>
          )}
          <LogoutButton />
        </div>
      </header>

      <motion.section
        className="command-welcome"
        initial="hidden"
        animate="visible"
        transition={{ staggerChildren: 0.08 }}
      >
        <motion.div variants={enter}>
          <p className="command-kicker">
            <span /> COMMAND CENTER · READ-ONLY
          </p>
          <h1>Welcome back, {firstName(user)}.</h1>
          <p>
            Your financial system at a glance—connections, market coverage,
            controls, and decision evidence without manufactured certainty.
          </p>
        </motion.div>
        <motion.div className="command-primary-actions" variants={enter}>
          <Link className="command-primary-link" href="/markets">
            Open market intelligence <span aria-hidden="true">↗</span>
          </Link>
          <Link className="command-secondary-link" href="/activity">
            Review decision journal
          </Link>
        </motion.div>
      </motion.section>

      <motion.section
        className="command-metric-rail"
        aria-label="Command center summary"
        initial="hidden"
        animate="visible"
        transition={{ delayChildren: 0.18, staggerChildren: 0.06 }}
      >
        <motion.article variants={enter}>
          <span>Financial accounts</span>
          <strong>{accountCount}</strong>
          <small>
            {accountCount > 0 ? "Broker connected" : "Awaiting connection"}
          </small>
        </motion.article>
        <motion.article variants={enter}>
          <span>AI providers</span>
          <strong>{connectionCount}</strong>
          <small>
            {connectionCount > 0 ? "Advisory access ready" : "Not configured"}
          </small>
        </motion.article>
        <motion.article variants={enter}>
          <span>Market sources</span>
          <strong>
            {availableSources}/{sources.length}
          </strong>
          <small>Verified adapters only</small>
        </motion.article>
        <motion.article variants={enter}>
          <span>Execution path</span>
          <strong>None</strong>
          <small>PAPER / SHADOW evidence only</small>
        </motion.article>
      </motion.section>

      <section className="command-workspace">
        <motion.article
          className="command-panel command-market-overview"
          initial={{ opacity: 0, y: 20 }}
          whileInView={{ opacity: 1, y: 0 }}
          viewport={{ once: true, amount: 0.2 }}
        >
          <header className="command-panel-header">
            <div>
              <p className="command-kicker">MARKET INTELLIGENCE</p>
              <h2>Source coverage</h2>
            </div>
            <Link href="/markets">View full surface ↗</Link>
          </header>
          <p className="command-panel-copy">
            Every provider remains visibly labeled by feed and quality. Standby
            means not yet configured—not unavailable market activity.
          </p>
          <div
            className="command-source-matrix"
            aria-label="Market source coverage"
          >
            {sources.length === 0 ? (
              <p className="command-empty-state">
                Source metadata is temporarily unavailable.
              </p>
            ) : (
              sources.map((source, index) => {
                const state = sourceState(source);
                return (
                  <motion.div
                    className={`command-source-cell ${state.className}`}
                    initial={{ opacity: 0, scale: 0.94 }}
                    whileInView={{ opacity: 1, scale: 1 }}
                    viewport={{ once: true }}
                    transition={{ delay: index * 0.045 }}
                    key={source.id}
                  >
                    <span>{source.label}</span>
                    <strong>{readable(source.quality)}</strong>
                    <small>
                      <i /> {state.label}
                    </small>
                  </motion.div>
                );
              })
            )}
          </div>
          <footer className="command-panel-footer">
            <span>
              <i className="ready" /> Ready
            </span>
            <span>
              <i className="standby" /> Standby
            </span>
            <span>
              <i className="degraded" /> Degraded
            </span>
            <strong>LIVE EXECUTION UNAVAILABLE</strong>
          </footer>
        </motion.article>

        <motion.article
          className="command-panel command-activity-panel"
          initial={{ opacity: 0, y: 20 }}
          whileInView={{ opacity: 1, y: 0 }}
          viewport={{ once: true, amount: 0.25 }}
        >
          <header className="command-panel-header">
            <div>
              <p className="command-kicker">DECISION EVIDENCE</p>
              <h2>35-day activity</h2>
            </div>
            <Link href="/activity">Open journal ↗</Link>
          </header>
          <div
            className="command-heatmap"
            role="img"
            aria-label={`Decision activity for the last 35 days. ${journalEntries.length} recent decision${journalEntries.length === 1 ? "" : "s"} loaded.`}
          >
            {heatmap.map((day, index) => {
              const level = Math.min(day.count, 4);
              return (
                <motion.span
                  className={`level-${level}`}
                  aria-label={`${day.label}: ${day.count} decision${day.count === 1 ? "" : "s"}`}
                  title={`${day.label}: ${day.count}`}
                  initial={{ opacity: 0, scale: 0.65 }}
                  whileInView={{ opacity: 1, scale: 1 }}
                  viewport={{ once: true }}
                  transition={{ delay: index * 0.012 }}
                  key={day.date}
                />
              );
            })}
          </div>
          <div className="command-heatmap-legend" aria-hidden="true">
            <span>Less</span>
            {[0, 1, 2, 3, 4].map((level) => (
              <i className={`level-${level}`} key={level} />
            ))}
            <span>More</span>
          </div>
          <dl className="command-activity-facts">
            <div>
              <dt>Recent decisions</dt>
              <dd>{journalEntries.length}</dd>
            </div>
            <div>
              <dt>Risk denied</dt>
              <dd>{denied}</dd>
            </div>
            <div>
              <dt>Shadow records</dt>
              <dd>{shadow}</dd>
            </div>
          </dl>
          {journalEntries.length === 0 && (
            <p className="command-empty-state">
              No decision evidence yet. The grid will populate from real PAPER
              and SHADOW evaluations—never sample trades.
            </p>
          )}
        </motion.article>
      </section>

      <section
        className="command-workspaces"
        aria-labelledby="workspaces-title"
      >
        <header>
          <div>
            <p className="command-kicker">WORKSPACES</p>
            <h2 id="workspaces-title">Move with context.</h2>
          </div>
          <p>
            Each workspace preserves Arbion&apos;s read-only and evidence-first
            boundaries.
          </p>
        </header>
        <div className="command-workspace-grid">
          {[
            [
              "01",
              "Portfolio",
              "Balances and positions from your connected broker.",
              "/accounts",
            ],
            [
              "02",
              "Automation",
              "Mandates, controls, and non-live strategy state.",
              "/automations",
            ],
            [
              "03",
              "Market intelligence",
              "Equities, options, crypto, and primary filings.",
              "/markets",
            ],
            [
              "04",
              "Decision journal",
              "Structured rationale and deterministic risk evidence.",
              "/activity",
            ],
          ].map(([index, title, copy, href], itemIndex) => (
            <motion.article
              initial={{ opacity: 0, y: 18 }}
              whileInView={{ opacity: 1, y: 0 }}
              whileHover={{ y: -5 }}
              viewport={{ once: true, amount: 0.25 }}
              transition={{ delay: itemIndex * 0.06 }}
              key={title}
            >
              <span>{index}</span>
              <h3>{title}</h3>
              <p>{copy}</p>
              <Link href={href}>
                Open workspace <span aria-hidden="true">↗</span>
              </Link>
            </motion.article>
          ))}
        </div>
      </section>

      <section className="command-neural-shell">
        <header>
          <p className="command-kicker">NEURAL ENGINE</p>
          <span>ADVISORY · NEVER AUTHORITATIVE</span>
        </header>
        {children}
      </section>

      <footer className="command-footer">
        <span>Plan · {readable(user.entitlement)}</span>
        <span>Arbion operates without a live broker execution path.</span>
      </footer>
    </main>
  );
}
