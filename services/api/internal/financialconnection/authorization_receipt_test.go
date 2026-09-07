package financialconnection

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/authorization"
)

const (
	authorizationReceiptID1    = "10000000-0000-4000-8000-000000000001"
	authorizationReceiptID2    = "10000000-0000-4000-8000-000000000002"
	authorizationConnectionID1 = "10000000-0000-4000-8000-000000000011"
	authorizationConnectionID2 = "10000000-0000-4000-8000-000000000012"
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
		ID:                                   authorizationReceiptID1,
		AttemptID:                            strings.Repeat("a", 64),
		Provider:                             "schwab",
		Status:                               "COMPLETED",
		ConnectionID:                         authorizationConnectionID1,
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
	if receipts[0].Provider != "schwab" || receipts[0].Status != "COMPLETED" || receipts[0].ConnectionID != authorizationConnectionID1 || !receipts[0].AuthorizationExpiresAt.Equal(expiresAt) || !receipts[0].CurrentLastVerifiedAt.Equal(completedAt) || receipts[0].AuthorizationExpiryMatchesConnection == nil || !*receipts[0].AuthorizationExpiryMatchesConnection {
		t.Fatalf("exact authorization receipt changed: %#v", receipts[0])
	}
}

func TestAuthorizationReceiptsRejectMalformedOrAmbiguousAttemptEvidence(t *testing.T) {
	completedAt := time.Now().UTC()
	verifiedAt := completedAt
	expiryMatches := true
	for _, test := range []struct {
		name     string
		receipts []AuthorizationReceipt
	}{
		{
			name: "malformed attempt identity",
			receipts: []AuthorizationReceipt{{
				ID: authorizationReceiptID1, AttemptID: "not-an-attempt", Provider: "schwab", Status: "STARTED", OccurredAt: completedAt,
			}},
		},
		{
			name: "two terminal outcomes",
			receipts: []AuthorizationReceipt{
				{ID: authorizationReceiptID1, AttemptID: strings.Repeat("b", 64), Provider: "schwab", Status: "FAILED", ConnectionID: authorizationConnectionID1, OccurredAt: completedAt.Add(-time.Second)},
				{ID: authorizationReceiptID2, AttemptID: strings.Repeat("b", 64), Provider: "schwab", Status: "COMPLETED", ConnectionID: authorizationConnectionID1, CurrentLastVerifiedAt: &verifiedAt, AuthorizationExpiryMatchesConnection: &expiryMatches, OccurredAt: completedAt},
			},
		},
		{
			name: "cross connection attempt",
			receipts: []AuthorizationReceipt{
				{ID: authorizationReceiptID1, AttemptID: strings.Repeat("c", 64), Provider: "schwab", Status: "STARTED", ConnectionID: authorizationConnectionID1, OccurredAt: completedAt.Add(-time.Second)},
				{ID: authorizationReceiptID2, AttemptID: strings.Repeat("c", 64), Provider: "schwab", Status: "COMPLETED", ConnectionID: authorizationConnectionID2, CurrentLastVerifiedAt: &verifiedAt, AuthorizationExpiryMatchesConnection: &expiryMatches, OccurredAt: completedAt},
			},
		},
		{
			name: "terminal before start",
			receipts: []AuthorizationReceipt{
				{ID: authorizationReceiptID1, AttemptID: strings.Repeat("d", 64), Provider: "schwab", Status: "STARTED", ConnectionID: authorizationConnectionID1, OccurredAt: completedAt},
				{ID: authorizationReceiptID2, AttemptID: strings.Repeat("d", 64), Provider: "schwab", Status: "FAILED", ConnectionID: authorizationConnectionID1, OccurredAt: completedAt.Add(-time.Second)},
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			service := NewService(&authorizationReceiptStoreFake{receipts: test.receipts}, nil, nil, nil, nil)
			_, err := service.AuthorizationReceipts(context.Background(), authorization.Principal{
				UserID: "owner-1", Entitlement: authorization.EntitlementFounder,
			})
			if !errors.Is(err, ErrAuthorizationReceiptUnavailable) {
				t.Fatalf("ambiguous attempt evidence did not fail closed: %v", err)
			}
		})
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
		{ID: authorizationReceiptID1, Provider: "schwab", Status: "STARTED", OccurredAt: occurredAt},
		{ID: authorizationReceiptID1, Provider: "schwab", Status: "FAILED", OccurredAt: occurredAt.Add(-time.Minute)},
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
