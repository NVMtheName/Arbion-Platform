import {
  executionObject,
  isExecutionID,
  parseApproveCommand,
  parseOwnerOrder,
  parsePrepareCommand,
  parseSendCommand,
  sameExecutionAmount,
  type ApproveCommand,
  type OwnerOrder,
  type OwnerPreflight,
  type PrepareCommand,
  type SendCommand,
} from "./contract";

export type {
  OwnerOrder,
  OwnerPreflight,
  PrepareCommand,
  ApproveCommand,
  SendCommand,
} from "./contract";
export interface OwnerExecutionClient {
  prepare(command: PrepareCommand): Promise<OwnerOrder>;
  get(id: string): Promise<OwnerOrder>;
  approve(id: string, command: ApproveCommand): Promise<OwnerOrder>;
  preflight(id: string): Promise<OwnerPreflight>;
  send(id: string, command: SendCommand): Promise<OwnerOrder>;
  revoke(id: string): Promise<OwnerOrder>;
  recover(id: string): Promise<OwnerOrder>;
  reconcile(id: string): Promise<OwnerOrder>;
  cancel(id: string): Promise<OwnerOrder>;
  settle(id: string): Promise<OwnerOrder>;
}

const messages = {
  INVALID_COMMAND: "The execution command is invalid. Nothing was requested.",
  INVALID_RESPONSE:
    "The response could not be verified. Read saved status before further action; do not repeat a send or cancellation.",
  EXECUTION_OUTCOME_UNKNOWN:
    "The outcome is unknown. Read saved status and recover or reconcile; do not retry the send or cancellation.",
  EXECUTION_NOT_SENT:
    "The sender was not entered and the original attempt is closed. Do not resend this order.",
  EXECUTION_ALREADY_ATTEMPTED:
    "An attempt already exists. Recover or reconcile; do not resend.",
  EXECUTION_REJECTED:
    "Submission rejection was recorded. Capital is not settled; do not resend.",
  EXECUTION_CANCELLATION_NOT_ACCEPTED:
    "Cancellation was not accepted. Reconcile the original order; do not retry cancellation.",
  EXECUTION_NOT_FOUND: "The order is unavailable.",
  EXECUTION_NOT_AUTHORIZED: "Current owner authorization is unavailable.",
  EXECUTION_RATE_LIMITED: "Too many verification attempts.",
  EXECUTION_REVIEW_REQUIRED:
    "The order requires review or reconciliation before further action.",
  OWNER_EXECUTION_UNAVAILABLE:
    "Execution is unavailable. Read saved status before further action; do not repeat a send or cancellation.",
  unauthenticated: "Sign in again to read saved order status.",
  csrf_rejected:
    "The request origin was rejected. Nothing is authorized by this response.",
  invalid_request: "The execution request was rejected as invalid.",
} as const;
type ErrorCode = keyof typeof messages;
export class ExecutionClientError extends Error {
  readonly code: ErrorCode;
  constructor(code: string) {
    const safe: ErrorCode = Object.hasOwn(messages, code)
      ? (code as ErrorCode)
      : "EXECUTION_OUTCOME_UNKNOWN";
    super(messages[safe]);
    this.name = "ExecutionClientError";
    this.code = safe;
  }
}

const base = "/api/personal-execution/orders";
const limit = 32 * 1024;

// Bound the entire response, including ignored/error bodies. No raw body or
// server message is returned to the screen, logs, storage or an Error cause.
async function responseJSON(response: Response): Promise<unknown> {
  if (!response.body) throw new ExecutionClientError("INVALID_RESPONSE");
  const reader = response.body.getReader();
  const decoder = new TextDecoder("utf-8", { fatal: true });
  let length = 0,
    raw = "";
  try {
    for (;;) {
      const { value, done } = await reader.read();
      if (done) break;
      length += value.byteLength;
      if (length > limit) throw new ExecutionClientError("INVALID_RESPONSE");
      raw += decoder.decode(value, { stream: true });
    }
    raw += decoder.decode();
    const parsed: unknown = JSON.parse(raw);
    // JSON.parse discards duplicate members. Check decoded names as well, so
    // escaped duplicates cannot turn a contradictory response into a valid DTO.
    const stack: { object: boolean; key: boolean; seen: Set<string> }[] = [];
    const tokens =
      raw.match(/"(?:\\.|[^"\\])*"|[{}[\],:]|[^{}[\],:\s]+/g) ?? [];
    if (tokens.length > 512) throw new ExecutionClientError("INVALID_RESPONSE");
    for (const token of tokens) {
      if (token === "{" || token === "[") {
        stack.push({ object: token === "{", key: true, seen: new Set() });
        if (stack.length > 6)
          throw new ExecutionClientError("INVALID_RESPONSE");
      } else if (token === "}" || token === "]") stack.pop();
      else {
        const top = stack.at(-1);
        if (!top?.object) continue;
        if (token === ",") top.key = true;
        else if (top.key && token.startsWith('"')) {
          const key: string = JSON.parse(token);
          if (top.seen.has(key))
            throw new ExecutionClientError("INVALID_RESPONSE");
          top.seen.add(key);
          top.key = false;
        }
      }
    }
    return parsed;
  } finally {
    await reader.cancel().catch(() => undefined);
    reader.releaseLock();
  }
}

// There is deliberately no base-URL option, injected authorizer, retry loop,
// polling, preview refresh or persistent browser credential/evidence storage.
export function createOwnerExecutionClient(): OwnerExecutionClient {
  const bindings = new Map<string, string>();
  function bind(order: OwnerOrder, expected?: string): OwnerOrder {
    if (expected && order.id !== expected)
      throw new ExecutionClientError("INVALID_RESPONSE");
    const binding = JSON.stringify([
      order.request_digest,
      order.product_id,
      order.side,
      order.base_size,
      order.limit_price,
      order.fee_allowance_usd,
      order.maximum_debit_usd,
      order.created_at,
    ]);
    const previous = bindings.get(order.id);
    if (
      (previous && previous !== binding) ||
      (!previous && bindings.size >= 64)
    )
      throw new ExecutionClientError("INVALID_RESPONSE");
    bindings.set(order.id, binding);
    return order;
  }
  function command<T>(parse: (value: unknown) => T, value: unknown): T {
    try {
      return parse(value);
    } catch {
      throw new ExecutionClientError("INVALID_COMMAND");
    }
  }
  async function request(
    action: string,
    id?: string,
    body?: object,
  ): Promise<unknown> {
    if (id !== undefined && !isExecutionID(id))
      throw new ExecutionClientError("INVALID_COMMAND");
    const path =
      id === undefined ? base : `${base}/${id}${action ? `/${action}` : ""}`;
    const abort = new AbortController();
    const timer = setTimeout(() => abort.abort(), 45_000);
    try {
      const response = await fetch(path, {
        method: body ? "POST" : "GET",
        credentials: "same-origin",
        cache: "no-store",
        redirect: "error",
        signal: abort.signal,
        headers: {
          Accept: "application/json",
          ...(body ? { "Content-Type": "application/json" } : {}),
        },
        ...(body ? { body: JSON.stringify(body) } : {}),
      });
      if (response.redirected || response.type === "opaqueredirect")
        throw new ExecutionClientError("INVALID_RESPONSE");
      const raw = await responseJSON(response);
      if (!response.ok) {
        const envelope = executionObject(raw, ["error"]);
        const error = executionObject(envelope.error, ["code", "message"]);
        if (
          typeof error.code !== "string" ||
          typeof error.message !== "string" ||
          error.message.length > 1024
        )
          throw new ExecutionClientError("INVALID_RESPONSE");
        throw new ExecutionClientError(error.code);
      }
      if (response.status !== 200)
        throw new ExecutionClientError("INVALID_RESPONSE");
      return raw;
    } catch (error) {
      if (error instanceof ExecutionClientError) throw error;
      throw new ExecutionClientError("EXECUTION_OUTCOME_UNKNOWN");
    } finally {
      clearTimeout(timer);
    }
  }
  async function order(
    action: string,
    id?: string,
    body?: object,
  ): Promise<OwnerOrder> {
    const raw = await request(action, id, body);
    try {
      return bind(parseOwnerOrder(executionObject(raw, ["order"]).order), id);
    } catch {
      throw new ExecutionClientError("INVALID_RESPONSE");
    }
  }
  return {
    async prepare(input) {
      const c = command(parsePrepareCommand, input);
      const o = await order("", undefined, c);
      if (
        o.side !== c.side ||
        !sameExecutionAmount(o.base_size, c.base_size) ||
        !sameExecutionAmount(o.limit_price, c.limit_price) ||
        !sameExecutionAmount(o.fee_allowance_usd, c.fee_allowance_usd) ||
        !sameExecutionAmount(o.maximum_debit_usd, c.maximum_debit_usd)
      )
        throw new ExecutionClientError("INVALID_RESPONSE");
      return o;
    },
    get: (id) => order("", id),
    async approve(id, input) {
      const c = command(parseApproveCommand, input);
      const o = await order("approve", id, c);
      if (o.request_digest !== c.expected_digest)
        throw new ExecutionClientError("INVALID_RESPONSE");
      return o;
    },
    async preflight(id) {
      const raw = await request("preflight", id, {});
      try {
        const envelope = executionObject(raw, ["order", "evidence_id"]);
        if (!isExecutionID(envelope.evidence_id))
          throw new ExecutionClientError("INVALID_RESPONSE");
        return {
          order: bind(parseOwnerOrder(envelope.order), id),
          evidence_id: envelope.evidence_id,
        };
      } catch {
        throw new ExecutionClientError("INVALID_RESPONSE");
      }
    },
    send: async (id, input) =>
      order("send", id, command(parseSendCommand, input)),
    revoke: (id) => order("revoke", id, {}),
    recover: (id) => order("recover", id, {}),
    reconcile: (id) => order("reconcile", id, {}),
    cancel: (id) => order("cancel", id, {}),
    settle: (id) => order("settle", id, {}),
  };
}
