package risk

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

var (
	ErrCommitCircuitBreakerActive = errors.New("circuit breaker prevents non-live commit")
	ErrCommitGuardUnavailable     = errors.New("non-live commit guard unavailable")
)

// LockCircuitBreakersForCommit coordinates an already validated non-live action
// with all four stop scopes. It does not authenticate the caller, evaluate risk,
// or authorize execution. The caller must retain this READ COMMITTED transaction
// through its final write and must not call a provider while holding these locks.
//
// Readers take shared locks in GLOBAL -> USER -> ACCOUNT -> AUTOMATION order.
// The database mutation trigger takes the matching exclusive scope lock, even
// for the first inserted breaker. Unrelated accounts therefore remain independent
// and multiple fills can share the global/user locks without serializing a fleet.
func LockCircuitBreakersForCommit(ctx context.Context, tx pgx.Tx, userID, accountID, mandateID string) error {
	if userID == "" || accountID == "" || mandateID == "" {
		return ErrCommitGuardUnavailable
	}
	var isolation string
	if err := tx.QueryRow(ctx, `SELECT current_setting('transaction_isolation')`).Scan(&isolation); err != nil {
		return err
	}
	if isolation != "read committed" {
		return ErrCommitGuardUnavailable
	}
	for _, scope := range []struct {
		kind string
		id   any
	}{{"GLOBAL", nil}, {"USER", userID}, {"ACCOUNT", accountID}, {"AUTOMATION", mandateID}} {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock_shared(risk_breaker_scope_lock_key($1,$2::uuid))`, scope.kind, scope.id); err != nil {
			return err
		}
	}
	// Deliberately a separate statement after all locks have been acquired. Under
	// READ COMMITTED it sees a stop that committed while this transaction waited.
	var stopped bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM risk_circuit_breakers
		WHERE state='OPEN' AND (scope='GLOBAL' OR (scope='USER' AND scope_id=$1)
		OR (scope='ACCOUNT' AND scope_id=$2) OR (scope='AUTOMATION' AND scope_id=$3)))`,
		userID, accountID, mandateID).Scan(&stopped); err != nil {
		return err
	}
	if stopped {
		return ErrCommitCircuitBreakerActive
	}
	return nil
}
