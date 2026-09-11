import Link from "next/link";

export function AutomationsHeader() {
  return (
    <section className="strategy-fleet-hero">
      <div>
        <p className="eyebrow">YOUR ENGINES</p>
        <h1 id="automations-page-title">AI Operations</h1>
        <p className="lede">
          Follow your engines, review saved decisions, and see what runs next.
        </p>
      </div>
      <div className="strategy-fleet-actions">
        <Link className="button-link" href="/automations/new">
          Set up AI engine
        </Link>
        <div className="strategy-fleet-secondary-actions">
          <Link href="/capital">Capital budgets</Link>
          <Link href="/activity">Decision journal</Link>
        </div>
      </div>
    </section>
  );
}
