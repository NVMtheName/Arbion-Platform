export type PaperMarketSnapshot = {
  symbol: string;
  assetClass: "EQUITY" | "CRYPTO";
  price: string;
  priceBasis: "MARK" | "LAST" | "BID" | "ASK";
  changePercent24H?: string;
  provider: string;
  feed: string;
  quality: string;
  observedAt: string;
  decisionAt: string;
};

type PaperPortfolioInput = {
  starting_cash: string;
  cash: string;
  positions: Array<{
    symbol: string;
    instrument: "EQUITY" | "OPTION" | "CRYPTO";
    quantity: string;
    average_price: string;
    is_open: boolean;
  }>;
};

export type PaperPositionValuation = PaperMarketSnapshot & {
  key: string;
  marketValue: string;
  costBasis: string;
  unrealizedProfitLoss: string;
  unrealizedProfitLossPercent: string;
};

export type PaperPerformance = {
  status: "AVAILABLE" | "CASH_ONLY" | "UNAVAILABLE";
  simulatedEquity?: string;
  investedExposure?: string;
  totalProfitLoss?: string;
  totalReturnPercent?: string;
  valuedAt?: string;
  positions: PaperPositionValuation[];
};

export type PaperPerformanceHistoryPoint = {
  decisionId: string;
  decisionAt: string;
  decision: "ABSTAIN" | "PROPOSE";
  symbol?: string;
  side?: string;
  cash: string;
  investedExposure: string;
  simulatedEquity: string;
  totalProfitLoss: string;
  totalReturnPercent: string;
  provider: string;
  marketCount: number;
  marketObservedAt: string;
};

export type PaperPerformanceHistory = {
  points: PaperPerformanceHistoryPoint[];
  unavailableDecisionCount: number;
};

type ExactDecimal = { units: bigint; scale: number };

const decimalPattern = /^-?(0|[1-9][0-9]*)(\.[0-9]+)?$/;
const symbolPattern = /^[A-Z][A-Z0-9.-]{0,15}$/;

function record(value: unknown): Record<string, unknown> | undefined {
  return value && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : undefined;
}

function string(value: unknown): string | undefined {
  return typeof value === "string" && value.length > 0 ? value : undefined;
}

function parseDecimal(value: unknown): ExactDecimal | undefined {
  if (typeof value !== "string" || !decimalPattern.test(value)) return;
  const negative = value.startsWith("-");
  const unsigned = negative ? value.slice(1) : value;
  const [whole, fraction = ""] = unsigned.split(".");
  if (fraction.length > 30) return;
  let units = BigInt(`${whole}${fraction}`);
  if (negative) units = -units;
  return normalize({ units, scale: fraction.length });
}

function normalize(value: ExactDecimal): ExactDecimal {
  let { units, scale } = value;
  while (scale > 0 && units % BigInt(10) === BigInt(0)) {
    units /= BigInt(10);
    scale -= 1;
  }
  return { units, scale };
}

function power10(exponent: number) {
  return BigInt(10) ** BigInt(exponent);
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

function subtract(left: ExactDecimal, right: ExactDecimal): ExactDecimal {
  return add(left, { units: -right.units, scale: right.scale });
}

function multiply(left: ExactDecimal, right: ExactDecimal): ExactDecimal {
  return normalize({
    units: left.units * right.units,
    scale: left.scale + right.scale,
  });
}

function percentage(
  value: ExactDecimal,
  basis: ExactDecimal,
): ExactDecimal | undefined {
  if (basis.units === BigInt(0)) return;
  const precision = 4;
  const numerator =
    value.units * power10(basis.scale) * BigInt(100) * power10(precision);
  const denominator = basis.units * power10(value.scale);
  let quotient = numerator / denominator;
  const remainder = numerator % denominator;
  if (
    remainder !== BigInt(0) &&
    (remainder < BigInt(0) ? -remainder : remainder) * BigInt(2) >=
      (denominator < BigInt(0) ? -denominator : denominator)
  ) {
    const negative = numerator < BigInt(0) !== denominator < BigInt(0);
    quotient += negative ? -BigInt(1) : BigInt(1);
  }
  return normalize({ units: quotient, scale: precision });
}

function decimalText(value: ExactDecimal) {
  const sign = value.units < BigInt(0) ? "-" : "";
  const absolute = value.units < BigInt(0) ? -value.units : value.units;
  const digits = absolute.toString().padStart(value.scale + 1, "0");
  if (value.scale === 0) return `${sign}${digits}`;
  return `${sign}${digits.slice(0, -value.scale)}.${digits.slice(-value.scale)}`;
}

function positiveDecimal(value: unknown): string | undefined {
  const parsed = parseDecimal(value);
  return parsed && parsed.units > BigInt(0) ? String(value) : undefined;
}

function marketPrice(market: Record<string, unknown>) {
  for (const [field, basis] of [
    ["mark", "MARK"],
    ["last", "LAST"],
    ["bid", "BID"],
    ["ask", "ASK"],
  ] as const) {
    const price = positiveDecimal(market[field]);
    if (price) return { price, basis };
  }
}

// Reconstructs one exact pre-decision Paper valuation from each immutable AI
// journal entry. Invalid or incomplete entries remain explicit coverage gaps;
// points are never joined across decisions and never treated as live quotes.
export function extractPaperPerformanceHistory(
  decisions: Record<string, unknown>[],
  startingCashValue: string,
): PaperPerformanceHistory {
  const startingCash = parseDecimal(startingCashValue);
  if (!startingCash || startingCash.units <= BigInt(0)) {
    return { points: [], unavailableDecisionCount: decisions.length };
  }

  const points: PaperPerformanceHistoryPoint[] = [];
  let unavailableDecisionCount = 0;
  for (const entry of decisions) {
    if ((entry.source ?? entry.Source) !== "AI") continue;
    const rationale = record(
      entry.structured_rationale ?? entry.StructuredRationale,
    );
    if (rationale?.execution_mode !== "PAPER") continue;
    const evidence = record(rationale.input_evidence);
    const decisionId = string(entry.id ?? entry.ID);
    const decisionAt = string(entry.created_at ?? entry.CreatedAt);
    const provider = string(evidence?.provider);
    const cash = parseDecimal(evidence?.available_cash_usd);
    const decision = string(rationale.decision);
    const rawMarkets = Array.isArray(evidence?.markets) ? evidence.markets : [];
    const rawPositions = Array.isArray(evidence?.positions)
      ? evidence.positions
      : [];
    if (
      !decisionId ||
      !decisionAt ||
      Number.isNaN(Date.parse(decisionAt)) ||
      !provider ||
      !cash ||
      cash.units < BigInt(0) ||
      (decision !== "ABSTAIN" && decision !== "PROPOSE") ||
      rawMarkets.length === 0
    ) {
      unavailableDecisionCount += 1;
      continue;
    }

    const marketSymbols = new Set<string>();
    let marketObservedAt: string | undefined;
    let valid = true;
    for (const rawMarket of rawMarkets) {
      const market = record(rawMarket);
      const symbol = string(market?.symbol);
      const feed = string(market?.feed);
      const quality = string(market?.quality);
      const observedAt = string(market?.observed_at);
      if (
        !symbol ||
        !symbolPattern.test(symbol) ||
        marketSymbols.has(symbol) ||
        !feed ||
        !quality ||
        !observedAt ||
        Number.isNaN(Date.parse(observedAt)) ||
        !marketPrice(market ?? {})
      ) {
        valid = false;
        break;
      }
      marketSymbols.add(symbol);
      if (
        !marketObservedAt ||
        Date.parse(observedAt) < Date.parse(marketObservedAt)
      ) {
        marketObservedAt = observedAt;
      }
    }

    let investedExposure: ExactDecimal = { units: BigInt(0), scale: 0 };
    const positionSymbols = new Set<string>();
    for (const rawPosition of rawPositions) {
      const position = record(rawPosition);
      const symbol = string(position?.symbol);
      const instrument = string(position?.instrument);
      const quantity = parseDecimal(position?.quantity);
      const marketValue = parseDecimal(position?.market_value_usd);
      const performanceStatus = string(position?.performance_status);
      if (
        !symbol ||
        !symbolPattern.test(symbol) ||
        positionSymbols.has(symbol) ||
        (instrument !== "EQUITY" && instrument !== "CRYPTO") ||
        !quantity ||
        quantity.units < BigInt(0) ||
        !marketValue ||
        marketValue.units < BigInt(0) ||
        (quantity.units > BigInt(0) &&
          (!marketSymbols.has(symbol) ||
            (performanceStatus !== "PARTIAL" &&
              performanceStatus !== "AVAILABLE")))
      ) {
        valid = false;
        break;
      }
      positionSymbols.add(symbol);
      investedExposure = add(investedExposure, marketValue);
    }
    if (!valid || !marketObservedAt) {
      unavailableDecisionCount += 1;
      continue;
    }

    const simulatedEquity = add(cash, investedExposure);
    const totalProfitLoss = subtract(simulatedEquity, startingCash);
    const totalReturnPercent = percentage(totalProfitLoss, startingCash);
    if (!totalReturnPercent) {
      unavailableDecisionCount += 1;
      continue;
    }
    points.push({
      decisionId,
      decisionAt,
      decision,
      symbol: string(rationale.symbol),
      side: string(rationale.side),
      cash: decimalText(cash),
      investedExposure: decimalText(investedExposure),
      simulatedEquity: decimalText(simulatedEquity),
      totalProfitLoss: decimalText(totalProfitLoss),
      totalReturnPercent: decimalText(totalReturnPercent),
      provider,
      marketCount: marketSymbols.size,
      marketObservedAt,
    });
  }
  points.sort((left, right) => {
    const order = Date.parse(left.decisionAt) - Date.parse(right.decisionAt);
    return order || left.decisionId.localeCompare(right.decisionId);
  });
  return { points, unavailableDecisionCount };
}

// Uses one immutable AI decision as a point-in-time valuation set. It never
// combines markets from different decisions or treats the snapshot as live.
export function extractLatestPaperMarketSnapshots(
  decisions: Record<string, unknown>[],
): PaperMarketSnapshot[] {
  const newestFirst = [...decisions].sort((left, right) => {
    const leftAt = Date.parse(
      string(left.created_at ?? left.CreatedAt) ?? "invalid",
    );
    const rightAt = Date.parse(
      string(right.created_at ?? right.CreatedAt) ?? "invalid",
    );
    return (
      (Number.isNaN(rightAt) ? 0 : rightAt) -
      (Number.isNaN(leftAt) ? 0 : leftAt)
    );
  });
  for (const decision of newestFirst) {
    if ((decision.source ?? decision.Source) !== "AI") continue;
    const rationale = record(
      decision.structured_rationale ?? decision.StructuredRationale,
    );
    if (rationale?.execution_mode !== "PAPER") continue;
    const evidence = record(rationale.input_evidence);
    const provider = string(evidence?.provider);
    const decisionAt = string(decision.created_at ?? decision.CreatedAt);
    if (!provider || !decisionAt || Number.isNaN(Date.parse(decisionAt)))
      continue;
    const markets = Array.isArray(evidence?.markets) ? evidence.markets : [];
    const snapshots: PaperMarketSnapshot[] = [];
    const seen = new Set<string>();
    let valid = markets.length > 0;
    for (const rawMarket of markets) {
      const market = record(rawMarket);
      const symbol = string(market?.symbol);
      const assetClass = string(market?.asset_class);
      const feed = string(market?.feed);
      const quality = string(market?.quality);
      const observedAt = string(market?.observed_at);
      const selected = market ? marketPrice(market) : undefined;
      if (
        !symbol ||
        !symbolPattern.test(symbol) ||
        seen.has(symbol) ||
        (assetClass !== "EQUITY" && assetClass !== "CRYPTO") ||
        !feed ||
        !quality ||
        !observedAt ||
        Number.isNaN(Date.parse(observedAt)) ||
        !selected
      ) {
        valid = false;
        break;
      }
      seen.add(symbol);
      const changePercent24H = parseDecimal(market?.change_percent_24h)
        ? String(market?.change_percent_24h)
        : undefined;
      snapshots.push({
        symbol,
        assetClass,
        price: selected.price,
        priceBasis: selected.basis,
        changePercent24H,
        provider,
        feed,
        quality,
        observedAt,
        decisionAt,
      });
    }
    if (valid) return snapshots;
  }
  return [];
}

export function calculatePaperPerformance(
  portfolio: PaperPortfolioInput,
  markets: PaperMarketSnapshot[],
): PaperPerformance {
  const startingCash = parseDecimal(portfolio.starting_cash);
  const cash = parseDecimal(portfolio.cash);
  if (
    !startingCash ||
    !cash ||
    startingCash.units <= BigInt(0) ||
    cash.units < BigInt(0)
  ) {
    return { status: "UNAVAILABLE", positions: [] };
  }
  const openPositions = portfolio.positions.filter(
    (position) => position.is_open,
  );
  if (openPositions.length === 0) {
    const profitLoss = subtract(cash, startingCash);
    return {
      status: "CASH_ONLY",
      simulatedEquity: decimalText(cash),
      investedExposure: "0",
      totalProfitLoss: decimalText(profitLoss),
      totalReturnPercent: decimalText(
        percentage(profitLoss, startingCash) ?? { units: BigInt(0), scale: 0 },
      ),
      positions: [],
    };
  }

  const snapshots = new Map(
    markets.map((market) => [`${market.assetClass}:${market.symbol}`, market]),
  );
  const valuations: PaperPositionValuation[] = [];
  let investedExposure: ExactDecimal = { units: BigInt(0), scale: 0 };
  let valuedAt: string | undefined;
  for (const position of openPositions) {
    if (position.instrument === "OPTION")
      return { status: "UNAVAILABLE", positions: valuations };
    const snapshot = snapshots.get(`${position.instrument}:${position.symbol}`);
    const quantity = parseDecimal(position.quantity);
    const averagePrice = parseDecimal(position.average_price);
    const currentPrice = parseDecimal(snapshot?.price);
    if (
      !snapshot ||
      !quantity ||
      !averagePrice ||
      !currentPrice ||
      quantity.units <= BigInt(0) ||
      averagePrice.units <= BigInt(0) ||
      currentPrice.units <= BigInt(0)
    ) {
      return { status: "UNAVAILABLE", positions: valuations };
    }
    const marketValue = multiply(quantity, currentPrice);
    const costBasis = multiply(quantity, averagePrice);
    const profitLoss = subtract(marketValue, costBasis);
    const profitLossPercent = percentage(profitLoss, costBasis);
    if (!profitLossPercent)
      return { status: "UNAVAILABLE", positions: valuations };
    investedExposure = add(investedExposure, marketValue);
    if (!valuedAt || Date.parse(snapshot.observedAt) < Date.parse(valuedAt)) {
      valuedAt = snapshot.observedAt;
    }
    valuations.push({
      ...snapshot,
      key: `${position.instrument}:${position.symbol}`,
      marketValue: decimalText(marketValue),
      costBasis: decimalText(costBasis),
      unrealizedProfitLoss: decimalText(profitLoss),
      unrealizedProfitLossPercent: decimalText(profitLossPercent),
    });
  }
  const simulatedEquity = add(cash, investedExposure);
  const totalProfitLoss = subtract(simulatedEquity, startingCash);
  const totalReturnPercent = percentage(totalProfitLoss, startingCash);
  if (!totalReturnPercent)
    return { status: "UNAVAILABLE", positions: valuations };
  return {
    status: "AVAILABLE",
    simulatedEquity: decimalText(simulatedEquity),
    investedExposure: decimalText(investedExposure),
    totalProfitLoss: decimalText(totalProfitLoss),
    totalReturnPercent: decimalText(totalReturnPercent),
    valuedAt,
    positions: valuations,
  };
}
