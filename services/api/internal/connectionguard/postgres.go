package connectionguard

import (
	"context"
	"errors"
	"sort"

	"github.com/jackc/pgx/v5"
)

var ErrUnavailable = errors.New("connection dependency is unavailable")

// LockActive serializes mandate and runtime attachment with connection
// lifecycle changes. Callers must keep the transaction open through their
// write. The post-lock reads are deliberate: a service-level validation that
// ran before this transaction cannot authorize a stale attachment.
func LockActive(ctx context.Context, tx pgx.Tx, userID, financialAccountID string, aiConnectionID *string) error {
	var financialConnectionID string
	if err := tx.QueryRow(ctx, `SELECT p.id::text
		FROM financial_accounts a
		JOIN provider_connections p ON p.id=a.provider_connection_id
		WHERE a.id=$1 AND a.user_id=$2 AND p.user_id=$2
		  AND p.provider_category='financial'`, financialAccountID, userID).Scan(&financialConnectionID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrUnavailable
		}
		return err
	}

	connectionIDs := []string{financialConnectionID}
	if aiConnectionID != nil {
		if *aiConnectionID == "" {
			return ErrUnavailable
		}
		connectionIDs = append(connectionIDs, *aiConnectionID)
	}
	sort.Strings(connectionIDs)
	previous := ""
	for _, connectionID := range connectionIDs {
		if connectionID == previous {
			continue
		}
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, connectionID); err != nil {
			return err
		}
		previous = connectionID
	}

	var financialActive bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(
		SELECT 1
		FROM financial_accounts a
		JOIN provider_connections p ON p.id=a.provider_connection_id
		WHERE a.id=$1 AND a.user_id=$2 AND a.status='active'
		  AND p.id=$3 AND p.user_id=$2 AND p.provider_category='financial'
		  AND p.status='active'
	)`, financialAccountID, userID, financialConnectionID).Scan(&financialActive); err != nil {
		return err
	}
	if !financialActive {
		return ErrUnavailable
	}

	if aiConnectionID != nil {
		var aiActive bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(
			SELECT 1 FROM provider_connections
			WHERE id=$1 AND user_id=$2 AND provider_category='ai' AND status='active'
		)`, *aiConnectionID, userID).Scan(&aiActive); err != nil {
			return err
		}
		if !aiActive {
			return ErrUnavailable
		}
	}
	return nil
}
