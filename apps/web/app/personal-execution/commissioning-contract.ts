import { executionObject, isExecutionID } from "./contract";

export type CommissioningReview = {
  terms_digest: string;
  product_id: string;
  account_label: string;
  allocation_usd: string;
  maximum_order_usd: string;
  effective_from: string;
  expires_at: string;
  model_id: string;
  objective: string;
  interval_minutes: number;
  max_trades_per_day: number;
  max_capital_deployed_usd: string;
  max_single_position_usd: string;
  minimum_cash_reserve_usd: string;
};
export type CommissioningReceipt = CommissioningReview & {
  snapshot_digest: string;
};
export type CommissioningConsent = {
  id: string;
  snapshot_digest: string;
  approved_at: string;
  expires_at: string;
  revoked_at: string | null;
};
export type CommissioningContext =
  | { available: false }
  | { available: true; session_binding: string; review: CommissioningReview };

function invalid(): never {
  throw new Error("Commissioning response is invalid.");
}
export function commissioningDigest(value: unknown): string {
  return typeof value === "string" && /^[0-9a-f]{64}$/.test(value)
    ? value
    : invalid();
}
function text(value: unknown, max: number): string {
  return typeof value === "string" &&
    value.trim() === value &&
    value.length > 0 &&
    value.length <= max &&
    !/[\u0000-\u001f\u007f]/.test(value)
    ? value
    : invalid();
}
function amount(value: unknown, positive = true): string {
  return typeof value === "string" &&
    /^(0|[1-9][0-9]{0,17})(\.[0-9]{1,18})?$/.test(value) &&
    (!positive || /[1-9]/.test(value))
    ? value
    : invalid();
}
function timestamp(value: unknown): string {
  if (
    typeof value !== "string" ||
    !/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?Z$/.test(value) ||
    value.startsWith("0000")
  )
    return invalid();
  const date = new Date(value);
  if (
    !Number.isFinite(date.getTime()) ||
    date.toISOString().slice(0, 19) !== value.slice(0, 19)
  )
    return invalid();
  return value;
}
function integer(value: unknown, min: number, max: number): number {
  return typeof value === "number" &&
    Number.isInteger(value) &&
    value >= min &&
    value <= max
    ? value
    : invalid();
}
const reviewFields = [
  "terms_digest",
  "product_id",
  "account_label",
  "allocation_usd",
  "maximum_order_usd",
  "effective_from",
  "expires_at",
  "model_id",
  "objective",
  "interval_minutes",
  "max_trades_per_day",
  "max_capital_deployed_usd",
  "max_single_position_usd",
  "minimum_cash_reserve_usd",
];

export function parseCommissioningReview(value: unknown): CommissioningReview {
  const v = executionObject(value, reviewFields);
  const product = text(v.product_id, 20),
    model = text(v.model_id, 128);
  if (
    !/^[A-Z][A-Z0-9]{0,15}-USD$/.test(product) ||
    product === "USD-USD" ||
    !/^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$/.test(model)
  )
    return invalid();
  const result = {
    terms_digest: commissioningDigest(v.terms_digest),
    product_id: product,
    account_label: text(v.account_label, 120),
    allocation_usd: amount(v.allocation_usd),
    maximum_order_usd: amount(v.maximum_order_usd),
    effective_from: timestamp(v.effective_from),
    expires_at: timestamp(v.expires_at),
    model_id: model,
    objective: text(v.objective, 2000),
    interval_minutes: integer(v.interval_minutes, 30, 1440),
    max_trades_per_day: integer(v.max_trades_per_day, 1, 48),
    max_capital_deployed_usd: amount(v.max_capital_deployed_usd),
    max_single_position_usd: amount(v.max_single_position_usd),
    minimum_cash_reserve_usd: amount(v.minimum_cash_reserve_usd, false),
  };
  const duration =
    Date.parse(result.expires_at) - Date.parse(result.effective_from);
  if (
    duration <= 0 ||
    duration > 86_400_000 ||
    result.effective_from.includes(".") ||
    result.expires_at.includes(".")
  )
    return invalid();
  return result;
}
export function parseCommissioningContext(
  value: unknown,
): CommissioningContext {
  const v = executionObject(
    value,
    ["available"],
    ["session_binding", "review"],
  );
  if (v.available === false) {
    executionObject(value, ["available"]);
    return { available: false };
  }
  if (v.available !== true) return invalid();
  return {
    available: true,
    session_binding: commissioningDigest(v.session_binding),
    review: parseCommissioningReview(v.review),
  };
}
export function parseCommissioningReceipt(
  value: unknown,
  review: CommissioningReview,
): CommissioningReceipt {
  const v = executionObject(value, [...reviewFields, "snapshot_digest"]);
  const { snapshot_digest, ...terms } = v;
  const parsed = parseCommissioningReview(terms);
  if (JSON.stringify(parsed) !== JSON.stringify(review)) return invalid();
  return { ...parsed, snapshot_digest: commissioningDigest(snapshot_digest) };
}
export function parseCommissioningConsent(
  value: unknown,
  receipt: CommissioningReceipt,
): CommissioningConsent {
  const v = executionObject(value, [
    "id",
    "snapshot_digest",
    "approved_at",
    "expires_at",
    "revoked_at",
  ]);
  if (!isExecutionID(v.id)) return invalid();
  const result = {
    id: v.id,
    snapshot_digest: commissioningDigest(v.snapshot_digest),
    approved_at: timestamp(v.approved_at),
    expires_at: timestamp(v.expires_at),
    revoked_at: v.revoked_at === null ? null : timestamp(v.revoked_at),
  };
  const approved = Date.parse(result.approved_at),
    expires = Date.parse(result.expires_at);
  if (
    result.snapshot_digest !== receipt.snapshot_digest ||
    approved < Date.parse(receipt.effective_from) ||
    expires <= approved ||
    expires > Date.parse(receipt.expires_at) ||
    expires - approved > 86_400_000 ||
    (result.revoked_at !== null && Date.parse(result.revoked_at) < approved)
  )
    return invalid();
  return result;
}
