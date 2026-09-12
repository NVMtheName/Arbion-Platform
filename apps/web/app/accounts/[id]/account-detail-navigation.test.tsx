import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { AccountDetailNavigation } from "./account-detail-navigation";

describe("Account section navigation", () => {
  afterEach(cleanup);
  it.each(["crypto", "broker"] as const)(
    "links to the exact %s holdings and existing evidence",
    (portfolio) => {
      render(<AccountDetailNavigation portfolio={portfolio} />);
      const nav = screen.getByRole("navigation", { name: "Account sections" });
      expect(within(nav).getAllByRole("link")).toHaveLength(5);
      expect(
        within(nav).getByRole("link", { name: "Holdings" }),
      ).toHaveAttribute(
        "href",
        portfolio === "crypto"
          ? "#crypto-position-title"
          : "#holdings-command-title",
      );
      expect(
        within(nav).getByRole("link", { name: "Saved syncs" }),
      ).toHaveAttribute("href", "#account-sync-history-title");
      expect(
        within(nav).queryByRole("link", { name: /dashboard/i }),
      ).not.toBeInTheDocument();
    },
  );
  it("omits holdings and reconciliation links when the portfolio view is unavailable", () => {
    render(<AccountDetailNavigation portfolio="unavailable" />);
    expect(screen.getAllByRole("link")).toHaveLength(3);
    expect(
      screen.queryByRole("link", { name: "Holdings" }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("link", { name: "Reconciliation" }),
    ).not.toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: "AI input evidence" }),
    ).toHaveAttribute("href", "#dashboard-input-chain-title");
  });
});
