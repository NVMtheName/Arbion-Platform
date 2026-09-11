import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

vi.mock("../app-page-header", () => ({
  AppPageHeader: ({ contentHeadingId }: { contentHeadingId: string }) => (
    <header data-testid="shared-header" aria-labelledby={contentHeadingId}>
      Shared navigation
    </header>
  ),
}));

import { PortfolioWorkspace } from "./portfolio-workspace";

const account = {
  id: "coinbase-account",
  provider_connection_id: "coinbase-connection",
  provider: "coinbase",
  display_name: "Primary crypto account",
  status: "active",
};

describe("PortfolioWorkspace", () => {
  afterEach(cleanup);

  it("keeps shared navigation and holdings ahead of account workspaces", () => {
    render(
      <PortfolioWorkspace
        accounts={[account]}
        accountsAvailable
        holdings={[]}
        unavailableAccounts={[]}
      />,
    );
    expect(
      screen.getByRole("heading", { level: 1, name: "Portfolio" }),
    ).toBeInTheDocument();
    expect(screen.getByTestId("shared-header")).toHaveAttribute(
      "aria-labelledby",
      "portfolio-page-title",
    );
    const headings = screen
      .getAllByRole("heading", { level: 2 })
      .map((element) => element.textContent);
    expect(headings).toEqual(["Holdings", "Your connected accounts"]);
    expect(
      screen.getByRole("link", { name: "Open Primary crypto account" }),
    ).toHaveAttribute("href", "/accounts/coinbase-account");
    expect(screen.getByText("Coinbase", { exact: true })).toBeInTheDocument();
    expect(screen.getByText("active", { exact: true })).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: "Manage connections" }),
    ).toHaveAttribute("href", "/connections#financial-accounts");
  });

  it("does not portray an unavailable account list as empty or show stale holdings", () => {
    render(
      <PortfolioWorkspace
        accounts={[account]}
        accountsAvailable={false}
        holdings={[]}
        unavailableAccounts={[]}
      />,
    );
    expect(screen.getByRole("status")).toHaveTextContent(
      "Your accounts could not be loaded",
    );
    expect(screen.getByRole("status")).toHaveTextContent(
      "This is not an empty portfolio",
    );
    expect(
      screen.queryByRole("heading", { name: "Connect your first account" }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("heading", { name: "Holdings" }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("link", { name: "Open Primary crypto account" }),
    ).not.toBeInTheDocument();
  });

  it("shows onboarding only for an available empty account list", () => {
    render(
      <PortfolioWorkspace
        accounts={[]}
        accountsAvailable
        holdings={[]}
        unavailableAccounts={[]}
      />,
    );
    expect(
      screen.getByRole("heading", { name: "Connect your first account" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: "Open connection hub" }),
    ).toHaveAttribute("href", "/connections#financial-accounts");
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
  });

  it("keeps partial-account warnings visible without classifying an unknown provider as Schwab", () => {
    render(
      <PortfolioWorkspace
        accounts={[{ ...account, provider: "etrade", status: "expired" }]}
        accountsAvailable
        holdings={[]}
        unavailableAccounts={[account.display_name]}
      />,
    );
    expect(screen.getByRole("status")).toHaveTextContent(
      "Primary crypto account could not refresh",
    );
    expect(screen.getByText("expired", { exact: true })).toHaveClass(
      "needs-review",
    );
    expect(screen.getByText("etrade", { exact: true })).toBeInTheDocument();
    expect(
      screen.queryByText("Charles Schwab", { exact: true }),
    ).not.toBeInTheDocument();
  });
});
