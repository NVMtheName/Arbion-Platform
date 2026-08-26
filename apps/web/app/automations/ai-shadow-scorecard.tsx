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

function object(record: RawScore | undefined, key: string, legacy: string) {
  const value = record ? read(record, key, legacy) : undefined;
  return value && typeof value === "object" && !Array.isArray(value)
    ? (value as RawScore)
    : undefined;
}

function strings(value: unknown) {
  return Array.isArray(value)
    ? value.filter((item): item is string => typeof item === "string")
    : [];
}

function blockerLabel(value: string) {
  const labels: Record<string, string> = {
    ONE_HOUR_SAMPLE_INCOMPLETE: "Collect more 1-hour outcome marks",
    TWENTY_FOUR_HOUR_SAMPLE_INCOMPLETE: "Collect more 24-hour outcome marks",
    EVIDENCE_WINDOW_INCOMPLETE: "Observe the mandate across a longer window",
    SCHEDULE_NOT_VERIFIED: "Complete a healthy scheduled cycle",
    SCHEDULE_UNHEALTHY: "Resolve the current scheduler failure",
  };
  return labels[value] ?? "Resolve an unrecognized evidence blocker";
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
  const evidenceGate = object(scorecard, "evidence_gate", "EvidenceGate");
  const evidenceStatus = String(
    evidenceGate
      ? (read(evidenceGate, "status", "Status") ?? "COLLECTING_EVIDENCE")
      : "COLLECTING_EVIDENCE",
  );
  const minimumSample = evidenceGate
    ? number(
        evidenceGate,
        "minimum_sample_per_horizon",
        "MinimumSamplePerHorizon",
      ) || 20
    : 20;
  const minimumWindow = evidenceGate
    ? number(
        evidenceGate,
        "minimum_evidence_window_hours",
        "MinimumEvidenceWindowHours",
      ) || 168
    : 168;
  const gateBlockers = strings(
    evidenceGate ? read(evidenceGate, "blockers", "Blockers") : undefined,
  );

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

      {evidenceGate && (
        <section
          className="shadow-evidence-gate"
          aria-label="Autonomy evidence gate"
        >
          <header>
            <div>
              <p className="eyebrow">AUTONOMY EVIDENCE GATE</p>
              <h3>Is the shadow record mature enough to review?</h3>
            </div>
            <span
              className={
                evidenceStatus === "EVIDENCE_REVIEWABLE" ? "reviewable" : ""
              }
            >
              {evidenceStatus === "EVIDENCE_REVIEWABLE"
                ? "Reviewable evidence"
                : "Collecting evidence"}
            </span>
          </header>
          <dl>
            <div>
              <dt>1-hour marks</dt>
              <dd>
                {number(
                  evidenceGate,
                  "one_hour_sample_size",
                  "OneHourSampleSize",
                )}{" "}
                / {minimumSample}
              </dd>
            </div>
            <div>
              <dt>24-hour marks</dt>
              <dd>
                {number(
                  evidenceGate,
                  "twenty_four_hour_sample_size",
                  "TwentyFourHourSampleSize",
                )}{" "}
                / {minimumSample}
              </dd>
            </div>
            <div>
              <dt>Evidence window</dt>
              <dd>
                {number(
                  evidenceGate,
                  "evidence_window_hours",
                  "EvidenceWindowHours",
                )}{" "}
                / {minimumWindow} hours
              </dd>
            </div>
            <div>
              <dt>Scheduler</dt>
              <dd>
                {read(evidenceGate, "schedule_healthy", "ScheduleHealthy")
                  ? "Healthy"
                  : "Not verified"}
              </dd>
            </div>
          </dl>
          {gateBlockers.length > 0 && (
            <div className="shadow-evidence-blockers">
              <strong>Still needed</strong>
              <ul>
                {gateBlockers.map((blocker) => (
                  <li key={blocker}>{blockerLabel(blocker)}</li>
                ))}
              </ul>
            </div>
          )}
          <p className="security-note">
            Passing this gate makes evidence reviewable. It does not authorize
            live trading, claim profitability, or add a broker-write path.
          </p>
        </section>
      )}

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
