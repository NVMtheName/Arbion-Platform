import type {
  CommissioningContext,
  CommissioningReview,
  CommissioningReceipt,
  CommissioningConsent,
} from "./commissioning-contract";

export const review: CommissioningReview = {
  terms_digest: "a".repeat(64),
  product_id: "BTC-USD",
  account_label: "Connected Coinbase portfolio",
  allocation_usd: "100",
  maximum_order_usd: "25",
  effective_from: "2099-10-07T12:00:00Z",
  expires_at: "2099-10-08T12:00:00Z",
  model_id: "pinned-test-model",
  objective: "Evaluate bounded spot proposals.",
  interval_minutes: 30,
  max_trades_per_day: 4,
  max_capital_deployed_usd: "100",
  max_single_position_usd: "25",
  minimum_cash_reserve_usd: "0",
};
export const receipt: CommissioningReceipt = {
  ...review,
  snapshot_digest: "b".repeat(64),
};
export const consent: CommissioningConsent = {
  id: "11111111-1111-4111-8111-111111111111",
  snapshot_digest: receipt.snapshot_digest,
  approved_at: "2099-10-07T12:01:00Z",
  expires_at: review.expires_at,
  revoked_at: null,
};
export const context: CommissioningContext = {
  available: true,
  session_binding: "c".repeat(64),
  review,
};
export const response = (value: unknown, status = 200) =>
  new Response(JSON.stringify(value), { status });
export const missing = () =>
  response(
    {
      error: { code: "EXECUTION_NOT_FOUND", message: "Private upstream text" },
    },
    404,
  );
