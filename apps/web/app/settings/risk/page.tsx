export default function RiskSafetyPage() {
  return (
    <main className="dashboard-shell">
      <section className="hero-panel">
        <p className="eyebrow">Safety controls</p>
        <h1>Risk / Control status</h1>
        <p>
          These controls prevent authorization of new automated actions. They
          never close positions or submit a trade.
        </p>
      </section>
      <section className="content-card">
        <h2>User automation status</h2>
        <p className="status-badge">ACTIVE</p>
        <button type="button" disabled>
          PAUSE ALL AUTOMATION
        </button>
        <p>
          This will prevent Arbion from authorizing new automated actions. It
          will not close existing positions.
        </p>
      </section>
      <section className="content-card">
        <h2>Account and mandate controls</h2>
        <p>
          Account-scoped breakers and mandate PAUSED status are evaluated before
          all risk rules.
        </p>
        <p>No trading controls are available on this page.</p>
      </section>
    </main>
  );
}
