import { cookies } from "next/headers";
import Link from "next/link";
import { notFound, redirect } from "next/navigation";

import { AppPageHeader } from "../../app-page-header";
import {
  AccountCircuitBreakerControls,
  type AccountCircuitBreaker,
} from "./account-circuit-breaker-controls";
import {
  CryptoPortfolioCommandCenter,
  type CoinbaseOrderHistory,
  type CoinbaseTradeActivity,
  type CoinbaseTradingCostSummary,
  type CryptoCandleSeries,
  type CryptoLiquiditySnapshot,
  type CryptoPublicTradeTape,
  type CryptoPortfolioSnapshot,
  type CryptoVenueStats,
} from "./crypto-portfolio-command-center";
import type { CoinbaseCapitalPolicy } from "./coinbase-order-preview";
import {
  PortfolioReconciliationPanel,
  type PortfolioReconciliation,
  type PortfolioReconciliationHistory,
} from "./portfolio-reconciliation-panel";
import {
  PortfolioHoldingsLedger,
  type PortfolioHolding,
} from "../portfolio-holdings-ledger";
type Money = { amount: string; currency: string };
type Account = {
  id: string;
  provider: string;
  display_name: string;
  account_type: string;
  status: string;
};
type Balances = {
  account_value?: Money;
  cash?: Money;
  available_cash?: Money;
  buying_power?: Money;
};
type Position = {
  symbol: string;
  instrument_type: string;
  quantity: string;
  direction: string;
  market_value?: Money;
  cost_basis?: Money;
  current_price?: Money;
  day_profit_loss?: Money;
  day_profit_loss_percent?: string;
  open_profit_loss?: Money;
  open_profit_loss_percent?: string;
  price_basis?: string;
};
const show = (m?: Money) =>
  m
    ? new Intl.NumberFormat("en-US", {
        style: "currency",
        currency: m.currency,
      }).format(Number(m.amount))
    : "Unavailable";
export default async function AccountPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  const jar = await cookies();
  const base = process.env.API_BASE_URL ?? "http://localhost:8080";
  const headers = { cookie: jar.toString() };
  const ar = await fetch(`${base}/api/accounts/${id}`, {
    headers,
    cache: "no-store",
  });
  if (ar.status === 401) redirect("/login");
  if (ar.status === 404) notFound();
  if (!ar.ok) throw new Error("Unable to load account");
  const account = ((await ar.json()) as { account: Account }).account;
  const breakerResponse = await fetch(
    `${base}/api/accounts/${id}/circuit-breaker`,
    { headers, cache: "no-store" },
  );
  const accountBreaker = breakerResponse.ok
    ? ((
        (await breakerResponse.json()) as {
          circuit_breaker?: AccountCircuitBreaker | null;
        }
      ).circuit_breaker ?? null)
    : undefined;

  if (account.provider === "coinbase") {
    const portfolioResponse = await fetch(
      `${base}/api/accounts/${id}/portfolio/crypto`,
      { headers, cache: "no-store" },
    );
    if (portfolioResponse.status === 401) redirect("/login");
    if (portfolioResponse.status === 404) notFound();
    if (!portfolioResponse.ok) {
      return (
        <main className="connections-page crypto-account-page">
          <AppPageHeader backHref="/accounts" backLabel="Accounts" />
          <p className="eyebrow">COINBASE · READ-ONLY CONNECTION</p>
          <h1>{account.display_name}</h1>
          <p className="unavailable">
            Coinbase holdings could not be refreshed. Arbion has not substituted
            cached balances or estimated values.
          </p>
          <Link href="/connections">Review connection settings</Link>
        </main>
      );
    }
    const snapshot = (
      (await portfolioResponse.json()) as {
        portfolio: CryptoPortfolioSnapshot;
      }
    ).portfolio;
    const initialSymbol = snapshot.positions.find(
      (position) => position.market_value && position.provenance,
    )?.symbol;
    let initialHistory: CryptoCandleSeries | undefined;
    let initialHistoryCached = false;
    let initialLiquidity: CryptoLiquiditySnapshot | undefined;
    let initialLiquidityCached = false;
    let initialMarketTrades: CryptoPublicTradeTape | undefined;
    let initialMarketTradesCached = false;
    let initialVenueStats: CryptoVenueStats | undefined;
    let initialVenueStatsCached = false;
    let initialActivity: CoinbaseTradeActivity | undefined;
    let initialOrderHistory: CoinbaseOrderHistory | undefined;
    let initialTradingCosts: CoinbaseTradingCostSummary | undefined;
    let initialReconciliation: PortfolioReconciliation | undefined;
    let initialReconciliationHistory:
      | PortfolioReconciliationHistory
      | undefined;
    const [
      activityResponse,
      orderResponse,
      costsResponse,
      historyResponse,
      liquidityResponse,
      marketTradesResponse,
      venueStatsResponse,
      reconciliationResponse,
      reconciliationHistoryResponse,
      bucketsResponse,
    ] = await Promise.all([
      fetch(`${base}/api/accounts/${encodeURIComponent(id)}/activity/fills`, {
        headers,
        cache: "no-store",
      }),
      fetch(`${base}/api/accounts/${encodeURIComponent(id)}/activity/orders`, {
        headers,
        cache: "no-store",
      }),
      fetch(
        `${base}/api/accounts/${encodeURIComponent(id)}/activity/trading-costs`,
        {
          headers,
          cache: "no-store",
        },
      ),
      initialSymbol
        ? fetch(
            `${base}/api/accounts/${encodeURIComponent(id)}/markets/crypto/${encodeURIComponent(initialSymbol)}/candles`,
            { headers, cache: "no-store" },
          )
        : Promise.resolve(undefined),
      initialSymbol
        ? fetch(
            `${base}/api/accounts/${encodeURIComponent(id)}/markets/crypto/${encodeURIComponent(initialSymbol)}/liquidity`,
            { headers, cache: "no-store" },
          )
        : Promise.resolve(undefined),
      initialSymbol
        ? fetch(
            `${base}/api/accounts/${encodeURIComponent(id)}/markets/crypto/${encodeURIComponent(initialSymbol)}/trades`,
            { headers, cache: "no-store" },
          )
        : Promise.resolve(undefined),
      initialSymbol
        ? fetch(
            `${base}/api/accounts/${encodeURIComponent(id)}/markets/crypto/${encodeURIComponent(initialSymbol)}/stats`,
            { headers, cache: "no-store" },
          )
        : Promise.resolve(undefined),
      fetch(
        `${base}/api/accounts/${encodeURIComponent(id)}/reconciliations/latest`,
        { headers, cache: "no-store" },
      ),
      fetch(
        `${base}/api/accounts/${encodeURIComponent(id)}/reconciliations?limit=8`,
        { headers, cache: "no-store" },
      ),
      fetch(`${base}/api/capital-buckets`, {
        headers,
        cache: "no-store",
      }),
    ]);
    let capitalPolicies: CoinbaseCapitalPolicy[] = [];
    if (bucketsResponse.ok) {
      const bucketPayload = (await bucketsResponse.json()) as {
        capital_buckets?: Record<string, unknown>[];
      };
      capitalPolicies = (bucketPayload.capital_buckets ?? [])
        .map((bucket): CoinbaseCapitalPolicy | undefined => {
          const financialAccountID = String(
            bucket.FinancialAccountID ?? bucket.financial_account_id ?? "",
          );
          const status = String(bucket.Status ?? bucket.status ?? "");
          const currency = String(bucket.Currency ?? bucket.currency ?? "");
          const isReserve = Boolean(bucket.IsReserve ?? bucket.is_reserve);
          const allocationType = String(
            bucket.AllocationType ?? bucket.allocation_type ?? "",
          );
          if (
            financialAccountID !== id ||
            status !== "ACTIVE" ||
            currency !== "USD" ||
            isReserve ||
            ![
              "FIXED_AMOUNT",
              "PERCENT_OF_AVAILABLE_CASH",
              "PERCENT_OF_BUYING_POWER",
            ].includes(allocationType)
          ) {
            return undefined;
          }
          const rawLimit =
            bucket.AllocationLimit ?? bucket.allocation_limit ?? undefined;
          return {
            id: String(bucket.ID ?? bucket.id ?? ""),
            financialAccountID,
            name: String(bucket.Name ?? bucket.name ?? "Capital policy"),
            allocationType:
              allocationType as CoinbaseCapitalPolicy["allocationType"],
            allocationValue: String(
              bucket.AllocationValue ?? bucket.allocation_value ?? "",
            ),
            currency: "USD",
            isReserve: false,
            protectedAmount: String(
              bucket.ProtectedAmount ?? bucket.protected_amount ?? "0",
            ),
            ...(rawLimit === undefined || rawLimit === null
              ? {}
              : { allocationLimit: String(rawLimit) }),
            status: "ACTIVE",
          };
        })
        .filter((policy): policy is CoinbaseCapitalPolicy =>
          Boolean(policy?.id && policy.allocationValue),
        );
    }
    if (activityResponse.ok) {
      const activityPayload = (await activityResponse.json()) as {
        activity: CoinbaseTradeActivity;
      };
      initialActivity = activityPayload.activity;
    }
    if (orderResponse.ok) {
      const orderPayload = (await orderResponse.json()) as {
        orders: CoinbaseOrderHistory;
      };
      initialOrderHistory = orderPayload.orders;
    }
    if (costsResponse.ok) {
      const costsPayload = (await costsResponse.json()) as {
        trading_costs: CoinbaseTradingCostSummary;
      };
      initialTradingCosts = costsPayload.trading_costs;
    }
    if (reconciliationResponse.ok) {
      const reconciliationPayload = (await reconciliationResponse.json()) as {
        reconciliation: PortfolioReconciliation;
      };
      initialReconciliation = reconciliationPayload.reconciliation;
    }
    if (reconciliationHistoryResponse.ok) {
      const historyPayload = (await reconciliationHistoryResponse.json()) as {
        history: PortfolioReconciliationHistory;
      };
      initialReconciliationHistory = historyPayload.history;
    }
    if (historyResponse?.ok) {
      const historyPayload = (await historyResponse.json()) as {
        history: CryptoCandleSeries;
        cached?: boolean;
      };
      initialHistory = historyPayload.history;
      initialHistoryCached = Boolean(historyPayload.cached);
    }
    if (liquidityResponse?.ok) {
      const liquidityPayload = (await liquidityResponse.json()) as {
        liquidity: CryptoLiquiditySnapshot;
        cached?: boolean;
      };
      initialLiquidity = liquidityPayload.liquidity;
      initialLiquidityCached = Boolean(liquidityPayload.cached);
    }
    if (marketTradesResponse?.ok) {
      const marketTradesPayload = (await marketTradesResponse.json()) as {
        market_trades: CryptoPublicTradeTape;
        cached?: boolean;
      };
      initialMarketTrades = marketTradesPayload.market_trades;
      initialMarketTradesCached = Boolean(marketTradesPayload.cached);
    }
    if (venueStatsResponse?.ok) {
      const venueStatsPayload = (await venueStatsResponse.json()) as {
        venue_stats: CryptoVenueStats;
        cached?: boolean;
      };
      initialVenueStats = venueStatsPayload.venue_stats;
      initialVenueStatsCached = Boolean(venueStatsPayload.cached);
    }
    return (
      <main className="connections-page crypto-account-page">
        <AppPageHeader backHref="/accounts" backLabel="Accounts" />
        <CryptoPortfolioCommandCenter
          accountID={account.id}
          capitalPolicies={capitalPolicies}
          initialActivity={initialActivity}
          initialHistory={initialHistory}
          initialHistoryCached={initialHistoryCached}
          initialLiquidity={initialLiquidity}
          initialLiquidityCached={initialLiquidityCached}
          initialMarketTrades={initialMarketTrades}
          initialMarketTradesCached={initialMarketTradesCached}
          initialVenueStats={initialVenueStats}
          initialVenueStatsCached={initialVenueStatsCached}
          initialOrderHistory={initialOrderHistory}
          initialSnapshot={snapshot}
          initialTradingCosts={initialTradingCosts}
        />
        <PortfolioReconciliationPanel
          accountID={account.id}
          accountName={account.display_name}
          initialReport={initialReconciliation}
          initialHistory={initialReconciliationHistory}
        />
        {accountBreaker !== undefined && (
          <AccountCircuitBreakerControls
            accountId={account.id}
            accountName={account.display_name}
            breaker={accountBreaker}
          />
        )}
      </main>
    );
  }

  const [br, pr, rr, rh] = await Promise.all([
    fetch(`${base}/api/accounts/${id}/balances`, {
      headers,
      cache: "no-store",
    }),
    fetch(`${base}/api/accounts/${id}/positions`, {
      headers,
      cache: "no-store",
    }),
    fetch(`${base}/api/accounts/${id}/reconciliations/latest`, {
      headers,
      cache: "no-store",
    }),
    fetch(`${base}/api/accounts/${id}/reconciliations?limit=8`, {
      headers,
      cache: "no-store",
    }),
  ]);
  const balances = br.ok
    ? ((await br.json()) as { balances: Balances }).balances
    : {};
  const positions = pr.ok
    ? ((await pr.json()) as { positions: Position[] }).positions
    : [];
  const initialReconciliation = rr.ok
    ? ((await rr.json()) as { reconciliation: PortfolioReconciliation })
        .reconciliation
    : undefined;
  const initialReconciliationHistory = rh.ok
    ? ((await rh.json()) as { history: PortfolioReconciliationHistory }).history
    : undefined;
  const holdings: PortfolioHolding[] = positions.map((position, index) => ({
    key: `${account.id}-${position.symbol}-${index}`,
    accountID: account.id,
    accountName: account.display_name,
    provider: "schwab",
    symbol: position.symbol,
    instrumentType: position.instrument_type,
    direction: position.direction,
    quantity: position.quantity,
    averagePrice: position.cost_basis,
    currentPrice: position.current_price,
    dayProfitLoss: position.day_profit_loss,
    dayProfitLossPercent: position.day_profit_loss_percent,
    marketValue: position.market_value,
    totalProfitLoss: position.open_profit_loss,
    totalProfitLossPercent: position.open_profit_loss_percent,
    changeWindow: "DAY",
    costBasisStatus: position.cost_basis
      ? "AVAILABLE"
      : "UNAVAILABLE_FROM_PROVIDER",
    priceBasis: position.price_basis,
  }));
  return (
    <main className="connections-page portfolio-ledger-page schwab-account-page">
      <AppPageHeader backHref="/accounts" backLabel="Accounts" />
      <p className="eyebrow">CHARLES SCHWAB · CONNECTED ACCOUNT</p>
      <h1>{account.display_name}</h1>
      <section className="dashboard-grid">
        <article>
          <h2>Account Value</h2>
          <p>{show(balances.account_value)}</p>
        </article>
        <article>
          <h2>Cash</h2>
          <p>{show(balances.cash)}</p>
        </article>
        <article>
          <h2>Buying Power</h2>
          <p>{show(balances.buying_power)}</p>
        </article>
      </section>
      <PortfolioHoldingsLedger
        holdings={holdings}
        unavailableAccounts={pr.ok ? [] : [account.display_name]}
      />
      <PortfolioReconciliationPanel
        accountID={account.id}
        accountName={account.display_name}
        initialReport={initialReconciliation}
        initialHistory={initialReconciliationHistory}
      />
      {accountBreaker !== undefined && (
        <AccountCircuitBreakerControls
          accountId={account.id}
          accountName={account.display_name}
          breaker={accountBreaker}
        />
      )}
      <p className="security-note">
        Connected-account data is informational. This page cannot place orders
        or move assets.
      </p>
    </main>
  );
}
