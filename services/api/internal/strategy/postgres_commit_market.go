package strategy

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

var ErrCommitMarketDataStale = errors.New("saved AI quote is not current at non-live commit")

// The original quote validation is relative to the evaluation's saved start
// time. Repeat only its timing rule at the final persistence boundary, after
// every row lock, so model latency and ledger contention cannot extend its age.
// Never replace the saved quote or fetch a new one to rescue a stale decision.
func checkAICommitMarketTime(ctx context.Context, tx pgx.Tx, reference *AIProposalQuoteReference, mode ExecutionMode) error {
	var now time.Time
	if err := tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return err
	}
	if !currentAICommitMarketTime(reference, mode, now) {
		return ErrCommitMarketDataStale
	}
	return nil
}

func currentAICommitMarketTime(reference *AIProposalQuoteReference, mode ExecutionMode, now time.Time) bool {
	if reference == nil || now.IsZero() || !freshMarketTimestamp(reference.ObservedAt, now) {
		return false
	}
	switch mode {
	case Paper:
		// Paper already imposes tighter timing in the pure simulator; retain it.
		return !reference.ObservedAt.Before(now.Add(-aiPaperMaximumMarketAge)) && !reference.ObservedAt.After(now.Add(aiPaperMaximumFutureOffset))
	case Shadow:
		return true
	default:
		return false
	}
}
