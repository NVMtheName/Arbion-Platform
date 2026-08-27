export type PlatformOperationsSignal = {
  code: string;
  state: "PASS" | "ATTENTION" | "STOPPED";
  count: number;
  summary: string;
};

export type PlatformOperationsOverview = {
  generated_at: string;
  operational_status: "NOMINAL" | "ATTENTION" | "STOPPED";
  active_ai_shadow_instances: number;
  unhealthy_ai_schedules: number;
  unhealthy_ai_reconciliations: number;
  unavailable_financial_connections: number;
  open_global_breakers: number;
  open_scoped_breakers: number;
  execution_boundary: {
    live_mandates: number;
    non_shadow_ai_instances: number;
    non_shadow_ai_executions: number;
    executable_risk_evaluations: number;
    non_executing_ai_proposals: number;
    reviewed_non_executing_proposals: number;
  };
  signals: PlatformOperationsSignal[];
  live_execution_available: false;
  broker_action_requested: false;
};

export function PlatformOperationsReadiness({
  operations,
}: {
  operations: PlatformOperationsOverview;
}) {
  return (
    <section
      className="content-card platform-operations"
      aria-label="Production operations"
    >
      <p className="eyebrow">PRODUCTION OPERATIONS</p>
      <h2>Shadow control plane</h2>
      <p
        className={
          "status-badge platform-operations-status status-" +
          operations.operational_status.toLowerCase()
        }
      >
        {operations.operational_status}
      </p>
      <p>
        Credential-free aggregate evidence for the current production control
        plane. This surface is read-only and cannot contact a broker or enable
        live execution.
      </p>
      <dl className="circuit-breaker-facts platform-operations-metrics">
        <div>
          <dt>Active AI Shadow engines</dt>
          <dd>{operations.active_ai_shadow_instances}</dd>
        </div>
        <div>
          <dt>Schedule issues</dt>
          <dd>{operations.unhealthy_ai_schedules}</dd>
        </div>
        <div>
          <dt>Reconciliation issues</dt>
          <dd>{operations.unhealthy_ai_reconciliations}</dd>
        </div>
        <div>
          <dt>Connection issues</dt>
          <dd>{operations.unavailable_financial_connections}</dd>
        </div>
        <div>
          <dt>Open scoped stops</dt>
          <dd>{operations.open_scoped_breakers}</dd>
        </div>
        <div>
          <dt>Non-executing AI proposals</dt>
          <dd>{operations.execution_boundary.non_executing_ai_proposals}</dd>
        </div>
      </dl>
      <h3>Safety signals</h3>
      <ul className="platform-operations-signals">
        {operations.signals.map((signal) => (
          <li
            key={signal.code}
            className={"signal-" + signal.state.toLowerCase()}
          >
            <strong>{signal.code.replaceAll("_", " ")}</strong>: {signal.state}
            {signal.count > 0 ? " (" + signal.count + ")" : ""}.{" "}
            {signal.summary}
          </li>
        ))}
      </ul>
      <p className="platform-operations-footer">
        Snapshot generated{" "}
        <time dateTime={operations.generated_at}>
          {new Date(operations.generated_at).toUTCString()}
        </time>
        . Live execution remains unavailable.
      </p>
    </section>
  );
}
