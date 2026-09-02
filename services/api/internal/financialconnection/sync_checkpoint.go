package financialconnection

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode"

	"github.com/arbion/platform/services/api/internal/authorization"
)

var (
	ErrInvalidSyncCheckpointHistory     = errors.New("financial account sync checkpoint history query invalid")
	ErrSyncCheckpointHistoryUnavailable = errors.New("financial account sync checkpoint history unavailable")
)

const (
	defaultSyncCheckpointHistoryLimit = 12
	maxSyncCheckpointHistoryLimit     = 50
)

type AccountSyncCheckpoint struct {
	ID                   string    `json:"id"`
	OperationID          string    `json:"operation_id"`
	FinancialAccountID   string    `json:"financial_account_id"`
	ProviderConnectionID string    `json:"provider_connection_id"`
	Provider             string    `json:"provider"`
	SourceOperation      string    `json:"source_operation"`
	Outcome              string    `json:"outcome"`
	AccountCount         int       `json:"account_count"`
	ObservedAt           time.Time `json:"observed_at"`
	CompletedAt          time.Time `json:"completed_at"`
	CreatedAt            time.Time `json:"created_at"`
}

type SyncCheckpointHistoryQuery struct {
	Limit  int
	Cursor string
}

type SyncCheckpointHistoryPage struct {
	Checkpoints []AccountSyncCheckpoint `json:"checkpoints"`
	NextCursor  string                  `json:"next_cursor,omitempty"`
}

type SyncCheckpointStore interface {
	ListAccountSyncCheckpoints(context.Context, string, string, int, string) ([]AccountSyncCheckpoint, error)
}

func normalizeSyncCheckpointHistoryQuery(query SyncCheckpointHistoryQuery) (SyncCheckpointHistoryQuery, error) {
	query.Cursor = strings.TrimSpace(query.Cursor)
	if query.Limit == 0 {
		query.Limit = defaultSyncCheckpointHistoryLimit
	}
	if query.Limit < 1 || query.Limit > maxSyncCheckpointHistoryLimit || len(query.Cursor) > 128 || strings.IndexFunc(query.Cursor, unicode.IsControl) >= 0 {
		return SyncCheckpointHistoryQuery{}, ErrInvalidSyncCheckpointHistory
	}
	return query, nil
}

// SyncCheckpointHistory reads only forward-collected, immutable account-sync
// evidence already saved by Arbion. It never contacts a financial provider.
func (s *Service) SyncCheckpointHistory(ctx context.Context, principal authorization.Principal, accountID string, query SyncCheckpointHistoryQuery) (SyncCheckpointHistoryPage, error) {
	if !allowed(principal) {
		return SyncCheckpointHistoryPage{}, ErrForbidden
	}
	if s.syncCheckpoints == nil {
		return SyncCheckpointHistoryPage{}, ErrSyncCheckpointHistoryUnavailable
	}
	query, err := normalizeSyncCheckpointHistoryQuery(query)
	if err != nil {
		return SyncCheckpointHistoryPage{}, err
	}
	account, err := s.GetAccount(ctx, principal, accountID)
	if err != nil {
		return SyncCheckpointHistoryPage{}, err
	}
	checkpoints, err := s.syncCheckpoints.ListAccountSyncCheckpoints(ctx, principal.UserID, account.ID, query.Limit+1, query.Cursor)
	if err != nil {
		return SyncCheckpointHistoryPage{}, err
	}
	page := SyncCheckpointHistoryPage{Checkpoints: []AccountSyncCheckpoint{}}
	if len(checkpoints) > query.Limit {
		checkpoints = checkpoints[:query.Limit]
		page.NextCursor = checkpoints[len(checkpoints)-1].ID
	}
	page.Checkpoints = append(page.Checkpoints, checkpoints...)
	return page, nil
}
