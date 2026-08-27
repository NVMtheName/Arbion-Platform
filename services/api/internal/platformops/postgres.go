package platformops

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresStore struct{ db *pgxpool.Pool }

func NewPostgresStore(db *pgxpool.Pool) *PostgresStore { return &PostgresStore{db: db} }

func (store *PostgresStore) ActiveSuperadmin(ctx context.Context, userID string) (bool, error) {
	var active bool
	err := store.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE id=$1 AND role='superadmin' AND status='active')`, userID).Scan(&active)
	return active, err
}

func (store *PostgresStore) Snapshot(ctx context.Context, observedAt time.Time) (facts Facts, err error) {
	err = store.db.QueryRow(ctx, `
WITH active_ai AS (
  SELECT i.id,i.user_id,i.financial_account_id,a.status AS account_status,p.status AS connection_status
  FROM strategy_instances i
  JOIN automation_mandates m ON m.id=i.automation_mandate_id
  JOIN financial_accounts a ON a.id=i.financial_account_id AND a.user_id=i.user_id
  JOIN provider_connections p ON p.id=a.provider_connection_id AND p.user_id=i.user_id AND p.provider_category='financial'
  WHERE i.status='ACTIVE' AND i.execution_mode='SHADOW' AND m.automation_type='AI_AUTONOMOUS'
), latest_reconciliation AS (
  SELECT DISTINCT ON (user_id,financial_account_id)
    user_id,financial_account_id,comparison_status,balances_status,positions_status,
    autonomy_enforcement_active,blocks_new_actions,observed_at
  FROM portfolio_reconciliations
  ORDER BY user_id,financial_account_id,observed_at DESC,id DESC
)
SELECT
  (SELECT count(*) FROM active_ai),
  (SELECT count(*) FROM active_ai a LEFT JOIN nonlive_strategy_schedules s ON s.strategy_instance_id=a.id
    WHERE s.strategy_instance_id IS NULL OR s.consecutive_failures>0 OR s.last_status='FAILED'),
  (SELECT count(*) FROM active_ai a LEFT JOIN latest_reconciliation r ON r.user_id=a.user_id AND r.financial_account_id=a.financial_account_id
    WHERE r.financial_account_id IS NULL OR r.comparison_status<>'MATCHED' OR r.balances_status<>'READY' OR r.positions_status<>'READY'
      OR NOT r.autonomy_enforcement_active OR r.blocks_new_actions OR r.observed_at<$1-interval '24 hours' OR r.observed_at>$1+interval '5 minutes'),
  (SELECT count(*) FROM active_ai WHERE account_status<>'active' OR connection_status<>'active'),
  (SELECT count(*) FROM risk_circuit_breakers WHERE scope='GLOBAL' AND state='OPEN'),
  (SELECT count(*) FROM risk_circuit_breakers WHERE scope<>'GLOBAL' AND state='OPEN'),
  (SELECT count(*) FROM automation_mandates WHERE execution_mode='LIVE'),
  (SELECT count(*) FROM strategy_instances i JOIN automation_mandates m ON m.id=i.automation_mandate_id WHERE m.automation_type='AI_AUTONOMOUS' AND i.execution_mode<>'SHADOW'),
  (SELECT count(*) FROM nonlive_execution_records e JOIN automation_mandates m ON m.id=e.mandate_id WHERE m.automation_type='AI_AUTONOMOUS' AND e.mode<>'SHADOW'),
  (SELECT count(*) FROM risk_evaluations WHERE platform_execution_available),
  (SELECT count(*) FROM order_intents WHERE source='AI'),
  (SELECT count(*) FROM order_intents WHERE status='USER_APPROVED_NONEXECUTABLE')
`, observedAt).Scan(
		&facts.ActiveAIShadowInstances,
		&facts.UnhealthyAISchedules,
		&facts.UnhealthyAIReconciliations,
		&facts.UnavailableFinancialConnections,
		&facts.OpenGlobalBreakers,
		&facts.OpenScopedBreakers,
		&facts.ExecutionBoundary.LiveMandates,
		&facts.ExecutionBoundary.NonShadowAIInstances,
		&facts.ExecutionBoundary.NonShadowAIExecutions,
		&facts.ExecutionBoundary.ExecutableRiskEvaluations,
		&facts.ExecutionBoundary.NonExecutingAIProposals,
		&facts.ExecutionBoundary.ReviewedNonExecutingProposals,
	)
	return facts, err
}
