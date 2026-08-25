type RawScore = Record<string, unknown>;

function read(record: RawScore, key: string, legacy: string) {
  return record[key] ?? record[legacy];
}

function number(record: RawScore, key: string, legacy: string) {
  const value = Number(read(record, key, legacy) ?? 0);
  return Number.isFinite(value) ? value : 0;
}

function decimal(value: unknown) {
  if (typeof value !== "string" || !/^-?\d+(\.\d+)?$/.test(value)) return "—";
  const [whole, fraction = ""] = value.split(".");
  const trimmed = fraction.replace(/0+$/, "");
  return trimmed ? `${whole}.${trimmed}` : whole;
}

function signedPercent(value: unknown) {
  const formatted = decimal(value);
  if (formatted === "—") return formatted;
  if (formatted.startsWith("-") || formatted === "0") return `${formatted}%`;
  return `+${formatted}%`;
}

function horizonLabel(value: unknown) {
  return value === "TWENTY_FOUR_HOURS" ? "24-hour horizon" : "1-hour horizon";
}

export function AIShadowScorecard({ scorecard }: { scorecard?: RawScore }) {
  const horizons = Array.isArray(scorecard?.horizons)
    ? (scorecard.horizons as RawScore[])
    : Array.isArray(scorecard?.Horizons)
      ? (scorecard.Horizons as RawScore[])
      : [];
  const totalMarks = scorecard
    ? number(scorecard, "total_marks", "TotalMarks")
    : 0;

  return (
    <section className="shadow-scorecard" aria-label="AI shadow scorecard">
      <p className="eyebrow">SHADOW EVIDENCE SCORECARD</p>
      <h2>How hypothetical decisions moved</h2>
      <p>
        Favorable means the market later moved in the proposal&apos;s direction.
        Horizons stay separate, and every figure comes from immutable SHADOW
        marks.
      </p>
      <p className="shadow-scorecard-total">
        <strong>{totalMarks}</strong> total horizon mark
        {totalMarks === 1 ? "" : "s"}
      </p>

      {horizons.length === 0 ? (
        <p className="security-note">
          No outcome scorecard is available yet. Arbion never estimates missing
          marks.
        </p>
      ) : (
        <div className="shadow-scorecard-grid">
          {horizons.map((horizon, index) => {
            const sampleSize = number(horizon, "sample_size", "SampleSize");
            const minimum = number(
              horizon,
              "minimum_sample_for_observational_label",
              "MinimumSampleForObservationalLabel",
            );
            const interpretation = String(
              read(horizon, "interpretation", "Interpretation") ??
                "INSUFFICIENT_SAMPLE",
            );
            const favorableRate = read(
              horizon,
              "favorable_rate_percent",
              "FavorableRatePercent",
            );
            const averageChange = read(
              horizon,
              "average_directional_change_percent",
              "AverageDirectionalChangePercent",
            );
            const horizonValue = read(horizon, "horizon", "Horizon");
            return (
              <article key={String(horizonValue ?? index)}>
                <header>
                  <h3>{horizonLabel(horizonValue)}</h3>
                  <span>
                    {interpretation === "OBSERVATIONAL"
                      ? "Observational only"
                      : "Early evidence"}
                  </span>
                </header>
                <p className="shadow-scorecard-movement">
                  <strong>{signedPercent(averageChange)}</strong>
                  <small>average directional movement</small>
                </p>
                {sampleSize === 0 ? (
                  <p>Awaiting the first fresh mark at this horizon.</p>
                ) : (
                  <dl>
                    <div>
                      <dt>Samples</dt>
                      <dd>{sampleSize}</dd>
                    </div>
                    <div>
                      <dt>Favorable</dt>
                      <dd>
                        {number(horizon, "favorable_marks", "FavorableMarks")}
                      </dd>
                    </div>
                    <div>
                      <dt>Unfavorable</dt>
                      <dd>
                        {number(
                          horizon,
                          "unfavorable_marks",
                          "UnfavorableMarks",
                        )}
                      </dd>
                    </div>
                    <div>
                      <dt>Flat</dt>
                      <dd>{number(horizon, "flat_marks", "FlatMarks")}</dd>
                    </div>
                    <div>
                      <dt>Observed favorable rate</dt>
                      <dd>{signedPercent(favorableRate).replace("+", "")}</dd>
                    </div>
                  </dl>
                )}
                <p>
                  {sampleSize} of {minimum || 20} marks before Arbion changes
                  the label from early evidence to observational.
                </p>
              </article>
            );
          })}
        </div>
      )}

      <p className="security-note">
        This is hypothetical directional evidence—not prediction accuracy, a
        fill, realized return, account P&amp;L, or permission to trade.
      </p>
    </section>
  );
}
