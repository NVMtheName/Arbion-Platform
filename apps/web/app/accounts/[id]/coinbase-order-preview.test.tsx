import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { CoinbaseOrderPreview } from "./coinbase-order-preview";

describe("CoinbaseOrderPreview", () => {
  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
  });

  it("requests a provider preview without exposing an execution action", async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({
        preview: {
          provider: "coinbase",
          feed: "advanced_trade_order_preview",
          product_id: "BTC-USD",
          base_asset: "BTC",
          quote_currency: "USD",
          side: "BUY",
          order_type: "MARKET_IOC",
          requested_size: { amount: "25.50", currency: "USD" },
          base_size: "0.0004249",
          quote_size: "25.50",
          order_total: { amount: "25.50", currency: "USD" },
          commission_total: { amount: "0.15", currency: "USD" },
          best_bid: { amount: "59990.12", currency: "USD" },
          best_ask: { amount: "60001.34", currency: "USD" },
          estimated_average_filled_price: {
            amount: "60000.45",
            currency: "USD",
          },
          preview_state: "READY",
          block_reasons: [],
          warnings: ["SMALL_ORDER"],
          provider_trading_authorized: true,
          previewed_at: "2026-08-21T17:00:00Z",
        },
        preview_semantics: "PROVIDER_ESTIMATE_ONLY",
        provider_trading_authorized: true,
        provider_preview_id_exposed: false,
        order_created: false,
        submission_available: false,
        ai_execution_authority: false,
        live_execution_available: false,
      }),
    });
    vi.stubGlobal("fetch", fetchMock);
    render(
      <CoinbaseOrderPreview
        accountID="coinbase-account"
        symbols={["ETH", "BTC"]}
        tradingAuthorized
      />,
    );

    expect(screen.getByText("Trade key granted")).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText("Coinbase preview amount"), {
      target: { value: "25.50" },
    });
    fireEvent.click(
      screen.getByRole("button", { name: "Preview with Coinbase" }),
    );

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/accounts/coinbase-account/orders/preview",
      {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ symbol: "BTC", side: "BUY", size: "25.50" }),
      },
    );
    expect(await screen.findByText("READY")).toBeInTheDocument();
    expect(screen.getByText("60000.45 USD")).toBeInTheDocument();
    expect(screen.getByText(/No order was created/)).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /submit|place|confirm order/i }),
    ).not.toBeInTheDocument();
  });

  it("rejects a response that claims submission capability", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue({
        ok: true,
        json: async () => ({
          preview: {
            provider: "coinbase",
            feed: "advanced_trade_order_preview",
            product_id: "BTC-USD",
            quote_currency: "USD",
            side: "BUY",
            preview_state: "READY",
          },
          preview_semantics: "PROVIDER_ESTIMATE_ONLY",
          provider_preview_id_exposed: false,
          order_created: false,
          submission_available: true,
          ai_execution_authority: false,
          live_execution_available: false,
        }),
      }),
    );
    render(
      <CoinbaseOrderPreview
        accountID="coinbase-account"
        symbols={["BTC"]}
        tradingAuthorized={false}
      />,
    );
    fireEvent.change(screen.getByLabelText("Coinbase preview amount"), {
      target: { value: "10" },
    });
    fireEvent.click(
      screen.getByRole("button", { name: "Preview with Coinbase" }),
    );
    expect(
      await screen.findByText(
        "Arbion rejected an unsafe Coinbase preview response.",
      ),
    ).toBeInTheDocument();
  });
});
