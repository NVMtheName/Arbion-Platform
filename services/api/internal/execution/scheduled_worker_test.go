package execution

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/authorization"
	"github.com/arbion/platform/services/api/internal/neural"
)

func TestScheduledWorkerDisabledAndConstructionNeverPerformIO(t *testing.T) {
	w, err := NewScheduledWorker(nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, disabled := range []*ScheduledWorker{nil, w, {}} {
		r, err := disabled.RunOnce(context.Background())
		if err != nil || r.Status != "DISABLED" || r.Order != nil || r.Generation != nil || !r.ScheduledFor.IsZero() {
			t.Fatal("disabled worker produced work", r, err)
		}
		if err = disabled.Run(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = NewScheduledWorker(&ScheduledWorkflow{}); !errors.Is(err, ErrNotAuthorized) {
		t.Fatal("partial workflow registered", err)
	}
	c := generationFixture(time.Now().UTC())
	f, err := NewScheduledWorkflow(NewPostgresStore(ownerNoDatabase{}), workflowScope(c), ownerNoIODependencies(),
		generationMarketFunc(func(context.Context, string, string, string) (ScheduledGenerationMarket, error) { panic("market I/O") }),
		generationDecisionFunc(func(context.Context, authorization.Principal, string, string, neural.LivePilotDecisionRequest) (neural.LivePilotDecision, error) {
			panic("model I/O")
		}),
		workflowRefreshFunc(func(context.Context, authorization.Principal, string) error { panic("reconciliation I/O") }))
	if err != nil {
		t.Fatal(err)
	}
	w, err = NewScheduledWorker(f)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = w.RunOnce(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled worker admitted work", err)
	}
	if err = w.Run(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal("worker did not stop on cancellation", err)
	}
}

func TestScheduledWorkerLifecycleNeverSelectsResubmissionOrCancellation(t *testing.T) {
	for state, want := range map[string]string{
		"SUBMISSION_UNKNOWN": "RECOVERY", "REJECTED_HELD": "RECOVERY",
		"BROKER_ACKNOWLEDGED": "RECONCILE", "PARTIALLY_FILLED": "RECONCILE",
		"AWAITING_ACCOUNT_SETTLEMENT": "SETTLE", "PREPARED": "WAITING",
		"SETTLED": "SLOT_RECORDED", "NOT_SENT": "SLOT_RECORDED",
		"": "UNAVAILABLE", "UNAVAILABLE": "UNAVAILABLE", "APPROVED": "UNAVAILABLE", "FILLED": "UNAVAILABLE",
	} {
		if got := scheduledWorkerAction(state); got != want {
			t.Errorf("%s selected %s, want %s", state, got, want)
		}
	}
}
