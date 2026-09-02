"use client";

import Link from "next/link";
import { useMemo, useState } from "react";

import { formatExactMoney, sumExactMoney } from "../exact-money";

export type HoldingMoney = { amount: string; currency: string };

export type PortfolioHolding = {
  key: string;
  accountID: string;
  accountName: string;
  provider: "coinbase" | "schwab" | string;
  symbol: string;
  instrumentType: string;
  direction: string;
  quantity: string;
  availableQuantity?: string;
  unavailableQuantity?: string;
  averagePrice?: HoldingMoney;
  currentPrice?: HoldingMoney;
  dayProfitLoss?: HoldingMoney;
  dayProfitLossPercent?: string;
  marketValue?: HoldingMoney;
  totalProfitLoss?: HoldingMoney;
  totalProfitLossPercent?: string;
  changeWindow: "DAY" | "24H";
  costBasisStatus: "AVAILABLE" | "UNAVAILABLE_FROM_PROVIDER";
  priceBasis?: string;
};

function numeric(value?: string) {
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : undefined;
}

function money(value?: HoldingMoney, signed = false) {
  return formatExactMoney(value, {
    maximumFractionDigits: 6,
    signDisplay: signed ? "exceptZero" : "auto",
    negativeSign: signed ? "−" : "-",
    unavailable: "—",
  });
}

function decimal(value: string) {
  const parsed = numeric(value);
  if (parsed === undefined) return value;
  return new Intl.NumberFormat("en-US", {
    maximumFractionDigits: 8,
  }).format(parsed);
}

function percent(value?: string) {
  const parsed = numeric(value);
  if (parsed === undefined) return "—";
  const formatted = new Intl.NumberFormat("en-US", {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  }).format(Math.abs(parsed));
  if (parsed === 0) return `${formatted}%`;
  return `${parsed > 0 ? "+" : "−"}${formatted}%`;
}

function movementClass(value?: string) {
  const parsed = numeric(value);
  if (parsed === undefined || parsed === 0) return "is-flat";
  return parsed > 0 ? "is-positive" : "is-negative";
}

function providerLabel(provider: string) {
  if (provider === "coinbase") return "Coinbase";
  if (provider === "schwab") return "Charles Schwab";
  return provider.charAt(0).toUpperCase() + provider.slice(1);
}

function sumMoney(
  holdings: PortfolioHolding[],
  select: (holding: PortfolioHolding) => HoldingMoney | undefined,
) {
  const selected = holdings
    .map(select)
    .filter((value): value is HoldingMoney => Boolean(value));
  return {
    money: sumExactMoney(selected),
    coverage: selected.length,
  };
}

export function PortfolioHoldingsLedger({
  holdings,
  unavailableAccounts = [],
  showSummary = true,
}: {
  holdings: PortfolioHolding[];
  unavailableAccounts?: string[];
  showSummary?: boolean;
}) {
  const providers = Array.from(
    new Set(holdings.map((holding) => holding.provider)),
  );
  const [provider, setProvider] = useState("all");
  const [query, setQuery] = useState("");
  const visible = useMemo(() => {
    const normalized = query.trim().toLowerCase();
    return holdings
      .filter(
        (holding) =>
          (provider === "all" || holding.provider === provider) &&
          (!normalized ||
            holding.symbol.toLowerCase().includes(normalized) ||
            holding.accountName.toLowerCase().includes(normalized) ||
            holding.instrumentType.toLowerCase().includes(normalized)),
      )
      .sort((left, right) => {
        const byValue =
          (numeric(right.marketValue?.amount) ?? -1) -
          (numeric(left.marketValue?.amount) ?? -1);
        return byValue || left.symbol.localeCompare(right.symbol);
      });
  }, [holdings, provider, query]);
  const value = sumMoney(holdings, (holding) => holding.marketValue);
  const daily = sumMoney(holdings, (holding) => holding.dayProfitLoss);
  const total = sumMoney(holdings, (holding) => holding.totalProfitLoss);

  return (
    <section
      className="holdings-command"
      aria-labelledby="holdings-command-title"
    >
      <header className="holdings-command-header">
        <div>
          <p className="eyebrow">UNIFIED HOLDINGS</p>
          <h2 id="holdings-command-title">Every position. One ledger.</h2>
          <p>
            Live connected holdings across Coinbase and Charles Schwab, with
            provider-supplied performance kept separate from observed market
            data.
          </p>
        </div>
        <span className="holdings-live-state">
          <i /> {holdings.length} holding{holdings.length === 1 ? "" : "s"}
        </span>
      </header>

      {showSummary && holdings.length > 0 && (
        <div className="holdings-value-rail">
          <article>
            <span>Observed holdings value</span>
            <strong>{money(value.money)}</strong>
            <small>
              {value.coverage}/{holdings.length} positions valued
            </small>
          </article>
          <article>
            <span>Day / 24h movement</span>
            <strong className={movementClass(daily.money?.amount)}>
              {money(daily.money, true)}
            </strong>
            <small>
              {daily.coverage}/{holdings.length} positions reporting
            </small>
          </article>
          <article>
            <span>Provider total return</span>
            <strong className={movementClass(total.money?.amount)}>
              {money(total.money, true)}
            </strong>
            <small>
              {total.coverage}/{holdings.length} with supplied cost basis
            </small>
          </article>
          <article>
            <span>Connections represented</span>
            <strong>
              {new Set(holdings.map((holding) => holding.accountID)).size}
            </strong>
            <small>{providers.map(providerLabel).join(" + ")}</small>
          </article>
        </div>
      )}

      {unavailableAccounts.length > 0 && (
        <p className="holdings-partial" role="status">
          {unavailableAccounts.join(", ")} could not refresh. Other connected
          accounts remain visible and unaffected.
        </p>
      )}

      {holdings.length === 0 ? (
        <div className="holdings-empty">
          <strong>No connected holdings are reporting yet.</strong>
          <p>
            Connect an account or refresh its authorization to populate this
            ledger.
          </p>
          <Link href="/connections#financial-accounts">
            Manage connections →
          </Link>
        </div>
      ) : (
        <>
          <div className="holdings-toolbar">
            <div role="group" aria-label="Filter holdings by provider">
              <button
                className={provider === "all" ? "is-active" : ""}
                onClick={() => setProvider("all")}
                type="button"
              >
                All
              </button>
              {providers.map((item) => (
                <button
                  className={provider === item ? "is-active" : ""}
                  key={item}
                  onClick={() => setProvider(item)}
                  type="button"
                >
                  {providerLabel(item)}
                </button>
              ))}
            </div>
            <label>
              <span>Search holdings</span>
              <input
                onChange={(event) => setQuery(event.target.value)}
                placeholder="Symbol or account"
                type="search"
                value={query}
              />
            </label>
          </div>

          <p className="command-data-scroll-hint" id="holdings-scroll-hint">
            Swipe or scroll horizontally to review every saved holdings field.
          </p>
          <div
            aria-describedby="holdings-scroll-hint"
            aria-label="Unified holdings table"
            className="holdings-table-wrap command-data-scroll"
            role="region"
            tabIndex={0}
          >
            <table>
              <thead>
                <tr>
                  <th>Asset</th>
                  <th>Account</th>
                  <th>Quantity</th>
                  <th>Avg. purchase price</th>
                  <th>Current price</th>
                  <th>Day / 24h change</th>
                  <th>Market value</th>
                  <th>Total return</th>
                </tr>
              </thead>
              <tbody>
                {visible.map((holding) => (
                  <tr key={holding.key}>
                    <td data-label="Asset">
                      <strong>{holding.symbol}</strong>
                      <small>
                        {holding.instrumentType
                          .replaceAll("_", " ")
                          .toLowerCase()}
                        {holding.direction === "short" ? " · short" : ""}
                      </small>
                    </td>
                    <td data-label="Account">
                      <Link href={`/accounts/${holding.accountID}`}>
                        {holding.accountName}
                      </Link>
                      <small>{providerLabel(holding.provider)}</small>
                    </td>
                    <td data-label="Quantity">
                      <strong>{decimal(holding.quantity)}</strong>
                      {(holding.availableQuantity !== undefined ||
                        holding.unavailableQuantity !== undefined) && (
                        <small>
                          {holding.availableQuantity === undefined
                            ? ""
                            : `${decimal(holding.availableQuantity)} available`}
                          {holding.availableQuantity !== undefined &&
                          holding.unavailableQuantity !== undefined
                            ? " · "
                            : ""}
                          {holding.unavailableQuantity === undefined
                            ? ""
                            : `${decimal(holding.unavailableQuantity)} staked / unavailable`}
                        </small>
                      )}
                    </td>
                    <td data-label="Avg. purchase price">
                      {holding.averagePrice ? (
                        <strong>{money(holding.averagePrice)}</strong>
                      ) : (
                        <>
                          <strong>—</strong>
                          <small>
                            {holding.provider === "coinbase"
                              ? "Not supplied by Coinbase"
                              : "Not supplied by provider"}
                          </small>
                        </>
                      )}
                    </td>
                    <td data-label="Current price">
                      <strong>{money(holding.currentPrice)}</strong>
                      <small>
                        {holding.priceBasis
                          ?.replaceAll("_", " ")
                          .toLowerCase() ?? "Price unavailable"}
                      </small>
                    </td>
                    <td
                      className={movementClass(holding.dayProfitLoss?.amount)}
                      data-label="Day / 24h change"
                    >
                      <strong>{money(holding.dayProfitLoss, true)}</strong>
                      <small>
                        {percent(holding.dayProfitLossPercent)} ·{" "}
                        {holding.changeWindow}
                      </small>
                    </td>
                    <td data-label="Market value">
                      <strong>{money(holding.marketValue)}</strong>
                    </td>
                    <td
                      className={movementClass(holding.totalProfitLoss?.amount)}
                      data-label="Total return"
                    >
                      {holding.totalProfitLoss ? (
                        <>
                          <strong>
                            {money(holding.totalProfitLoss, true)}
                          </strong>
                          <small>
                            {percent(holding.totalProfitLossPercent)}
                          </small>
                        </>
                      ) : (
                        <>
                          <strong>—</strong>
                          <small>Cost basis unavailable</small>
                        </>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
            {visible.length === 0 && (
              <p className="holdings-no-match">
                No holdings match this filter.
              </p>
            )}
          </div>
        </>
      )}

      <footer>
        Coinbase shows rolling 24-hour venue movement. Schwab shows its
        provider-reported current-day and open-position performance. Arbion
        never reconstructs missing tax lots from partial trade history.
      </footer>
    </section>
  );
}
