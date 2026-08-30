import {
  extractPaperPerformanceHistory,
  type PaperPerformanceHistoryPoint,
} from "./paper-performance";

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

function signedMoney(value: string, currency: string) {
  const amount = Number(value);
  if (!Number.isFinite(amount) || amount === 0) return money(value, currency);
  return `${amount > 0 ? "+" : "-"}${money(String(Math.abs(amount)), currency)}`;
}

function signedPercent(value: string) {
  const amount = Number(value);
  if (!Number.isFinite(amount)) return `${value}%`;
  return `${amount > 0 ? "+" : ""}${new Intl.NumberFormat("en-US", {
    minimumFractionDigits: 2,
    maximumFractionDigits: 4,
  }).format(amount)}%`;
}

function performanceClass(value: string) {
  const amount = Number(value);
  if (!Number.isFinite(amount) || amount === 0) return "is-flat";
  return amount > 0 ? "is-positive" : "is-negative";
}

function chart(points: PaperPerformanceHistoryPoint[], startingCash: string) {
  const width = 760;
  const height = 190;
  const padding = 18;
  const values = points.map((point) => Number(point.simulatedEquity));
  const baseline = Number(startingCash);
  if (
    values.length === 0 ||
    values.some((value) => !Number.isFinite(value)) ||
    !Number.isFinite(baseline)
  ) {
    return;
  }
  const low = Math.min(...values, baseline);
  const high = Math.max(...values, baseline);
  const spread = high - low || Math.max(Math.abs(high) * 0.002, 1);
  const x = (index: number) =>
    points.length === 1
      ? width / 2
      : padding + (index * (width - padding * 2)) / (points.length - 1);
  const y = (value: number) =>
    padding + ((high - value) * (height - padding * 2)) / spread;
  return {
    width,
    height,
    baselineY: y(baseline),
    line: values.map((value, index) => `${x(index)},${y(value)}`).join(" "),
    dots: values.map((value, index) => ({
      x: x(index),
      y: y(value),
      point: points[index],
    })),
  };
}

export function PaperPerformanceHistory({
  decisions,
  startingCash,
  currency,
}: {
  decisions: Record<string, unknown>[];
  startingCash: string;
  currency: string;
}) {
  const history = extractPaperPerformanceHistory(decisions, startingCash);
  const latest = history.points.at(-1);
  const plot = chart(history.points, startingCash);
  const recent = history.points.slice(-8).reverse();

  return (
    <section
      className="paper-performance-history"
      aria-label="Decision-time Paper performance history"
    >
      <header className="paper-performance-header">
        <div>
          <p className="eyebrow">PAPER HISTORY · IMMUTABLE AI EVIDENCE</p>
          <h3>Decision-time performance</h3>
        </div>
        <span>{history.points.length} exact marks</span>
      </header>
      {!latest || !plot ? (
        <div className="paper-performance-unavailable">
          <strong>Performance history is collecting</strong>
          <p>
            Arbion will add a point after a successful scheduled AI Paper
            decision contains complete simulated-ledger and provider-market
            evidence. Missing evidence is never estimated.
          </p>
        </div>
      ) : (
        <>
          <div className="paper-performance-grid paper-history-metrics">
            <article>
              <span>Latest decision-time equity</span>
              <strong>{money(latest.simulatedEquity, currency)}</strong>
            </article>
            <article>
              <span>P&amp;L since Paper launch</span>
              <strong className={performanceClass(latest.totalProfitLoss)}>
                {signedMoney(latest.totalProfitLoss, currency)}
              </strong>
            </article>
            <article>
              <span>Return since Paper launch</span>
              <strong className={performanceClass(latest.totalReturnPercent)}>
                {signedPercent(latest.totalReturnPercent)}
              </strong>
            </article>
            <article>
              <span>Provider context</span>
              <strong>{latest.provider}</strong>
            </article>
          </div>
          <div className="paper-history-chart-wrap">
            <svg
              className="paper-history-chart"
              viewBox={`0 0 ${plot.width} ${plot.height}`}
              role="img"
              aria-label={`${history.points.length} decision-time simulated equity marks`}
            >
              <title>Decision-time simulated Paper equity</title>
              <line
                className="paper-history-baseline"
                x1="18"
                y1={plot.baselineY}
                x2={plot.width - 18}
                y2={plot.baselineY}
              />
              {history.points.length > 1 ? (
                <polyline className="paper-history-line" points={plot.line} />
              ) : null}
              {plot.dots.map(({ x, y, point }) => (
                <circle
                  className="paper-history-dot"
                  key={point.decisionId}
                  cx={x}
                  cy={y}
                  r="4.5"
                />
              ))}
            </svg>
            <div className="paper-history-axis-copy">
              <span>
                {new Date(history.points[0].decisionAt).toLocaleString()}
              </span>
              <span>{new Date(latest.decisionAt).toLocaleString()}</span>
            </div>
          </div>
          <p className="paper-market-source">
            Each point uses one immutable AI Decision Journal input snapshot and
            the isolated Paper ledger as it existed before that decision. It is
            neither a live quote nor a connected-account valuation.
          </p>
          <div className="paper-position-table-wrap">
            <table
              className="paper-position-table paper-history-table"
              aria-label="Immutable decision-time Paper performance marks"
            >
              <thead>
                <tr>
                  <th>Decision time</th>
                  <th>AI disposition</th>
                  <th>Simulated equity</th>
                  <th>Total P&amp;L</th>
                  <th>Market evidence</th>
                </tr>
              </thead>
              <tbody>
                {recent.map((point) => (
                  <tr key={point.decisionId}>
                    <td>{new Date(point.decisionAt).toLocaleString()}</td>
                    <td>
                      {point.decision === "ABSTAIN"
                        ? "Abstained"
                        : `${point.side ?? "Proposed"} ${point.symbol ?? ""}`.trim()}
                    </td>
                    <td>{money(point.simulatedEquity, currency)}</td>
                    <td className={performanceClass(point.totalProfitLoss)}>
                      {signedMoney(point.totalProfitLoss, currency)} ·{" "}
                      {signedPercent(point.totalReturnPercent)}
                    </td>
                    <td>
                      {point.provider} · {point.marketCount} market
                      {point.marketCount === 1 ? "" : "s"}
                      <small className="paper-position-source">
                        oldest required snapshot{" "}
                        {new Date(point.marketObservedAt).toLocaleString()}
                      </small>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </>
      )}
      {history.unavailableDecisionCount > 0 ? (
        <p className="paper-history-coverage-gap">
          {history.unavailableDecisionCount} Paper decision
          {history.unavailableDecisionCount === 1 ? " was" : "s were"} omitted
          because complete exact valuation evidence was unavailable.
        </p>
      ) : null}
    </section>
  );
}
