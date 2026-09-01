import { describe, expect, it } from "vitest";

import { formatConnectionTimestamp } from "./connection-timestamp";

describe("formatConnectionTimestamp", () => {
  it("renders one explicit UTC value without environment locale defaults", () => {
    expect(formatConnectionTimestamp("2026-09-01T13:52:51Z")).toBe(
      "Sep 1, 2026, 1:52:51 PM UTC",
    );
  });

  it("handles midnight and noon without ambiguous hours", () => {
    expect(formatConnectionTimestamp("2026-09-01T00:05:09Z")).toBe(
      "Sep 1, 2026, 12:05:09 AM UTC",
    );
    expect(formatConnectionTimestamp("2026-09-01T12:05:09Z")).toBe(
      "Sep 1, 2026, 12:05:09 PM UTC",
    );
  });

  it("fails closed when saved timestamp evidence is malformed", () => {
    expect(formatConnectionTimestamp("not-a-timestamp")).toBe("UNAVAILABLE");
  });
});
