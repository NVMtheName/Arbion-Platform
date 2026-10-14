package execution

import (
	"context"
	"time"

	"github.com/arbion/platform/services/api/internal/authorization"
)

// ScheduledWorker is an inert registration of exactly one reviewed workflow.
// No production startup constructs or starts it. There is deliberately no
// environment flag, browser command, account discovery or mutable registration.
type ScheduledWorker struct{ workflow *ScheduledWorkflow }

type ScheduledWorkerReceipt struct {
	Status       string
	ScheduledFor time.Time
	Order        *OwnerOrder
	Generation   *ScheduledGenerationReceipt
}

type scheduledWorkerWork struct {
	ScheduledFor time.Time
	Admitted     bool
	OrderID      string
}

// Nil registers nothing and performs no I/O. A nonnil workflow must be fully
// constructed through the existing fixed-scope composition boundary.
func NewScheduledWorker(workflow *ScheduledWorkflow) (*ScheduledWorker, error) {
	if workflow != nil && (workflow.owner == nil || workflow.owner.store == nil || workflow.owner.store.db == nil ||
		workflow.generator == nil || workflow.refresh == nil || !validScheduledWorkerScope(workflow.scope)) {
		return nil, ErrNotAuthorized
	}
	return &ScheduledWorker{workflow: workflow}, nil
}

// RunOnce performs one historical lifecycle action OR one newly admitted slot,
// never both. Neither browser time nor a caller-selected slot controls it.
// A saved/ambiguous admission cannot re-enter generation, preview or submission.
func (w *ScheduledWorker) RunOnce(ctx context.Context) (ScheduledWorkerReceipt, error) {
	if w == nil || w.workflow == nil {
		return ScheduledWorkerReceipt{Status: "DISABLED"}, nil
	}
	if err := ctx.Err(); err != nil {
		return ScheduledWorkerReceipt{}, err
	}
	f := w.workflow
	p := authorization.Principal{UserID: f.scope.Owner.OwnerID, Entitlement: authorization.EntitlementFounder}
	if err := f.owner.checkOwner(ctx, p); err != nil {
		return ScheduledWorkerReceipt{}, err
	}
	work, err := f.owner.store.claimScheduledWork(ctx, f.scope)
	if err != nil {
		return ScheduledWorkerReceipt{}, err
	}
	r := ScheduledWorkerReceipt{Status: "IDLE", ScheduledFor: work.ScheduledFor}
	if work.OrderID != "" {
		// A second scoped read protects this callback even if another worker
		// progressed the order after the transactional work selection.
		o, err := f.Get(ctx, p, work.OrderID)
		if err != nil {
			return r, err
		}
		r.Status = scheduledWorkerAction(o.State)
		switch r.Status {
		case "RECOVERY":
			o, err = f.Recover(ctx, p, o.ID)
		case "RECONCILE":
			o, err = f.Reconcile(ctx, p, o.ID)
		case "SETTLE":
			o, err = f.Settle(ctx, p, o.ID)
		case "WAITING", "SLOT_RECORDED":
			// PREPARED is not a retry permit; closed history is not a new slot.
		default:
			return r, ErrUnreconciled
		}
		if err != nil {
			return r, err
		}
		r.Order = &o
		return r, nil
	}
	if !work.Admitted {
		if !work.ScheduledFor.IsZero() {
			r.Status = "SLOT_RECORDED"
		}
		return r, nil
	}
	r.Status = "RUN"
	g, o, err := f.RunSlot(ctx, p, work.ScheduledFor)
	r.Generation, r.Order = &g, o
	return r, err
}

func scheduledWorkerAction(state string) string {
	switch state {
	case "SUBMISSION_UNKNOWN", "REJECTED_HELD":
		return "RECOVERY"
	case "BROKER_ACKNOWLEDGED", "PARTIALLY_FILLED":
		return "RECONCILE"
	case "AWAITING_ACCOUNT_SETTLEMENT":
		return "SETTLE"
	case "PREPARED":
		return "WAITING"
	case "SETTLED", "NOT_SENT":
		return "SLOT_RECORDED"
	default:
		return "UNAVAILABLE"
	}
}

// Run is opt-in server composition, not activation. It stops on any error;
// provider failures are not blindly retried. Normal pending-order observations
// advance one stage per bounded poll, independently of the browser session.
func (w *ScheduledWorker) Run(ctx context.Context) error {
	for {
		r, err := w.RunOnce(ctx)
		if err != nil || r.Status == "DISABLED" {
			return err
		}
		timer := time.NewTimer(30 * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}
