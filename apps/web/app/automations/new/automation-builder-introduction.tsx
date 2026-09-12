export function AutomationBuilderIntroduction() {
  return (
    <section className="automation-builder-intro">
      <p className="eyebrow">AUTOMATIONS · NEW ENGINE</p>
      <h1 id="automation-builder-title">Create an AI engine</h1>
      <p>
        Choose an account, model and boundaries for Paper simulation or
        observation-only Shadow.
      </p>
      <nav className="automation-builder-links" aria-label="Setup sections">
        <a href="#builder-context">
          <span>01</span> Account &amp; model
        </a>
        <a href="#builder-mandate">
          <span>02</span> Mandate
        </a>
        <a href="#builder-guardrails">
          <span>03</span> Risk limits
        </a>
        <a href="#builder-capital">
          <span>04</span> Capital &amp; schedule
        </a>
      </nav>
    </section>
  );
}
