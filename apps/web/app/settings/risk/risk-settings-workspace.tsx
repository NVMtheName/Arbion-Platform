import { AppPageHeader } from "../../app-page-header";
import {
  UserCircuitBreakerControls,
  type UserCircuitBreaker,
} from "./user-circuit-breaker-controls";

export function RiskSettingsWorkspace({
  breaker,
}: {
  breaker: UserCircuitBreaker | null | undefined;
}) {
  return (
    <main className="dashboard-shell risk-settings-page command-content-continuity">
      <AppPageHeader contentHeadingId="risk-settings-title" />
      <section className="risk-settings-intro">
        <p className="eyebrow">SAFETY CONTROLS</p>
        <h1 id="risk-settings-title">Risk &amp; safety</h1>
        <p>
          These controls prevent authorization of new automated actions. They
          never close positions or submit a trade.
        </p>
        <nav className="risk-settings-links" aria-label="Safety sections">
          <a href="#owner-wide-stop">Owner-wide stop</a>
          <a href="#account-mandate-controls">Account &amp; mandate controls</a>
        </nav>
      </section>
      <div id="owner-wide-stop">
        {breaker !== undefined ? (
          <UserCircuitBreakerControls breaker={breaker} />
        ) : (
          <section className="risk-state-unavailable" role="status">
            <h2>Owner-wide safety control unavailable</h2>
            <p>
              Arbion could not verify the current owner-stop state, so this page
              will not present a potentially stale engage or release action.
            </p>
          </section>
        )}
      </div>
      <section
        className="risk-scope-guidance"
        id="account-mandate-controls"
        aria-labelledby="risk-scope-title"
      >
        <h2 id="risk-scope-title">Account and mandate controls</h2>
        <p>
          Account-scoped breakers are available on each connected account, and
          automation-scoped breakers remain on each automation. All applicable
          stops are evaluated before capital and strategy rules.
        </p>
        <p>These controls do not close positions or grant trading authority.</p>
      </section>
    </main>
  );
}
