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

export type PaperPortfolio = {
  strategy_instance_id: string;
  currency: string;
  starting_cash: string;
  cash: string;
  version: number;
  positions: PaperPosition[];
  updated_at: string;
};

function money(value: string, currency: string) {
  const amount = Number(value);
  if (!Number.isFinite(amount)) return `${value} ${currency}`;
  try {
    return new Intl.NumberFormat("en-US", {
      style: "currency",
      currency,
      minimumFractionDigits: 2,
      maximumFractionDigits: 4,
    }).format(amount);
  } catch {
    return `${value} ${currency}`;
  }
}

function quantity(value: string) {
  const amount = Number(value);
  if (!Number.isFinite(amount)) return value;
  return new Intl.NumberFormat("en-US", {
    maximumFractionDigits: 4,
  }).format(amount);
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
}: {
  portfolio?: PaperPortfolio;
  executionMode: string;
  fills?: AIPaperSpotFill[];
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
  return (
    <section className="paper-portfolio-card" aria-label="Paper portfolio">
      <p className="eyebrow">PAPER PORTFOLIO · SIMULATION ONLY</p>
      <h2>{money(portfolio.cash, portfolio.currency)} simulated cash</h2>
      <p>
        This is Arbion&apos;s isolated simulation ledger. These amounts and
        positions are not connected-account balances or provider holdings.
      </p>
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
              </tr>
            </thead>
            <tbody>
              {positions.map((position, index) => (
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
                </tr>
              ))}
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
