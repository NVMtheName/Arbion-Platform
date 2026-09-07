package financialconnection

import (
	"context"
)

func (s *PostgresStore) RecordConnectionSyncFailure(ctx context.Context, userID string, failure ConnectionSyncFailure) error {
	_, err := s.db.Exec(ctx, `INSERT INTO financial_connection_sync_failures(
		user_id,provider_connection_id,provider_name,source_operation,outcome,
		failure_stage,error_code,observed_at,completed_at,created_at
	) VALUES($1,$2,$3,'PROVIDER_ACCOUNT_DISCOVERY','FAILED',$4,$5,$6,$7,$7)`,
		userID, failure.ProviderConnectionID, failure.Provider, failure.FailureStage,
		failure.ErrorCode, failure.ObservedAt, failure.CompletedAt)
	return err
}

func (s *PostgresStore) ListConnectionSyncAttempts(ctx context.Context, userID, connectionID string, limit int) ([]ConnectionSyncAttempt, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text,provider_connection_id::text,provider_name,source_operation,outcome,
		       NULL::text AS failure_stage,NULL::text AS error_code,account_count,
		       observed_at,completed_at,created_at
		FROM financial_account_sync_operations
		WHERE user_id=$1 AND provider_connection_id=$2
		UNION ALL
		SELECT id::text,provider_connection_id::text,provider_name,source_operation,outcome,
		       failure_stage,error_code,NULL::integer AS account_count,
		       observed_at,completed_at,created_at
		FROM financial_connection_sync_failures
		WHERE user_id=$1 AND provider_connection_id=$2
		ORDER BY completed_at DESC,id DESC
		LIMIT $3`, userID, connectionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	attempts := make([]ConnectionSyncAttempt, 0, limit)
	for rows.Next() {
		var attempt ConnectionSyncAttempt
		if err = rows.Scan(
			&attempt.ID, &attempt.ProviderConnectionID, &attempt.Provider,
			&attempt.SourceOperation, &attempt.Outcome, &attempt.FailureStage,
			&attempt.ErrorCode, &attempt.AccountCount, &attempt.ObservedAt,
			&attempt.CompletedAt, &attempt.CreatedAt,
		); err != nil {
			return nil, err
		}
		attempts = append(attempts, attempt)
	}
	return attempts, rows.Err()
}

var _ SyncAttemptStore = (*PostgresStore)(nil)
