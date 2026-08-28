package auth

import (
	"context"
	"errors"
	"testing"
	"time"
)

type securityActivityAuditFake struct {
	activities      []SecurityActivity
	requestedUser   string
	requestedLimit  int
	requestedCursor *SecurityActivityCursor
}

func (*securityActivityAuditFake) Record(context.Context, *string, string, map[string]any) error {
	return nil
}

func (fake *securityActivityAuditFake) SecurityActivities(_ context.Context, userID string, limit int, cursor *SecurityActivityCursor) ([]SecurityActivity, error) {
	fake.requestedUser = userID
	fake.requestedLimit = limit
	fake.requestedCursor = cursor
	if len(fake.activities) < limit {
		limit = len(fake.activities)
	}
	return fake.activities[:limit], nil
}

func TestSecurityActivityIsOwnerScopedAndBuildsStableCursor(t *testing.T) {
	now := time.Date(2026, 8, 28, 1, 30, 0, 0, time.UTC)
	audit := &securityActivityAuditFake{activities: []SecurityActivity{
		{ID: "11111111-1111-4111-8111-111111111111", Action: "auth.login", OccurredAt: now},
		{ID: "22222222-2222-4222-8222-222222222222", Action: "auth.mfa_enabled", OccurredAt: now.Add(-time.Minute)},
		{ID: "33333333-3333-4333-8333-333333333333", Action: "financial.connection_disabled", OccurredAt: now.Add(-2 * time.Minute)},
	}}
	service := NewService(nil, nil, nil, audit, time.Hour)
	page, err := service.SecurityActivity(context.Background(), "owner", 2, nil)
	if err != nil {
		t.Fatal(err)
	}
	if audit.requestedUser != "owner" || audit.requestedLimit != 3 || audit.requestedCursor != nil {
		t.Fatalf("security activity request crossed its boundary: %#v", audit)
	}
	if len(page.Activities) != 2 || page.NextCursor == nil || page.NextCursor.ID != page.Activities[1].ID || !page.NextCursor.OccurredAt.Equal(page.Activities[1].OccurredAt) {
		t.Fatalf("unexpected security activity page: %#v", page)
	}
}

func TestSecurityActivityFailsClosedForInvalidOrUnavailableReader(t *testing.T) {
	audit := &securityActivityAuditFake{}
	service := NewService(nil, nil, nil, audit, time.Hour)
	if _, err := service.SecurityActivity(context.Background(), "", 20, nil); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("anonymous security activity was accepted: %v", err)
	}
	if _, err := service.SecurityActivity(context.Background(), "owner", 0, nil); !errors.Is(err, ErrSecurityActivityInvalid) {
		t.Fatalf("invalid security activity limit was accepted: %v", err)
	}
	withoutReader := NewService(nil, nil, nil, auditSinkWithoutActivity{}, time.Hour)
	if _, err := withoutReader.SecurityActivity(context.Background(), "owner", 20, nil); !errors.Is(err, ErrSecurityActivityUnavailable) {
		t.Fatalf("missing security activity reader did not fail closed: %v", err)
	}
}

type auditSinkWithoutActivity struct{}

func (auditSinkWithoutActivity) Record(context.Context, *string, string, map[string]any) error {
	return nil
}
