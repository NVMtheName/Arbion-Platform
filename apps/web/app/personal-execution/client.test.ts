import { afterEach, describe, expect, it, vi } from "vitest";
import {
  createOwnerExecutionClient,
  ExecutionClientError,
  type OwnerOrder,
  type PrepareCommand,
} from "./client";

const id = "11111111-1111-4111-8111-111111111111";
const evidence = "22222222-2222-4222-8222-222222222222";
const base = "/api/personal-execution/orders";
const fixture = (): OwnerOrder => ({
  id,
  request_digest: "a".repeat(64),
  product_id: "BTC-USD",
  side: "BUY",
  base_size: "0.001",
  limit_price: "30000",
  fee_allowance_usd: "0.10",
  maximum_debit_usd: "30.10",
  created_at: "2026-09-30T12:00:00Z",
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
const prepare: PrepareCommand = {
  request_key: evidence,
  side: "BUY",
  base_size: "0.0010",
  limit_price: "30000.00",
  fee_allowance_usd: "0.10",
  maximum_debit_usd: "30.10",
};
const response = (value: unknown, status = 200) =>
  new Response(JSON.stringify(value), {
    status,
    headers: { "Content-Type": "application/json" },
  });
afterEach(() => {
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

describe("fixed owner execution client", () => {
  it("sends every exact command once to the fixed same-origin path", async () => {
    const fetch = vi.fn(async (path: string) =>
      response(
        path.endsWith("/preflight")
          ? { order: fixture(), evidence_id: evidence }
          : { order: fixture() },
      ),
    );
    vi.stubGlobal("fetch", fetch);
    const client = createOwnerExecutionClient();
    const approval = {
      expected_digest: fixture().request_digest,
      mfa_code: "123456",
    };
    await client.prepare(prepare);
    await client.get(id);
    await client.approve(id, approval);
    await client.preflight(id);
    await client.send(id, { evidence_id: evidence });
    for (const action of [
      "revoke",
      "recover",
      "reconcile",
      "cancel",
      "settle",
    ] as const)
      await client[action](id);
    const expected = [
      [base, prepare],
      [`${base}/${id}`, undefined],
      [`${base}/${id}/approve`, approval],
      [`${base}/${id}/preflight`, {}],
      [`${base}/${id}/send`, { evidence_id: evidence }],
      ...["revoke", "recover", "reconcile", "cancel", "settle"].map(
        (action) => [`${base}/${id}/${action}`, {}],
      ),
    ];
    expect(fetch).toHaveBeenCalledTimes(expected.length);
    expected.forEach(([path, body], i) => {
      const [url, options] = fetch.mock.calls[i] as unknown as [
        string,
        RequestInit,
      ];
      expect(url).toBe(path);
      expect(options).toEqual({
        method: body ? "POST" : "GET",
        credentials: "same-origin",
        cache: "no-store",
        redirect: "error",
        signal: expect.any(AbortSignal),
        headers: {
          Accept: "application/json",
          ...(body ? { "Content-Type": "application/json" } : {}),
        },
        ...(body ? { body: JSON.stringify(body) } : {}),
      });
      expect(JSON.stringify(options)).not.toContain("Authorization");
    });
    expect(
      JSON.stringify(
        fetch.mock.calls.filter(([path]) => !path.endsWith("/approve")),
      ),
    ).not.toContain("123456");
  });

  it("rejects widened commands and path injection before any request", async () => {
    const fetch = vi.fn();
    vi.stubGlobal("fetch", fetch);
    const client = createOwnerExecutionClient();
    await expect(
      client.prepare({ ...prepare, owner_id: "other" } as PrepareCommand),
    ).rejects.toMatchObject({ code: "INVALID_COMMAND" });
    await expect(client.get("../cancel")).rejects.toMatchObject({
      code: "INVALID_COMMAND",
    });
    await expect(
      client.get("https://evil.example/" + id),
    ).rejects.toMatchObject({ code: "INVALID_COMMAND" });
    await expect(
      client.approve(id, { expected_digest: "bad", mfa_code: "123456" }),
    ).rejects.toMatchObject({ code: "INVALID_COMMAND" });
    await expect(
      client.send(id, { evidence_id: "not-a-uuid" }),
    ).rejects.toMatchObject({ code: "INVALID_COMMAND" });
    expect(fetch).not.toHaveBeenCalled();
  });

  it.each(["get", "send", "cancel", "recover", "preflight"] as const)(
    "never retries a lost %s response or exposes network errors",
    async (action) => {
      const fetch = vi
        .fn()
        .mockRejectedValue(new Error("private API key and provider body"));
      vi.stubGlobal("fetch", fetch);
      const client = createOwnerExecutionClient();
      const promise =
        action === "send"
          ? client.send(id, { evidence_id: evidence })
          : client[action](id);
      await expect(promise).rejects.toMatchObject({
        code: "EXECUTION_OUTCOME_UNKNOWN",
      });
      await expect(promise).rejects.toThrow("do not retry");
      expect(fetch).toHaveBeenCalledTimes(1);
    },
  );

  it("bounds a request timeout without starting another operation", async () => {
    vi.useFakeTimers();
    const fetch = vi.fn(
      (_url: string, init: RequestInit) =>
        new Promise<Response>((_resolve, reject) =>
          init.signal?.addEventListener("abort", () =>
            reject(new Error("secret abort reason")),
          ),
        ),
    );
    vi.stubGlobal("fetch", fetch);
    const pending = createOwnerExecutionClient().send(id, {
      evidence_id: evidence,
    });
    const assertion = expect(pending).rejects.toMatchObject({
      code: "EXECUTION_OUTCOME_UNKNOWN",
    });
    await vi.advanceTimersByTimeAsync(45_000);
    await assertion;
    expect(fetch).toHaveBeenCalledTimes(1);
  });

  it("uses only local allowlisted error messages", async () => {
    for (const code of [
      "EXECUTION_NOT_SENT",
      "EXECUTION_OUTCOME_UNKNOWN",
      "EXECUTION_NOT_AUTHORIZED",
      "unknown_secret",
      "__proto__",
    ]) {
      const fetch = vi
        .fn()
        .mockResolvedValue(
          response(
            { error: { code, message: "PRIVATE PROVIDER SECRET" } },
            409,
          ),
        );
      vi.stubGlobal("fetch", fetch);
      const error = await createOwnerExecutionClient()
        .send(id, { evidence_id: evidence })
        .catch((e: unknown) => e);
      expect(error).toBeInstanceOf(ExecutionClientError);
      expect(String(error)).not.toMatch(
        /PRIVATE|SECRET|unknown_secret|__proto__/,
      );
      expect((error as ExecutionClientError).code).toBe(
        code.startsWith("EXECUTION_") ? code : "EXECUTION_OUTCOME_UNKNOWN",
      );
      expect(fetch).toHaveBeenCalledTimes(1);
    }
  });

  it("fails closed on wrong IDs, private fields, duplicate JSON and malformed envelopes", async () => {
    const valid = JSON.stringify({ order: fixture() });
    for (const raw of [
      JSON.stringify({ order: { ...fixture(), id: evidence } }),
      JSON.stringify({ order: { ...fixture(), provider_payload: "secret" } }),
      JSON.stringify({ order: fixture(), evidence_id: evidence }),
      valid.replace(
        '"state":"PREPARED"',
        '"state":"SETTLED","state":"PREPARED"',
      ),
      valid.replace(
        '"state":"PREPARED"',
        '"state":"SETTLED","\\u0073tate":"PREPARED"',
      ),
      valid + "{}",
      "null",
      "[]",
      "{",
      " ".repeat(32 * 1024) + valid,
    ]) {
      const fetch = vi
        .fn()
        .mockResolvedValue(new Response(raw, { status: 200 }));
      vi.stubGlobal("fetch", fetch);
      await expect(createOwnerExecutionClient().get(id)).rejects.toBeInstanceOf(
        ExecutionClientError,
      );
      expect(fetch).toHaveBeenCalledTimes(1);
    }
  });

  it("binds immutable terms and approval digest across saved reads", async () => {
    for (const changed of [
      { request_digest: "b".repeat(64) },
      { limit_price: "31000" },
      { product_id: "ETH-USD" },
      { created_at: "2026-09-29T12:00:00Z" },
    ]) {
      const fetch = vi
        .fn()
        .mockResolvedValueOnce(response({ order: fixture() }))
        .mockResolvedValueOnce(
          response({ order: { ...fixture(), ...changed } }),
        );
      vi.stubGlobal("fetch", fetch);
      const client = createOwnerExecutionClient();
      await client.get(id);
      await expect(client.recover(id)).rejects.toMatchObject({
        code: "INVALID_RESPONSE",
      });
      expect(fetch).toHaveBeenCalledTimes(2);
    }
    vi.stubGlobal(
      "fetch",
      vi.fn().mockImplementation(async () => response({ order: fixture() })),
    );
    await expect(
      createOwnerExecutionClient().approve(id, {
        expected_digest: "b".repeat(64),
        mfa_code: "123456",
      }),
    ).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    await expect(
      createOwnerExecutionClient().prepare({ ...prepare, base_size: "0.002" }),
    ).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
  });

  it("requires an exact preflight envelope and never follows a redirect", async () => {
    for (const envelope of [
      { order: fixture() },
      { order: fixture(), evidence_id: "bad" },
      { order: { ...fixture(), id: evidence }, evidence_id: evidence },
    ]) {
      vi.stubGlobal("fetch", vi.fn().mockResolvedValue(response(envelope)));
      await expect(
        createOwnerExecutionClient().preflight(id),
      ).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    }
    const redirected = response({ order: fixture() });
    Object.defineProperty(redirected, "redirected", { value: true });
    const fetch = vi.fn().mockResolvedValue(redirected);
    vi.stubGlobal("fetch", fetch);
    await expect(createOwnerExecutionClient().cancel(id)).rejects.toMatchObject(
      { code: "INVALID_RESPONSE" },
    );
    expect(fetch).toHaveBeenCalledTimes(1);
  });
});
