package strategy

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/authorization"
	"github.com/arbion/platform/services/api/internal/automation"
)

type journalPersistenceFake struct {
	entries         []JournalActivity
	requestedUser   string
	requestedLimit  int
	requestedAfter  *JournalCursor
	lifecycleID     string
	lifecycle       LifecycleCommand
	lifecycleAt     time.Time
	lifecycleResult LifecycleResult
	lifecycleError  error
}

func (*journalPersistenceFake) Initialize(context.Context, string, automation.Mandate, string, State) (Instance, error) {
	return Instance{}, nil
}
func (*journalPersistenceFake) List(context.Context, string) ([]Instance, error) { return nil, nil }
func (*journalPersistenceFake) Get(context.Context, string, string) (Instance, error) {
	return Instance{}, nil
}
func (*journalPersistenceFake) History(context.Context, string, string) ([]Transition, error) {
	return nil, nil
}
func (*journalPersistenceFake) Decisions(context.Context, string, string) ([]DecisionJournalEntry, error) {
	return nil, nil
}
func (*journalPersistenceFake) Executions(context.Context, string, string) ([]ExecutionRecord, error) {
	return nil, nil
}
func (f *journalPersistenceFake) Journal(_ context.Context, userID string, limit int, after *JournalCursor) ([]JournalActivity, error) {
	f.requestedUser = userID
	f.requestedLimit = limit
	f.requestedAfter = after
	if len(f.entries) < limit {
		limit = len(f.entries)
	}
	return f.entries[:limit], nil
}
func (*journalPersistenceFake) Schedule(context.Context, string, string) (ScheduleStatus, error) {
	return ScheduleStatus{}, nil
}
func (f *journalPersistenceFake) RecordLifecycle(_ context.Context, userID, instanceID string, command LifecycleCommand, occurredAt time.Time) (LifecycleResult, error) {
	f.requestedUser = userID
	f.lifecycleID = instanceID
	f.lifecycle = command
	f.lifecycleAt = occurredAt
	return f.lifecycleResult, f.lifecycleError
}

func TestJournalIsOwnerScopedAndBuildsStableNextCursor(t *testing.T) {
	now := time.Date(2026, 8, 18, 18, 0, 0, 0, time.UTC)
	store := &journalPersistenceFake{entries: []JournalActivity{
		{ID: "11111111-1111-4111-8111-111111111111", CreatedAt: now},
		{ID: "22222222-2222-4222-8222-222222222222", CreatedAt: now.Add(-time.Minute)},
		{ID: "33333333-3333-4333-8333-333333333333", CreatedAt: now.Add(-2 * time.Minute)},
	}}
	service := NewInstanceService(store, nil)
	page, err := service.Journal(context.Background(), authorization.Principal{UserID: "owner", Entitlement: authorization.EntitlementFounder}, 2, nil)
	if err != nil {
		t.Fatal(err)
	}
	if store.requestedUser != "owner" || store.requestedLimit != 3 {
		t.Fatalf("owner or lookahead was not preserved: user=%q limit=%d", store.requestedUser, store.requestedLimit)
	}
	if len(page.Entries) != 2 || page.NextCursor == nil || page.NextCursor.ID != page.Entries[1].ID || !page.NextCursor.CreatedAt.Equal(page.Entries[1].CreatedAt) {
		t.Fatalf("unexpected page: %#v", page)
	}
}

func TestJournalRequiresAutomationEntitlement(t *testing.T) {
	store := &journalPersistenceFake{}
	service := NewInstanceService(store, nil)
	_, err := service.Journal(context.Background(), authorization.Principal{UserID: "owner", Entitlement: authorization.EntitlementFree}, 25, nil)
	if !errors.Is(err, ErrForbidden) || store.requestedUser != "" {
		t.Fatalf("unentitled journal request was not rejected: %v", err)
	}
}

func TestRecordLifecycleRequiresExplicitPaperConfirmationAndOwnerEntitlement(t *testing.T) {
	store := &journalPersistenceFake{}
	service := NewInstanceService(store, nil)
	command := LifecycleCommand{EventID: "manual-lifecycle:event-1", EventType: Assignment, ExpectedStateVersion: 2}
	if _, err := service.RecordLifecycle(context.Background(), authorization.Principal{UserID: "owner", Entitlement: authorization.EntitlementFounder}, "instance-1", command); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unconfirmed paper event was accepted: %v", err)
	}
	command.ConfirmPaperSimulation = true
	if _, err := service.RecordLifecycle(context.Background(), authorization.Principal{UserID: "owner", Entitlement: authorization.EntitlementFree}, "instance-1", command); !errors.Is(err, ErrForbidden) {
		t.Fatalf("unentitled paper event was accepted: %v", err)
	}
	if store.lifecycleID != "" {
		t.Fatal("rejected lifecycle command reached persistence")
	}
}

func TestRecordLifecycleIsOwnerScopedAndUsesServerTime(t *testing.T) {
	now := time.Date(2026, 8, 18, 20, 0, 0, 0, time.UTC)
	store := &journalPersistenceFake{lifecycleResult: LifecycleResult{ID: "event-record-1", EventType: ExpireWorthless}}
	service := NewInstanceService(store, nil)
	service.now = func() time.Time { return now }
	command := LifecycleCommand{EventID: "manual-lifecycle:event-1", EventType: ExpireWorthless, ExpectedStateVersion: 2, ConfirmPaperSimulation: true}
	result, err := service.RecordLifecycle(context.Background(), authorization.Principal{UserID: "owner", Entitlement: authorization.EntitlementFounder}, "instance-1", command)
	if err != nil {
		t.Fatal(err)
	}
	if result.ID != "event-record-1" || store.requestedUser != "owner" || store.lifecycleID != "instance-1" || store.lifecycle != command || !store.lifecycleAt.Equal(now) {
		t.Fatalf("lifecycle command boundary changed: result=%#v store=%#v", result, store)
	}
}
