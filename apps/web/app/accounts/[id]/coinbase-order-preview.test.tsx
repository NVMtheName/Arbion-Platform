import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { CoinbaseOrderPreview } from "./coinbase-order-preview";

const capitalPolicies = [
  {
    id: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
    financialAccountID: "coinbase-account",
    name: "Coinbase manual",
    allocationType: "FIXED_AMOUNT" as const,
    allocationValue: "100",
    currency: "USD" as const,
    isReserve: false as const,
    protectedAmount: "10",
    status: "ACTIVE" as const,
  },
];

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
          product_rules: {
            provider: "coinbase",
            feed: "advanced_trade_product",
            product_id: "BTC-USD",
            product_type: "SPOT",
            base_asset: "BTC",
            quote_currency: "USD",
            base_increment: "0.00000001",
            quote_increment: "0.01",
            base_min_size: "0.00000001",
            base_max_size: "1000",
            quote_min_size: "1",
            quote_max_size: "1000000",
            status: "ONLINE",
            market_ioc_enabled: true,
            block_reasons: [],
            observed_at: "2026-08-21T17:00:00Z",
          },
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
        capitalPolicies={capitalPolicies}
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
    expect(
      screen.getByText("Market IOC enabled · 0.01 increment"),
    ).toBeInTheDocument();
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
        capitalPolicies={capitalPolicies}
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

  it("requires an account capital policy before a preview can be saved", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue({
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
            requested_size: { amount: "25", currency: "USD" },
            base_size: "0.0004",
            quote_size: "25",
            order_total: { amount: "25", currency: "USD" },
            commission_total: { amount: "0.15", currency: "USD" },
            preview_state: "READY",
            block_reasons: [],
            warnings: [],
            provider_trading_authorized: true,
            previewed_at: "2026-08-22T02:00:00Z",
            product_rules: {
              provider: "coinbase",
              feed: "advanced_trade_product",
              product_id: "BTC-USD",
              product_type: "SPOT",
              base_asset: "BTC",
              quote_currency: "USD",
              base_increment: "0.00000001",
              quote_increment: "0.01",
              base_min_size: "0.00000001",
              base_max_size: "1000",
              quote_min_size: "1",
              quote_max_size: "1000000",
              status: "ONLINE",
              market_ioc_enabled: true,
              block_reasons: [],
              observed_at: "2026-08-22T02:00:00Z",
            },
          },
          preview_semantics: "PROVIDER_ESTIMATE_ONLY",
          provider_trading_authorized: true,
          provider_preview_id_exposed: false,
          order_created: false,
          submission_available: false,
          ai_execution_authority: false,
          live_execution_available: false,
        }),
      }),
    );
    render(
      <CoinbaseOrderPreview
        accountID="coinbase-account"
        capitalPolicies={[]}
        symbols={["BTC"]}
        tradingAuthorized
      />,
    );
    fireEvent.change(screen.getByLabelText("Coinbase preview amount"), {
      target: { value: "25" },
    });
    fireEvent.click(
      screen.getByRole("button", { name: "Preview with Coinbase" }),
    );
    expect(await screen.findByText("READY")).toBeInTheDocument();
    expect(
      screen.getByText(/Create an active, non-reserve USD capital bucket/),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Save reviewable proposal" }),
    ).toBeDisabled();
  });

  it("persists and MFA-reviews a proposal without gaining execution authority", async () => {
    const preview = {
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
      estimated_average_filled_price: {
        amount: "60000.45",
        currency: "USD",
      },
      preview_state: "READY",
      block_reasons: [],
      warnings: [],
      provider_trading_authorized: true,
      previewed_at: "2026-08-22T02:00:00Z",
      product_rules: {
        provider: "coinbase",
        feed: "advanced_trade_product",
        product_id: "BTC-USD",
        product_type: "SPOT",
        base_asset: "BTC",
        quote_currency: "USD",
        base_increment: "0.00000001",
        quote_increment: "0.01",
        base_min_size: "0.00000001",
        base_max_size: "1000",
        quote_min_size: "1",
        quote_max_size: "1000000",
        status: "ONLINE",
        market_ioc_enabled: true,
        block_reasons: [],
        observed_at: "2026-08-22T02:00:00Z",
      },
    };
    const envelope = (
      status: "REVIEW_REQUIRED" | "USER_APPROVED_NONEXECUTABLE",
      version: number,
    ) => ({
      order_intent: {
        id: "intent-1",
        financial_account_id: "coinbase-account",
        capital_bucket_id: capitalPolicies[0].id,
        source: "UI",
        provider: "coinbase",
        product_id: "BTC-USD",
        base_asset: "BTC",
        quote_currency: "USD",
        side: "BUY",
        order_type: "MARKET_IOC",
        requested_size: { amount: "25.5", currency: "USD" },
        status,
        version,
        preview: {
          ...preview,
          expires_at: "2026-08-22T02:01:00Z",
        },
        risk: {
          policy_version: "manual_coinbase_spot.v1",
          evaluation_id: "66666666-6666-4666-8666-666666666666",
          capital_bucket_id: capitalPolicies[0].id,
          capital_bucket_name: "Coinbase manual",
          allocation_type: "FIXED_AMOUNT",
          allocation_value: "100",
          protected_amount: "10",
          account_available_cash: { amount: "200", currency: "USD" },
          target_available_quantity: "0.01",
          proposed_notional: { amount: "25.65", currency: "USD" },
          decision: "ALLOW",
          reason_codes: ["ALLOWED"],
          warnings: ["AUTONOMY_REQUIRES_APPROVAL"],
          checks: [
            {
              code: "CAPITAL_POLICY_REQUIRED",
              result: "PASS",
              message:
                "The manual proposal is bound to an active owner/account capital policy.",
            },
          ],
          approval_required: true,
          platform_execution_available: false,
          observed_at: "2026-08-22T02:00:01Z",
        },
        review_scope: "PROPOSAL_REVIEW_ONLY",
        submission_available: false,
        risk_approval_available: false,
        ai_execution_authority: false,
        live_execution_available: false,
      },
      approval_scope: "PROPOSAL_REVIEW_ONLY",
      provider_order_created: false,
      submission_available: false,
      risk_approval_available: false,
      ai_execution_authority: false,
      live_execution_available: false,
    });
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          preview,
          preview_semantics: "PROVIDER_ESTIMATE_ONLY",
          provider_trading_authorized: true,
          provider_preview_id_exposed: false,
          order_created: false,
          submission_available: false,
          ai_execution_authority: false,
          live_execution_available: false,
        }),
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => envelope("REVIEW_REQUIRED", 1),
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => envelope("USER_APPROVED_NONEXECUTABLE", 2),
      });
    vi.stubGlobal("fetch", fetchMock);
    render(
      <CoinbaseOrderPreview
        accountID="coinbase-account"
        capitalPolicies={capitalPolicies}
        symbols={["BTC"]}
        tradingAuthorized
      />,
    );

    fireEvent.change(screen.getByLabelText("Coinbase preview amount"), {
      target: { value: "25.50" },
    });
    fireEvent.click(
      screen.getByRole("button", { name: "Preview with Coinbase" }),
    );
    await screen.findByText("READY");
    fireEvent.click(
      screen.getByRole("button", { name: "Save reviewable proposal" }),
    );
    expect(await screen.findByText("review required")).toBeInTheDocument();
    expect(
      screen.getByText(/Coinbase re-quoted the proposal/),
    ).toHaveTextContent("estimated total 25.50 USD plus 0.15 USD commission");
    expect(screen.getByText(/product status ONLINE/)).toBeInTheDocument();
    expect(screen.getByText("DETERMINISTIC CAPITAL GATE")).toBeInTheDocument();
    expect(screen.getByText(/proposed notional 25.65 USD/)).toBeInTheDocument();

    const createBody = JSON.parse(
      fetchMock.mock.calls[1][1].body as string,
    ) as {
      symbol: string;
      side: string;
      size: string;
      capital_bucket_id: string;
      idempotency_key: string;
    };
    expect(createBody).toMatchObject({
      symbol: "BTC",
      side: "BUY",
      size: "25.50",
      capital_bucket_id: capitalPolicies[0].id,
    });
    expect(createBody.idempotency_key).toMatch(/^[0-9a-f-]{36}$/);

    fireEvent.change(
      screen.getByLabelText("Order proposal authenticator code"),
      { target: { value: "123456" } },
    );
    fireEvent.click(
      screen.getByRole("button", { name: "Review proposal with MFA" }),
    );
    expect(
      await screen.findByText("user approved nonexecutable"),
    ).toBeInTheDocument();
    expect(fetchMock.mock.calls[2]).toEqual([
      "/api/order-intents/intent-1/review",
      {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ expected_version: 1, mfa_code: "123456" }),
      },
    ]);
    expect(
      screen.getByText(/No execution approval exists/),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /submit|place|confirm order/i }),
    ).not.toBeInTheDocument();
  });
});
