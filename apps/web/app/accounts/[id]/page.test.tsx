import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
const mocks = vi.hoisted(() => ({
  input: vi.fn(),
  sync: vi.fn(),
  attempts: vi.fn(),
  crypto: vi.fn(),
  redirect: vi.fn((path: string) => {
    throw new Error("redirect:" + path);
  }),
  notFound: vi.fn(() => {
    throw new Error("not-found");
  }),
}));
vi.mock("next/headers", () => ({
  cookies: async () => ({ toString: () => "session=test-only" }),
}));
vi.mock("next/navigation", () => ({
  redirect: mocks.redirect,
  notFound: mocks.notFound,
}));
vi.mock("./account-financial-input-chain", () => ({
  loadAccountFinancialInputChain: mocks.input,
}));
vi.mock("./account-sync-history", () => ({
  loadAccountSyncHistory: mocks.sync,
}));
vi.mock("../../settings/connections/connection-sync-attempt-history", () => ({
  loadConnectionSyncAttemptHistory: mocks.attempts,
}));
vi.mock("../../app-page-header", () => ({
  AppPageHeader: ({
    backHref,
    backLabel,
  }: {
    backHref: string;
    backLabel: string;
  }) => (
    <header>
      <a href={backHref}>{backLabel}</a>
    </header>
  ),
}));
vi.mock("../../dashboard/financial-input-chain-summary", () => ({
  FinancialInputChainSummary: () => (
    <h2 id="dashboard-input-chain-title">Input evidence</h2>
  ),
}));
vi.mock("./account-sync-history-panel", () => ({
  AccountSyncHistoryPanel: () => (
    <h2 id="account-sync-history-title">Sync evidence</h2>
  ),
}));
vi.mock("./account-circuit-breaker-controls", () => ({
  AccountCircuitBreakerControls: () => <section>Existing breaker</section>,
}));
vi.mock("./portfolio-reconciliation-panel", () => ({
  PortfolioReconciliationPanel: () => (
    <h2 id="reconciliation-title">Reconciliation</h2>
  ),
}));
vi.mock("./crypto-portfolio-command-center", () => ({
  CryptoPortfolioCommandCenter: (props: Record<string, unknown>) => {
    mocks.crypto(props);
    return <div>Crypto overview</div>;
  },
}));
import AccountPage from "./page";

function setup(provider = "schwab", failedPath = "", status = 503) {
  mocks.input.mockResolvedValue({ available: false, unauthorized: false });
  mocks.sync.mockResolvedValue({
    state: "UNAVAILABLE",
    checkpoints: [],
    unauthorized: false,
  });
  mocks.attempts.mockResolvedValue({
    state: "UNAVAILABLE",
    attempts: [],
    unauthorized: false,
  });
  const account = {
    id: "account-one",
    provider_connection_id: "connection-one",
    provider,
    display_name: "My account",
    account_type: "CASH",
    status: "active",
  };
  const snapshot = { account, positions: [], holdings_state: "READY" };
  const request = vi.fn(async (url: string) => {
    const path = new URL(url).pathname;
    const body =
      path === "/api/accounts/account-one"
        ? { account }
        : path.endsWith("/portfolio/crypto")
          ? { portfolio: snapshot }
          : path.endsWith("/balances")
            ? {
                balances: {
                  account_value: { amount: "1234.5678901234", currency: "USD" },
                },
              }
            : path.endsWith("/positions")
              ? { positions: [] }
              : {};
    return {
      ok: path !== failedPath,
      status: path === failedPath ? status : 200,
      json: async () => body,
    };
  });
  vi.stubGlobal("fetch", request);
  return { request, account, snapshot };
}
const params = Promise.resolve({ id: "account-one" });
describe("Account detail presentation preserves server loading", () => {
  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
    vi.unstubAllGlobals();
  });
  it("keeps six broker requests no-store and puts title and values ahead of saved evidence", async () => {
    const { request } = setup();
    render(await AccountPage({ params }));
    expect(request).toHaveBeenCalledTimes(6);
    for (const call of request.mock.calls)
      expect(call[0]).toContain("/api/accounts/account-one");
    for (const call of vi.mocked(fetch).mock.calls)
      expect(call[1]).toEqual({
        headers: { cookie: "session=test-only" },
        cache: "no-store",
      });
    const heading = screen.getByRole("heading", { name: "My account" });
    const summary = screen.getByRole("region", { name: "Account summary" });
    expect(
      heading.compareDocumentPosition(summary) &
        Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
    expect(
      summary.compareDocumentPosition(screen.getByText("Input evidence")) &
        Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
    expect(screen.getByRole("link", { name: "Accounts" })).toHaveAttribute(
      "href",
      "/accounts",
    );
    for (const link of within(
      screen.getByRole("navigation", { name: "Account sections" }),
    ).getAllByRole("link"))
      expect(document.querySelector(link.getAttribute("href")!)).not.toBeNull();
  });
  it("passes the same Coinbase snapshot and account evidence through without additional requests", async () => {
    const { request, snapshot } = setup("coinbase");
    render(await AccountPage({ params }));
    expect(request).toHaveBeenCalledTimes(9);
    const props = mocks.crypto.mock.calls[0][0];
    expect(props.initialSnapshot).toBe(snapshot);
    expect(props.accountID).toBe("account-one");
    expect(props.accountEvidence).toBeDefined();
    expect(props.navigation.props.portfolio).toBe("crypto");
    expect(request.mock.calls.some(([url]) => url.includes("/candles"))).toBe(
      false,
    );
    for (const call of vi.mocked(fetch).mock.calls)
      expect(call[1]).toEqual({
        headers: { cookie: "session=test-only" },
        cache: "no-store",
      });
  });
  it("keeps a failed Coinbase portfolio explicit with the account heading first", async () => {
    setup("coinbase", "/api/accounts/account-one/portfolio/crypto");
    render(await AccountPage({ params }));
    expect(
      screen.getByText(/Coinbase holdings could not be refreshed/),
    ).toBeVisible();
    expect(
      screen
        .getByRole("heading", { name: "My account" })
        .compareDocumentPosition(screen.getByText("Input evidence")) &
        Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
    expect(
      screen.queryByRole("link", { name: "Holdings" }),
    ).not.toBeInTheDocument();
    expect(mocks.crypto).not.toHaveBeenCalled();
  });
  it.each([401, 404])(
    "preserves account identity failure %s",
    async (status) => {
      setup("schwab", "/api/accounts/account-one", status);
      await expect(AccountPage({ params })).rejects.toThrow(
        status === 401 ? "redirect:/login" : "not-found",
      );
    },
  );
  it.each(["input", "sync", "attempts"] as const)(
    "preserves the %s evidence authorization redirect",
    async (key) => {
      setup();
      mocks[key].mockResolvedValueOnce({ unauthorized: true });
      await expect(AccountPage({ params })).rejects.toThrow("redirect:/login");
    },
  );
  it.each([401, 404])(
    "preserves Coinbase portfolio failure %s",
    async (status) => {
      setup("coinbase", "/api/accounts/account-one/portfolio/crypto", status);
      await expect(AccountPage({ params })).rejects.toThrow(
        status === 401 ? "redirect:/login" : "not-found",
      );
    },
  );
});
