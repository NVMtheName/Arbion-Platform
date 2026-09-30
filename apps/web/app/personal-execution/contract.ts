export type PrepareCommand = {
  request_key: string;
  side: "BUY" | "SELL";
  base_size: string;
  limit_price: string;
  fee_allowance_usd: string;
  maximum_debit_usd: string;
};
export type ApproveCommand = { expected_digest: string; mfa_code: string };
export type SendCommand = { evidence_id: string };
export type OwnerOrder = {
  id: string;
  request_digest: string;
  product_id: string;
  side: "BUY" | "SELL";
  base_size: string;
  limit_price: string;
  fee_allowance_usd: string;
  maximum_debit_usd: string;
  created_at: string;
  state:
    | "PREPARED"
    | "APPROVED"
    | "SUBMISSION_UNKNOWN"
    | "REJECTED_HELD"
    | "BROKER_ACKNOWLEDGED"
    | "PARTIALLY_FILLED"
    | "AWAITING_ACCOUNT_SETTLEMENT"
    | "SETTLED"
    | "NOT_SENT"
    | "UNAVAILABLE";
  summary: string;
  approval_status: "NONE" | "RECORDED" | "REVOKED" | "EXPIRED";
  approval_expires_at?: string;
  cancellation_status: "NONE" | "UNKNOWN" | "ACCEPTED" | "NOT_ACCEPTED";
  account_held: boolean;
  capital_held: boolean;
  account_blocked: boolean;
  fill_count: number;
  base_filled: string;
  gross_usd: string;
  fee_usd: string;
  terminal_status?: "FILLED" | "CANCELLED" | "REJECTED" | "EXPIRED";
  accounting?: {
    opening_cash_usd: string;
    opening_base: string;
    closing_cash_usd: string;
    closing_base: string;
    recorded_at: string;
  };
};
export type OwnerPreflight = { evidence_id: string; order: OwnerOrder };

const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;
const digest = /^[0-9a-f]{64}$/;
const decimal = /^(0|[1-9][0-9]{0,17})(\.[0-9]{1,18})?$/;
const aggregate = /^(0|[1-9][0-9]{0,35})(\.[0-9]{1,36})?$/;
const states = [
  "PREPARED",
  "APPROVED",
  "SUBMISSION_UNKNOWN",
  "REJECTED_HELD",
  "BROKER_ACKNOWLEDGED",
  "PARTIALLY_FILLED",
  "AWAITING_ACCOUNT_SETTLEMENT",
  "SETTLED",
  "NOT_SENT",
  "UNAVAILABLE",
];

function invalid(): never {
  throw new Error("Invalid execution contract.");
}

// Accept only plain own data properties: never getters, inherited authority,
// prototypes, arrays or silently ignored provider fields.
export function executionObject(
  value: unknown,
  required: string[],
  optional: string[] = [],
): Record<string, unknown> {
  if (
    !value ||
    typeof value !== "object" ||
    Object.getPrototypeOf(value) !== Object.prototype
  )
    return invalid();
  const properties = Object.getOwnPropertyDescriptors(value);
  const keys = Reflect.ownKeys(properties);
  if (
    keys.length > required.length + optional.length ||
    required.some((key) => !Object.hasOwn(properties, key))
  )
    return invalid();
  for (const key of keys) {
    if (
      typeof key !== "string" ||
      (!required.includes(key) && !optional.includes(key)) ||
      !Object.hasOwn(properties[key], "value") ||
      !properties[key].enumerable
    )
      return invalid();
  }
  return value as Record<string, unknown>;
}

function text(value: unknown, maximum = 512): string {
  if (
    typeof value !== "string" ||
    !value ||
    value.length > maximum ||
    /[\u0000-\u001f\u007f]/.test(value)
  )
    return invalid();
  return value;
}

export function isExecutionID(value: unknown): value is string {
  return (
    typeof value === "string" &&
    uuid.test(value) &&
    value !== "00000000-0000-0000-0000-000000000000"
  );
}

function id(value: unknown): string {
  return isExecutionID(value) ? value : invalid();
}
function hash(value: unknown): string {
  return typeof value === "string" && digest.test(value) ? value : invalid();
}
function choice<T extends string>(value: unknown, choices: readonly T[]): T {
  return typeof value === "string" && choices.includes(value as T)
    ? (value as T)
    : invalid();
}
function amount(value: unknown, positive = false, wide = false): string {
  if (
    typeof value !== "string" ||
    !(wide ? aggregate : decimal).test(value) ||
    (positive && !/[1-9]/.test(value))
  )
    return invalid();
  return value;
}
function flag(value: unknown): boolean {
  return typeof value === "boolean" ? value : invalid();
}
function timestamp(value: unknown): string {
  const s = text(value, 40);
  const m =
    /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.\d{1,9})?(?:Z|[+-]\d{2}:\d{2})$/.exec(
      s,
    );
  if (
    !m ||
    +m[1] < 1 ||
    +m[2] < 1 ||
    +m[2] > 12 ||
    +m[3] < 1 ||
    +m[3] > new Date(Date.UTC(+m[1], +m[2], 0)).getUTCDate() ||
    +m[4] > 23 ||
    +m[5] > 59 ||
    +m[6] > 59 ||
    !Number.isFinite(Date.parse(s))
  )
    return invalid();
  return s;
}

export function sameExecutionAmount(a: string, b: string): boolean {
  const canonical = (s: string) =>
    s.includes(".") ? s.replace(/0+$/, "").replace(/\.$/, "") : s;
  return decimal.test(a) && decimal.test(b) && canonical(a) === canonical(b);
}

export function parsePrepareCommand(value: unknown): PrepareCommand {
  const o = executionObject(value, [
    "request_key",
    "side",
    "base_size",
    "limit_price",
    "fee_allowance_usd",
    "maximum_debit_usd",
  ]);
  const side = choice(o.side, ["BUY", "SELL"] as const);
  return {
    request_key: id(o.request_key),
    side,
    base_size: amount(o.base_size, true),
    limit_price: amount(o.limit_price, true),
    fee_allowance_usd: amount(o.fee_allowance_usd),
    maximum_debit_usd: amount(o.maximum_debit_usd, side === "BUY"),
  };
}
export function parseApproveCommand(value: unknown): ApproveCommand {
  const o = executionObject(value, ["expected_digest", "mfa_code"]);
  return {
    expected_digest: hash(o.expected_digest),
    mfa_code: text(o.mfa_code, 128),
  };
}
export function parseSendCommand(value: unknown): SendCommand {
  const o = executionObject(value, ["evidence_id"]);
  return { evidence_id: id(o.evidence_id) };
}

export function parseOwnerOrder(value: unknown): OwnerOrder {
  const o = executionObject(
    value,
    [
      "id",
      "request_digest",
      "product_id",
      "side",
      "base_size",
      "limit_price",
      "fee_allowance_usd",
      "maximum_debit_usd",
      "created_at",
      "state",
      "summary",
      "approval_status",
      "cancellation_status",
      "account_held",
      "capital_held",
      "account_blocked",
      "fill_count",
      "base_filled",
      "gross_usd",
      "fee_usd",
    ],
    ["approval_expires_at", "terminal_status", "accounting"],
  );
  if (
    typeof o.product_id !== "string" ||
    !/^[A-Z][A-Z0-9]{0,15}-USD$/.test(o.product_id) ||
    o.product_id === "USD-USD" ||
    typeof o.fill_count !== "number" ||
    !Number.isSafeInteger(o.fill_count) ||
    o.fill_count < 0 ||
    o.fill_count > 1000
  )
    return invalid();
  const side = choice(o.side, ["BUY", "SELL"] as const);
  const result: OwnerOrder = {
    id: id(o.id),
    request_digest: hash(o.request_digest),
    product_id: o.product_id,
    side,
    base_size: amount(o.base_size, true),
    limit_price: amount(o.limit_price, true),
    fee_allowance_usd: amount(o.fee_allowance_usd),
    maximum_debit_usd: amount(o.maximum_debit_usd, side === "BUY"),
    created_at: timestamp(o.created_at),
    state: choice(o.state, states) as OwnerOrder["state"],
    summary: text(o.summary),
    approval_status: choice(o.approval_status, [
      "NONE",
      "RECORDED",
      "REVOKED",
      "EXPIRED",
    ] as const),
    cancellation_status: choice(o.cancellation_status, [
      "NONE",
      "UNKNOWN",
      "ACCEPTED",
      "NOT_ACCEPTED",
    ] as const),
    account_held: flag(o.account_held),
    capital_held: flag(o.capital_held),
    account_blocked: flag(o.account_blocked),
    fill_count: o.fill_count,
    base_filled: amount(o.base_filled, false, true),
    gross_usd: amount(o.gross_usd, false, true),
    fee_usd: amount(o.fee_usd, false, true),
  };
  if (Object.hasOwn(o, "approval_expires_at"))
    result.approval_expires_at = timestamp(o.approval_expires_at);
  if (Object.hasOwn(o, "terminal_status"))
    result.terminal_status = choice(o.terminal_status, [
      "FILLED",
      "CANCELLED",
      "REJECTED",
      "EXPIRED",
    ] as const);
  if (Object.hasOwn(o, "accounting")) {
    const a = executionObject(o.accounting, [
      "opening_cash_usd",
      "opening_base",
      "closing_cash_usd",
      "closing_base",
      "recorded_at",
    ]);
    result.accounting = {
      opening_cash_usd: amount(a.opening_cash_usd),
      opening_base: amount(a.opening_base),
      closing_cash_usd: amount(a.closing_cash_usd),
      closing_base: amount(a.closing_base),
      recorded_at: timestamp(a.recorded_at),
    };
  }
  if (!coherentOwnerState(result)) return invalid();
  return result;
}

// Check only the documented projection shape. These checks cannot prove broker
// completion or balance arithmetic and never grant permission to act.
function coherentOwnerState(o: OwnerOrder): boolean {
  const final =
    o.state === "SETTLED" || o.state === "AWAITING_ACCOUNT_SETTLEMENT";
  if (final !== (o.terminal_status !== undefined)) return false;
  if ((o.state === "SETTLED") !== (o.accounting !== undefined)) return false;
  // An unavailable snapshot remains non-actionable; do not repair its facts or
  // infer a valid lifecycle from missing/inconsistent evidence.
  if (o.state === "UNAVAILABLE") return true;
  if ((o.approval_status === "NONE") !== (o.approval_expires_at === undefined))
    return false;
  const hasFills = o.fill_count > 0;
  const nonzero = (value: string) => /[1-9]/.test(value);
  if (
    hasFills
      ? !nonzero(o.base_filled) || !nonzero(o.gross_usd)
      : nonzero(o.base_filled) || nonzero(o.gross_usd) || nonzero(o.fee_usd)
  )
    return false;
  if (o.terminal_status === "FILLED" && !hasFills) return false;
  if (o.terminal_status === "REJECTED" && hasFills) return false;
  const noHolds = !o.account_held && !o.capital_held;
  const bothHolds = o.account_held && o.capital_held;
  const noCancellation = o.cancellation_status === "NONE";
  switch (o.state) {
    case "PREPARED":
      return (
        noHolds &&
        !hasFills &&
        noCancellation &&
        o.approval_status !== "RECORDED"
      );
    case "APPROVED":
      return (
        noHolds &&
        !hasFills &&
        noCancellation &&
        o.approval_status === "RECORDED"
      );
    default:
      // Every post-claim state in the server projection retains an approval
      // record, even when that approval is now expired or revoked.
      if (o.approval_status === "NONE") return false;
  }
  switch (o.state) {
    case "SETTLED":
      return noHolds;
    case "AWAITING_ACCOUNT_SETTLEMENT":
      return !o.account_held && o.capital_held;
    case "NOT_SENT":
      return noHolds && !hasFills && noCancellation;
    case "SUBMISSION_UNKNOWN":
    case "REJECTED_HELD":
      return bothHolds && !hasFills && noCancellation;
    case "BROKER_ACKNOWLEDGED":
      return bothHolds && !hasFills;
    case "PARTIALLY_FILLED":
      return bothHolds && hasFills;
  }
}
