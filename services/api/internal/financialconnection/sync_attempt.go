package financialconnection

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/arbion/platform/services/api/internal/authorization"
)

var (
	ErrInvalidSyncAttemptHistory     = errors.New("financial connection sync attempt history query invalid")
	ErrSyncAttemptHistoryUnavailable = errors.New("financial connection sync attempt history unavailable")
)

const (
	defaultSyncAttemptHistoryLimit = 12
	maxSyncAttemptHistoryLimit     = 50
)

type ConnectionSyncAttempt struct {
	ID                   string    `json:"id"`
	ProviderConnectionID string    `json:"provider_connection_id"`
	Provider             string    `json:"provider"`
	SourceOperation      string    `json:"source_operation"`
	Outcome              string    `json:"outcome"`
	FailureStage         *string   `json:"failure_stage,omitempty"`
	ErrorCode            *string   `json:"error_code,omitempty"`
	AccountCount         *int      `json:"account_count,omitempty"`
	ObservedAt           time.Time `json:"observed_at"`
	CompletedAt          time.Time `json:"completed_at"`
	CreatedAt            time.Time `json:"created_at"`
}

type ConnectionSyncFailure struct {
	ProviderConnectionID string
	Provider             string
	FailureStage         string
	ErrorCode            string
	ObservedAt           time.Time
	CompletedAt          time.Time
}

type SyncAttemptHistoryQuery struct{ Limit int }

type SyncAttemptHistoryPage struct {
	Attempts []ConnectionSyncAttempt `json:"attempts"`
}

type SyncAttemptStore interface {
	RecordConnectionSyncFailure(context.Context, string, ConnectionSyncFailure) error
	ListConnectionSyncAttempts(context.Context, string, string, int) ([]ConnectionSyncAttempt, error)
}

func normalizeSyncAttemptHistoryQuery(query SyncAttemptHistoryQuery) (SyncAttemptHistoryQuery, error) {
	if query.Limit == 0 {
		query.Limit = defaultSyncAttemptHistoryLimit
	}
	if query.Limit < 1 || query.Limit > maxSyncAttemptHistoryLimit {
		return SyncAttemptHistoryQuery{}, ErrInvalidSyncAttemptHistory
	}
	return query, nil
}

// SyncAttemptHistory reads only immutable, credential-free attempt evidence
// already saved by Arbion. It never contacts a financial provider.
func (s *Service) SyncAttemptHistory(ctx context.Context, principal authorization.Principal, accountID string, query SyncAttemptHistoryQuery) (SyncAttemptHistoryPage, error) {
	if !allowed(principal) {
		return SyncAttemptHistoryPage{}, ErrForbidden
	}
	if s.syncAttempts == nil {
		return SyncAttemptHistoryPage{}, ErrSyncAttemptHistoryUnavailable
	}
	query, err := normalizeSyncAttemptHistoryQuery(query)
	if err != nil {
		return SyncAttemptHistoryPage{}, err
	}
	accountID = strings.TrimSpace(accountID)
	account, err := s.GetAccount(ctx, principal, accountID)
	if err != nil {
		return SyncAttemptHistoryPage{}, err
	}
	attempts, err := s.syncAttempts.ListConnectionSyncAttempts(ctx, principal.UserID, account.ProviderConnectionID, query.Limit)
	if err != nil {
		return SyncAttemptHistoryPage{}, err
	}
	return SyncAttemptHistoryPage{Attempts: attempts}, nil
}
