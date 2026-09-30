package execution

import (
	"context"
	"errors"
	"testing"

	"github.com/arbion/platform/services/api/internal/financial"
)

func TestPostgresOwnerWorkflowBindsEveryFixedScopeField(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	f := newSendFixture(t, ctx, pool, "BUY")
	for name, change := range map[string]func(*OwnerScope){
		"account":    func(s *OwnerScope) { s.AccountID = f.order.Request.OwnerID },
		"connection": func(s *OwnerScope) { s.ConnectionID = f.order.Request.OwnerID },
		"allocation": func(s *OwnerScope) { s.CapitalBucketID = f.order.Request.OwnerID },
		"product":    func(s *OwnerScope) { s.ProductID = "ETH-USD" },
	} {
		t.Run(name, func(t *testing.T) {
			scope := ownerScopeFor(f.order.Request)
			change(&scope)
			w, err := NewOwnerWorkflow(NewPostgresStore(pool), scope, ownerNoIODependencies())
			if err != nil {
				t.Fatal(err)
			}
			for command, run := range ownerCommands(w, f.principal, f.order.ID) {
				if err := run(ctx); !errors.Is(err, ErrNotFound) {
					t.Fatal("same-owner out-of-scope order admitted", command, err)
				}
			}
		})
	}
	w, err := NewOwnerWorkflow(NewPostgresStore(pool), ownerScopeFor(f.order.Request), ownerNoIODependencies())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE user_entitlements SET status='revoked' WHERE user_id=$1 AND entitlement_key='founder'`, f.principal.UserID); err != nil {
		t.Fatal(err)
	}
	for command, run := range ownerCommands(w, f.principal, f.order.ID) {
		if err := run(ctx); !errors.Is(err, ErrNotAuthorized) {
			t.Fatal("stale founder principal admitted", command, err)
		}
	}
}

func TestPostgresOwnerWorkflowPrepareReplayAndLostResponse(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	f := newSendFixture(t, ctx, pool, "BUY")
	r := f.order.Request
	scope, deps := ownerScopeFor(r), ownerNoIODependencies()
	w, err := NewOwnerWorkflow(NewPostgresStore(lostCommitDB{pool}), scope, deps)
	if err != nil {
		t.Fatal(err)
	}
	c := OwnerPrepareCommand{r.ClientOrderID, r.Side, r.BaseSize, r.LimitPrice, r.FeeAllowanceUSD, r.MaximumDebitUSD}
	if _, err = w.Prepare(ctx, f.principal, c); !errors.Is(err, ErrCommitUnknown) {
		t.Fatal("lost prepare response not retained", err)
	}
	w, err = NewOwnerWorkflow(NewPostgresStore(pool), scope, deps)
	if err != nil {
		t.Fatal(err)
	}
	o, err := w.Prepare(ctx, f.principal, c)
	if err != nil || o.State != "PREPARED" || o.Attempted || o.AccountHeld || o.CapitalHeld {
		t.Fatal("prepare implied authority or lost saved request", err, o)
	}
	again, err := w.Prepare(ctx, f.principal, c)
	if err != nil || again.ID != o.ID || again.RequestDigest != o.RequestDigest {
		t.Fatal("prepare minted another identity", err)
	}
	c.LimitPrice = "59999"
	if _, err = w.Prepare(ctx, f.principal, c); !errors.Is(err, ErrConflict) {
		t.Fatal("request key rebound to changed exact terms", err)
	}
	var count int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM execution_orders WHERE client_order_id=$1`, ownerClientID(r.OwnerID, c.RequestKey)).Scan(&count); err != nil || count != 1 {
		t.Fatal("lost prepare response duplicated durable order", err, count)
	}
}

func TestPostgresOwnerWorkflowCannotRestartPreparationAfterAttempt(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	for _, rejected := range []bool{false, true} {
		f := newSendFixture(t, ctx, pool, "BUY")
		_, err := NewPostgresStore(pool).SendConfirmed(ctx, f.principal.UserID, f.order.ID, f.evidence, f.vault, sendFunc(func(context.Context, *financial.Credentials, ConfirmedSubmission) (SubmissionAcknowledgement, error) {
			if rejected {
				return SubmissionAcknowledgement{}, &SubmissionRejectedError{Code: "REJECTED"}
			}
			return SubmissionAcknowledgement{}, errors.New("synthetic uncertain transport")
		}))
		if err == nil {
			t.Fatal("fixture did not remain unresolved")
		}
		w, err := NewOwnerWorkflow(NewPostgresStore(pool), ownerScopeFor(f.order.Request), ownerNoIODependencies())
		if err != nil {
			t.Fatal(err)
		}
		o, err := w.Get(ctx, f.principal, f.order.ID)
		want := "SUBMISSION_UNKNOWN"
		if rejected {
			want = "REJECTED_HELD"
		}
		if err != nil || o.State != want || !o.Attempted || !o.AccountHeld || !o.CapitalHeld {
			t.Fatal("ambiguous or rejected submission presented as complete", err, o)
		}
		for name, run := range ownerCommands(w, f.principal, f.order.ID) {
			if name == "send" || name == "preflight" || name == "approve" {
				if err := run(ctx); !errors.Is(err, ErrAlreadyAttempted) {
					t.Fatal("attempt allowed preparation/retry", name, err)
				}
			}
		}
		assertSendHeld(t, ctx, pool, f.order, "")
	}
}
