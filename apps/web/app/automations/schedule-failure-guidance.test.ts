import { describe, expect, it } from "vitest";

import { scheduleFailureGuidance } from "./schedule-failure-guidance";

describe("scheduleFailureGuidance", () => {
  it.each([
    ["AI_PROVIDER_RATE_LIMITED", "AUTOMATIC_RETRY"],
    ["AI_PROVIDER_UNAVAILABLE", "AUTOMATIC_RETRY"],
    ["AI_CONNECTION_UNAVAILABLE", "OWNER_REVIEW"],
    ["AI_REQUEST_INVALID", "OPERATOR_REVIEW"],
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
});
