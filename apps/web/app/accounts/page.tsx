import { cookies } from "next/headers";
import { redirect } from "next/navigation";

import type { FinancialAccount } from "../settings/connections/page";
import type {
  HoldingMoney,
  PortfolioHolding,
} from "./portfolio-holdings-ledger";
import { PortfolioWorkspace } from "./portfolio-workspace";

type CryptoPosition = {
  symbol: string;
  quantity: string;
  available_quantity?: string;
  unavailable_to_trade_quantity?: string;
  unit_price?: HoldingMoney;
  market_value?: HoldingMoney;
  position_change_24h?: HoldingMoney;
  change_percent_24h?: string;
  cost_basis_status?: "UNAVAILABLE_FROM_PROVIDER";
  valuation_basis?: string;
};

type SchwabPosition = {
  symbol: string;
  instrument_type: string;
  quantity: string;
  direction: string;
  market_value?: HoldingMoney;
  cost_basis?: HoldingMoney;
  current_price?: HoldingMoney;
  day_profit_loss?: HoldingMoney;
  day_profit_loss_percent?: string;
  open_profit_loss?: HoldingMoney;
  open_profit_loss_percent?: string;
  price_basis?: string;
};

type AccountHoldings = {
  holdings: PortfolioHolding[];
  unavailable?: string;
};

async function loadAccountHoldings(
  account: FinancialAccount,
  base: string,
  headers: { cookie: string },
): Promise<AccountHoldings> {
  if (account.status !== "active") {
    return { holdings: [], unavailable: account.display_name };
  }
  const id = encodeURIComponent(account.id);
  try {
    if (account.provider === "coinbase") {
      const response = await fetch(
        `${base}/api/accounts/${id}/portfolio/crypto`,
        {
          headers,
          cache: "no-store",
        },
      );
      if (!response.ok)
        return { holdings: [], unavailable: account.display_name };
      const payload = (await response.json()) as {
        portfolio?: { holdings_state?: string; positions?: CryptoPosition[] };
      };
      if (!payload.portfolio || payload.portfolio.holdings_state !== "READY") {
        return { holdings: [], unavailable: account.display_name };
      }
      return {
        holdings: (payload.portfolio.positions ?? []).map(
          (position, index) => ({
            key: `${account.id}-${position.symbol}-${index}`,
            accountID: account.id,
            accountName: account.display_name,
            provider: "coinbase",
            symbol: position.symbol,
            instrumentType: "CRYPTO",
            direction: "long",
            quantity: position.quantity,
            availableQuantity: position.available_quantity,
            unavailableQuantity: position.unavailable_to_trade_quantity,
            currentPrice: position.unit_price,
            dayProfitLoss: position.position_change_24h,
            dayProfitLossPercent: position.change_percent_24h,
            marketValue: position.market_value,
            changeWindow: "24H",
            costBasisStatus: "UNAVAILABLE_FROM_PROVIDER",
            priceBasis: position.valuation_basis,
          }),
        ),
      };
    }

    const response = await fetch(`${base}/api/accounts/${id}/positions`, {
      headers,
      cache: "no-store",
    });
    if (!response.ok)
      return { holdings: [], unavailable: account.display_name };
    const positions = (
      (await response.json()) as { positions?: SchwabPosition[] }
    ).positions;
    return {
      holdings: (positions ?? []).map((position, index) => ({
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
      })),
    };
  } catch {
    return { holdings: [], unavailable: account.display_name };
  }
}

export default async function Accounts() {
  const jar = await cookies();
  const headers = { cookie: jar.toString() };
  const base = process.env.API_BASE_URL ?? "http://localhost:8080";
  const response = await fetch(`${base}/api/accounts`, {
    headers,
    cache: "no-store",
  });
  if (response.status === 401) redirect("/login");
  const data = response.ok
    ? ((await response.json()) as { accounts: FinancialAccount[] })
    : { accounts: [] };

  const results = await Promise.all(
    data.accounts.map((account) => loadAccountHoldings(account, base, headers)),
  );
  const holdings = results.flatMap((result) => result.holdings);
  const unavailableAccounts = results.flatMap((result) =>
    result.unavailable ? [result.unavailable] : [],
  );

  return (
    <PortfolioWorkspace
      accounts={data.accounts}
      accountsAvailable={response.ok}
      holdings={holdings}
      unavailableAccounts={unavailableAccounts}
    />
  );
}
