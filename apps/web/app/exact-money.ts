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

export type ExactDecimalFormatOptions = ExactMoneyFormatOptions & {
  suffix?: string;
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

function fractionDigits(
  options: ExactMoneyFormatOptions,
): { minimum: number; maximum: number } | undefined {
  const minimum = options.minimumFractionDigits ?? 2;
  const maximum = options.maximumFractionDigits ?? 2;
  if (
    !Number.isInteger(minimum) ||
    !Number.isInteger(maximum) ||
    minimum < 0 ||
    maximum < minimum ||
    maximum > 12
  ) {
    return;
  }
  return { minimum, maximum };
}

function formatDecimal(amount: ExactDecimal, options: ExactMoneyFormatOptions) {
  const digits = fractionDigits(options);
  if (!digits) return;
  const minorUnits = roundedMinorUnits(amount, digits.maximum);
  const negative = minorUnits < BigInt(0);
  const absolute = negative ? -minorUnits : minorUnits;
  const scale = power10(digits.maximum);
  const whole = (absolute / scale)
    .toString()
    .replace(/\B(?=(\d{3})+(?!\d))/g, ",");
  let fraction =
    digits.maximum === 0
      ? ""
      : String(absolute % scale).padStart(digits.maximum, "0");
  while (fraction.length > digits.minimum && fraction.endsWith("0")) {
    fraction = fraction.slice(0, -1);
  }
  const formatted = fraction ? `${whole}.${fraction}` : whole;
  const positive = minorUnits > BigInt(0);
  const sign = negative
    ? (options.negativeSign ?? "-")
    : options.signDisplay === "exceptZero" && positive
      ? "+"
      : "";
  return { formatted, sign };
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

  const formatted = formatDecimal(amount, options);
  if (!formatted) return unavailable;
  return currency === "USD"
    ? `${formatted.sign}$${formatted.formatted}`
    : `${formatted.sign}${currency} ${formatted.formatted}`;
}

export function formatExactDecimal(
  value: string | undefined,
  options: ExactDecimalFormatOptions = {},
) {
  const unavailable = options.unavailable ?? "Unavailable";
  const amount = value === undefined ? undefined : parseExactDecimal(value);
  if (!amount) return unavailable;
  const formatted = formatDecimal(amount, options);
  if (!formatted) return unavailable;
  return `${formatted.sign}${formatted.formatted}${options.suffix ?? ""}`;
}

export function exactDecimalSign(value: string | undefined) {
  const amount = value === undefined ? undefined : parseExactDecimal(value);
  if (!amount) return;
  if (amount.units === BigInt(0)) return 0;
  return amount.units > BigInt(0) ? 1 : -1;
}

export function compareExactDecimals(left: string, right: string) {
  const leftAmount = parseExactDecimal(left);
  const rightAmount = parseExactDecimal(right);
  if (!leftAmount || !rightAmount) return;
  const scale = Math.max(leftAmount.scale, rightAmount.scale);
  const leftUnits = leftAmount.units * power10(scale - leftAmount.scale);
  const rightUnits = rightAmount.units * power10(scale - rightAmount.scale);
  if (leftUnits === rightUnits) return 0;
  return leftUnits > rightUnits ? 1 : -1;
}

// Projects exact decimal values onto a bounded 0..1 visual range. BigInt does
// every financial comparison; Number receives only a nine-digit screen ratio.
export function projectExactDecimalRange(values: string[]) {
  if (values.length === 0) return;
  const parsed = values.map(parseExactDecimal);
  if (parsed.some((value) => !value)) return;
  const decimals = parsed as ExactDecimal[];
  const scale = Math.max(...decimals.map((value) => value.scale));
  const units = decimals.map(
    (value) => value.units * power10(scale - value.scale),
  );
  let low = units[0];
  let high = units[0];
  for (const value of units.slice(1)) {
    if (value < low) low = value;
    if (value > high) high = value;
  }
  const spread = high - low;
  if (spread === BigInt(0)) return units.map(() => 0.5);
  const visualPrecision = BigInt(1_000_000_000);
  return units.map(
    (value) =>
      Number(((value - low) * visualPrecision) / spread) /
      Number(visualPrecision),
  );
}
