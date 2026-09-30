import { describe, expect, it } from "vitest";
import {
  parseOwnerOrder,
  parsePrepareCommand,
  type OwnerOrder,
} from "./contract";

const fixture = (): OwnerOrder => ({
  id: "11111111-1111-4111-8111-111111111111",
  request_digest: "a".repeat(64),
  product_id: "BTC-USD",
  side: "BUY",
  base_size: "0.001",
  limit_price: "30000",
  fee_allowance_usd: "0.10",
  maximum_debit_usd: "30.10",
  created_at: "2026-09-30T12:00:00.123456789Z",
  state: "PREPARED",
  summary: "Order prepared. It has not been submitted.",
  approval_status: "NONE",
  cancellation_status: "NONE",
  account_held: false,
  capital_held: false,
  account_blocked: false,
  fill_count: 0,
  base_filled: "0",
  gross_usd: "0",
  fee_usd: "0",
});

describe("owner execution safe contract", () => {
  it("copies exact safe fields and accepts documented optional accounting", () => {
    const source = fixture();
    expect(parseOwnerOrder(source)).toEqual(source);
    expect(parseOwnerOrder(source)).not.toBe(source);
    const settled = {
      ...source,
      state: "SETTLED",
      terminal_status: "FILLED",
      approval_status: "EXPIRED",
      approval_expires_at: "2026-09-30T12:01:00Z",
      fill_count: 1,
      base_filled: "0.001",
      gross_usd: "30",
      fee_usd: "0.10",
      accounting: {
        opening_cash_usd: "1000.10",
        opening_base: "1",
        closing_cash_usd: "970",
        closing_base: "1.001",
        recorded_at: "2026-09-30T12:02:00Z",
      },
    };
    expect(parseOwnerOrder(settled)).toEqual(settled);
  });

  it("requires state-specific terminal, accounting, hold and fill evidence", () => {
    const approved: OwnerOrder = {
      ...fixture(),
      state: "APPROVED",
      approval_status: "RECORDED",
      approval_expires_at: "2026-09-30T12:01:00Z",
    };
    const unknown: OwnerOrder = {
      ...approved,
      state: "SUBMISSION_UNKNOWN",
      account_held: true,
      capital_held: true,
    };
    const ack: OwnerOrder = {
      ...unknown,
      state: "BROKER_ACKNOWLEDGED",
      cancellation_status: "ACCEPTED",
    };
    const partial: OwnerOrder = {
      ...ack,
      state: "PARTIALLY_FILLED",
      fill_count: 1,
      base_filled: "0.0005",
      gross_usd: "15",
      fee_usd: "0.05",
    };
    const terminal: OwnerOrder = {
      ...partial,
      state: "AWAITING_ACCOUNT_SETTLEMENT",
      account_held: false,
      terminal_status: "CANCELLED",
    };
    const settled: OwnerOrder = {
      ...terminal,
      state: "SETTLED",
      capital_held: false,
      accounting: {
        opening_cash_usd: "1000.10",
        opening_base: "1",
        closing_cash_usd: "985.05",
        closing_base: "1.0005",
        recorded_at: "2026-09-30T12:02:00Z",
      },
    };
    const notSent: OwnerOrder = { ...approved, state: "NOT_SENT" };
    const rejected: OwnerOrder = { ...unknown, state: "REJECTED_HELD" };
    const unavailable: OwnerOrder = { ...partial, state: "UNAVAILABLE" };
    for (const value of [
      approved,
      unknown,
      ack,
      partial,
      terminal,
      settled,
      notSent,
      rejected,
      unavailable,
      {
        ...fixture(),
        approval_status: "REVOKED",
        approval_expires_at: approved.approval_expires_at,
      },
    ]) {
      expect(parseOwnerOrder(value)).toEqual(value);
    }
    for (const value of [
      { ...fixture(), state: "SETTLED" },
      { ...settled, accounting: undefined },
      { ...settled, terminal_status: undefined },
      { ...settled, capital_held: true },
      { ...settled, account_held: true },
      { ...terminal, capital_held: false },
      { ...terminal, account_held: true },
      { ...terminal, terminal_status: undefined },
      { ...terminal, accounting: settled.accounting },
      { ...fixture(), account_held: true },
      { ...fixture(), capital_held: true },
      { ...fixture(), cancellation_status: "UNKNOWN" },
      {
        ...fixture(),
        approval_status: "RECORDED",
        approval_expires_at: approved.approval_expires_at,
      },
      { ...approved, approval_status: "EXPIRED" },
      { ...approved, approval_expires_at: undefined },
      { ...approved, cancellation_status: "ACCEPTED" },
      { ...approved, capital_held: true },
      { ...unknown, account_held: false },
      { ...unknown, capital_held: false },
      { ...unknown, cancellation_status: "ACCEPTED" },
      { ...unknown, approval_status: "NONE", approval_expires_at: undefined },
      { ...rejected, cancellation_status: "UNKNOWN" },
      { ...ack, capital_held: false },
      { ...partial, fill_count: 0 },
      { ...partial, base_filled: "0" },
      { ...partial, gross_usd: "0" },
      { ...partial, account_held: false },
      { ...notSent, account_held: true },
      { ...notSent, capital_held: true },
      { ...notSent, cancellation_status: "ACCEPTED" },
      { ...unknown, terminal_status: "CANCELLED" },
      { ...partial, accounting: settled.accounting },
      { ...unavailable, accounting: settled.accounting },
      { ...fixture(), base_filled: "0.0001" },
      { ...fixture(), gross_usd: "1" },
      { ...fixture(), fee_usd: "0.01" },
      { ...terminal, terminal_status: "REJECTED" },
      {
        ...unknown,
        state: "AWAITING_ACCOUNT_SETTLEMENT",
        terminal_status: "FILLED",
        account_held: false,
      },
    ]) {
      expect(() => parseOwnerOrder(value)).toThrow(
        "Invalid execution contract.",
      );
    }
    for (const state of [
      "PREPARED",
      "APPROVED",
      "SUBMISSION_UNKNOWN",
      "REJECTED_HELD",
      "BROKER_ACKNOWLEDGED",
      "NOT_SENT",
    ] as const) {
      expect(() => parseOwnerOrder({ ...partial, state })).toThrow();
    }
  });

  it.each([
    ["provider_payload", "secret"],
    ["client_order_id", "secret"],
    ["credential_generation", 1],
    ["account_id", "secret"],
    ["id", "../cancel"],
    ["id", "00000000-0000-0000-0000-000000000000"],
    ["request_digest", "z".repeat(64)],
    ["product_id", "BTC-USDC"],
    ["product_id", "USD-USD"],
    ["side", "buy"],
    ["base_size", "0"],
    ["limit_price", 30000],
    ["fee_allowance_usd", "-1"],
    ["maximum_debit_usd", "1e3"],
    ["base_filled", "01"],
    ["gross_usd", "1/2"],
    ["fee_usd", "0."],
    ["state", "EXECUTABLE"],
    ["approval_status", "LIVE_AUTHORIZED"],
    ["cancellation_status", "FILLED"],
    ["terminal_status", "OPEN"],
    ["account_held", "false"],
    ["capital_held", 0],
    ["account_blocked", null],
    ["fill_count", -1],
    ["fill_count", 0.5],
    ["fill_count", 1001],
    ["created_at", "2026-02-30T00:00:00Z"],
    ["created_at", "2026-01-01T24:00:00Z"],
    ["created_at", "2026-01-01"],
    ["created_at", "0000-01-01T00:00:00Z"],
    ["approval_expires_at", null],
    ["summary", "x".repeat(513)],
    ["summary", "private\nmessage"],
    ["accounting", null],
  ])("rejects invalid or private %s", (key, value) => {
    expect(() => parseOwnerOrder({ ...fixture(), [key]: value })).toThrow(
      "Invalid execution contract.",
    );
  });

  it("rejects missing properties, prototypes, getters and nested unknown data", () => {
    for (const key of Object.keys(fixture())) {
      const value = { ...fixture() } as Record<string, unknown>;
      delete value[key];
      expect(() => parseOwnerOrder(value)).toThrow();
    }
    const inherited = Object.assign(
      Object.create({ owner: "other" }),
      fixture(),
    );
    expect(() => parseOwnerOrder(inherited)).toThrow();
    const getter = Object.defineProperty(fixture(), "summary", {
      get: () => {
        throw new Error("getter entered");
      },
      enumerable: true,
    });
    expect(() => parseOwnerOrder(getter)).toThrow(
      "Invalid execution contract.",
    );
    expect(() =>
      parseOwnerOrder({
        ...fixture(),
        accounting: {
          opening_cash_usd: "1",
          opening_base: "1",
          closing_cash_usd: "1",
          closing_base: "1",
          recorded_at: "2026-01-01T00:00:00Z",
          raw: "secret",
        },
      }),
    ).toThrow();
    expect(() =>
      parseOwnerOrder(JSON.parse('{"__proto__":{},"id":"ignored"}')),
    ).toThrow();
  });

  it("does not coerce or accept authority fields in prepare commands", () => {
    const command = {
      request_key: fixture().id,
      side: "SELL",
      base_size: "0.0010",
      limit_price: "30000",
      fee_allowance_usd: "0.10",
      maximum_debit_usd: "0",
    };
    expect(parsePrepareCommand(command)).toEqual(command);
    for (const extra of [
      "owner_id",
      "product_id",
      "portfolio_id",
      "client_order_id",
      "credentials",
      "mode",
      "authorization",
    ]) {
      expect(() =>
        parsePrepareCommand({ ...command, [extra]: "untrusted" }),
      ).toThrow();
    }
    expect(() =>
      parsePrepareCommand({ ...command, base_size: 0.001 }),
    ).toThrow();
  });
});
