import Link from "next/link";

import { AppPageHeader } from "../app-page-header";
import type { FinancialAccount } from "../settings/connections/page";
import {
  PortfolioHoldingsLedger,
  type PortfolioHolding,
} from "./portfolio-holdings-ledger";

function providerName(provider: string) {
  if (provider === "coinbase") return "Coinbase";
  if (provider === "schwab") return "Charles Schwab";
  return provider || "Provider unavailable";
}

export function PortfolioWorkspace({
  accounts,
  accountsAvailable,
  holdings,
  unavailableAccounts,
}: {
  accounts: FinancialAccount[];
  accountsAvailable: boolean;
  holdings: PortfolioHolding[];
  unavailableAccounts: string[];
}) {
  return (
    <main className="connections-page portfolio-ledger-page command-content-continuity">
      <AppPageHeader contentHeadingId="portfolio-page-title" />
      <header className="portfolio-workspace-heading">
        <div>
          <p className="eyebrow">CONNECTED ASSETS</p>
          <h1 id="portfolio-page-title">Portfolio</h1>
          <p>Holdings, prices and provider-supplied returns in one view.</p>
        </div>
        <Link
          className="portfolio-manage-link"
          href="/connections#financial-accounts"
        >
          Manage connections <span aria-hidden="true">↗</span>
        </Link>
      </header>

      {!accountsAvailable ? (
        <section className="portfolio-empty-state is-unavailable" role="status">
          <p className="eyebrow">DATA UNAVAILABLE</p>
          <h2>Your accounts could not be loaded</h2>
          <p>
            This is not an empty portfolio. Reload the page to try again, or
            review your connections. No account or position was changed by this
            view.
          </p>
        </section>
      ) : accounts.length === 0 ? (
        <section className="portfolio-empty-state">
          <p className="eyebrow">YOUR PORTFOLIO STARTS HERE</p>
          <h2>Connect your first account</h2>
          <p>
            Connect Coinbase or Schwab to see balances and positions here. Your
            assets stay with your provider.
          </p>
          <Link className="button-link" href="/connections#financial-accounts">
            Open connection hub <span aria-hidden="true">↗</span>
          </Link>
        </section>
      ) : (
        <>
          <PortfolioHoldingsLedger
            holdings={holdings}
            unavailableAccounts={unavailableAccounts}
          />
          <section
            className="portfolio-connected-accounts"
            aria-labelledby="accounts-title"
          >
            <header>
              <div>
                <p className="eyebrow">ACCOUNT WORKSPACES</p>
                <h2 id="accounts-title">Your connected accounts</h2>
              </div>
              <p>Balances, authorization and saved account evidence</p>
            </header>
            <div className="provider-list">
              {accounts.map((account) => (
                <article key={account.id}>
                  <span
                    className={`provider-mark provider-${account.provider}`}
                    aria-hidden="true"
                  >
                    {account.provider === "coinbase"
                      ? "C"
                      : account.provider === "schwab"
                        ? "S"
                        : "·"}
                  </span>
                  <div>
                    <h3>{account.display_name}</h3>
                    <p>{providerName(account.provider)}</p>
                    <span
                      className={`portfolio-account-status ${account.status === "active" ? "" : "needs-review"}`}
                    >
                      {account.status || "Status unavailable"}
                    </span>
                  </div>
                  <Link
                    href={`/accounts/${account.id}`}
                    aria-label={`Open ${account.display_name}`}
                  >
                    Open account <span aria-hidden="true">↗</span>
                  </Link>
                </article>
              ))}
            </div>
          </section>
        </>
      )}
    </main>
  );
}
