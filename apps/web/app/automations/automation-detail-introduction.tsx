export function automationDetailTitle(
  automationType: string,
  executionMode: string,
) {
  if (automationType !== "AI_AUTONOMOUS") return automationType || "Automation";
  if (executionMode === "PAPER") return "AI Paper Engine";
  if (executionMode === "SHADOW") return "AI Shadow Engine";
  return "AI Engine";
}

export function AutomationDetailIntroduction({
  automationType = "",
  executionMode = "",
  available = true,
}: {
  automationType?: string;
  executionMode?: string;
  available?: boolean;
}) {
  return (
    <section className="automation-detail-intro" id="automation-overview">
      <p className="eyebrow">AUTOMATION · SAVED CONFIGURATION</p>
      <h1 id="automation-page-title">
        {available
          ? automationDetailTitle(automationType, executionMode)
          : "Mandate unavailable"}
      </h1>
      {available && (
        <>
          <p>
            Review the saved setup and the next step. Operating controls below
            retain their existing checks.
          </p>
          <nav
            className="automation-detail-links"
            aria-label="Automation sections"
          >
            <a href="#automation-overview">Overview</a>
            <a href="#mandate-identity">Account &amp; setup</a>
            <a href="#automation-next-step">Next step</a>
          </nav>
        </>
      )}
    </section>
  );
}
