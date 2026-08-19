package risk

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresBreakerStore struct{ db *pgxpool.Pool }

func NewPostgresBreakerStore(db *pgxpool.Pool) *PostgresBreakerStore {
	return &PostgresBreakerStore{db: db}
}

const breakerColumns = `id::text,scope,scope_id::text,state,reason,source,engaged_by_user_id::text,engaged_at,released_by_user_id::text,released_at`

func scanBreaker(row pgx.Row) (breaker CircuitBreaker, err error) {
	err = row.Scan(&breaker.ID, &breaker.Scope, &breaker.ScopeID, &breaker.State, &breaker.Reason, &breaker.Source, &breaker.EngagedByUserID, &breaker.EngagedAt, &breaker.ReleasedByUserID, &breaker.ReleasedAt)
	return
}

func (store *PostgresBreakerStore) AutomationOwned(ctx context.Context, userID, automationID string) (bool, error) {
	var owned bool
	err := store.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM automation_mandates WHERE id=$1 AND user_id=$2)`, automationID, userID).Scan(&owned)
	return owned, err
}

func (store *PostgresBreakerStore) OpenAutomationBreaker(ctx context.Context, userID, automationID string) (*CircuitBreaker, error) {
	breaker, err := scanBreaker(store.db.QueryRow(ctx, `SELECT `+breakerColumns+` FROM risk_circuit_breakers b WHERE b.scope='AUTOMATION' AND b.scope_id=$1 AND b.state='OPEN' AND EXISTS(SELECT 1 FROM automation_mandates m WHERE m.id=b.scope_id AND m.user_id=$2)`, automationID, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &breaker, nil
}

func (store *PostgresBreakerStore) EngageAutomationBreaker(ctx context.Context, userID, automationID, reason string, engagedAt time.Time) (CircuitBreaker, error) {
	breaker, err := scanBreaker(store.db.QueryRow(ctx, `INSERT INTO risk_circuit_breakers(scope,scope_id,state,reason,source,engaged_by_user_id,engaged_at) SELECT 'AUTOMATION',m.id,'OPEN',$3,'UI',$2,$4 FROM automation_mandates m WHERE m.id=$1 AND m.user_id=$2 RETURNING `+breakerColumns, automationID, userID, reason, engagedAt))
	if errors.Is(err, pgx.ErrNoRows) {
		return CircuitBreaker{}, ErrBreakerNotFound
	}
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) && postgresError.Code == "23505" {
		return CircuitBreaker{}, ErrBreakerConflict
	}
	return breaker, err
}

func (store *PostgresBreakerStore) ReleaseAutomationBreaker(ctx context.Context, userID, automationID string, releasedAt time.Time) (CircuitBreaker, error) {
	breaker, err := scanBreaker(store.db.QueryRow(ctx, `UPDATE risk_circuit_breakers b SET state='CLOSED',released_by_user_id=$2,released_at=$3 WHERE b.scope='AUTOMATION' AND b.scope_id=$1 AND b.state='OPEN' AND EXISTS(SELECT 1 FROM automation_mandates m WHERE m.id=b.scope_id AND m.user_id=$2) RETURNING `+breakerColumns, automationID, userID, releasedAt))
	if errors.Is(err, pgx.ErrNoRows) {
		return CircuitBreaker{}, ErrBreakerConflict
	}
	return breaker, err
}
