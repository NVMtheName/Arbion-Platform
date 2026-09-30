package execution

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/financial"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func readOwnerState(t *testing.T, ctx context.Context, pool *pgxpool.Pool, order Order, state string) OwnerOrder {
	t.Helper()
	o, err := NewPostgresStore(pool).ReadOwnerOrder(ctx, order.Request.OwnerID, order.ID)
	if err != nil || o.State != state || o.ID != order.ID || o.RequestDigest != order.RequestDigest || o.AccountID != order.Request.AccountID || o.ConnectionID != order.Request.ConnectionID || o.CapitalBucketID != order.Request.CapitalBucketID {
		t.Fatal("incorrect owner snapshot", err, o)
	}
	return o
}

func TestPostgresOwnerOrderLifecycleAndExactOwnSlot(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	s := NewPostgresStore(pool)
	r := newExecutionFixture(t, ctx, pool)
	prepared, err := s.Prepare(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	if got := readOwnerState(t, ctx, pool, prepared, "PREPARED"); got.Attempted || got.ApprovalStatus != "NONE" || got.AccountHeld || got.CapitalHeld {
		t.Fatal("prepared order acquired authority or holds", got)
	}
	f := newSendFixture(t, ctx, pool, "BUY")
	approved := readOwnerState(t, ctx, pool, f.order, "APPROVED")
	if approved.ApprovalStatus != "RECORDED" || approved.ApprovalExpiresAt == nil || !strings.Contains(approved.Summary, "requires current checks") {
		t.Fatal("approval projection implied executable authority", approved)
	}
	if err = s.RevokeOwnerApproval(ctx, f.order.Request.OwnerID, f.order.ID); err != nil {
		t.Fatal(err)
	}
	if revoked := readOwnerState(t, ctx, pool, f.order, "PREPARED"); revoked.ApprovalStatus != "REVOKED" {
		t.Fatal("approval revocation disappeared", revoked)
	}
	if _, err = s.ReadOwnerOrder(ctx, r.OwnerID, f.order.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("cross-owner order exposed", err)
	}
	if _, err = s.ReadOwnerOrder(ctx, "invalid", f.order.ID); !errors.Is(err, ErrInvalid) {
		t.Fatal("invalid owner accepted", err)
	}

	f, observation := newPostgresObservationFixture(t, ctx, pool)
	r = f.order.Request
	ack := readOwnerState(t, ctx, pool, f.order, "BROKER_ACKNOWLEDGED")
	if !ack.Attempted || !ack.AccountHeld || !ack.CapitalHeld || ack.FillCount != 0 || ack.Accounting != nil {
		t.Fatal("acknowledgement was displayed as settled", ack)
	}
	observation.Status, observation.Totals = "OPEN", Totals{1, "0.0004", "24", "0.24"}
	observation.Fills[0].BaseQuantity, observation.Fills[0].GrossUSD, observation.Fills[0].FeeUSD = "0.0004", "24", "0.24"
	observation.Fills[0].ProviderEvidence.Size = "0.0004"
	if _, err = s.ReconcileBrokerOrder(ctx, r.OwnerID, f.order.ID, f.vault, savedObservation(observation)); err != nil {
		t.Fatal(err)
	}
	partial := readOwnerState(t, ctx, pool, f.order, "PARTIALLY_FILLED")
	if partial.FillCount != 1 || partial.BaseFilled != "0.0004" || partial.GrossUSD != "24" || partial.FeeUSD != "0.24" || !partial.AccountHeld || !partial.CapitalHeld {
		t.Fatal("partial evidence lost exact amounts or holds", partial)
	}
	if _, err = s.CancelBrokerOrder(ctx, r.OwnerID, f.order.ID, f.vault, cancellationFunc(func(context.Context, *financial.Credentials, ConfirmedSubmission, Attempt) (CancellationAcknowledgement, error) {
		return CancellationAcknowledgement{ProviderOrderID: f.ack.ProviderOrderID, Accepted: true}, nil
	})); err != nil {
		t.Fatal(err)
	}
	if cancelled := readOwnerState(t, ctx, pool, f.order, "PARTIALLY_FILLED"); cancelled.CancellationStatus != "ACCEPTED" || !cancelled.CapitalHeld || !cancelled.AccountHeld {
		t.Fatal("cancellation receipt falsely closed the order", cancelled)
	}
	observation.Status = "CANCELLED"
	if _, err = s.ReconcileBrokerOrder(ctx, r.OwnerID, f.order.ID, f.vault, savedObservation(observation)); err != nil {
		t.Fatal(err)
	}
	terminal := readOwnerState(t, ctx, pool, f.order, "AWAITING_ACCOUNT_SETTLEMENT")
	if terminal.AccountHeld || !terminal.CapitalHeld || terminal.TerminalStatus != "CANCELLED" || terminal.Accounting != nil {
		t.Fatal("terminal order falsely implied account settlement", terminal)
	}
	var now time.Time
	if err = pool.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		t.Fatal(err)
	}
	observation.StartedAt, observation.ObservedAt, observation.Fills[0].ObservedAt = now, now, now
	evidence := AccountSettlementEvidence{PortfolioID: f.preflight.PortfolioID, Observation: observation, CashUSD: "975.76", AvailableCashUSD: "975.76", TotalBase: "1.0004", AvailableBase: "1.0004", Complete: true, NoOpenOrders: true, StartedAt: now, CompletedAt: now}
	if _, err = s.SettleBrokerAccount(ctx, r.OwnerID, f.order.ID, f.vault, savedSettlement(evidence)); err != nil {
		t.Fatal(err)
	}
	settled := readOwnerState(t, ctx, pool, f.order, "SETTLED")
	if settled.AccountHeld || settled.CapitalHeld || settled.Accounting == nil || settled.Accounting.ClosingCashUSD != "975.76" || settled.Accounting.ClosingBase != "1.0004" {
		t.Fatal("exact settlement receipt was not projected", settled)
	}
	// A later order's own account slot cannot corrupt this historical closure.
	nextRequest := r
	if err = pool.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(&nextRequest.ClientOrderID); err != nil {
		t.Fatal(err)
	}
	next, err := s.Prepare(ctx, nextRequest)
	if err != nil {
		t.Fatal(err)
	}
	nextAuthority := authorityFunc(func(c context.Context, tx pgx.Tx, o Order, at time.Time) (Authorization, error) {
		a, err := fixtureAuthority.AuthorizeDispatch(c, tx, o, at)
		a.CredentialGeneration = f.approval.CredentialGeneration
		return a, err
	})
	if _, err = s.Claim(ctx, r.OwnerID, next.ID, nextAuthority); err != nil {
		t.Fatal("synthetic later order could not claim its own slot", err)
	}
	if got := readOwnerState(t, ctx, pool, f.order, "SETTLED"); got.AccountHeld || got.CapitalHeld {
		t.Fatal("another order's hold contaminated historical status", got)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO execution_reconciliation_blocks(order_id,owner_id,financial_account_id,reason,payload) VALUES($1,$2,$3,'INVALID_FILL','{}')`, f.order.ID, r.OwnerID, r.AccountID); err != nil {
		t.Fatal(err)
	}
	if got := readOwnerState(t, ctx, pool, f.order, "SETTLED"); !got.AccountBlocked || !strings.Contains(got.Summary, "blocked for reconciliation review") {
		t.Fatal("historical settlement concealed current account quarantine", got)
	}
}

type ownerStatusPausedCommitTx struct {
	pgx.Tx
	entered chan struct{}
	release chan struct{}
}

func (tx ownerStatusPausedCommitTx) Commit(ctx context.Context) error {
	close(tx.entered)
	select {
	case <-tx.release:
		return tx.Tx.Commit(ctx)
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestPostgresOwnerOrderNoSendSnapshotAndRejectedHeld(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	f := newSendFixture(t, ctx, pool, "BUY")
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	db := &sendBoundaryDB{Database: pool, beforeBegin: func(_ context.Context, n int) error {
		if n == 3 {
			return ErrNotAuthorized
		}
		return nil
	}, wrap: func(n int, tx pgx.Tx) pgx.Tx {
		if n == 4 {
			return ownerStatusPausedCommitTx{tx, entered, release}
		}
		return tx
	}}
	result := make(chan error, 1)
	go func() {
		_, err := NewPostgresStore(db).SendConfirmed(ctx, f.order.Request.OwnerID, f.order.ID, f.evidence, f.vault, sendFunc(func(context.Context, *financial.Credentials, ConfirmedSubmission) (SubmissionAcknowledgement, error) {
			t.Error("no-send fixture entered sender")
			return f.ack, nil
		}))
		result <- err
	}()
	select {
	case <-entered:
	case err := <-result:
		t.Fatal("closure failed before snapshot boundary", err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	before := readOwnerState(t, ctx, pool, f.order, "SUBMISSION_UNKNOWN")
	if !before.AccountHeld || !before.CapitalHeld || !before.Attempted {
		t.Fatal("snapshot saw uncommitted closure or released holds", before)
	}
	once.Do(func() { close(release) })
	if err := <-result; !errors.Is(err, ErrSubmissionNotSent) {
		t.Fatal(err)
	}
	closed := readOwnerState(t, ctx, pool, f.order, "NOT_SENT")
	if closed.AccountHeld || closed.CapitalHeld || !closed.Attempted || closed.Accounting != nil {
		t.Fatal("positive no-send receipt projected as broker settlement", closed)
	}
	f = newSendFixture(t, ctx, pool, "BUY")
	if _, err := NewPostgresStore(pool).SendConfirmed(ctx, f.order.Request.OwnerID, f.order.ID, f.evidence, f.vault, sendFunc(func(context.Context, *financial.Credentials, ConfirmedSubmission) (SubmissionAcknowledgement, error) {
		return SubmissionAcknowledgement{}, &SubmissionRejectedError{Code: "INSUFFICIENT_FUND"}
	})); !errors.Is(err, ErrSubmissionRejected) {
		t.Fatal(err)
	}
	rejected := readOwnerState(t, ctx, pool, f.order, "REJECTED_HELD")
	if !rejected.AccountHeld || !rejected.CapitalHeld || rejected.Accounting != nil || rejected.TerminalStatus != "" {
		t.Fatal("submission rejection falsely implied final accounting", rejected)
	}
}
