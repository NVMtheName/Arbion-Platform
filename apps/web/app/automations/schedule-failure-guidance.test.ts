import { describe, expect, it } from "vitest";

import { scheduleFailureGuidance } from "./schedule-failure-guidance";

describe("scheduleFailureGuidance", () => {
  it("requires review of the pinned mandate window without extending it", () => {
    expect(
      scheduleFailureGuidance("COMMIT_MANDATE_WINDOW_CLOSED"),
    ).toMatchObject({
      action: "OWNER_REVIEW",
      message: expect.stringMatching(
        /no broker order.*newer draft does not extend.*Do not rerun.*automatically extend/,
      ),
    });
  });
  it("uses the next normal cycle after a quote expires at commit", () => {
    expect(scheduleFailureGuidance("COMMIT_MARKET_DATA_STALE")).toMatchObject({
      action: "AUTOMATIC_RETRY",
      message: expect.stringMatching(
        /no broker order.*normal schedule.*do not rerun the old proposal or loosen/,
      ),
    });
  });
  it("does not confuse revoked automation access with browser sign-in", () => {
    expect(scheduleFailureGuidance("COMMIT_ACCESS_REVOKED")).toMatchObject({
      action: "OWNER_REVIEW",
      message: expect.stringMatching(
        /logging in again does not restore revoked permissions.*No broker order/,
      ),
    });
  });
  it("keeps intentional connection revocation in place", () => {
    expect(
      scheduleFailureGuidance("COMMIT_CONNECTION_UNAVAILABLE"),
    ).toMatchObject({
      action: "OWNER_REVIEW",
      message: expect.stringMatching(
        /do not re-enable an intentionally disabled connection.*no simulated fill.*no broker order/,
      ),
    });
  });
  it("explains a late stop without asking the owner to release it", () => {
    expect(scheduleFailureGuidance("CIRCUIT_BREAKER_ACTIVE")).toMatchObject({
      action: "OWNER_REVIEW",
      message: expect.stringMatching(
        /emergency stop.*no simulated fill.*intentional stop should remain engaged.*No broker order/i,
      ),
    });
  });
  it.each([
    ["AI_PROVIDER_RATE_LIMITED", "AUTOMATIC_RETRY"],
    ["AI_PROVIDER_UNAVAILABLE", "AUTOMATIC_RETRY"],
    ["AI_CONNECTION_UNAVAILABLE", "OWNER_REVIEW"],
    ["AI_REQUEST_INVALID", "OPERATOR_REVIEW"],
    ["AUTHORIZATION_FAILED", "OWNER_REVIEW"],
    ["AUTHORIZATION_EXPIRED", "OWNER_REVIEW"],
    ["INVALID_CREDENTIAL_FORMAT", "OWNER_REVIEW"],
    ["ACCOUNT_NOT_FOUND", "OWNER_REVIEW"],
    ["PERMISSION_DENIED", "OWNER_REVIEW"],
    ["INVALID_PROVIDER_RESPONSE", "OPERATOR_REVIEW"],
    ["PROVIDER", "OPERATOR_REVIEW"],
    ["PROVIDER_UNAVAILABLE", "AUTOMATIC_RETRY"],
    ["RATE_LIMITED", "AUTOMATIC_RETRY"],
    ["TIMEOUT", "AUTOMATIC_RETRY"],
    ["MARKET_DATA_DELAYED", "OWNER_REVIEW"],
    ["MARKET_DATA_REALTIME_UNCONFIRMED", "OWNER_REVIEW"],
    ["MARKET_DATA_NOT_REALTIME", "OWNER_REVIEW"],
    ["MARKET_DATA_INVALID", "OPERATOR_REVIEW"],
    ["INVALID", "OPERATOR_REVIEW"],
    ["OUTSIDE_SESSION", "NO_ACTION"],
    ["WAITING_FOR_LIFECYCLE", "OWNER_REVIEW"],
    ["INTERNAL", "OPERATOR_REVIEW"],
  ] as const)("classifies %s as %s", (code, action) => {
    expect(scheduleFailureGuidance(code).action).toBe(action);
  });

  it("uses only allowlisted provider labels", () => {
    expect(scheduleFailureGuidance("PROVIDER", "coinbase").message).toContain(
      "Coinbase read data",
    );
    expect(scheduleFailureGuidance("PROVIDER", "schwab").message).toContain(
      "Schwab read data",
    );
    expect(
      scheduleFailureGuidance("PROVIDER", "<unsafe-provider>").message,
    ).toContain("The financial provider read data");
    expect(
      scheduleFailureGuidance("PROVIDER", "<unsafe-provider>").message,
    ).not.toContain("unsafe-provider");
  });

  it("keeps reconciliation and model failures distinct", () => {
    expect(
      scheduleFailureGuidance("RECONCILIATION_REFRESH_FAILED", "coinbase"),
    ).toMatchObject({
      action: "AUTOMATIC_RETRY",
      actionLabel: "Automatic retry",
    });
    expect(
      scheduleFailureGuidance("AI_REQUEST_INVALID", "coinbase"),
    ).toMatchObject({
      action: "OPERATOR_REVIEW",
      actionLabel: "Operator correction",
    });
  });

  it("does not infer a temporary outage or entitlement from legacy provider errors", () => {
    expect(scheduleFailureGuidance("PROVIDER", "schwab").message).toMatch(
      /does not identify why.*does not prove a temporary outage or a quote-entitlement problem/i,
    );
    expect(
      scheduleFailureGuidance("AUTHORIZATION_EXPIRED", "schwab").message,
    ).toMatch(/Reconnect Schwab before the next scheduled evaluation/i);
    expect(
      scheduleFailureGuidance("PERMISSION_DENIED", "schwab").message,
    ).toMatch(
      /denied the requested read access.*does not establish quote entitlement/i,
    );
    expect(
      scheduleFailureGuidance("secret raw error", "<unsafe>").message,
    ).not.toMatch(/secret raw error|<unsafe>/);
  });

  it("explains that ambiguous Schwab entitlement stops before the model", () => {
    expect(
      scheduleFailureGuidance("MARKET_DATA_NOT_REALTIME", "schwab"),
    ).toMatchObject({
      action: "OWNER_REVIEW",
      actionLabel: "Owner review",
      message: expect.stringMatching(
        /Schwab.*did not explicitly mark.*real-time.*stopped before the AI model.*will not use delayed or ambiguous market prices/i,
      ),
    });
  });

  it("keeps explicit delayed and missing quote-quality evidence distinct", () => {
    expect(
      scheduleFailureGuidance("MARKET_DATA_DELAYED", "schwab").message,
    ).toMatch(/explicitly marked.*delayed.*reconnecting.*does not change/i);
    expect(
      scheduleFailureGuidance("MARKET_DATA_REALTIME_UNCONFIRMED", "schwab")
        .message,
    ).toMatch(/did not include a real-time status.*rather than guessing/i);
  });

  it("explains that unusable Schwab prices stop before the model", () => {
    expect(
      scheduleFailureGuidance("MARKET_DATA_INVALID", "schwab"),
    ).toMatchObject({
      action: "OPERATOR_REVIEW",
      actionLabel: "Operator correction",
      message: expect.stringMatching(
        /Schwab.*malformed or unusable.*stopped before the AI model.*negative-zero.*one-sided/i,
      ),
    });
  });
});
