import { act, cleanup, render } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { MarketCommandSurface } from "./market-command-surface";
import { MarketWatchlist, emptyMarketWatchlist } from "./market-watchlist";
import { MarketSourceGrid } from "./market-source-grid";
import { MarketHealthTimeline } from "./market-health-timeline";

describe("Market UI refresh cadence", () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => {
    cleanup();
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });
  it("retains the five-second crypto polling interval and cancels on unmount", async () => {
    const request = vi
      .fn()
      .mockResolvedValue({ ok: true, json: async () => ({ markets: [] }) });
    vi.stubGlobal("fetch", request);
    const view = render(
      <MarketCommandSurface
        sources={[
          {
            id: "coinbase_exchange",
            label: "Coinbase",
            role: "MARKET_OBSERVATION",
            feed: "rest_ticker",
            quality: "REAL_TIME_SINGLE_VENUE",
            capabilities: ["CRYPTO_MARKETS"],
            enabled: true,
            healthy: true,
          },
        ]}
      />,
    );
    await act(() => vi.advanceTimersByTimeAsync(0));
    expect(request).toHaveBeenCalledExactlyOnceWith(
      "/api/markets/crypto?currency=usd&limit=8",
      { cache: "no-store" },
    );
    await act(() => vi.advanceTimersByTimeAsync(4999));
    expect(request).toHaveBeenCalledTimes(1);
    await act(() => vi.advanceTimersByTimeAsync(1));
    expect(request).toHaveBeenCalledTimes(2);
    view.unmount();
    await act(() => vi.advanceTimersByTimeAsync(5000));
    expect(request).toHaveBeenCalledTimes(2);
  });
  it.each([
    {
      name: "watchlist",
      interval: 30000,
      endpoint: "/api/markets/watchlist",
      body: emptyMarketWatchlist,
      element: <MarketWatchlist />,
    },
    {
      name: "source status",
      interval: 30000,
      endpoint: "/api/markets/sources",
      body: {
        sources: [],
        status_semantics: "PROCESS_LOCAL_TIME_BOUNDED_PROVIDER_VERIFICATION",
        request_usage_semantics: "PROCESS_LOCAL_BOUNDED_AGGREGATES",
        provider_quota_exposed: false,
        live_execution_available: false,
        provider_errors_exposed: false,
      },
      element: <MarketSourceGrid sources={[]} />,
    },
    {
      name: "source history",
      interval: 300000,
      endpoint: "/api/markets/source-history",
      body: {},
      element: <MarketHealthTimeline sources={[]} />,
    },
  ])(
    "retains the existing $name timer without a new mount request",
    async ({ interval, endpoint, body, element }) => {
      const request = vi
        .fn()
        .mockResolvedValue({ ok: true, json: async () => body });
      vi.stubGlobal("fetch", request);
      const view = render(element);
      expect(request).not.toHaveBeenCalled();
      await act(() => vi.advanceTimersByTimeAsync(interval - 1));
      expect(request).not.toHaveBeenCalled();
      await act(() => vi.advanceTimersByTimeAsync(1));
      expect(request).toHaveBeenCalledExactlyOnceWith(endpoint, {
        cache: "no-store",
      });
      view.unmount();
      await act(() => vi.advanceTimersByTimeAsync(interval));
      expect(request).toHaveBeenCalledTimes(1);
    },
  );
});
