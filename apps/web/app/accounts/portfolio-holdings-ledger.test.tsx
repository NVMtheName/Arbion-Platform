import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import {
  PortfolioHoldingsLedger,
  type PortfolioHolding,
} from "./portfolio-holdings-ledger";

const holdings: PortfolioHolding[] = [
  {
    key: "coinbase-btc",
    accountID: "coinbase-1",
    accountName: "Coinbase Portfolio",
    provider: "coinbase",
    symbol: "BTC",
    instrumentType: "CRYPTO",
    direction: "long",
    quantity: "0.5",
    availableQuantity: "0.3",
    unavailableQuantity: "0.2",
    currentPrice: { amount: "60250", currency: "USD" },
    dayProfitLoss: { amount: "125", currency: "USD" },
    dayProfitLossPercent: "2.5",
    marketValue: { amount: "30125", currency: "USD" },
    changeWindow: "24H",
    costBasisStatus: "UNAVAILABLE_FROM_PROVIDER",
    priceBasis: "VENUE_LAST_TRADE",
  },
  {
    key: "schwab-aapl",
    accountID: "schwab-1",
    accountName: "Schwab Brokerage",
    provider: "schwab",
    symbol: "AAPL",
    instrumentType: "EQUITY",
    direction: "long",
    quantity: "10",
    averagePrice: { amount: "100", currency: "USD" },
    currentPrice: { amount: "105", currency: "USD" },
    dayProfitLoss: { amount: "-5", currency: "USD" },
    dayProfitLossPercent: "-0.47",
    marketValue: { amount: "1050", currency: "USD" },
    totalProfitLoss: { amount: "50", currency: "USD" },
    totalProfitLossPercent: "5",
    changeWindow: "DAY",
    costBasisStatus: "AVAILABLE",
    priceBasis: "PROVIDER_POSITION_MARKET_VALUE_PER_UNIT",
  },
];

describe("PortfolioHoldingsLedger", () => {
  afterEach(cleanup);

  it("shows traditional holding fields without inventing Coinbase cost basis", () => {
    render(
      <PortfolioHoldingsLedger
        holdings={holdings}
        unavailableAccounts={["Secondary Coinbase"]}
      />,
    );

    expect(
      screen.getByRole("heading", { name: "Every position. One ledger." }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("columnheader", { name: "Avg. purchase price" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("columnheader", { name: "Current price" }),
    ).toBeInTheDocument();
    expect(screen.getByText("BTC")).toBeInTheDocument();
    expect(screen.getByText("AAPL")).toBeInTheDocument();
    expect(
      screen.getByText(/0.3 available · 0.2 staked \/ unavailable/),
    ).toBeInTheDocument();
    expect(screen.getByText("Not supplied by Coinbase")).toBeInTheDocument();
    expect(screen.getByText("+$125.00")).toBeInTheDocument();
    expect(screen.getByText("+2.50% · 24H")).toBeInTheDocument();
    expect(screen.getByText("−$5.00")).toBeInTheDocument();
    expect(screen.getByText("−0.47% · DAY")).toBeInTheDocument();
    expect(screen.getAllByText("+$50.00").length).toBeGreaterThan(0);
    expect(screen.getByText("+5.00%")).toBeInTheDocument();
    expect(
      screen.getByText(/Secondary Coinbase could not refresh/),
    ).toBeInTheDocument();
  });

  it("filters across providers and account names", () => {
    render(<PortfolioHoldingsLedger holdings={holdings} />);

    fireEvent.click(screen.getByRole("button", { name: "Coinbase" }));
    expect(screen.getByText("BTC")).toBeInTheDocument();
    expect(screen.queryByText("AAPL")).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "All" }));
    fireEvent.change(
      screen.getByRole("searchbox", { name: "Search holdings" }),
      {
        target: { value: "brokerage" },
      },
    );
    expect(screen.queryByText("BTC")).not.toBeInTheDocument();
    expect(screen.getByText("AAPL")).toBeInTheDocument();
  });
});
