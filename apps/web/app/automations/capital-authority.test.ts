import { describe, expect, it } from "vitest";

import { capitalReservationMatchesPolicy } from "./capital-authority";

describe("capitalReservationMatchesPolicy", () => {
  it("matches an exact fixed allocation, protected amount, and shared ceiling", () => {
    expect(
      capitalReservationMatchesPolicy({
        allocationType: "FIXED_AMOUNT",
        allocationValue: "1000.0000000000",
        protectedAmount: "100.0000000000",
        allocationLimit: "800.0000000000",
        reservationAmount: "700.0000000000",
        reservationBasis: "BUCKET_FIXED_CAPACITY",
        reservationAccountLimit: "800",
      }),
    ).toBe(true);
  });

  it("matches a percentage allocation only through its exact absolute cap", () => {
    expect(
      capitalReservationMatchesPolicy({
        allocationType: "PERCENT_OF_AVAILABLE_CASH",
        allocationValue: "25.0000000000",
        protectedAmount: "250.0000000000",
        allocationLimit: "5000.0000000000",
        reservationAmount: "4750.0000000000",
        reservationBasis: "BUCKET_ABSOLUTE_LIMIT",
        reservationAccountLimit: undefined,
      }),
    ).toBe(true);
  });

  it("rejects a reservation amount that differs by one exact decimal unit", () => {
    expect(
      capitalReservationMatchesPolicy({
        allocationType: "FIXED_AMOUNT",
        allocationValue: "1000.0000000000",
        protectedAmount: "0.0000000000",
        allocationLimit: undefined,
        reservationAmount: "999.9999999999",
        reservationBasis: "BUCKET_FIXED_CAPACITY",
        reservationAccountLimit: undefined,
      }),
    ).toBe(false);
  });

  it("rejects mismatched account ceilings and percentage sharing claims", () => {
    expect(
      capitalReservationMatchesPolicy({
        allocationType: "FIXED_AMOUNT",
        allocationValue: "1000.0000000000",
        protectedAmount: "0.0000000000",
        allocationLimit: "1000.0000000000",
        reservationAmount: "1000.0000000000",
        reservationBasis: "BUCKET_FIXED_CAPACITY",
        reservationAccountLimit: undefined,
      }),
    ).toBe(false);
    expect(
      capitalReservationMatchesPolicy({
        allocationType: "PERCENT_OF_BUYING_POWER",
        allocationValue: "20.0000000000",
        protectedAmount: "0.0000000000",
        allocationLimit: "1000.0000000000",
        reservationAmount: "1000.0000000000",
        reservationBasis: "BUCKET_ABSOLUTE_LIMIT",
        reservationAccountLimit: "1000.0000000000",
      }),
    ).toBe(false);
  });

  it("rejects unsupported, malformed, zero-capacity, and over-100-percent policies", () => {
    const base = {
      allocationType: "FIXED_AMOUNT",
      allocationValue: "1000.0000000000",
      protectedAmount: "0.0000000000",
      allocationLimit: undefined,
      reservationAmount: "1000.0000000000",
      reservationBasis: "BUCKET_FIXED_CAPACITY",
      reservationAccountLimit: undefined,
    };

    expect(
      capitalReservationMatchesPolicy({
        ...base,
        allocationValue: "1.00000000001",
      }),
    ).toBe(false);
    expect(
      capitalReservationMatchesPolicy({
        ...base,
        protectedAmount: "1000.0000000000",
        reservationAmount: "0.0000000000",
      }),
    ).toBe(false);
    expect(
      capitalReservationMatchesPolicy({
        ...base,
        allocationType: "PERCENT_OF_AVAILABLE_CASH",
        allocationValue: "100.0000000001",
        allocationLimit: "1000.0000000000",
        reservationBasis: "BUCKET_ABSOLUTE_LIMIT",
      }),
    ).toBe(false);
    expect(
      capitalReservationMatchesPolicy({
        ...base,
        allocationType: "BROKER_BUYING_POWER",
      }),
    ).toBe(false);
  });
});
