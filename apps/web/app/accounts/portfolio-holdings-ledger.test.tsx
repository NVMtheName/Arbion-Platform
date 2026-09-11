import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from "@testing-library/react";
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
      screen.getByRole("heading", { name: "Holdings" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("columnheader", { name: "Avg. purchase price" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("columnheader", { name: "Current price" }),
    ).toBeInTheDocument();
    expect(screen.getByText("BTC")).toBeInTheDocument();
    expect(screen.getByText("AAPL")).toBeInTheDocument();
    const btcRow = screen.getByText("BTC").closest("tr");
    expect(btcRow).not.toBeNull();
    expect(
      within(btcRow!)
        .getAllByRole("cell")
        .map((cell) => cell.dataset.label),
    ).toEqual([
      "Asset",
      "Account",
      "Quantity",
      "Avg. purchase price",
      "Current price",
      "Day / 24h change",
      "Market value",
      "Total return",
    ]);
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
    const holdingsRegion = screen.getByRole("region", {
      name: "Unified holdings table",
    });
    expect(holdingsRegion).toHaveClass("command-data-scroll");
    expect(holdingsRegion).toHaveAttribute("tabindex", "0");
    expect(holdingsRegion).toHaveAttribute(
      "aria-describedby",
      "holdings-scroll-hint",
    );
    expect(
      screen.getByText(/scroll horizontally.*holdings field/i),
    ).toHaveClass("command-data-scroll-hint");
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

  it("announces filtering, keeps the full summary stable and clears only local filters", () => {
    render(<PortfolioHoldingsLedger holdings={holdings} />);
    const summary = screen.getByLabelText(
      "All loaded holdings, before filters",
    );
    const before = summary.textContent;
    expect(screen.getByRole("button", { name: "All" })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
    expect(
      screen.getByRole("button", { name: "Clear filters" }),
    ).toBeDisabled();

    fireEvent.click(screen.getByRole("button", { name: "Coinbase" }));
    expect(screen.getByRole("button", { name: "Coinbase" })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
    expect(screen.getByRole("status")).toHaveTextContent(
      "Showing 1 of 2 holdings",
    );
    expect(summary.textContent).toBe(before);
    fireEvent.change(screen.getByRole("searchbox"), {
      target: { value: "missing" },
    });
    expect(screen.getByRole("status")).toHaveTextContent(
      "Showing 0 of 2 holdings",
    );
    expect(
      screen.getByText("No holdings match this filter."),
    ).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Clear filters" }));
    expect(screen.getByRole("searchbox")).toHaveValue("");
    expect(screen.getByRole("button", { name: "All" })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
    expect(screen.getByRole("status")).toHaveTextContent(
      "Showing 2 of 2 holdings",
    );
    expect(screen.getByText("BTC")).toBeInTheDocument();
    expect(screen.getByText("AAPL")).toBeInTheDocument();
    expect(summary.textContent).toBe(before);
  });

  it("retains every source field and explicit table headers without a live-price claim", () => {
    render(<PortfolioHoldingsLedger holdings={holdings} showSummary={false} />);
    expect(
      screen.queryByLabelText("All loaded holdings, before filters"),
    ).not.toBeInTheDocument();
    expect(screen.getByRole("status")).not.toHaveTextContent("Summary");
    expect(screen.queryByText(/live connected/i)).not.toBeInTheDocument();
    for (const header of screen.getAllByRole("columnheader")) {
      expect(header).toHaveAttribute("scope", "col");
    }
    expect(screen.getByRole("table")).toHaveAccessibleName(
      /Missing provider values/,
    );
    expect(screen.getByText("venue last trade")).toBeInTheDocument();
    expect(
      screen.getByText("provider position market value per unit"),
    ).toBeInTheDocument();
  });

  it("aggregates holdings exactly and fails mixed currencies closed", () => {
    const exactHoldings: PortfolioHolding[] = [
      {
        ...holdings[0],
        key: "exact-large",
        marketValue: {
          amount: "9007199254740993.005",
          currency: "USD",
        },
      },
      {
        ...holdings[1],
        key: "exact-fraction",
        marketValue: { amount: "0.005", currency: "USD" },
      },
    ];
    const { rerender } = render(
      <PortfolioHoldingsLedger holdings={exactHoldings} />,
    );

    const observedValue = screen
      .getByText("Observed holdings value")
      .closest("article");
    expect(observedValue).not.toBeNull();
    expect(
      within(observedValue!).getByText("$9,007,199,254,740,993.01"),
    ).toBeInTheDocument();

    rerender(
      <PortfolioHoldingsLedger
        holdings={[
          exactHoldings[0],
          {
            ...exactHoldings[1],
            marketValue: { amount: "0.005", currency: "EUR" },
          },
        ]}
      />,
    );
    const mixedValue = screen
      .getByText("Observed holdings value")
      .closest("article");
    expect(mixedValue).not.toBeNull();
    expect(within(mixedValue!).getByText("—")).toBeInTheDocument();
    expect(mixedValue).toHaveTextContent("2/2 positions valued");
  });
});
