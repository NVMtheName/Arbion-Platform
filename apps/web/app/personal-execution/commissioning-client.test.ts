import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createCommissioningClient } from "./commissioning-client";
import {
  review,
  receipt,
  consent,
  response,
  missing,
} from "./commissioning.test-fixtures";
beforeEach(() => vi.stubGlobal("fetch", vi.fn()));
afterEach(() => vi.unstubAllGlobals());
const binding = "c".repeat(64);

describe("fixed commissioning transport", () => {
  it("performs only explicit exact setup/read/consent/revoke requests", async () => {
    const fetchMock = vi
      .mocked(fetch)
      .mockResolvedValueOnce(response({ receipt }))
      .mockResolvedValueOnce(response({ receipt }))
      .mockResolvedValueOnce(response({ consent }))
      .mockResolvedValueOnce(response({ consent }))
      .mockResolvedValueOnce(
        response({
          consent: { ...consent, revoked_at: "2099-10-07T12:02:00Z" },
        }),
      );
    const client = createCommissioningClient(binding, review);
    expect(fetch).not.toHaveBeenCalled();
    await client.prepare();
    await client.readReceipt();
    await client.approve("123456");
    await client.readConsent();
    await client.revoke();
    const calls = fetchMock.mock.calls;
    expect(calls.map(([url]) => url)).toEqual(
      ["prepare", "receipt", "approve", "consent", "revoke"].map(
        (path) => `/api/personal-execution/commissioning/${path}`,
      ),
    );
    expect(calls.map(([, init]) => init?.method)).toEqual([
      "POST",
      "GET",
      "POST",
      "GET",
      "POST",
    ]);
    expect(JSON.parse(calls[0][1]!.body as string)).toEqual({
      expected_terms_digest: review.terms_digest,
    });
    expect(JSON.parse(calls[2][1]!.body as string)).toEqual({
      expected_terms_digest: review.terms_digest,
      expected_snapshot_digest: receipt.snapshot_digest,
      mfa_code: "123456",
    });
    expect(calls[4][1]?.body).toBe("{}");
    for (const [, init] of calls)
      expect(init).toMatchObject({
        credentials: "same-origin",
        cache: "no-store",
        redirect: "error",
        headers: { "X-Arbion-Execution-Session": binding },
      });
  });
  it("never automatically retries failed commands and hides raw error bodies", async () => {
    vi.mocked(fetch).mockRejectedValueOnce(new Error("secret broker response"));
    const client = createCommissioningClient(binding, review);
    await expect(client.prepare()).rejects.toMatchObject({
      code: "EXECUTION_OUTCOME_UNKNOWN",
    });
    expect(fetch).toHaveBeenCalledTimes(1);
    vi.mocked(fetch).mockResolvedValueOnce(missing());
    await expect(client.readReceipt()).rejects.toMatchObject({
      code: "EXECUTION_NOT_FOUND",
    });
    expect(fetch).toHaveBeenCalledTimes(2);
  });
  it("recovers a lost consent response using only fixed reads, including a new client", async () => {
    vi.mocked(fetch)
      .mockResolvedValueOnce(response({ receipt }))
      .mockRejectedValueOnce(new Error("lost"));
    const first = createCommissioningClient(binding, review);
    await first.prepare();
    await expect(first.approve("123456")).rejects.toMatchObject({
      code: "EXECUTION_OUTCOME_UNKNOWN",
    });
    first.invalidate();
    vi.mocked(fetch)
      .mockResolvedValueOnce(response({ receipt }))
      .mockResolvedValueOnce(response({ consent }));
    const restored = createCommissioningClient(binding, review);
    await restored.readReceipt();
    expect(await restored.readConsent()).toEqual(consent);
    expect(
      vi.mocked(fetch).mock.calls.filter(([, init]) => init?.method === "POST"),
    ).toHaveLength(2);
  });
  it("rejects conflicting terms, changed receipt identity and revoked-to-unrevoked regression", async () => {
    const client = createCommissioningClient(binding, review);
    vi.mocked(fetch).mockResolvedValueOnce(
      response({ receipt: { ...receipt, allocation_usd: "999" } }),
    );
    await expect(client.prepare()).rejects.toMatchObject({
      code: "INVALID_RESPONSE",
    });
    vi.mocked(fetch)
      .mockResolvedValueOnce(response({ receipt }))
      .mockResolvedValueOnce(
        response({
          consent: { ...consent, revoked_at: "2099-10-07T12:02:00Z" },
        }),
      );
    await client.readReceipt();
    await client.readConsent();
    vi.mocked(fetch).mockResolvedValueOnce(response({ consent }));
    await expect(client.readConsent()).rejects.toMatchObject({
      code: "INVALID_RESPONSE",
    });
    vi.mocked(fetch).mockResolvedValueOnce(
      response({
        consent: { ...consent, id: "22222222-2222-4222-8222-222222222222" },
      }),
    );
    await expect(client.readConsent()).rejects.toMatchObject({
      code: "INVALID_RESPONSE",
    });
  });
  it.each([401, 403])("invalidates permanently on HTTP %s", async (status) => {
    const invalidated = vi.fn(),
      client = createCommissioningClient(binding, review, invalidated);
    vi.mocked(fetch).mockResolvedValueOnce(
      response({ private: "secret" }, status),
    );
    await expect(client.prepare()).rejects.toMatchObject({
      code: "execution_session_changed",
    });
    await expect(client.readReceipt()).rejects.toMatchObject({
      code: "execution_session_changed",
    });
    expect(invalidated).toHaveBeenCalledOnce();
    expect(fetch).toHaveBeenCalledOnce();
  });
  it("rejects late responses after local invalidation", async () => {
    let resolve!: (value: Response) => void;
    vi.mocked(fetch).mockImplementation(
      () =>
        new Promise((done) => {
          resolve = done;
        }),
    );
    const client = createCommissioningClient(binding, review),
      pending = client.prepare();
    client.invalidate();
    resolve(response({ receipt }));
    await expect(pending).rejects.toMatchObject({
      code: "execution_session_changed",
    });
  });
  it("rejects duplicate fields and authority-bearing response envelopes", async () => {
    const client = createCommissioningClient(binding, review);
    vi.mocked(fetch).mockResolvedValueOnce(
      new Response(
        `{"receipt":${JSON.stringify(receipt)},"receipt":${JSON.stringify(receipt)}}`,
      ),
    );
    await expect(client.readReceipt()).rejects.toMatchObject({
      code: "EXECUTION_OUTCOME_UNKNOWN",
    });
    vi.mocked(fetch).mockResolvedValueOnce(
      response({ receipt, account_id: "private" }),
    );
    await expect(client.readReceipt()).rejects.toMatchObject({
      code: "INVALID_RESPONSE",
    });
  });
  it("requires saved terms before consent and never sends a malformed MFA code", async () => {
    const client = createCommissioningClient(binding, review);
    await expect(client.approve("123456")).rejects.toMatchObject({
      code: "INVALID_COMMAND",
    });
    expect(fetch).not.toHaveBeenCalled();
    vi.mocked(fetch).mockResolvedValueOnce(response({ receipt }));
    await client.readReceipt();
    await expect(client.approve("1234567")).rejects.toMatchObject({
      code: "INVALID_COMMAND",
    });
    expect(fetch).toHaveBeenCalledOnce();
  });
});
