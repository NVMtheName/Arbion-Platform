"use client";

import { motion } from "motion/react";
import Link from "next/link";

import { ArbionBrand } from "../brand";
import { LogoutButton } from "./logout-button";

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

type DashboardProps = {
  user: DashboardUser;
  connectionCount: number;
  modelConfigured: boolean;
  modelID?: string;
  accounts: DashboardAccountSummary[];
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
  if (!money) return "Unavailable";
  const amount = Number(money.amount);
  if (!Number.isFinite(amount)) return `${money.amount} ${money.currency}`;
  return new Intl.NumberFormat("en-US", {
    style: "currency",
    currency: money.currency,
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  }).format(amount);
}

function observedPortfolio(accounts: DashboardAccountSummary[]) {
  const values = accounts.filter(
    (
      account,
    ): account is DashboardAccountSummary & {
      observedValue: DashboardMoney;
    } => Boolean(account.observedValue),
  );
  const currencies = new Set(
    values.map((account) => account.observedValue.currency),
  );
  if (values.length === 0 || currencies.size !== 1) return undefined;
  const amount = values.reduce(
    (total, account) => total + Number(account.observedValue.amount),
    0,
  );
  if (!Number.isFinite(amount)) return undefined;
  return {
    money: {
      amount: String(amount),
      currency: values[0].observedValue.currency,
    },
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
  if (account.availability === "partial") return "Partial";
  return "Refresh unavailable";
}

export function CommandCenterDashboard({
  user,
  connectionCount,
  modelConfigured,
  modelID,
  accounts,
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
        <nav aria-label="Command center navigation">
          <Link className="is-current" href="/dashboard">
            Home
          </Link>
          <Link href="/accounts">Portfolio</Link>
          <Link href="/automations">Strategies</Link>
          <Link href="/markets">Markets</Link>
        </nav>
        <div className="command-account-actions">
          <Link href="/connections">Connections</Link>
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
