package execution

import (
	"context"
	"time"

	"github.com/arbion/platform/services/api/internal/authorization"
)

// ScheduledScope is an immutable server selection, never a browser/model tool
// argument. Construction grants no authority and does not start a worker.
type ScheduledScope struct {
	Owner                        OwnerScope
	MandateApprovalID, MandateID string
	MandateVersion               int
}

// ScheduledWorkflow joins existing generation, intake and autonomous dispatch.
// It has no timer, retry loop, activation switch or production registration.
type ScheduledWorkflow struct {
	owner     *OwnerWorkflow
	generator *ScheduledGenerator
	scope     ScheduledScope
	refresh   ScheduledReconciliationRefresher
}

// This read-only provider operation persists a comparison, never an owner drift
// acknowledgment. Capture and final send independently validate its evidence.
type ScheduledReconciliationRefresher interface {
	RefreshScheduledExecutionReconciliation(context.Context, authorization.Principal, string) error
}

func NewScheduledWorkflow(store *PostgresStore, scope ScheduledScope, deps OwnerWorkflowDependencies, market ScheduledGenerationMarketCollector, model ScheduledDecisionGenerator, refresh ScheduledReconciliationRefresher) (*ScheduledWorkflow, error) {
	if !validUUID(scope.MandateApprovalID) || !validUUID(scope.MandateID) || scope.MandateVersion < 1 || refresh == nil {
		return nil, ErrNotAuthorized
	}
	owner, err := NewOwnerWorkflow(store, scope.Owner, deps)
	if err != nil {
		return nil, err
	}
	g, err := NewScheduledGenerator(store, market, model)
	if err != nil {
		return nil, err
	}
	g.store = scopedGenerationStore{PostgresStore: store, scope: scope}
	return &ScheduledWorkflow{owner: owner, generator: g, scope: scope, refresh: refresh}, nil
}

// RunSlot executes at most one generation/intake/dispatch chain. A durable
// attempt always returns saved state, never another preview or submission.
// Concurrent fresh callers may preview, but the existing transactional Claim
// permits only one sender entry. Errors never trigger automatic retries.
func (w *ScheduledWorkflow) RunSlot(ctx context.Context, p authorization.Principal, scheduledFor time.Time) (ScheduledGenerationReceipt, *OwnerOrder, error) {
	if w == nil || w.owner == nil || w.generator == nil || w.refresh == nil || !canonicalScheduledSlot(scheduledFor) {
		return ScheduledGenerationReceipt{}, nil, ErrNotAuthorized
	}
	if err := w.owner.checkOwner(ctx, p); err != nil {
		return ScheduledGenerationReceipt{}, nil, err
	}
	slot := ScheduledGenerationSlot{MandateApprovalID: w.scope.MandateApprovalID, MandateID: w.scope.MandateID, MandateVersion: w.scope.MandateVersion, ScheduledFor: scheduledFor}
	receipt, order, err := w.generator.Generate(ctx, p, slot)
	if err != nil || order == nil {
		return receipt, nil, err
	}
	// Bind the durable result/order again before preview or any financial vault
	// access. A receipt or in-memory proposal alone cannot authorize dispatch.
	if receipt.Result == nil || receipt.Result.Outcome != "PROPOSE" || order.Request.MandateApprovalID != w.scope.MandateApprovalID ||
		order.RequestDigest != receipt.Result.RequestDigest || order.Request.ClientOrderID != receipt.Result.ClientOrderID {
		return receipt, nil, ErrConflict
	}
	saved, err := w.Get(ctx, p, order.ID)
	if err != nil || saved.RequestDigest != order.RequestDigest {
		if err == nil {
			err = ErrConflict
		}
		return receipt, nil, err
	}
	if saved.State == "UNAVAILABLE" {
		return receipt, nil, ErrUnreconciled
	}
	if saved.Attempted {
		return receipt, &saved, nil
	}
	if err := w.refresh.RefreshScheduledExecutionReconciliation(ctx, p, w.scope.Owner.AccountID); err != nil {
		return receipt, nil, err
	}
	preflight, err := w.owner.Capture(ctx, p, order.ID)
	if err != nil {
		return receipt, nil, err
	}
	_, err = w.owner.store.SendAutonomous(ctx, p.UserID, order.ID, preflight.EvidenceID, w.owner.deps.Vault, w.owner.deps.Sender)
	if err != nil {
		return receipt, nil, err // an unknown outcome is never projected as success
	}
	saved, err = w.Get(ctx, p, order.ID)
	if err != nil {
		return receipt, nil, err
	}
	return receipt, &saved, nil
}

// Get binds lifecycle operations to this exact historical mandate/source, not
// current consent validity. Expiry/revocation must not strand unresolved orders.
func (w *ScheduledWorkflow) Get(ctx context.Context, p authorization.Principal, id string) (OwnerOrder, error) {
	if w == nil || w.owner == nil {
		return OwnerOrder{}, ErrNotAuthorized
	}
	o, err := w.owner.Get(ctx, p, id)
	if err != nil {
		return OwnerOrder{}, err
	}
	var bound bool
	err = w.owner.store.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM execution_scheduled_proposals
	 WHERE order_id=$1 AND owner_id=$2 AND mandate_approval_id=$3 AND mandate_id=$4 AND mandate_version=$5
	 AND financial_account_id=$6 AND provider_connection_id=$7 AND capital_bucket_id=$8 AND request_digest=$9 AND source_mode='LIVE')`,
		id, p.UserID, w.scope.MandateApprovalID, w.scope.MandateID, w.scope.MandateVersion,
		w.scope.Owner.AccountID, w.scope.Owner.ConnectionID, w.scope.Owner.CapitalBucketID, o.RequestDigest).Scan(&bound)
	if err != nil {
		return OwnerOrder{}, err
	}
	if !bound {
		return OwnerOrder{}, ErrNotFound
	}
	return o, nil
}

func (w *ScheduledWorkflow) Recover(ctx context.Context, p authorization.Principal, id string) (OwnerOrder, error) {
	if _, err := w.Get(ctx, p, id); err != nil {
		return OwnerOrder{}, err
	}
	return w.owner.Recover(ctx, p, id)
}
func (w *ScheduledWorkflow) Reconcile(ctx context.Context, p authorization.Principal, id string) (OwnerOrder, error) {
	if _, err := w.Get(ctx, p, id); err != nil {
		return OwnerOrder{}, err
	}
	return w.owner.Reconcile(ctx, p, id)
}
func (w *ScheduledWorkflow) Cancel(ctx context.Context, p authorization.Principal, id string) (OwnerOrder, error) {
	if _, err := w.Get(ctx, p, id); err != nil {
		return OwnerOrder{}, err
	}
	return w.owner.Cancel(ctx, p, id)
}
func (w *ScheduledWorkflow) Settle(ctx context.Context, p authorization.Principal, id string) (OwnerOrder, error) {
	if _, err := w.Get(ctx, p, id); err != nil {
		return OwnerOrder{}, err
	}
	return w.owner.Settle(ctx, p, id)
}

// Apply the same immutable selection to current facts, the committed claim and
// historical replay. Market scope alone cannot bind connection/bucket/limits.
type scopedGenerationStore struct {
	*PostgresStore
	scope ScheduledScope
}

func (s scopedGenerationStore) matches(f ScheduledGenerationFacts, slot ScheduledGenerationSlot) bool {
	p, o := f.Pilot, s.scope.Owner
	return p.OwnerID == o.OwnerID && p.AccountID == o.AccountID && p.ConnectionID == o.ConnectionID &&
		p.CapitalBucketID == o.CapitalBucketID && p.ProductID == o.ProductID && samePilotLimits(&p.Limits, o.PilotLimits) &&
		slot.MandateApprovalID == s.scope.MandateApprovalID && slot.MandateID == s.scope.MandateID && slot.MandateVersion == s.scope.MandateVersion
}

func (s scopedGenerationStore) ReadScheduledGeneration(ctx context.Context, owner string, slot ScheduledGenerationSlot) (ScheduledGenerationReceipt, error) {
	r, err := s.PostgresStore.ReadScheduledGeneration(ctx, owner, slot)
	if err != nil {
		return ScheduledGenerationReceipt{}, err
	}
	if !s.matches(r.Claim.Facts, r.Claim.Slot) {
		return ScheduledGenerationReceipt{}, ErrNotAuthorized
	}
	return r, nil
}
func (s scopedGenerationStore) GenerationFacts(ctx context.Context, owner string, slot ScheduledGenerationSlot) (ScheduledGenerationFacts, error) {
	f, err := s.PostgresStore.GenerationFacts(ctx, owner, slot)
	if err != nil {
		return ScheduledGenerationFacts{}, err
	}
	if !s.matches(f, slot) {
		return ScheduledGenerationFacts{}, ErrNotAuthorized
	}
	return f, nil
}
func (s scopedGenerationStore) ClaimScheduledGeneration(ctx context.Context, owner string, slot ScheduledGenerationSlot, m ScheduledGenerationMarket) (ScheduledGenerationClaim, bool, error) {
	c, admitted, err := s.PostgresStore.ClaimScheduledGeneration(ctx, owner, slot, m)
	if err != nil {
		return c, false, err
	}
	if !s.matches(c.Facts, c.Slot) {
		return ScheduledGenerationClaim{}, false, ErrNotAuthorized
	}
	return c, admitted, nil
}
