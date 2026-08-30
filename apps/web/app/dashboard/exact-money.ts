export type ExactMoney = {
  amount: string;
  currency: string;
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

export function formatExactMoney(value?: ExactMoney) {
  if (!value) return "Unavailable";
  const currency = value.currency.trim().toUpperCase();
  const amount = parseExactDecimal(value.amount);
  if (!amount || !/^[A-Z]{3}$/.test(currency)) {
    return "Unavailable";
  }

  const minorUnits = roundedMinorUnits(amount, 2);
  const negative = minorUnits < BigInt(0);
  const absolute = negative ? -minorUnits : minorUnits;
  const whole = (absolute / BigInt(100))
    .toString()
    .replace(/\B(?=(\d{3})+(?!\d))/g, ",");
  const fraction = String(absolute % BigInt(100)).padStart(2, "0");
  const formatted = `${whole}.${fraction}`;
  return currency === "USD"
    ? `${negative ? "-" : ""}$${formatted}`
    : `${negative ? "-" : ""}${currency} ${formatted}`;
}
