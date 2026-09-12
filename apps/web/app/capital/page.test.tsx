import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const redirect = vi.hoisted(() =>
  vi.fn((path: string) => {
    throw new Error(`redirect:${path}`);
  }),
);
vi.mock("next/headers", () => ({
  cookies: async () => ({ toString: () => "session=test-only" }),
}));
vi.mock("next/navigation", () => ({
  redirect,
  useRouter: () => ({ refresh: vi.fn() }),
}));
vi.mock("../app-page-header", () => ({
  AppPageHeader: () => <header>Shared navigation</header>,
}));
import CapitalPage from "./page";

const keys = [
  "accounts",
  "capital_buckets",
  "capital_reservations",
  "strategy_instances",
];
function inventoryFetch(failedIndex = -1, status = 200) {
  const fetch = vi.fn();
  keys.forEach((key, index) =>
    fetch.mockResolvedValueOnce({
      status: index === failedIndex ? status : 200,
      ok: index !== failedIndex || status === 200,
      json: async () => ({
        [key]:
          key === "accounts"
            ? [
                {
                  id: "example-account",
                  display_name: "Example portfolio",
                  provider: "coinbase",
                  status: "active",
                  base_currency: "USD",
                },
              ]
            : [],
      }),
    }),
  );
  vi.stubGlobal("fetch", fetch);
  return fetch;
}

describe("Capital presentation retains its owner-scoped loading boundary", () => {
  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
    vi.clearAllMocks();
  });

  it("keeps the four exact no-store inventories and provides a compact budget workspace", async () => {
    const fetch = inventoryFetch();
    render(await CapitalPage());
    expect(
      screen.getByRole("heading", { level: 1, name: "Capital & budgets" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("region", { name: "Example portfolio" }),
    ).toBeInTheDocument();
    expect(fetch).toHaveBeenCalledTimes(4);
    [
      "accounts",
      "capital-buckets",
      "strategy-capital-reservations",
      "strategy-instances",
    ].forEach((path, index) => {
      expect(fetch).toHaveBeenNthCalledWith(
        index + 1,
        expect.stringMatching(new RegExp(`/api/${path}$`)),
        { headers: { cookie: "session=test-only" }, cache: "no-store" },
      );
    });
    expect(
      screen.getByText("No screen here can place or authorize a broker order."),
    ).toBeInTheDocument();
  });

  it.each([0, 1, 2, 3])(
    "keeps the authentication redirect if inventory %i is unauthorized",
    async (index) => {
      inventoryFetch(index, 401);
      await expect(CapitalPage()).rejects.toThrow("redirect:/login");
      expect(redirect).toHaveBeenCalledTimes(1);
    },
  );

  it.each([0, 1, 2, 3])(
    "keeps unavailable inventory %i separate from an empty or actionable workspace",
    async (index) => {
      const fetch = inventoryFetch(index, 503);
      render(await CapitalPage());
      expect(screen.getByRole("alert")).toHaveTextContent(
        "not showing partial totals",
      );
      expect(
        screen.queryByRole("region", { name: "Capital summary" }),
      ).not.toBeInTheDocument();
      expect(
        screen.queryByRole("button", { name: "Create Capital Bucket" }),
      ).not.toBeInTheDocument();
      expect(
        screen.queryByRole("link", { name: "Create a budget ↓" }),
      ).not.toBeInTheDocument();
      expect(fetch).toHaveBeenCalledTimes(4);
    },
  );
});
