package ownerattention

import (
	"context"
	"errors"
	"testing"
	"time"
)

type storeFake struct {
	items        []Item
	requestedID  string
	requestedMax int
	err          error
}

func (store *storeFake) Items(_ context.Context, userID string, limit int) ([]Item, error) {
	store.requestedID = userID
	store.requestedMax = limit
	return store.items, store.err
}

func TestOverviewIsOwnerScopedBoundedAndPrioritizesStops(t *testing.T) {
	now := time.Date(2026, 8, 28, 2, 30, 0, 0, time.UTC)
	store := &storeFake{items: []Item{
		{ID: "attention-1", Code: "SCHEDULE_FAILURE", Severity: SeverityAttention},
		{ID: "attention-2", Code: "OWNER_SAFETY_STOP", Severity: SeverityStopped},
	}}
	service := NewService(store)
	service.now = func() time.Time { return now }
	overview, err := service.Overview(context.Background(), "owner")
	if err != nil {
		t.Fatal(err)
	}
	if store.requestedID != "owner" || store.requestedMax != maximumItems {
		t.Fatalf("attention request crossed its owner boundary: %#v", store)
	}
	if overview.Status != StatusStopped || overview.Total != 2 || overview.AttentionCount != 1 || overview.StoppedCount != 1 || !overview.GeneratedAt.Equal(now) || overview.LiveExecutionAvailable || overview.BrokerActionRequested {
		t.Fatalf("unexpected attention overview: %#v", overview)
	}
}

func TestOverviewFailsClosedForMissingIdentityStoreOrSeverity(t *testing.T) {
	if _, err := NewService(&storeFake{}).Overview(context.Background(), ""); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("anonymous attention request was accepted: %v", err)
	}
	if _, err := NewService(nil).Overview(context.Background(), "owner"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("missing attention store was accepted: %v", err)
	}
	if _, err := NewService(&storeFake{items: []Item{{Severity: "UNKNOWN"}}}).Overview(context.Background(), "owner"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("unknown attention severity was accepted: %v", err)
	}
}

func TestOverviewPreservesStoreFailure(t *testing.T) {
	want := errors.New("database unavailable")
	if _, err := NewService(&storeFake{err: want}).Overview(context.Background(), "owner"); !errors.Is(err, want) {
		t.Fatalf("store failure changed: %v", err)
	}
}

func TestOverviewDefensivelyEnforcesItsHardMaximum(t *testing.T) {
	items := make([]Item, maximumItems+1)
	for index := range items {
		items[index] = Item{Severity: SeverityAttention}
	}
	overview, err := NewService(&storeFake{items: items}).Overview(context.Background(), "owner")
	if err != nil {
		t.Fatal(err)
	}
	if overview.Total != maximumItems || len(overview.Items) != maximumItems {
		t.Fatalf("attention projection exceeded its hard maximum: %#v", overview)
	}
}
