import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import {
  emptyMarketWatchlist,
  MarketWatchlist,
  type MarketWatchlistData,
} from "./market-watchlist";

const ready: MarketWatchlistData = {
  ...emptyMarketWatchlist,
  market_state: "READY",
  message:
    "Current Coinbase Exchange observations are shown with exact source and venue evidence.",
  generated_at: "2026-08-21T22:00:00Z",
  items: [
    {
      id: "3f6ab43c-8abd-4056-a858-ccb8051a045f",
      asset_class: "CRYPTO",
      symbol: "BTC",
      quote_currency: "USD",
      created_at: "2026-08-21T21:00:00Z",
      observation: {
        symbol: "BTC",
        name: "Bitcoin",
        currency: "USD",
        current_price: "70187.10",
        bid: "70186.90",
        ask: "70187.20",
        provenance: {
          provider: "coinbase",
          feed: "rest_ticker",
          quality: "REAL_TIME_SINGLE_VENUE",
          venue: "coinbase_exchange",
          provider_timestamp: "2026-08-21T22:00:00Z",
          received_at: "2026-08-21T22:00:01Z",
        },
      },
    },
  ],
};

describe("MarketWatchlist", () => {
  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
  });

  it("renders a branded empty state without invented market values", () => {
    render(<MarketWatchlist />);

    expect(
      screen.getByRole("heading", { name: "Your market radar." }),
    ).toBeInTheDocument();
    expect(screen.getByText("No saved assets yet")).toBeInTheDocument();
    expect(screen.getByText("OBSERVE ONLY")).toBeInTheDocument();
    expect(screen.queryByText(/\$0\.00/)).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /buy|sell|trade|order/i }),
    ).not.toBeInTheDocument();
  });

  it("shows exact Coinbase evidence and no execution controls", () => {
    render(<MarketWatchlist initialData={ready} />);

    expect(screen.getByText("$70,187.10")).toBeInTheDocument();
    expect(screen.getByText("coinbase_exchange")).toBeInTheDocument();
    expect(screen.getByText("REAL TIME SINGLE VENUE")).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Remove BTC" }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /buy|sell|trade|order/i }),
    ).not.toBeInTheDocument();
  });

  it("adds a canonical symbol and refreshes the durable view", async () => {
    const withEthereum: MarketWatchlistData = {
      ...ready,
      items: [
        ...ready.items,
        {
          id: "8b58869d-18a8-4cba-9a73-1e8fd372ab33",
          asset_class: "CRYPTO",
          symbol: "ETH",
          quote_currency: "USD",
          created_at: "2026-08-21T22:01:00Z",
        },
      ],
      market_state: "PARTIAL",
      unavailable_symbols: ["ETH"],
    };
    const request = vi
      .fn()
      .mockResolvedValueOnce({
        ok: true,
        status: 201,
        json: async () => ({ item: withEthereum.items[1] }),
      })
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => withEthereum,
      });
    vi.stubGlobal("fetch", request);
    render(<MarketWatchlist initialData={ready} />);

    fireEvent.change(screen.getByLabelText("Add a crypto asset"), {
      target: { value: "eth" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Add to radar" }));

    await waitFor(() => expect(screen.getByText("ETH")).toBeInTheDocument());
    expect(request).toHaveBeenNthCalledWith(
      1,
      "/api/markets/watchlist",
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({ symbol: "ETH" }),
      }),
    );
    expect(request).toHaveBeenNthCalledWith(2, "/api/markets/watchlist", {
      cache: "no-store",
    });
    expect(
      screen.getByText(/No approved USD observation is available/),
    ).toBeInTheDocument();
  });

  it("keeps saved assets visible during a venue outage", () => {
    render(
      <MarketWatchlist
        initialData={{
          ...ready,
          items: [{ ...ready.items[0], observation: undefined }],
          market_state: "UNAVAILABLE",
          message:
            "The watchlist is saved, but current venue observations are temporarily unavailable. No values were substituted.",
          unavailable_symbols: ["BTC"],
        }}
      />,
    );

    expect(screen.getByText("BTC")).toBeInTheDocument();
    expect(screen.getByText("Unavailable")).toBeInTheDocument();
    expect(screen.getByText(/No estimate was substituted/)).toBeInTheDocument();
  });
});
