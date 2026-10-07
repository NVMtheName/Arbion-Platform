import { describe, it, expect } from "vitest";
import {
  parseCommissioningContext,
  parseCommissioningReview,
  parseCommissioningReceipt,
  parseCommissioningConsent,
} from "./commissioning-contract";
import {
  context,
  review,
  receipt,
  consent,
} from "./commissioning.test-fixtures";

describe("exact commissioning projections", () => {
  it("accepts only documented exact safe DTOs", () => {
    expect(parseCommissioningContext(context)).toEqual(context);
    expect(parseCommissioningContext({ available: false })).toEqual({
      available: false,
    });
    expect(parseCommissioningReceipt(receipt, review)).toEqual(receipt);
    expect(parseCommissioningConsent(consent, receipt)).toEqual(consent);
    expect(
      parseCommissioningConsent(
        { ...consent, revoked_at: "2099-10-07T12:02:00Z" },
        receipt,
      ).revoked_at,
    ).not.toBeNull();
  });
  it.each([
    { ...context, owner_id: "private" },
    { available: false, review },
    { ...context, session_binding: "bad" },
    { ...context, review: { ...review, account_id: "private" } },
    { ...context, review: { ...review, maximum_order_usd: 25 } },
    { ...context, review: { ...review, interval_minutes: 0 } },
    { ...context, review: { ...review, product_id: "USD-USD" } },
    { ...context, review: { ...review, expires_at: "2099-10-09T12:00:00Z" } },
    { ...context, review: { ...review, expires_at: "2099-02-31T12:00:00Z" } },
    { ...context, review: { ...review, objective: "bad\ntext" } },
    { ...context, review: { ...review, model_id: "<unsafe>" } },
  ])("rejects malformed or authority-bearing context %#", (value) =>
    expect(() => parseCommissioningContext(value)).toThrow(),
  );
  it("rejects getters and prototype authority without executing them", () => {
    let called = false;
    expect(() =>
      parseCommissioningReview({
        ...review,
        get terms_digest() {
          called = true;
          return review.terms_digest;
        },
      }),
    ).toThrow();
    expect(called).toBe(false);
    expect(() =>
      parseCommissioningReview(
        Object.assign(Object.create({ owner_id: "private" }), review),
      ),
    ).toThrow();
  });
  it("never tolerates different saved terms or consent scope", () => {
    for (const bad of [
      { ...receipt, allocation_usd: "100.0" },
      { ...receipt, model_id: "other" },
      { ...receipt, snapshot_digest: "bad" },
    ])
      expect(() => parseCommissioningReceipt(bad, review)).toThrow();
    for (const bad of [
      { ...consent, snapshot_digest: "d".repeat(64) },
      { ...consent, expires_at: "2099-10-09T12:00:00Z" },
      { ...consent, revoked_at: "2099-10-07T12:00:00Z" },
      { ...consent, owner_id: "private" },
    ])
      expect(() => parseCommissioningConsent(bad, receipt)).toThrow();
  });
});
