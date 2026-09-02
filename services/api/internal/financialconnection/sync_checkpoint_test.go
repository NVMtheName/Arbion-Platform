package financialconnection

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/financial"
)

type syncCheckpointStoreFake struct {
	*connectionStoreFake
	items []AccountSyncCheckpoint
	reads int
}

func (store *syncCheckpointStoreFake) GetAccount(_ context.Context, _ string, id string) (financial.FinancialAccount, error) {
	if store.account.ID == "" || store.account.ID != id {
		return financial.FinancialAccount{}, ErrNotFound
	}
	return store.account, nil
}

func (store *syncCheckpointStoreFake) ListAccountSyncCheckpoints(_ context.Context, _, accountID string, limit int, cursor string) ([]AccountSyncCheckpoint, error) {
	store.reads++
	start := len(store.items) - 1
	if cursor != "" {
		start = -1
		for index := len(store.items) - 1; index >= 0; index-- {
			if store.items[index].FinancialAccountID == accountID && store.items[index].ID == cursor {
				start = index - 1
				break
			}
		}
		if start == -1 && (len(store.items) == 0 || store.items[0].ID != cursor) {
			return nil, ErrInvalidSyncCheckpointHistory
		}
	}
	items := []AccountSyncCheckpoint{}
	for index := start; index >= 0 && len(items) < limit; index-- {
		if store.items[index].FinancialAccountID == accountID {
			items = append(items, store.items[index])
		}
	}
	return items, nil
}

func TestSyncCheckpointHistoryIsBoundedOwnerScopedSavedEvidence(t *testing.T) {
	observedAt := time.Date(2026, time.September, 2, 20, 0, 0, 0, time.UTC)
	base := &connectionStoreFake{account: financial.FinancialAccount{ID: "account-1", UserID: founder().UserID, ProviderConnectionID: "connection-1", Provider: "coinbase"}}
	store := &syncCheckpointStoreFake{connectionStoreFake: base, items: []AccountSyncCheckpoint{
		{ID: "checkpoint-1", OperationID: "operation-1", FinancialAccountID: "account-1", ProviderConnectionID: "connection-1", Provider: "coinbase", SourceOperation: "PROVIDER_ACCOUNT_DISCOVERY", Outcome: "SAVED", AccountCount: 1, ObservedAt: observedAt, CompletedAt: observedAt.Add(time.Second), CreatedAt: observedAt.Add(time.Second)},
		{ID: "other-account", OperationID: "other-operation", FinancialAccountID: "account-2", ProviderConnectionID: "connection-2", Provider: "coinbase", SourceOperation: "PROVIDER_ACCOUNT_DISCOVERY", Outcome: "SAVED", AccountCount: 1, ObservedAt: observedAt.Add(time.Minute), CompletedAt: observedAt.Add(time.Minute + time.Second), CreatedAt: observedAt.Add(time.Minute + time.Second)},
		{ID: "checkpoint-2", OperationID: "operation-2", FinancialAccountID: "account-1", ProviderConnectionID: "connection-1", Provider: "coinbase", SourceOperation: "PROVIDER_ACCOUNT_DISCOVERY", Outcome: "SAVED", AccountCount: 1, ObservedAt: observedAt.Add(2 * time.Minute), CompletedAt: observedAt.Add(2*time.Minute + time.Second), CreatedAt: observedAt.Add(2*time.Minute + time.Second)},
		{ID: "checkpoint-3", OperationID: "operation-3", FinancialAccountID: "account-1", ProviderConnectionID: "connection-1", Provider: "coinbase", SourceOperation: "PROVIDER_ACCOUNT_DISCOVERY", Outcome: "SAVED", AccountCount: 1, ObservedAt: observedAt.Add(3 * time.Minute), CompletedAt: observedAt.Add(3*time.Minute + time.Second), CreatedAt: observedAt.Add(3*time.Minute + time.Second)},
	}}
	service := NewService(store, nil, nil, nil, nil)

	first, err := service.SyncCheckpointHistory(context.Background(), founder(), "account-1", SyncCheckpointHistoryQuery{Limit: 2})
	if err != nil || len(first.Checkpoints) != 2 || first.Checkpoints[0].ID != "checkpoint-3" || first.Checkpoints[1].ID != "checkpoint-2" || first.NextCursor != "checkpoint-2" {
		t.Fatalf("unexpected first sync history page: %#v err=%v", first, err)
	}
	second, err := service.SyncCheckpointHistory(context.Background(), founder(), "account-1", SyncCheckpointHistoryQuery{Limit: 2, Cursor: first.NextCursor})
	if err != nil || len(second.Checkpoints) != 1 || second.Checkpoints[0].ID != "checkpoint-1" || second.NextCursor != "" {
		t.Fatalf("unexpected second sync history page: %#v err=%v", second, err)
	}
	if store.reads != 2 {
		t.Fatalf("expected two saved-evidence reads, got %d", store.reads)
	}
	if _, err = service.SyncCheckpointHistory(context.Background(), founder(), "account-1", SyncCheckpointHistoryQuery{Limit: 51}); !errors.Is(err, ErrInvalidSyncCheckpointHistory) {
		t.Fatalf("oversized sync history page was accepted: %v", err)
	}
	if _, err = service.SyncCheckpointHistory(context.Background(), founder(), "account-2", SyncCheckpointHistoryQuery{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-account sync history was accepted: %v", err)
	}
}

func TestSyncCheckpointHistoryFailsClosedWithoutSavedEvidenceStore(t *testing.T) {
	service := NewService(&connectionStoreFake{}, nil, nil, nil, nil)
	if _, err := service.SyncCheckpointHistory(context.Background(), founder(), "account-1", SyncCheckpointHistoryQuery{}); !errors.Is(err, ErrSyncCheckpointHistoryUnavailable) {
		t.Fatalf("missing saved-evidence store did not fail closed: %v", err)
	}
}
