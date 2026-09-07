package financialconnection

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/authorization"
)

type authorizationReceiptStoreFake struct {
	connectionStoreFake
	receipts []AuthorizationReceipt
	err      error
	userID   string
	limit    int
}

func (store *authorizationReceiptStoreFake) ListAuthorizationReceipts(_ context.Context, userID string, limit int) ([]AuthorizationReceipt, error) {
	store.userID = userID
	store.limit = limit
	return append([]AuthorizationReceipt(nil), store.receipts...), store.err
}

func TestAuthorizationReceiptsAreOwnerScopedBoundedAndCredentialFree(t *testing.T) {
	completedAt := time.Date(2026, 9, 7, 8, 30, 0, 0, time.UTC)
	expiresAt := completedAt.Add(7 * 24 * time.Hour)
	expiryMatches := true
	store := &authorizationReceiptStoreFake{receipts: []AuthorizationReceipt{{
		ID:                                   "receipt-1",
		Provider:                             "schwab",
		Status:                               "COMPLETED",
		ConnectionID:                         "connection-1",
		AuthorizationExpiresAt:               &expiresAt,
		CurrentLastVerifiedAt:                &completedAt,
		AuthorizationExpiryMatchesConnection: &expiryMatches,
		OccurredAt:                           completedAt,
	}}}
	service := NewService(store, nil, nil, nil, nil)
	receipts, err := service.AuthorizationReceipts(context.Background(), authorization.Principal{
		UserID:      "owner-1",
		Entitlement: authorization.EntitlementFounder,
	})
	if err != nil || len(receipts) != 1 {
		t.Fatalf("authorization receipts unavailable: %#v %v", receipts, err)
	}
	if store.userID != "owner-1" || store.limit != authorizationReceiptLimit {
		t.Fatalf("authorization receipt boundary changed: user=%q limit=%d", store.userID, store.limit)
	}
	if receipts[0].Provider != "schwab" || receipts[0].Status != "COMPLETED" || receipts[0].ConnectionID != "connection-1" || !receipts[0].AuthorizationExpiresAt.Equal(expiresAt) || !receipts[0].CurrentLastVerifiedAt.Equal(completedAt) || receipts[0].AuthorizationExpiryMatchesConnection == nil || !*receipts[0].AuthorizationExpiryMatchesConnection {
		t.Fatalf("exact authorization receipt changed: %#v", receipts[0])
	}
}

func TestAuthorizationReceiptsFailClosedOnMalformedSavedEvidence(t *testing.T) {
	store := &authorizationReceiptStoreFake{receipts: []AuthorizationReceipt{{
		ID:         "receipt-1",
		Provider:   "Schwab secret",
		Status:     "COMPLETED",
		OccurredAt: time.Now().UTC(),
	}}}
	service := NewService(store, nil, nil, nil, nil)
	_, err := service.AuthorizationReceipts(context.Background(), authorization.Principal{
		UserID:      "owner-1",
		Entitlement: authorization.EntitlementFounder,
	})
	if !errors.Is(err, ErrAuthorizationReceiptUnavailable) {
		t.Fatalf("malformed authorization receipt did not fail closed: %v", err)
	}
}

func TestAuthorizationReceiptsFailClosedOnDuplicateSavedIdentity(t *testing.T) {
	occurredAt := time.Now().UTC()
	store := &authorizationReceiptStoreFake{receipts: []AuthorizationReceipt{
		{ID: "receipt-1", Provider: "schwab", Status: "STARTED", OccurredAt: occurredAt},
		{ID: "receipt-1", Provider: "schwab", Status: "FAILED", OccurredAt: occurredAt.Add(-time.Minute)},
	}}
	service := NewService(store, nil, nil, nil, nil)
	_, err := service.AuthorizationReceipts(context.Background(), authorization.Principal{
		UserID:      "owner-1",
		Entitlement: authorization.EntitlementFounder,
	})
	if !errors.Is(err, ErrAuthorizationReceiptUnavailable) {
		t.Fatalf("duplicate authorization receipt identity did not fail closed: %v", err)
	}
}

func TestAuthorizationReceiptsRequireFinancialConnectionEntitlement(t *testing.T) {
	service := NewService(&authorizationReceiptStoreFake{}, nil, nil, nil, nil)
	_, err := service.AuthorizationReceipts(context.Background(), authorization.Principal{
		UserID:      "owner-1",
		Entitlement: authorization.EntitlementFree,
	})
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("free entitlement read authorization receipts: %v", err)
	}
}
