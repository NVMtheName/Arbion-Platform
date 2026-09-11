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
vi.mock("../../app-page-header", () => ({
  AppPageHeader: () => <header>Shared navigation</header>,
}));
import ConnectionsPage from "./page";

const payloads: Record<string, unknown> = {
  "/api/connections/ai": {
    connections: [],
    providers: [],
    can_use_neural_engine: true,
  },
  "/api/connections/financial/providers": {
    providers: [
      {
        id: "coinbase",
        label: "Coinbase",
        availability: "implemented",
        configured: true,
        auth_type: "api_key",
      },
    ],
    can_connect_financial_accounts: true,
  },
  "/api/connections/financial": { connections: [] },
  "/api/accounts": { accounts: [] },
  "/api/settings/neural-engine": { preference: null },
  "/api/strategy-instances": { strategy_instances: [] },
  "/api/connections/financial/authorization-receipts": {},
};
function mockReads(failedPath?: string, status = 503) {
  const fetch = vi.fn(async (url: string) => {
    const path = new URL(url).pathname;
    return path === failedPath
      ? { status, ok: false }
      : { status: 200, ok: true, json: async () => payloads[path] };
  });
  vi.stubGlobal("fetch", fetch);
  return fetch;
}
describe("Connections route presentation boundary", () => {
  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
    vi.clearAllMocks();
  });
  it("retains signed-out redirects before provider setup", async () => {
    const fetch = mockReads("/api/connections/ai", 401);
    await expect(ConnectionsPage()).rejects.toThrow("redirect:/login");
    expect(fetch).toHaveBeenCalledTimes(1);
  });
  it("keeps unavailable saved inventory out of the empty account connect flow", async () => {
    const fetch = mockReads("/api/accounts");
    render(await ConnectionsPage());
    expect(
      screen.getByText(/Some saved setup information is unavailable/),
    ).toBeVisible();
    expect(
      screen.queryByRole("button", { name: "Connect Coinbase" }),
    ).not.toBeInTheDocument();
    expect(fetch).toHaveBeenCalledTimes(7);
    for (const [, options] of fetch.mock.calls as unknown as [
      string,
      unknown,
    ][]) {
      expect(options).toEqual({
        headers: { cookie: "session=test-only" },
        cache: "no-store",
      });
    }
  });
  it("distinguishes available empty setup and unavailable preference without changing reads", async () => {
    mockReads("/api/settings/neural-engine");
    render(await ConnectionsPage());
    expect(
      screen.getByRole("button", { name: "Connect Coinbase" }),
    ).toBeEnabled();
    expect(
      screen.getByRole("link", { name: /3 Default model Status unavailable/ }),
    ).toBeInTheDocument();
    expect(screen.getByText(/0 of 3 essentials ready/)).toBeInTheDocument();
  });
});
