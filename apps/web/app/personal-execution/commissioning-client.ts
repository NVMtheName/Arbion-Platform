import { executionResponseJSON } from "./client";
import { executionObject } from "./contract";
import {
  commissioningDigest,
  parseCommissioningContext,
  parseCommissioningConsent,
  parseCommissioningReceipt,
  parseCommissioningReview,
  type CommissioningReview,
  type CommissioningReceipt,
  type CommissioningConsent,
} from "./commissioning-contract";

export interface CommissioningClient {
  prepare(): Promise<CommissioningReceipt>;
  readReceipt(): Promise<CommissioningReceipt>;
  readConsent(): Promise<CommissioningConsent>;
  approve(code: string): Promise<CommissioningConsent>;
  revoke(): Promise<CommissioningConsent>;
}
const messages = {
  EXECUTION_NOT_FOUND:
    "No saved receipt was found. This does not prove an earlier request failed.",
  EXECUTION_NOT_AUTHORIZED: "Current owner authorization is unavailable.",
  EXECUTION_RATE_LIMITED: "Too many verification attempts.",
  EXECUTION_REVIEW_REQUIRED:
    "The saved terms require review before further action.",
  EXECUTION_OUTCOME_UNKNOWN:
    "The outcome is unconfirmed. Read saved status; do not repeat setup, consent or revocation.",
  INVALID_RESPONSE:
    "The response could not be verified. Read saved status before further action.",
  INVALID_COMMAND: "The request is invalid. Nothing was requested.",
  execution_session_changed:
    "This setup session has closed. Reopen securely and read saved status.",
} as const;
export class CommissioningClientError extends Error {
  readonly code: keyof typeof messages;
  constructor(code: string) {
    const safe = Object.hasOwn(messages, code)
      ? (code as keyof typeof messages)
      : "EXECUTION_OUTCOME_UNKNOWN";
    super(messages[safe]);
    this.code = safe;
    this.name = "CommissioningClientError";
  }
}
export async function readCommissioningContext(response: Response) {
  if (response.status !== 200 || response.redirected)
    throw new CommissioningClientError("INVALID_RESPONSE");
  return parseCommissioningContext(await executionResponseJSON(response));
}
const base = "/api/personal-execution/commissioning";

// Fixed same-origin transport. No retries, stored credentials, caller-selected
// authority IDs, mutable terms or configurable upstream URLs.
export function createCommissioningClient(
  sessionBinding: string,
  terms: CommissioningReview,
  onInvalidated: () => void = () => undefined,
): CommissioningClient & { invalidate(): void } {
  commissioningDigest(sessionBinding);
  const review = parseCommissioningReview(terms);
  let invalidated = false,
    receipt: CommissioningReceipt | null = null,
    consent: CommissioningConsent | null = null;
  const pending = new Set<AbortController>();
  function invalidate() {
    if (invalidated) return;
    invalidated = true;
    receipt = null;
    consent = null;
    for (const request of pending) request.abort();
    onInvalidated();
  }
  async function request(path: string, body?: object): Promise<unknown> {
    if (invalidated)
      throw new CommissioningClientError("execution_session_changed");
    const abort = new AbortController();
    pending.add(abort);
    const timer = setTimeout(() => abort.abort(), 45_000);
    try {
      const response = await fetch(`${base}/${path}`, {
        method: body ? "POST" : "GET",
        credentials: "same-origin",
        cache: "no-store",
        redirect: "error",
        signal: abort.signal,
        headers: {
          Accept: "application/json",
          "X-Arbion-Execution-Session": sessionBinding,
          ...(body ? { "Content-Type": "application/json" } : {}),
        },
        ...(body ? { body: JSON.stringify(body) } : {}),
      });
      if (response.status === 401 || response.status === 403) {
        void response.body?.cancel().catch(() => undefined);
        invalidate();
      }
      if (invalidated)
        throw new CommissioningClientError("execution_session_changed");
      if (response.redirected || response.type === "opaqueredirect")
        throw new CommissioningClientError("INVALID_RESPONSE");
      const raw = await executionResponseJSON(response);
      if (invalidated)
        throw new CommissioningClientError("execution_session_changed");
      if (!response.ok) {
        const error = executionObject(executionObject(raw, ["error"]).error, [
          "code",
          "message",
        ]);
        if (
          typeof error.code !== "string" ||
          typeof error.message !== "string" ||
          error.message.length > 1024
        )
          throw new CommissioningClientError("INVALID_RESPONSE");
        throw new CommissioningClientError(error.code);
      }
      if (response.status !== 200)
        throw new CommissioningClientError("INVALID_RESPONSE");
      return raw;
    } catch (error) {
      if (invalidated)
        throw new CommissioningClientError("execution_session_changed");
      if (error instanceof CommissioningClientError) throw error;
      throw new CommissioningClientError("EXECUTION_OUTCOME_UNKNOWN");
    } finally {
      clearTimeout(timer);
      pending.delete(abort);
    }
  }
  async function getReceipt(path: string, body?: object) {
    const raw = await request(path, body);
    try {
      const next = parseCommissioningReceipt(
        executionObject(raw, ["receipt"]).receipt,
        review,
      );
      if (receipt && receipt.snapshot_digest !== next.snapshot_digest)
        throw new Error();
      receipt = next;
      return next;
    } catch {
      throw new CommissioningClientError("INVALID_RESPONSE");
    }
  }
  async function getConsent(path: string, body?: object) {
    if (!receipt) throw new CommissioningClientError("INVALID_COMMAND");
    const expected = receipt;
    const raw = await request(path, body);
    try {
      const next = parseCommissioningConsent(
        executionObject(raw, ["consent"]).consent,
        expected,
      );
      if (
        consent &&
        (next.id !== consent.id ||
          next.approved_at !== consent.approved_at ||
          next.expires_at !== consent.expires_at ||
          (consent.revoked_at !== null &&
            consent.revoked_at !== next.revoked_at))
      )
        throw new Error();
      consent = next;
      return next;
    } catch {
      throw new CommissioningClientError("INVALID_RESPONSE");
    }
  }
  return {
    invalidate,
    prepare: () =>
      getReceipt("prepare", { expected_terms_digest: review.terms_digest }),
    readReceipt: () => getReceipt("receipt"),
    readConsent: () => getConsent("consent"),
    approve: (code) => {
      if (!receipt || !/^\d{6}$/.test(code))
        return Promise.reject(new CommissioningClientError("INVALID_COMMAND"));
      return getConsent("approve", {
        expected_terms_digest: review.terms_digest,
        expected_snapshot_digest: receipt.snapshot_digest,
        mfa_code: code,
      });
    },
    revoke: async () => {
      const saved = await getConsent("revoke", {});
      if (saved.revoked_at === null)
        throw new CommissioningClientError("INVALID_RESPONSE");
      return saved;
    },
  };
}
