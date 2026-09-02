package financialconnection

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

const syncCheckpointColumns = `c.id::text,c.operation_id::text,c.financial_account_id::text,c.provider_connection_id::text,c.provider_name,o.source_operation,o.outcome,o.account_count,c.observed_at,c.completed_at,c.created_at`

func scanSyncCheckpoint(row pgx.Row) (AccountSyncCheckpoint, error) {
	var checkpoint AccountSyncCheckpoint
	err := row.Scan(&checkpoint.ID, &checkpoint.OperationID, &checkpoint.FinancialAccountID, &checkpoint.ProviderConnectionID, &checkpoint.Provider, &checkpoint.SourceOperation, &checkpoint.Outcome, &checkpoint.AccountCount, &checkpoint.ObservedAt, &checkpoint.CompletedAt, &checkpoint.CreatedAt)
	return checkpoint, err
}

func (s *PostgresStore) ListAccountSyncCheckpoints(ctx context.Context, userID, accountID string, limit int, cursor string) ([]AccountSyncCheckpoint, error) {
	var rows pgx.Rows
	var err error
	if cursor == "" {
		rows, err = s.db.Query(ctx, `SELECT `+syncCheckpointColumns+` FROM financial_account_sync_checkpoints c JOIN financial_account_sync_operations o ON o.id=c.operation_id AND o.user_id=c.user_id WHERE c.user_id=$1 AND c.financial_account_id=$2 ORDER BY c.completed_at DESC,c.id DESC LIMIT $3`, userID, accountID, limit)
	} else {
		var cursorCompletedAt time.Time
		var cursorID string
		err = s.db.QueryRow(ctx, `SELECT completed_at,id::text FROM financial_account_sync_checkpoints WHERE user_id=$1 AND financial_account_id=$2 AND id::text=$3`, userID, accountID, cursor).Scan(&cursorCompletedAt, &cursorID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrInvalidSyncCheckpointHistory
		}
		if err != nil {
			return nil, err
		}
		rows, err = s.db.Query(ctx, `SELECT `+syncCheckpointColumns+` FROM financial_account_sync_checkpoints c JOIN financial_account_sync_operations o ON o.id=c.operation_id AND o.user_id=c.user_id WHERE c.user_id=$1 AND c.financial_account_id=$2 AND (c.completed_at,c.id)<($3,$4::uuid) ORDER BY c.completed_at DESC,c.id DESC LIMIT $5`, userID, accountID, cursorCompletedAt, cursorID, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	checkpoints := make([]AccountSyncCheckpoint, 0, limit)
	for rows.Next() {
		checkpoint, scanErr := scanSyncCheckpoint(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		checkpoints = append(checkpoints, checkpoint)
	}
	return checkpoints, rows.Err()
}
