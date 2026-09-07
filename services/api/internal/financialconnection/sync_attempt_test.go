package financialconnection

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/credential"
	"github.com/arbion/platform/services/api/internal/financial"
)

type syncAttemptStoreFake struct {
	*connectionStoreFake
	attempts []ConnectionSyncAttempt
	failures []ConnectionSyncFailure
}

func (store *syncAttemptStoreFake) GetAccount(_ context.Context, _ string, accountID string) (financial.FinancialAccount, error) {
	if store.account.ID == "" || store.account.ID != accountID {
		return financial.FinancialAccount{}, ErrNotFound
	}
	return store.account, nil
}

func (store *syncAttemptStoreFake) RecordConnectionSyncFailure(_ context.Context, _ string, failure ConnectionSyncFailure) error {
	store.failures = append(store.failures, failure)
	return nil
}

func (store *syncAttemptStoreFake) ListConnectionSyncAttempts(_ context.Context, _, connectionID string, limit int) ([]ConnectionSyncAttempt, error) {
	items := make([]ConnectionSyncAttempt, 0, limit)
	for _, attempt := range store.attempts {
		if attempt.ProviderConnectionID == connectionID && len(items) < limit {
			items = append(items, attempt)
		}
	}
	return items, nil
}

func TestSyncFailureIsRecordedWithoutCredentialsOrRawProviderError(t *testing.T) {
	base := &connectionStoreFake{
		connection: Connection{ID: "connection-1", Provider: "coinbase", Status: "active"},
		account:    financial.FinancialAccount{ID: "account-1", UserID: founder().UserID, ProviderConnectionID: "connection-1", Provider: "coinbase"},
	}
	store := &syncAttemptStoreFake{connectionStoreFake: base}
	provider := &coinbaseProviderFake{accountsErr: &financial.ProviderError{Code: financial.RateLimited}}
	raw, err := json.Marshal(financial.Credentials{APIKeyName: "organizations/org/apiKeys/key", APIPrivateKey: "private-key", PortfolioID: "portfolio-1"})
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(store, &vaultFake{values: map[string][]byte{"connection-1": raw}}, nil, nil, nil, NamedProvider{ID: "coinbase", Provider: provider})

	err = service.Sync(context.Background(), founder(), "connection-1")
	if err == nil || len(store.failures) != 1 {
		t.Fatalf("provider failure was not preserved: err=%v failures=%#v", err, store.failures)
	}
	failure := store.failures[0]
	if failure.ProviderConnectionID != "connection-1" || failure.Provider != "coinbase" || failure.FailureStage != "ACCOUNT_DISCOVERY" || failure.ErrorCode != "RATE_LIMITED" || failure.CompletedAt.Before(failure.ObservedAt) {
		t.Fatalf("safe failure identity was incomplete: %#v", failure)
	}
	encoded, err := json.Marshal(failure)
	if err != nil {
		t.Fatal(err)
	}
	if containsAny(string(encoded), "private-key", "organizations/org/apiKeys/key") {
		t.Fatalf("failure evidence exposed credential material: %s", encoded)
	}
}

func TestSyncFailureClassificationCoversCredentialAndPersistenceBoundaries(t *testing.T) {
	base := &connectionStoreFake{connection: Connection{ID: "connection-1", Provider: "coinbase", Status: "active"}}
	store := &syncAttemptStoreFake{connectionStoreFake: base}
	service := NewService(store, &vaultFake{}, nil, nil, nil, NamedProvider{ID: "coinbase", Provider: &coinbaseProviderFake{}})
	if err := service.Sync(context.Background(), founder(), "connection-1"); !errors.Is(err, credential.ErrNotFound) {
		t.Fatalf("expected missing credential, got %v", err)
	}
	if len(store.failures) != 1 || store.failures[0].FailureStage != "CREDENTIAL_ACCESS" || store.failures[0].ErrorCode != "CREDENTIAL_UNAVAILABLE" {
		t.Fatalf("credential failure was not classified safely: %#v", store.failures)
	}

	base.syncErr = errors.New("database unavailable")
	raw := []byte(`{"api_key_name":"organizations/org/apiKeys/key","api_private_key":"private-key","portfolio_id":"portfolio-1"}`)
	service = NewService(store, &vaultFake{values: map[string][]byte{"connection-1": raw}}, nil, nil, nil, NamedProvider{ID: "coinbase", Provider: &coinbaseProviderFake{}})
	if err := service.Sync(context.Background(), founder(), "connection-1"); err == nil {
		t.Fatal("expected persistence failure")
	}
	if len(store.failures) != 2 || store.failures[1].FailureStage != "PERSISTENCE" || store.failures[1].ErrorCode != "INTERNAL_ERROR" {
		t.Fatalf("persistence failure was not classified safely: %#v", store.failures)
	}
}

func TestSyncAttemptHistoryIsBoundedAndOwnerScopedWithoutProviderReads(t *testing.T) {
	now := time.Date(2026, time.September, 7, 12, 0, 0, 0, time.UTC)
	base := &connectionStoreFake{account: financial.FinancialAccount{ID: "account-1", UserID: founder().UserID, ProviderConnectionID: "connection-1", Provider: "coinbase"}}
	store := &syncAttemptStoreFake{connectionStoreFake: base, attempts: []ConnectionSyncAttempt{
		{ID: "attempt-2", ProviderConnectionID: "connection-1", Provider: "coinbase", SourceOperation: "PROVIDER_ACCOUNT_DISCOVERY", Outcome: "SAVED", ObservedAt: now, CompletedAt: now, CreatedAt: now},
		{ID: "attempt-1", ProviderConnectionID: "connection-1", Provider: "coinbase", SourceOperation: "PROVIDER_ACCOUNT_DISCOVERY", Outcome: "FAILED", ObservedAt: now.Add(-time.Hour), CompletedAt: now.Add(-time.Hour), CreatedAt: now.Add(-time.Hour)},
	}}
	service := NewService(store, nil, nil, nil, nil)
	page, err := service.SyncAttemptHistory(context.Background(), founder(), "account-1", SyncAttemptHistoryQuery{Limit: 1})
	if err != nil || len(page.Attempts) != 1 || page.Attempts[0].ID != "attempt-2" {
		t.Fatalf("unexpected bounded history: %#v err=%v", page, err)
	}
	if _, err = service.SyncAttemptHistory(context.Background(), founder(), "account-1", SyncAttemptHistoryQuery{Limit: 51}); !errors.Is(err, ErrInvalidSyncAttemptHistory) {
		t.Fatalf("oversized attempt history was accepted: %v", err)
	}
	if _, err = service.SyncAttemptHistory(context.Background(), founder(), "account-2", SyncAttemptHistoryQuery{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-account attempt history was accepted: %v", err)
	}
}
