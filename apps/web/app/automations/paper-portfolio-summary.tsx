import {
  calculatePaperPerformance,
  type PaperMarketSnapshot,
} from "./paper-performance";
import { reconcilePaperOutcome } from "./paper-outcome-reconciliation";
import { PaperPerformanceHistory } from "./paper-performance-history";
import {
  compareExactDecimals,
  divideExactDecimals,
  exactDecimalSign,
  formatExactDecimal,
  formatExactMoney,
  multiplyExactDecimals,
  sumExactMoney,
} from "../exact-money";

export type PaperPosition = {
  symbol: string;
  instrument: "EQUITY" | "OPTION" | "CRYPTO";
  option_type?: "PUT" | "CALL";
  strike?: string;
  expiration?: string;
  quantity: string;
  average_price: string;
  is_open: boolean;
  updated_at: string;
};

export type AIPaperSpotFill = {
  id: string;
  symbol: string;
  instrument: "EQUITY" | "CRYPTO";
  side: "BUY" | "SELL";
  quantity: string;
  reference_price: string;
  fill_price: string;
  gross_notional: string;
  fee: string;
  resulting_cash: string;
  resulting_position_quantity: string;
  pricing_basis: string;
  market_provider: string;
  market_feed: string;
  market_quality: string;
  market_observed_at: string;
  simulated_at: string;
  simulation_only: boolean;
};

export type PaperRealizedSymbolOutcome = {
  symbol: string;
  instrument: "EQUITY" | "CRYPTO";
  realized_profit_loss: string;
  buy_fill_count: number;
  sell_fill_count: number;
  total_fees: string;
  ending_position_quantity: string;
  ending_average_cost?: string;
};

export type PaperRealizedOutcome = {
  status: "AVAILABLE" | "NO_REALIZED_SALES" | "UNAVAILABLE";
  calculation_method?: "AVERAGE_COST_INCLUDED_FEES";
  historical_coverage?: "COMPLETE_FROM_PORTFOLIO_GENESIS";
  total_realized_profit_loss?: string;
  fill_count: number;
  sell_fill_count: number;
  first_fill_at?: string;
  last_fill_at?: string;
  symbols: PaperRealizedSymbolOutcome[];
};

export type PaperExecutionSymbolCost = {
  symbol: string;
  instrument: "EQUITY" | "CRYPTO";
  total_fees: string;
  adverse_slippage: string;
  total_explicit_cost: string;
  provider_reference_notional: string;
  gross_notional: string;
  all_in_cost_rate_bps: string;
  fill_count: number;
  buy_fill_count: number;
  sell_fill_count: number;
};

export type PaperExecutionSideCost = {
  side: "BUY" | "SELL";
  total_fees: string;
  adverse_slippage: string;
  total_explicit_cost: string;
  provider_reference_notional: string;
  gross_notional: string;
  all_in_cost_rate_bps: string;
  fill_count: number;
};

export type PaperExecutionCosts = {
  status: "AVAILABLE" | "NO_FILLS" | "UNAVAILABLE";
  calculation_method?: "SAVED_REFERENCE_VERSUS_SIMULATED_FILL";
  historical_coverage?: "COMPLETE_FROM_PORTFOLIO_GENESIS";
  total_fees?: string;
  total_adverse_slippage?: string;
  total_explicit_cost?: string;
  provider_reference_notional?: string;
  gross_notional?: string;
  all_in_cost_rate_bps?: string;
  fill_notional_residual?: string;
  maximum_absolute_fill_residual?: string;
  residual_bound_per_fill?: string;
  fill_count: number;
  buy_fill_count: number;
  sell_fill_count: number;
  first_fill_at?: string;
  last_fill_at?: string;
  market_providers: string[];
  market_feeds: string[];
  market_qualities: string[];
  sides: PaperExecutionSideCost[];
  symbols: PaperExecutionSymbolCost[];
};

export type PaperPortfolio = {
  strategy_instance_id: string;
  currency: string;
  starting_cash: string;
  cash: string;
  version: number;
  positions: PaperPosition[];
  realized_outcome?: PaperRealizedOutcome;
  execution_costs?: PaperExecutionCosts;
  updated_at: string;
};

function money(value: string, currency: string) {
  return formatExactMoney(
    { amount: value, currency },
    { maximumFractionDigits: 4 },
  );
}

function quantity(value: string) {
  return formatExactDecimal(value, {
    minimumFractionDigits: 0,
    maximumFractionDigits: 4,
  });
}

function signedMoney(value: string, currency: string) {
  return formatExactMoney(
    { amount: value, currency },
    { maximumFractionDigits: 4, signDisplay: "exceptZero" },
  );
}

function signedPreciseMoney(value: string, currency: string) {
  return formatExactMoney(
    { amount: value, currency },
    {
      minimumFractionDigits: 10,
      maximumFractionDigits: 10,
      signDisplay: "exceptZero",
    },
  );
}

function signedPercent(value?: string) {
  return formatExactDecimal(value, {
    maximumFractionDigits: 4,
    signDisplay: "exceptZero",
    suffix: "%",
  });
}

function performanceClass(value?: string) {
  const sign = exactDecimalSign(value);
  if (sign === undefined || sign === 0) return "is-flat";
  return sign > 0 ? "is-positive" : "is-negative";
}

function isExactDecimal(value?: string) {
  return Boolean(value && /^-?(0|[1-9][0-9]*)(\.[0-9]+)?$/.test(value));
}

function marketSource(market: PaperMarketSnapshot) {
  return `${market.provider} · ${market.feed} · ${market.quality}`;
}

function contract(position: PaperPosition, currency: string) {
  if (position.instrument === "CRYPTO") return "Crypto spot";
  if (position.instrument !== "OPTION") return "Shares";
  return `${position.option_type ?? "Option"} · ${money(
    position.strike ?? "0",
    currency,
  )} strike · ${position.expiration ?? "unknown expiry"}`;
}

export function PaperPortfolioSummary({
  portfolio,
  executionMode,
  fills = [],
  markets = [],
  decisions = [],
}: {
  portfolio?: PaperPortfolio;
  executionMode: string;
  fills?: AIPaperSpotFill[];
  markets?: PaperMarketSnapshot[];
  decisions?: Record<string, unknown>[];
}) {
  if (executionMode !== "PAPER") {
    return (
      <section className="paper-portfolio-card" aria-label="Paper portfolio">
        <p className="eyebrow">PAPER PORTFOLIO</p>
        <h2>Not used in SHADOW mode</h2>
        <p>
          SHADOW records what Arbion would have submitted and does not create
          simulated cash or holdings.
        </p>
      </section>
    );
  }

  if (!portfolio) {
    return (
      <section className="paper-portfolio-card" aria-label="Paper portfolio">
        <p className="eyebrow">PAPER PORTFOLIO · SIMULATION ONLY</p>
        <h2>Ledger details unavailable</h2>
        <p className="security-note">
          The simulated ledger could not be loaded. PAPER lifecycle recording is
          disabled until these durable details are available.
        </p>
      </section>
    );
  }

  const positions = portfolio.positions ?? [];
  const openPositions = positions.filter((position) => position.is_open);
  const performance = calculatePaperPerformance(portfolio, markets);
  const valuations = new Map(
    performance.positions.map((position) => [position.key, position]),
  );
  const primaryMarket = performance.positions[0];
  const realized = portfolio.realized_outcome;
  const realizedSymbolsValid = Boolean(
    realized &&
      Array.isArray(realized.symbols) &&
      realized.symbols.every(
        (symbol) =>
          Boolean(symbol.symbol) &&
          ["EQUITY", "CRYPTO"].includes(symbol.instrument) &&
          isExactDecimal(symbol.realized_profit_loss) &&
          isExactDecimal(symbol.total_fees) &&
          isExactDecimal(symbol.ending_position_quantity) &&
          Number.isSafeInteger(symbol.buy_fill_count) &&
          symbol.buy_fill_count >= 0 &&
          Number.isSafeInteger(symbol.sell_fill_count) &&
          symbol.sell_fill_count >= 0,
      ),
  );
  const attributedRealizedProfitLoss = realized
    ? sumExactMoney(
        realized.symbols.map((symbol) => ({
          amount: symbol.realized_profit_loss,
          currency: portfolio.currency,
        })),
      )?.amount
    : undefined;
  const realizedAvailable = Boolean(
    realized &&
      ["AVAILABLE", "NO_REALIZED_SALES"].includes(realized.status) &&
      realized.calculation_method === "AVERAGE_COST_INCLUDED_FEES" &&
      realized.historical_coverage === "COMPLETE_FROM_PORTFOLIO_GENESIS" &&
      isExactDecimal(realized.total_realized_profit_loss) &&
      Number.isSafeInteger(realized.fill_count) &&
      realized.fill_count >= 0 &&
      Number.isSafeInteger(realized.sell_fill_count) &&
      realized.sell_fill_count >= 0 &&
      realized.sell_fill_count <= realized.fill_count &&
      (realized.status === "AVAILABLE"
        ? realized.sell_fill_count > 0
        : realized.sell_fill_count === 0) &&
      realizedSymbolsValid &&
      attributedRealizedProfitLoss &&
      compareExactDecimals(
        attributedRealizedProfitLoss,
        realized.total_realized_profit_loss!,
      ) === 0 &&
      new Set(
        realized.symbols.map(
          (symbol) => `${symbol.instrument}:${symbol.symbol}`,
        ),
      ).size === realized.symbols.length &&
      realized.symbols.reduce(
        (count, symbol) =>
          count + symbol.buy_fill_count + symbol.sell_fill_count,
        0,
      ) === realized.fill_count &&
      realized.symbols.reduce(
        (count, symbol) => count + symbol.sell_fill_count,
        0,
      ) === realized.sell_fill_count &&
      (realized.fill_count === 0 ||
        (Boolean(realized.first_fill_at) &&
          Boolean(realized.last_fill_at) &&
          !Number.isNaN(Date.parse(realized.first_fill_at ?? "")) &&
          !Number.isNaN(Date.parse(realized.last_fill_at ?? "")))),
  );
  const reconciliation = reconcilePaperOutcome({
    currency: portfolio.currency,
    cash: portfolio.cash,
    performance,
    realized,
    realizedContractAvailable: realizedAvailable,
  });
  const reconciliationAvailable = reconciliation.status !== "UNAVAILABLE";
  const reconciliationAttention = reconciliation.status === "MISMATCH";
  const executionCosts = portfolio.execution_costs;
  const executionCostSymbols = Array.isArray(executionCosts?.symbols)
    ? executionCosts.symbols
    : [];
  const executionCostSides = Array.isArray(executionCosts?.sides)
    ? executionCosts.sides
    : [];
  const sumExecutionValues = (values: string[]) =>
    values.length === 0
      ? "0"
      : sumExactMoney(
          values.map((amount) => ({ amount, currency: portfolio.currency })),
        )?.amount;
  const executionCostSymbolFees = executionCosts
    ? executionCostSymbols.length === 0
      ? "0"
      : sumExactMoney(
          executionCostSymbols.map((symbol) => ({
            amount: symbol.total_fees,
            currency: portfolio.currency,
          })),
        )?.amount
    : undefined;
  const executionCostSymbolSlippage = executionCosts
    ? executionCostSymbols.length === 0
      ? "0"
      : sumExactMoney(
          executionCostSymbols.map((symbol) => ({
            amount: symbol.adverse_slippage,
            currency: portfolio.currency,
          })),
        )?.amount
    : undefined;
  const executionCostSymbolGross = executionCosts
    ? executionCostSymbols.length === 0
      ? "0"
      : sumExactMoney(
          executionCostSymbols.map((symbol) => ({
            amount: symbol.gross_notional,
            currency: portfolio.currency,
          })),
        )?.amount
    : undefined;
  const executionCostSymbolExplicit = executionCosts
    ? sumExecutionValues(
        executionCostSymbols.map((symbol) => symbol.total_explicit_cost),
      )
    : undefined;
  const executionCostSymbolReference = executionCosts
    ? sumExecutionValues(
        executionCostSymbols.map(
          (symbol) => symbol.provider_reference_notional,
        ),
      )
    : undefined;
  const executionCostSideFees = executionCosts
    ? sumExecutionValues(executionCostSides.map((side) => side.total_fees))
    : undefined;
  const executionCostSideSlippage = executionCosts
    ? sumExecutionValues(
        executionCostSides.map((side) => side.adverse_slippage),
      )
    : undefined;
  const executionCostSideExplicit = executionCosts
    ? sumExecutionValues(
        executionCostSides.map((side) => side.total_explicit_cost),
      )
    : undefined;
  const executionCostSideReference = executionCosts
    ? sumExecutionValues(
        executionCostSides.map((side) => side.provider_reference_notional),
      )
    : undefined;
  const executionCostSideGross = executionCosts
    ? sumExecutionValues(executionCostSides.map((side) => side.gross_notional))
    : undefined;
  const expectedExecutionCostRate = executionCosts
    ? divideExactDecimals(
        multiplyExactDecimals(
          executionCosts.total_explicit_cost ?? "invalid",
          "10000",
        ) ?? "invalid",
        executionCosts.provider_reference_notional ?? "invalid",
        10,
      )
    : undefined;
  const executionCostsAvailable = Boolean(
    executionCosts &&
      ["AVAILABLE", "NO_FILLS"].includes(executionCosts.status) &&
      executionCosts.calculation_method ===
        "SAVED_REFERENCE_VERSUS_SIMULATED_FILL" &&
      executionCosts.historical_coverage ===
        "COMPLETE_FROM_PORTFOLIO_GENESIS" &&
      Array.isArray(executionCosts.market_providers) &&
      Array.isArray(executionCosts.market_feeds) &&
      Array.isArray(executionCosts.market_qualities) &&
      Array.isArray(executionCosts.sides) &&
      Array.isArray(executionCosts.symbols) &&
      isExactDecimal(executionCosts.total_fees) &&
      isExactDecimal(executionCosts.total_adverse_slippage) &&
      isExactDecimal(executionCosts.total_explicit_cost) &&
      isExactDecimal(executionCosts.provider_reference_notional) &&
      isExactDecimal(executionCosts.gross_notional) &&
      isExactDecimal(executionCosts.all_in_cost_rate_bps) &&
      isExactDecimal(executionCosts.fill_notional_residual) &&
      isExactDecimal(executionCosts.maximum_absolute_fill_residual) &&
      isExactDecimal(executionCosts.residual_bound_per_fill) &&
      compareExactDecimals(executionCosts.total_fees!, "0")! >= 0 &&
      compareExactDecimals(executionCosts.total_adverse_slippage!, "0")! >= 0 &&
      compareExactDecimals(executionCosts.total_explicit_cost!, "0")! >= 0 &&
      compareExactDecimals(executionCosts.provider_reference_notional!, "0")! >=
        0 &&
      compareExactDecimals(executionCosts.all_in_cost_rate_bps!, "0")! >= 0 &&
      compareExactDecimals(
        executionCosts.maximum_absolute_fill_residual!,
        executionCosts.residual_bound_per_fill!,
      )! <= 0 &&
      compareExactDecimals(
        sumExecutionValues([
          executionCosts.total_fees!,
          executionCosts.total_adverse_slippage!,
        ]) ?? "invalid",
        executionCosts.total_explicit_cost!,
      ) === 0 &&
      Number.isSafeInteger(executionCosts.fill_count) &&
      executionCosts.fill_count >= 0 &&
      (executionCosts.status === "AVAILABLE"
        ? executionCosts.fill_count > 0
        : executionCosts.fill_count === 0) &&
      Number.isSafeInteger(executionCosts.buy_fill_count) &&
      Number.isSafeInteger(executionCosts.sell_fill_count) &&
      executionCosts.buy_fill_count + executionCosts.sell_fill_count ===
        executionCosts.fill_count &&
      executionCostSides.every(
        (side) =>
          ["BUY", "SELL"].includes(side.side) &&
          isExactDecimal(side.total_fees) &&
          isExactDecimal(side.adverse_slippage) &&
          isExactDecimal(side.total_explicit_cost) &&
          isExactDecimal(side.provider_reference_notional) &&
          isExactDecimal(side.gross_notional) &&
          isExactDecimal(side.all_in_cost_rate_bps) &&
          side.fill_count > 0 &&
          compareExactDecimals(
            sumExecutionValues([side.total_fees, side.adverse_slippage]) ??
              "invalid",
            side.total_explicit_cost,
          ) === 0,
      ) &&
      new Set(executionCostSides.map((side) => side.side)).size ===
        executionCostSides.length &&
      executionCostSides.reduce((count, side) => count + side.fill_count, 0) ===
        executionCosts.fill_count &&
      executionCostSymbols.every(
        (symbol) =>
          Boolean(symbol.symbol) &&
          ["EQUITY", "CRYPTO"].includes(symbol.instrument) &&
          isExactDecimal(symbol.total_fees) &&
          isExactDecimal(symbol.adverse_slippage) &&
          isExactDecimal(symbol.total_explicit_cost) &&
          isExactDecimal(symbol.provider_reference_notional) &&
          isExactDecimal(symbol.gross_notional) &&
          isExactDecimal(symbol.all_in_cost_rate_bps) &&
          compareExactDecimals(
            sumExecutionValues([symbol.total_fees, symbol.adverse_slippage]) ??
              "invalid",
            symbol.total_explicit_cost,
          ) === 0 &&
          symbol.buy_fill_count + symbol.sell_fill_count === symbol.fill_count,
      ) &&
      executionCostSymbols.reduce(
        (count, symbol) => count + symbol.fill_count,
        0,
      ) === executionCosts.fill_count &&
      executionCostSymbolFees &&
      executionCostSymbolSlippage &&
      executionCostSymbolExplicit &&
      executionCostSymbolReference &&
      executionCostSymbolGross &&
      executionCostSideFees &&
      executionCostSideSlippage &&
      executionCostSideExplicit &&
      executionCostSideReference &&
      executionCostSideGross &&
      compareExactDecimals(
        executionCostSymbolFees,
        executionCosts.total_fees!,
      ) === 0 &&
      compareExactDecimals(
        executionCostSymbolSlippage,
        executionCosts.total_adverse_slippage!,
      ) === 0 &&
      compareExactDecimals(
        executionCostSymbolGross,
        executionCosts.gross_notional!,
      ) === 0 &&
      compareExactDecimals(
        executionCostSymbolExplicit,
        executionCosts.total_explicit_cost!,
      ) === 0 &&
      compareExactDecimals(
        executionCostSymbolReference,
        executionCosts.provider_reference_notional!,
      ) === 0 &&
      compareExactDecimals(
        executionCostSideFees,
        executionCosts.total_fees!,
      ) === 0 &&
      compareExactDecimals(
        executionCostSideSlippage,
        executionCosts.total_adverse_slippage!,
      ) === 0 &&
      compareExactDecimals(
        executionCostSideExplicit,
        executionCosts.total_explicit_cost!,
      ) === 0 &&
      compareExactDecimals(
        executionCostSideReference,
        executionCosts.provider_reference_notional!,
      ) === 0 &&
      compareExactDecimals(
        executionCostSideGross,
        executionCosts.gross_notional!,
      ) === 0 &&
      (executionCosts.fill_count === 0
        ? compareExactDecimals(executionCosts.all_in_cost_rate_bps!, "0") === 0
        : compareExactDecimals(
            expectedExecutionCostRate ?? "invalid",
            executionCosts.all_in_cost_rate_bps!,
          ) === 0) &&
      new Set(
        executionCostSymbols.map(
          (symbol) => `${symbol.instrument}:${symbol.symbol}`,
        ),
      ).size === executionCostSymbols.length &&
      (executionCosts.fill_count === 0 ||
        (Boolean(executionCosts.first_fill_at) &&
          Boolean(executionCosts.last_fill_at) &&
          !Number.isNaN(Date.parse(executionCosts.first_fill_at ?? "")) &&
          !Number.isNaN(Date.parse(executionCosts.last_fill_at ?? "")))),
  );
  return (
    <section className="paper-portfolio-card" aria-label="Paper portfolio">
      <p className="eyebrow">PAPER PORTFOLIO · SIMULATION ONLY</p>
      <h2>{money(portfolio.cash, portfolio.currency)} simulated cash</h2>
      <p>
        This is Arbion&apos;s isolated simulation ledger. These amounts and
        positions are not connected-account balances or provider holdings.
      </p>
      <div
        className="paper-performance-card"
        aria-label="Point-in-time Paper performance"
      >
        <header className="paper-performance-header">
          <div>
            <p className="eyebrow">PAPER PERFORMANCE · POINT IN TIME</p>
            <h3>Simulated portfolio valuation</h3>
          </div>
          <span>Immutable AI snapshot</span>
        </header>
        {performance.status === "UNAVAILABLE" ? (
          <div className="paper-performance-unavailable">
            <strong>Valuation unavailable</strong>
            <p>
              Arbion does not have one complete, exact AI market snapshot for
              every open simulated position. Current price, market value, and
              profit or loss are left unavailable rather than inferred.
            </p>
          </div>
        ) : (
          <>
            <div className="paper-performance-grid">
              <article>
                <span>Simulated equity</span>
                <strong>
                  {money(
                    performance.simulatedEquity ?? portfolio.cash,
                    portfolio.currency,
                  )}
                </strong>
              </article>
              <article>
                <span>Total simulated P&amp;L</span>
                <strong
                  className={performanceClass(performance.totalProfitLoss)}
                >
                  {signedMoney(
                    performance.totalProfitLoss ?? "0",
                    portfolio.currency,
                  )}
                </strong>
              </article>
              <article>
                <span>Return since Paper launch</span>
                <strong
                  className={performanceClass(performance.totalReturnPercent)}
                >
                  {signedPercent(performance.totalReturnPercent)}
                </strong>
              </article>
              <article>
                <span>Invested exposure</span>
                <strong>
                  {money(
                    performance.investedExposure ?? "0",
                    portfolio.currency,
                  )}
                </strong>
              </article>
            </div>
            <p className="paper-market-source">
              {primaryMarket && performance.valuedAt
                ? `${marketSource(primaryMarket)} · oldest required snapshot ${new Date(performance.valuedAt).toLocaleString()}`
                : "Cash-only Paper ledger · no market valuation required"}
              . This is the latest complete immutable AI market snapshot, not a
              live quote or connected-account valuation.
            </p>
          </>
        )}
      </div>
      <div
        className="paper-performance-card"
        aria-label="Exact Paper realized outcome"
      >
        <header className="paper-performance-header">
          <div>
            <p className="eyebrow">PAPER REALIZED OUTCOME · SIMULATION ONLY</p>
            <h3>Immutable average-cost evidence</h3>
          </div>
          <span>{realizedAvailable ? "Exact fill replay" : "Unavailable"}</span>
        </header>
        {realizedAvailable ? (
          <>
            <div className="paper-performance-grid">
              <article>
                <span>Realized simulated P&amp;L</span>
                <strong
                  className={performanceClass(
                    realized!.total_realized_profit_loss,
                  )}
                >
                  {signedMoney(
                    realized!.total_realized_profit_loss ?? "0",
                    portfolio.currency,
                  )}
                </strong>
              </article>
              <article>
                <span>Immutable fills replayed</span>
                <strong>{realized!.fill_count}</strong>
              </article>
              <article>
                <span>Simulated sales</span>
                <strong>{realized!.sell_fill_count}</strong>
              </article>
              <article>
                <span>Coverage</span>
                <strong>Portfolio genesis → now</strong>
              </article>
            </div>
            {(realized!.symbols ?? []).length > 0 ? (
              <div className="paper-position-table-wrap">
                <table
                  className="paper-position-table"
                  aria-label="Exact per-symbol Paper realized outcomes"
                >
                  <thead>
                    <tr>
                      <th>Symbol</th>
                      <th>Realized simulated P&amp;L</th>
                      <th>Buy / sale fills</th>
                      <th>Ending quantity</th>
                      <th>Ending average cost</th>
                    </tr>
                  </thead>
                  <tbody>
                    {realized!.symbols.map((symbol) => (
                      <tr key={`${symbol.instrument}:${symbol.symbol}`}>
                        <td>{symbol.symbol}</td>
                        <td>
                          <span
                            className={performanceClass(
                              symbol.realized_profit_loss,
                            )}
                          >
                            {signedMoney(
                              symbol.realized_profit_loss,
                              portfolio.currency,
                            )}
                          </span>
                        </td>
                        <td>
                          {symbol.buy_fill_count} / {symbol.sell_fill_count}
                        </td>
                        <td>{quantity(symbol.ending_position_quantity)}</td>
                        <td>
                          {symbol.ending_average_cost
                            ? money(
                                symbol.ending_average_cost,
                                portfolio.currency,
                              )
                            : "Closed"}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            ) : (
              <p className="security-note">
                No simulated fills have been recorded; exact realized outcome is
                $0.
              </p>
            )}
            <p className="paper-market-source">
              Exact average-cost replay includes simulated buy and sale fees.
              This is Arbion&apos;s isolated Paper ledger—not broker-reported
              profit and loss, a connected-account balance, or live authority.
            </p>
          </>
        ) : (
          <div className="paper-performance-unavailable">
            <strong>Realized outcome unavailable</strong>
            <p>
              Arbion could not prove one complete immutable fill chain from
              portfolio genesis. The value is left unavailable rather than
              reconstructed or inferred.
            </p>
          </div>
        )}
      </div>
      <details
        className="paper-performance-card"
        aria-label="Exact Paper execution costs"
        open={!executionCostsAvailable}
      >
        <summary className="paper-performance-header">
          <span>
            <span className="eyebrow">
              PAPER EXECUTION COSTS · SIMULATION ONLY
            </span>
            <strong>Exact all-in cost rate + turnover</strong>
          </span>
          <span>
            {executionCostsAvailable
              ? `${money(executionCosts!.total_explicit_cost!, portfolio.currency)} · ${formatExactDecimal(executionCosts!.all_in_cost_rate_bps, { maximumFractionDigits: 4, suffix: " bps" })}`
              : "Unavailable"}
          </span>
        </summary>
        {executionCostsAvailable ? (
          <>
            <div className="paper-performance-grid">
              <article>
                <span>Total explicit simulated cost</span>
                <strong>
                  {money(
                    executionCosts!.total_explicit_cost!,
                    portfolio.currency,
                  )}
                </strong>
              </article>
              <article>
                <span>All-in cost rate</span>
                <strong>
                  {formatExactDecimal(executionCosts!.all_in_cost_rate_bps, {
                    maximumFractionDigits: 4,
                    suffix: " bps",
                  })}
                </strong>
              </article>
              <article>
                <span>Provider-reference turnover</span>
                <strong>
                  {money(
                    executionCosts!.provider_reference_notional!,
                    portfolio.currency,
                  )}
                </strong>
              </article>
              <article>
                <span>Total simulated fees</span>
                <strong>
                  {money(executionCosts!.total_fees!, portfolio.currency)}
                </strong>
              </article>
              <article>
                <span>Adverse simulated slippage</span>
                <strong>
                  {money(
                    executionCosts!.total_adverse_slippage!,
                    portfolio.currency,
                  )}
                </strong>
              </article>
              <article>
                <span>Gross simulated notional</span>
                <strong>
                  {money(executionCosts!.gross_notional!, portfolio.currency)}
                </strong>
              </article>
              <article>
                <span>Buy / sale fills</span>
                <strong>
                  {executionCosts!.buy_fill_count} /{" "}
                  {executionCosts!.sell_fill_count}
                </strong>
              </article>
            </div>
            {executionCosts!.sides.length > 0 ? (
              <div className="paper-position-table-wrap">
                <table
                  className="paper-position-table"
                  aria-label="Exact buy-versus-sale Paper execution costs"
                >
                  <thead>
                    <tr>
                      <th>Side</th>
                      <th>Explicit cost</th>
                      <th>Cost rate</th>
                      <th>Reference turnover</th>
                      <th>Fills</th>
                    </tr>
                  </thead>
                  <tbody>
                    {executionCosts!.sides.map((side) => (
                      <tr key={side.side}>
                        <td>{side.side === "SELL" ? "SALE" : side.side}</td>
                        <td>
                          {money(side.total_explicit_cost, portfolio.currency)}
                        </td>
                        <td>
                          {formatExactDecimal(side.all_in_cost_rate_bps, {
                            maximumFractionDigits: 4,
                            suffix: " bps",
                          })}
                        </td>
                        <td>
                          {money(
                            side.provider_reference_notional,
                            portfolio.currency,
                          )}
                        </td>
                        <td>{side.fill_count}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            ) : null}
            {executionCosts!.symbols.length > 0 ? (
              <div className="paper-position-table-wrap">
                <table
                  className="paper-position-table"
                  aria-label="Exact per-symbol Paper execution costs"
                >
                  <thead>
                    <tr>
                      <th>Symbol</th>
                      <th>Explicit cost</th>
                      <th>Cost rate</th>
                      <th>Reference turnover</th>
                      <th>Buy / sale fills</th>
                    </tr>
                  </thead>
                  <tbody>
                    {executionCosts!.symbols.map((symbol) => (
                      <tr key={`${symbol.instrument}:${symbol.symbol}`}>
                        <td>{symbol.symbol}</td>
                        <td>
                          {money(
                            symbol.total_explicit_cost,
                            portfolio.currency,
                          )}
                        </td>
                        <td>
                          {formatExactDecimal(symbol.all_in_cost_rate_bps, {
                            maximumFractionDigits: 4,
                            suffix: " bps",
                          })}
                        </td>
                        <td>
                          {money(
                            symbol.provider_reference_notional,
                            portfolio.currency,
                          )}
                        </td>
                        <td>
                          {symbol.buy_fill_count} / {symbol.sell_fill_count}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            ) : (
              <p className="security-note">
                No simulated fills have been recorded; exact execution costs are
                $0.
              </p>
            )}
            <p className="paper-market-source">
              Exact immutable simulation evidence from portfolio genesis. Fees
              and adverse slippage versus each saved provider reference stay
              separate from realized, unrealized, and total outcomes. Sources:{" "}
              {executionCosts!.market_providers.join(", ") || "none yet"}
              {executionCosts!.market_feeds.length
                ? ` · ${executionCosts!.market_feeds.join(", ")}`
                : ""}
              {executionCosts!.market_qualities.length
                ? ` · ${executionCosts!.market_qualities.join(", ")}`
                : ""}
              . These are simulated costs—not broker-reported charges or live
              execution.
            </p>
            <p className="paper-market-source">
              Stored fill-notional rounding residual:{" "}
              {money(
                executionCosts!.fill_notional_residual!,
                portfolio.currency,
              )}
              {" · maximum absolute per fill "}
              {money(
                executionCosts!.maximum_absolute_fill_residual!,
                portfolio.currency,
              )}
              {" · strict per-fill bound "}
              {money(
                executionCosts!.residual_bound_per_fill!,
                portfolio.currency,
              )}
              . The all-in rate includes only saved simulated fees and adverse
              slippage. It does not infer spread, market impact, broker fees,
              performance, or causality.
            </p>
          </>
        ) : (
          <div className="paper-performance-unavailable">
            <strong>Execution-cost attribution unavailable</strong>
            <p>
              Arbion could not prove one complete immutable simulation fill and
              market-attribution chain. Fees and slippage are left unavailable
              rather than estimated.
            </p>
          </div>
        )}
      </details>
      <details
        className="paper-performance-card"
        aria-label="Exact Paper outcome reconciliation"
        open={!reconciliationAvailable || reconciliationAttention}
      >
        <summary className="paper-performance-header">
          <span>
            <span className="eyebrow">
              PAPER OUTCOME RECONCILIATION · SAVED EVIDENCE
            </span>
            <strong>Two independent accounting paths</strong>
          </span>
          <span>
            {reconciliation.status === "RECONCILED_EXACT"
              ? "Exact match"
              : reconciliation.status === "RECONCILED_WITH_DECIMAL_RESIDUAL"
                ? "Reconciled · residual disclosed"
                : reconciliationAttention
                  ? "Review required"
                  : "Unavailable"}
          </span>
        </summary>
        {reconciliationAvailable ? (
          <>
            <div className="paper-performance-grid">
              <article>
                <span>Realized fill outcome</span>
                <strong
                  className={performanceClass(
                    reconciliation.realizedProfitLoss,
                  )}
                >
                  {signedMoney(
                    reconciliation.realizedProfitLoss ?? "0",
                    portfolio.currency,
                  )}
                </strong>
              </article>
              <article>
                <span>Unrealized marked outcome</span>
                <strong
                  className={performanceClass(
                    reconciliation.unrealizedProfitLoss,
                  )}
                >
                  {signedMoney(
                    reconciliation.unrealizedProfitLoss ?? "0",
                    portfolio.currency,
                  )}
                </strong>
              </article>
              <article>
                <span>Realized + unrealized</span>
                <strong
                  className={performanceClass(
                    reconciliation.classifiedProfitLoss,
                  )}
                >
                  {signedMoney(
                    reconciliation.classifiedProfitLoss ?? "0",
                    portfolio.currency,
                  )}
                </strong>
              </article>
              <article>
                <span>Equity − starting cash</span>
                <strong
                  className={performanceClass(reconciliation.totalProfitLoss)}
                >
                  {signedMoney(
                    reconciliation.totalProfitLoss ?? "0",
                    portfolio.currency,
                  )}
                </strong>
              </article>
              <article>
                <span>Simulated equity</span>
                <strong>
                  {money(
                    reconciliation.simulatedEquity ?? "0",
                    portfolio.currency,
                  )}
                </strong>
                <small>
                  Cash + marked exposure
                  {compareExactDecimals(
                    reconciliation.equityResidual ?? "invalid",
                    "0",
                  ) === 0
                    ? " matches exactly"
                    : " does not match"}
                </small>
              </article>
              <article>
                <span>Disclosed decimal residual</span>
                <strong
                  className={
                    reconciliationAttention
                      ? "is-negative"
                      : performanceClass(reconciliation.outcomeResidual)
                  }
                >
                  {signedPreciseMoney(
                    reconciliation.outcomeResidual ?? "0",
                    portfolio.currency,
                  )}
                </strong>
                <small>
                  Strict review bound ±
                  {signedPreciseMoney(
                    reconciliation.residualLimit,
                    portfolio.currency,
                  ).replace("+", "")}
                </small>
              </article>
            </div>
            <p className="paper-market-source">
              {reconciliationAttention
                ? "The two saved outcome paths differ beyond Arbion's strict decimal residual bound. The mismatch is visible and requires review; no value is substituted."
                : "The exact fill replay and the immutable market valuation reconcile. Any sub-cent decimal residual remains separate and visible; it is never folded into realized or unrealized P&L."}
              {reconciliation.provider
                ? " Provider " + reconciliation.provider
                : " Provider attribution unavailable"}
              {reconciliation.marketFeeds.length > 0
                ? " · " + reconciliation.marketFeeds.join(", ")
                : ""}
              {reconciliation.marketQualities.length > 0
                ? " · " + reconciliation.marketQualities.join(", ")
                : ""}
              {reconciliation.valuedAt
                ? " · oldest required market observation " +
                  new Date(reconciliation.valuedAt).toLocaleString()
                : ""}
              . Simulation only; no broker balance, model rerun, order, or live
              authority.
            </p>
          </>
        ) : (
          <div className="paper-performance-unavailable">
            <strong>Outcome reconciliation unavailable</strong>
            <p>
              Arbion could not prove both an exact immutable realized fill
              replay and one complete saved market valuation. Realized,
              unrealized, total outcome, or equity evidence is left unavailable
              rather than inferred.
            </p>
          </div>
        )}
      </details>
      <PaperPerformanceHistory
        decisions={decisions}
        startingCash={portfolio.starting_cash}
        currency={portfolio.currency}
      />
      <div className="paper-portfolio-summary">
        <p>
          <strong>Starting cash</strong>
          {money(portfolio.starting_cash, portfolio.currency)}
        </p>
        <p>
          <strong>Open positions</strong>
          {openPositions.length}
        </p>
        <p>
          <strong>Ledger version</strong>
          {portfolio.version}
        </p>
      </div>
      {positions.length > 0 ? (
        <div className="paper-position-table-wrap">
          <table
            className="paper-position-table"
            aria-label="Simulated paper positions"
          >
            <thead>
              <tr>
                <th>Status</th>
                <th>Symbol</th>
                <th>Position</th>
                <th>Quantity</th>
                <th>Average simulation price</th>
                <th>Latest AI snapshot price</th>
                <th>Simulated market value</th>
                <th>Unrealized P&amp;L</th>
                <th>24h market move</th>
              </tr>
            </thead>
            <tbody>
              {positions.map((position, index) => {
                const valuation = valuations.get(
                  `${position.instrument}:${position.symbol}`,
                );
                return (
                  <tr
                    key={`${position.instrument}-${position.symbol}-${position.option_type ?? "equity"}-${position.expiration ?? "none"}-${index}`}
                  >
                    <td>
                      <span
                        className={`paper-position-status ${position.is_open ? "is-open" : "is-closed"}`}
                      >
                        {position.is_open ? "Open" : "Closed"}
                      </span>
                    </td>
                    <td>{position.symbol}</td>
                    <td>{contract(position, portfolio.currency)}</td>
                    <td>{quantity(position.quantity)}</td>
                    <td>{money(position.average_price, portfolio.currency)}</td>
                    <td>
                      {valuation ? (
                        <>
                          {money(valuation.price, portfolio.currency)} ·{" "}
                          {valuation.priceBasis.toLowerCase()}
                          <small className="paper-position-source">
                            {marketSource(valuation)} ·{" "}
                            {new Date(valuation.observedAt).toLocaleString()}
                          </small>
                        </>
                      ) : (
                        "Unavailable"
                      )}
                    </td>
                    <td>
                      {valuation
                        ? money(valuation.marketValue, portfolio.currency)
                        : "Unavailable"}
                    </td>
                    <td>
                      {valuation ? (
                        <span
                          className={performanceClass(
                            valuation.unrealizedProfitLoss,
                          )}
                        >
                          {signedMoney(
                            valuation.unrealizedProfitLoss,
                            portfolio.currency,
                          )}{" "}
                          ·{" "}
                          {signedPercent(valuation.unrealizedProfitLossPercent)}
                        </span>
                      ) : (
                        "Unavailable"
                      )}
                    </td>
                    <td>
                      {valuation ? (
                        <span
                          className={performanceClass(
                            valuation.changePercent24H,
                          )}
                        >
                          {signedPercent(valuation.changePercent24H)}
                        </span>
                      ) : (
                        "Unavailable"
                      )}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      ) : (
        <p className="security-note">
          No simulated positions have been created for this strategy yet.
        </p>
      )}
      <div className="paper-fill-ledger">
        <p className="eyebrow">AI PAPER FILL LEDGER · IMMUTABLE</p>
        <h3>Simulated spot executions</h3>
        <p>
          Each row records the exact provider market reference, deterministic
          slippage, and fee used by Arbion. It is simulation evidence—not a
          broker order or account transaction.
        </p>
        {fills.length > 0 ? (
          <div className="paper-position-table-wrap">
            <table
              className="paper-position-table"
              aria-label="Immutable AI Paper simulated fills"
            >
              <thead>
                <tr>
                  <th>Time</th>
                  <th>Action</th>
                  <th>Quantity</th>
                  <th>Market → simulated fill</th>
                  <th>Fee</th>
                  <th>Market source</th>
                </tr>
              </thead>
              <tbody>
                {fills.map((fill) => (
                  <tr key={fill.id}>
                    <td>{new Date(fill.simulated_at).toLocaleString()}</td>
                    <td>
                      {fill.side} {fill.symbol}
                    </td>
                    <td>{quantity(fill.quantity)}</td>
                    <td>
                      {money(fill.reference_price, portfolio.currency)} →{" "}
                      {money(fill.fill_price, portfolio.currency)}
                    </td>
                    <td>{money(fill.fee, portfolio.currency)}</td>
                    <td>
                      {fill.market_provider} · {fill.market_feed} ·{" "}
                      {fill.market_quality}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : (
          <p className="security-note">
            No AI Paper simulated fills exist for this strategy yet.
          </p>
        )}
      </div>
    </section>
  );
}
