export type ExactMoney = {
  amount: string;
  currency: string;
};

export type ExactMoneyFormatOptions = {
  minimumFractionDigits?: number;
  maximumFractionDigits?: number;
  signDisplay?: "auto" | "exceptZero";
  negativeSign?: "-" | "−";
  unavailable?: string;
};

type ExactDecimal = {
  units: bigint;
  scale: number;
};

const decimalPattern = /^-?(0|[1-9][0-9]*)(\.[0-9]+)?$/;

function power10(value: number) {
  return BigInt(10) ** BigInt(value);
}

function normalize(value: ExactDecimal): ExactDecimal {
  let { units, scale } = value;
  while (scale > 0 && units % BigInt(10) === BigInt(0)) {
    units /= BigInt(10);
    scale -= 1;
  }
  return { units, scale };
}

function parseExactDecimal(value: string): ExactDecimal | undefined {
  if (!decimalPattern.test(value)) return;
  const negative = value.startsWith("-");
  const unsigned = negative ? value.slice(1) : value;
  const [whole, fraction = ""] = unsigned.split(".");
  if (whole.length > 40 || fraction.length > 30) return;
  let units = BigInt(`${whole}${fraction}`);
  if (negative) units = -units;
  return normalize({ units, scale: fraction.length });
}

function decimalText(value: ExactDecimal) {
  const sign = value.units < BigInt(0) ? "-" : "";
  const absolute = value.units < BigInt(0) ? -value.units : value.units;
  const digits = absolute.toString().padStart(value.scale + 1, "0");
  if (value.scale === 0) return `${sign}${digits}`;
  return `${sign}${digits.slice(0, -value.scale)}.${digits.slice(-value.scale)}`;
}

function add(left: ExactDecimal, right: ExactDecimal): ExactDecimal {
  const scale = Math.max(left.scale, right.scale);
  return normalize({
    units:
      left.units * power10(scale - left.scale) +
      right.units * power10(scale - right.scale),
    scale,
  });
}

function roundedMinorUnits(value: ExactDecimal, fractionDigits: number) {
  if (value.scale <= fractionDigits) {
    return value.units * power10(fractionDigits - value.scale);
  }
  const divisor = power10(value.scale - fractionDigits);
  let quotient = value.units / divisor;
  const remainder = value.units % divisor;
  const absoluteRemainder = remainder < BigInt(0) ? -remainder : remainder;
  if (absoluteRemainder * BigInt(2) >= divisor) {
    quotient += value.units < BigInt(0) ? -BigInt(1) : BigInt(1);
  }
  return quotient;
}

export function sumExactMoney(values: ExactMoney[]): ExactMoney | undefined {
  if (values.length === 0) return;
  const currency = values[0].currency.trim().toUpperCase();
  if (!/^[A-Z]{3}$/.test(currency)) return;
  let total: ExactDecimal = { units: BigInt(0), scale: 0 };
  for (const value of values) {
    if (value.currency.trim().toUpperCase() !== currency) return;
    const amount = parseExactDecimal(value.amount);
    if (!amount) return;
    total = add(total, amount);
  }
  return { amount: decimalText(total), currency };
}

export function formatExactMoney(
  value?: ExactMoney,
  options: ExactMoneyFormatOptions = {},
) {
  const unavailable = options.unavailable ?? "Unavailable";
  if (!value) return unavailable;
  const currency = value.currency.trim().toUpperCase();
  const amount = parseExactDecimal(value.amount);
  if (!amount || !/^[A-Z]{3}$/.test(currency)) {
    return unavailable;
  }

  const minimumFractionDigits = options.minimumFractionDigits ?? 2;
  const maximumFractionDigits = options.maximumFractionDigits ?? 2;
  if (
    !Number.isInteger(minimumFractionDigits) ||
    !Number.isInteger(maximumFractionDigits) ||
    minimumFractionDigits < 0 ||
    maximumFractionDigits < minimumFractionDigits ||
    maximumFractionDigits > 12
  ) {
    return unavailable;
  }

  const minorUnits = roundedMinorUnits(amount, maximumFractionDigits);
  const negative = minorUnits < BigInt(0);
  const absolute = negative ? -minorUnits : minorUnits;
  const scale = power10(maximumFractionDigits);
  const whole = (absolute / scale)
    .toString()
    .replace(/\B(?=(\d{3})+(?!\d))/g, ",");
  let fraction =
    maximumFractionDigits === 0
      ? ""
      : String(absolute % scale).padStart(maximumFractionDigits, "0");
  while (fraction.length > minimumFractionDigits && fraction.endsWith("0")) {
    fraction = fraction.slice(0, -1);
  }
  const formatted = fraction ? `${whole}.${fraction}` : whole;
  const positive = minorUnits > BigInt(0);
  const sign = negative
    ? (options.negativeSign ?? "-")
    : options.signDisplay === "exceptZero" && positive
      ? "+"
      : "";
  return currency === "USD"
    ? `${sign}$${formatted}`
    : `${sign}${currency} ${formatted}`;
}
