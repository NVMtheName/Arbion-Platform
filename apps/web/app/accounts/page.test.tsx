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
vi.mock("next/navigation", () => ({ redirect }));
vi.mock("../app-page-header", () => ({
  AppPageHeader: () => <header>Shared navigation</header>,
}));

import Accounts from "./page";

describe("Accounts route boundary", () => {
  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
    vi.clearAllMocks();
  });

  it("preserves the signed-out redirect before requesting holdings", async () => {
    const fetch = vi.fn().mockResolvedValue({ status: 401, ok: false });
    vi.stubGlobal("fetch", fetch);
    await expect(Accounts()).rejects.toThrow("redirect:/login");
    expect(redirect).toHaveBeenCalledWith("/login");
    expect(fetch).toHaveBeenCalledTimes(1);
  });

  it("renders an unavailable account-list response distinctly from onboarding", async () => {
    const fetch = vi.fn().mockResolvedValue({ status: 503, ok: false });
    vi.stubGlobal("fetch", fetch);
    render(await Accounts());
    expect(screen.getByRole("status")).toHaveTextContent(
      "This is not an empty portfolio",
    );
    expect(
      screen.queryByText("Connect your first account"),
    ).not.toBeInTheDocument();
    expect(fetch).toHaveBeenCalledTimes(1);
  });

  it("keeps account-scoped read-only loading and independent failures intact", async () => {
    const accounts = [
      {
        id: "coinbase-1",
        provider: "coinbase",
        display_name: "Crypto",
        status: "active",
      },
      {
        id: "schwab-1",
        provider: "schwab",
        display_name: "Brokerage",
        status: "active",
      },
    ];
    const fetch = vi
      .fn()
      .mockResolvedValueOnce({
        status: 200,
        ok: true,
        json: async () => ({ accounts }),
      })
      .mockResolvedValueOnce({
        status: 200,
        ok: true,
        json: async () => ({
          portfolio: {
            holdings_state: "READY",
            positions: [
              {
                symbol: "BTC",
                quantity: "0.5",
                market_value: { amount: "30125", currency: "USD" },
              },
            ],
          },
        }),
      })
      .mockResolvedValueOnce({ status: 503, ok: false });
    vi.stubGlobal("fetch", fetch);
    render(await Accounts());
    expect(screen.getByText("BTC")).toBeInTheDocument();
    expect(screen.getByText(/Brokerage could not refresh/)).toBeInTheDocument();
    expect(fetch.mock.calls.map(([url]) => url)).toEqual([
      expect.stringMatching(/\/api\/accounts$/),
      expect.stringMatching(/\/api\/accounts\/coinbase-1\/portfolio\/crypto$/),
      expect.stringMatching(/\/api\/accounts\/schwab-1\/positions$/),
    ]);
    for (const [, options] of fetch.mock.calls) {
      expect(options).toEqual({
        headers: { cookie: "session=test-only" },
        cache: "no-store",
      });
    }
  });
});
