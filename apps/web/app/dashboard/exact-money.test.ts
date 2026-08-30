import { describe, expect, it } from "vitest";

import {
  compareExactDecimals,
  divideExactDecimals,
  exactDecimalSign,
  formatExactDecimal,
  formatExactMoney,
  projectExactDecimalRange,
  multiplyExactDecimals,
  subtractExactDecimals,
  sumExactMoney,
} from "../exact-money";

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
    expect(
      formatExactMoney(
        { amount: "0.1234564", currency: "USD" },
        { maximumFractionDigits: 6 },
      ),
    ).toBe("$0.123456");
    expect(
      formatExactMoney(
        { amount: "125", currency: "USD" },
        { maximumFractionDigits: 6, signDisplay: "exceptZero" },
      ),
    ).toBe("+$125.00");
    expect(
      formatExactMoney(
        { amount: "-5", currency: "USD" },
        {
          maximumFractionDigits: 6,
          signDisplay: "exceptZero",
          negativeSign: "−",
        },
      ),
    ).toBe("−$5.00");
    expect(
      formatExactMoney(
        { amount: "1.6", currency: "USD" },
        { minimumFractionDigits: 0, maximumFractionDigits: 0 },
      ),
    ).toBe("$2");
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

  it("formats signs and projects only bounded visual ratios", () => {
    expect(
      formatExactDecimal("9007199254740993.12345", {
        maximumFractionDigits: 4,
        signDisplay: "exceptZero",
        suffix: "%",
      }),
    ).toBe("+9,007,199,254,740,993.1235%");
    expect(exactDecimalSign("-0.00000000000000000001")).toBe(-1);
    expect(exactDecimalSign("malformed")).toBeUndefined();
    expect(compareExactDecimals("100.0000000000", "100")).toBe(0);
    expect(compareExactDecimals("99.9999999999", "100")).toBe(-1);
    expect(compareExactDecimals("1e2", "100")).toBeUndefined();
    expect(
      projectExactDecimalRange([
        "900719925474099300000000000000.005",
        "900719925474099300000000000000.010",
        "900719925474099300000000000000.015",
      ]),
    ).toEqual([0, 0.5, 1]);
    expect(projectExactDecimalRange(["1", "malformed"])).toBeUndefined();
  });

  it("calculates bounded financial scenarios without floating-point drift", () => {
    expect(multiplyExactDecimals("100.0000000000", "3")).toBe("300");
    expect(multiplyExactDecimals("0.1", "0.2")).toBe("0.02");
    expect(subtractExactDecimals("1000.0000000000", "0.1")).toBe("999.9");
    expect(divideExactDecimals("100", "3", 8)).toBe("33.33333333");
    expect(divideExactDecimals("-1", "8", 2)).toBe("-0.13");
    expect(divideExactDecimals("1", "0", 8)).toBeUndefined();
    expect(divideExactDecimals("1", "3", 13)).toBeUndefined();
  });
});
