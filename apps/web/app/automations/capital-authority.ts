export type CapitalReservationPolicy = {
  allocationType: string | undefined;
  allocationValue: string | undefined;
  protectedAmount: string | undefined;
  allocationLimit: string | undefined;
  reservationAmount: string | undefined;
  reservationBasis: string | undefined;
  reservationAccountLimit: string | undefined;
};

function decimalUnits(value: string | undefined) {
  if (!value || !/^\d+(\.\d{1,10})?$/.test(value)) return undefined;
  const [whole, fraction = ""] = value.split(".");
  return BigInt(`${whole}${fraction.padEnd(10, "0")}`);
}

export function capitalReservationMatchesPolicy({
  allocationType,
  allocationValue,
  protectedAmount,
  allocationLimit,
  reservationAmount,
  reservationBasis,
  reservationAccountLimit,
}: CapitalReservationPolicy) {
  const allocation = decimalUnits(allocationValue);
  const protectedUnits = decimalUnits(protectedAmount);
  const reservation = decimalUnits(reservationAmount);
  if (
    allocation === undefined ||
    protectedUnits === undefined ||
    reservation === undefined ||
    allocation <= BigInt(0) ||
    reservation <= BigInt(0)
  )
    return false;

  const allocationLimitUnits = decimalUnits(allocationLimit);
  const reservationAccountLimitUnits = decimalUnits(reservationAccountLimit);
  let capacity: bigint;
  if (allocationType === "FIXED_AMOUNT") {
    if (
      reservationBasis !== "BUCKET_FIXED_CAPACITY" ||
      (allocationLimit === undefined) !==
        (reservationAccountLimit === undefined) ||
      (allocationLimit !== undefined &&
        (allocationLimitUnits === undefined ||
          reservationAccountLimitUnits === undefined ||
          allocationLimitUnits <= BigInt(0) ||
          allocationLimitUnits !== reservationAccountLimitUnits))
    )
      return false;
    capacity =
      allocationLimitUnits !== undefined && allocationLimitUnits < allocation
        ? allocationLimitUnits
        : allocation;
  } else if (
    allocationType === "PERCENT_OF_AVAILABLE_CASH" ||
    allocationType === "PERCENT_OF_BUYING_POWER"
  ) {
    if (
      allocation > BigInt("1000000000000") ||
      allocationLimitUnits === undefined ||
      allocationLimitUnits <= BigInt(0) ||
      reservationAccountLimit !== undefined ||
      reservationBasis !== "BUCKET_ABSOLUTE_LIMIT"
    )
      return false;
    capacity = allocationLimitUnits;
  } else {
    return false;
  }

  const expectedReservation = capacity - protectedUnits;
  return expectedReservation > BigInt(0) && reservation === expectedReservation;
}
