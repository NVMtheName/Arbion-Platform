import { describe, expect, it } from "vitest";

import { formatExactMoney, sumExactMoney } from "./exact-money";

describe("exact dashboard money", () => {
  it("adds decimal account values without binary floating-point drift", () => {
    expect(
      sumExactMoney([
        { amount: "0.1", currency: "USD" },
        { amount: "0.2", currency: "usd" },
        { amount: "15249.7000000000", currency: " USD " },
      ]),
    ).toEqual({ amount: "15250", currency: "USD" });
  });

  it("formats and rounds values larger than JavaScript's safe integer limit", () => {
    expect(
      formatExactMoney({
        amount: "9007199254740993.005",
        currency: "USD",
      }),
    ).toBe("$9,007,199,254,740,993.01");
    expect(formatExactMoney({ amount: "-1250.555", currency: "USD" })).toBe(
      "-$1,250.56",
    );
    expect(formatExactMoney({ amount: "not-a-decimal", currency: "USD" })).toBe(
      "Unavailable",
    );
  });

  it("fails aggregation closed for mixed, malformed, or unbounded evidence", () => {
    expect(
      sumExactMoney([
        { amount: "100", currency: "USD" },
        { amount: "100", currency: "EUR" },
      ]),
    ).toBeUndefined();
    expect(
      sumExactMoney([{ amount: "not-a-decimal", currency: "USD" }]),
    ).toBeUndefined();
    expect(
      sumExactMoney([{ amount: `1.${"1".repeat(31)}`, currency: "USD" }]),
    ).toBeUndefined();
  });
});
