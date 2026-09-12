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
import MarketsPage from "./page";
import { emptyMarketWatchlist } from "./market-watchlist";

const source = {
  id: "test-source",
  enabled: false,
  healthy: false,
  capabilities: [],
};
const account = {
  id: "test-account",
  provider: "schwab",
  status: "active",
  base_currency: "USD",
  display_name: "Test account",
};
function load({
  failed = -1,
  status = 503,
  watchlist = emptyMarketWatchlist,
}: { failed?: number; status?: number; watchlist?: unknown } = {}) {
  const request = vi.fn();
  [
    {
      sources: [source],
      status_generated_at: "2026-09-12T12:00:00Z",
      live_execution_available: false,
    },
    { accounts: [account] },
    {},
    watchlist,
  ].forEach((body, index) =>
    request.mockResolvedValueOnce({
      status: index === failed ? status : 200,
      ok: index !== failed,
      json: async () => body,
    }),
  );
  vi.stubGlobal("fetch", request);
  return request;
}

describe("Markets server loading remains unchanged", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.clearAllMocks();
  });
  it("passes existing account-scoped evidence through after four no-store requests", async () => {
    const request = load();
    const view = await MarketsPage();
    expect(request).toHaveBeenCalledTimes(4);
    [
      "markets/sources",
      "accounts",
      "markets/source-history",
      "markets/watchlist",
    ].forEach((path, index) =>
      expect(request).toHaveBeenNthCalledWith(
        index + 1,
        expect.stringContaining(`/api/${path}`),
        { headers: { cookie: "session=test-only" }, cache: "no-store" },
      ),
    );
    expect(view.props).toMatchObject({
      sources: [source],
      accounts: [account],
      statusGeneratedAt: "2026-09-12T12:00:00Z",
      watchlist: emptyMarketWatchlist,
    });
    expect(view.props.history).toBeUndefined();
  });
  it("retains the source authentication redirect", async () => {
    load({ failed: 0, status: 401 });
    await expect(MarketsPage()).rejects.toThrow("redirect:/login");
  });
  it("preserves the existing source-unavailable fallback", async () => {
    load({ failed: 0 });
    expect((await MarketsPage()).props.sources).toEqual([]);
  });
  it.each([
    "provider_write_available",
    "order_actions_available",
    "live_execution_available",
  ])("does not pass through an unsafe %s watchlist", async (field) => {
    load({ watchlist: { ...emptyMarketWatchlist, [field]: true } });
    expect((await MarketsPage()).props.watchlist).toEqual(emptyMarketWatchlist);
  });
});
