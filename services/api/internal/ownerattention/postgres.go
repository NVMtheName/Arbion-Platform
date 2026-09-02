package ownerattention

import (
	"context"

	"github.com/jackc/pgx/v5"
)

type DB interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

type PostgresStore struct{ db DB }

func NewPostgresStore(db DB) *PostgresStore { return &PostgresStore{db: db} }

// Items returns active owner-scoped control conditions only. The projection
// deliberately excludes account names, provider names and responses, breaker
// reasons, reconciliation changes, symbols, quantities, credentials, and
// every field that could request or authorize a broker action.
func (store *PostgresStore) Items(ctx context.Context, userID string, limit int) ([]Item, error) {
	rows, err := store.db.Query(ctx, `
WITH latest_reconciliation AS (
  SELECT DISTINCT ON (financial_account_id)
    id,financial_account_id,comparison_status,blocks_new_actions,
    autonomy_enforcement_active,blocking_change_count,observed_at
  FROM portfolio_reconciliations
  WHERE user_id=$1
  ORDER BY financial_account_id,observed_at DESC,id DESC
), attention AS (
  SELECT
    s.strategy_instance_id::text AS id,
    CASE WHEN a.provider_name='schwab' AND p.provider_name='schwab'
      AND s.last_error_code IN (
        'MARKET_DATA_DELAYED',
        'MARKET_DATA_REALTIME_UNCONFIRMED',
        'MARKET_DATA_NOT_REALTIME'
      )
      THEN 'SCHWAB_MARKET_DATA_ATTENTION'
      ELSE 'SCHEDULE_FAILURE' END AS code,
    'ATTENTION'::text AS severity,
    'AUTOMATION'::text AS resource_type,
    i.automation_mandate_id::text AS resource_id,
    COALESCE(s.last_completed_at,s.updated_at) AS occurred_at,
    s.consecutive_failures::bigint AS count
  FROM nonlive_strategy_schedules s
  JOIN strategy_instances i
    ON i.id=s.strategy_instance_id AND i.user_id=s.user_id
  JOIN financial_accounts a
    ON a.id=i.financial_account_id AND a.user_id=i.user_id
  JOIN provider_connections p
    ON p.id=a.provider_connection_id AND p.user_id=a.user_id
  WHERE i.user_id=$1 AND i.status IN ('ACTIVE','PAUSED')
    AND (s.last_status='FAILED' OR s.consecutive_failures>0)

  UNION ALL

  SELECT
    r.id::text,
    CASE WHEN r.comparison_status='DRIFT_DETECTED'
      THEN 'PORTFOLIO_DRIFT_REVIEW_REQUIRED'
      ELSE 'PORTFOLIO_EVIDENCE_REQUIRED' END,
    'ATTENTION',
    'ACCOUNT',
    r.financial_account_id::text,
    r.observed_at,
    r.blocking_change_count::bigint
  FROM latest_reconciliation r
  WHERE r.autonomy_enforcement_active AND r.blocks_new_actions

  UNION ALL

  SELECT
    p.id::text,
    CASE WHEN p.provider_category='ai'
      THEN 'AI_CONNECTION_ATTENTION'
      ELSE 'FINANCIAL_CONNECTION_ATTENTION' END,
    'ATTENTION',
    'CONNECTION',
    p.id::text,
    p.updated_at,
    1::bigint
  FROM provider_connections p
  WHERE p.user_id=$1
    AND p.provider_category IN ('ai','financial')
    AND p.status IN ('expired','revoked','error')

  UNION ALL

  SELECT
    a.id::text,
    'FINANCIAL_ACCOUNT_UNAVAILABLE',
    'ATTENTION',
    'ACCOUNT',
    a.id::text,
    a.updated_at,
    1::bigint
  FROM financial_accounts a
  WHERE a.user_id=$1 AND a.status='unavailable'

  UNION ALL

  SELECT
    b.id::text,
    CASE b.scope
      WHEN 'GLOBAL' THEN 'GLOBAL_SAFETY_STOP'
      WHEN 'USER' THEN 'OWNER_SAFETY_STOP'
      WHEN 'ACCOUNT' THEN 'ACCOUNT_SAFETY_STOP'
      ELSE 'AUTOMATION_SAFETY_STOP' END,
    'STOPPED',
    CASE b.scope
      WHEN 'GLOBAL' THEN 'PLATFORM'
      WHEN 'USER' THEN 'OWNER'
      ELSE b.scope END,
    b.scope_id::text,
    b.engaged_at,
    1::bigint
  FROM risk_circuit_breakers b
  WHERE b.state='OPEN' AND (
    b.scope='GLOBAL' OR
    (b.scope='USER' AND b.scope_id=$1::uuid) OR
    (b.scope='ACCOUNT' AND EXISTS (
      SELECT 1 FROM financial_accounts a
      WHERE a.id=b.scope_id AND a.user_id=$1
    )) OR
    (b.scope='AUTOMATION' AND EXISTS (
      SELECT 1 FROM automation_mandates m
      WHERE m.id=b.scope_id AND m.user_id=$1
    ))
  )
)
SELECT id,code,severity,resource_type,resource_id,occurred_at,count
FROM attention
ORDER BY CASE severity WHEN 'STOPPED' THEN 0 ELSE 1 END,occurred_at DESC,id DESC
LIMIT $2`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Item{}
	for rows.Next() {
		var item Item
		if err = rows.Scan(&item.ID, &item.Code, &item.Severity, &item.ResourceType, &item.ResourceID, &item.OccurredAt, &item.Count); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
