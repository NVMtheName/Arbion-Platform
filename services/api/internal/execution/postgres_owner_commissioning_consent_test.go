package execution

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestPostgresOwnerCommissioningConsentLostCommitAndResponseRecovery(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	for _, committed := range []bool{false, true} {
		t.Run(map[bool]string{false: "commit rolled back", true: "commit response lost"}[committed], func(t *testing.T) {
			f := newOwnerCommissioningFixture(t, ctx, pool)
			r := prepareOwnerCommissioning(t, ctx, f)
			var verifies atomic.Int32
			stepUp := stepUpFunc(func(c context.Context, owner, code string) (string, time.Time, error) {
				verifies.Add(1)
				return mandateConsentStepUp(pool)(c, owner, code)
			})
			db := &sendBoundaryDB{Database: pool, wrap: func(_ int, tx pgx.Tx) pgx.Tx {
				if committed {
					return lostCommitTx{tx}
				}
				return failedNoSendCommitTx{tx}
			}}
			w, err := NewOwnerCommissioning(NewPostgresStore(db), f.terms, stepUp)
			if err != nil {
				t.Fatal(err)
			}
			a, err := w.Approve(ctx, f.principal, r.TermsDigest, r.SnapshotDigest, "synthetic")
			if !committed {
				if !errors.Is(err, ErrCommitUnknown) || a.ID != ownerCommissioningConsentID(f.terms.Pilot.OwnerID, f.terms.Pilot.AccountID) {
					t.Fatal("unknown rollback reported certainty or lost fixed identity", a, err)
				}
				if _, err := f.workflow.Consent(ctx, f.principal); !errors.Is(err, ErrNotFound) {
					t.Fatal("absent consent reported saved", err)
				}
				assertCommissioningCounts(t, ctx, pool, f, 1, 1, 1, 0, 0, 0)
				return
			}
			if err != nil {
				t.Fatal("committed receipt was not exactly recovered", err)
			}
			// Simulate the caller losing the entire HTTP response and constructing a
			// fresh process with a verifier that must never be entered for recovery.
			restarted, err := NewOwnerCommissioning(NewPostgresStore(pool), f.terms, ownerNoIO{})
			if err != nil {
				t.Fatal(err)
			}
			view, err := restarted.Consent(ctx, f.principal)
			if err != nil || view.ID != a.ID || view.SnapshotDigest != a.SnapshotDigest || !view.ApprovedAt.Equal(a.ApprovedAt) || view.RevokedAt != nil {
				t.Fatal("lost response recovery changed original", view, err)
			}
			for _, digests := range [][2]string{{"stale", r.SnapshotDigest}, {r.TermsDigest, "stale"}} {
				if _, err := restarted.Approve(ctx, f.principal, digests[0], digests[1], "must-not-use"); !errors.Is(err, ErrConflict) {
					t.Fatal("historical recovery bypassed exact digests", err)
				}
			}
			got, err := restarted.Approve(ctx, f.principal, r.TermsDigest, r.SnapshotDigest, "must-not-use")
			if err != nil || !reflect.DeepEqual(got, a) || verifies.Load() != 1 {
				t.Fatal("approval retry consumed MFA or renewed receipt", got, err)
			}
			revoked, err := restarted.RevokeInitialConsent(ctx, f.principal)
			if err != nil || revoked.RevokedAt == nil {
				t.Fatal("revocation receipt missing", revoked, err)
			}
			again, err := restarted.RevokeInitialConsent(ctx, f.principal)
			if err != nil || !reflect.DeepEqual(again, revoked) {
				t.Fatal("revoke replay changed receipt", again, err)
			}
			got, err = restarted.Approve(ctx, f.principal, r.TermsDigest, r.SnapshotDigest, "must-not-use")
			if err != nil || !reflect.DeepEqual(got, a) {
				t.Fatal("revoked consent renewed or lost historical recovery", got, err)
			}
			if latest, err := restarted.Consent(ctx, f.principal); err != nil || !reflect.DeepEqual(latest, revoked) {
				t.Fatal("historical approval cleared revocation", latest, err)
			}
			assertCommissioningCounts(t, ctx, pool, f, 1, 1, 1, 1, 0, 0)
		})
	}
}

func TestPostgresOwnerCommissioningConcurrentExplicitConsentHasOneReceipt(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	f := newOwnerCommissioningFixture(t, ctx, pool)
	r := prepareOwnerCommissioning(t, ctx, f)
	// All concurrent transports present the same synthetic verified challenge.
	// Its one durable receipt must be recovered, never consumed into another ID.
	method, verified, err := mandateConsentStepUp(pool)(ctx, f.principal.UserID, "synthetic")
	if err != nil {
		t.Fatal(err)
	}
	f.workflow.stepUp = stepUpFunc(func(context.Context, string, string) (string, time.Time, error) {
		return method, verified, nil
	})
	type result struct {
		consent MandateConsent
		err     error
	}
	results := make(chan result, 6)
	var wg sync.WaitGroup
	for i := 0; i < cap(results); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			a, err := f.workflow.Approve(ctx, f.principal, r.TermsDigest, r.SnapshotDigest, "synthetic")
			results <- result{a, err}
		}()
	}
	wg.Wait()
	close(results)
	var original MandateConsent
	for got := range results {
		if got.err != nil {
			t.Fatal(got.err)
		}
		if original.ID == "" {
			original = got.consent
		}
		if !reflect.DeepEqual(original, got.consent) {
			t.Fatal("concurrent explicit approval minted different receipts")
		}
	}
	if original.ID != ownerCommissioningConsentID(f.terms.Pilot.OwnerID, f.terms.Pilot.AccountID) {
		t.Fatal("initial consent is not stable")
	}
	assertCommissioningCounts(t, ctx, pool, f, 1, 1, 1, 1, 0, 0)
}

func TestPostgresOwnerCommissioningExpiredConsentRemainsHistoricalWithoutMFA(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	f := newOwnerCommissioningFixture(t, ctx, pool)
	if err := pool.QueryRow(ctx, `SELECT date_trunc('second',clock_timestamp())+interval '4 seconds'`).Scan(&f.terms.Pilot.Limits.ExpiresAt); err != nil {
		t.Fatal(err)
	}
	var err error
	f.workflow, err = NewOwnerCommissioning(NewPostgresStore(pool), f.terms, mandateConsentStepUp(pool))
	if err != nil {
		t.Fatal(err)
	}
	f.review, err = f.workflow.Review(ctx, f.principal)
	if err != nil {
		t.Fatal(err)
	}
	r := prepareOwnerCommissioning(t, ctx, f)
	a, err := f.workflow.Approve(ctx, f.principal, r.TermsDigest, r.SnapshotDigest, "synthetic")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `SELECT pg_sleep(GREATEST(0,extract(epoch FROM $1::timestamptz-clock_timestamp()))+0.01)`, a.ExpiresAt); err != nil {
		t.Fatal(err)
	}
	w, err := NewOwnerCommissioning(NewPostgresStore(pool), f.terms, ownerNoIO{})
	if err != nil {
		t.Fatal(err)
	}
	got, err := w.Approve(ctx, f.principal, r.TermsDigest, r.SnapshotDigest, "must-not-use")
	if err != nil || !reflect.DeepEqual(got, a) {
		t.Fatal("expiry renewed consent or prevented historical recovery", got, err)
	}
	view, err := w.Consent(ctx, f.principal)
	if err != nil || !view.ExpiresAt.Equal(a.ExpiresAt) {
		t.Fatal("historical expiry projection changed", view, err)
	}
	var until time.Time
	if err = pool.QueryRow(ctx, `SELECT check_execution_mandate_consent($1,$2,$3)`, a.ID, f.principal.UserID, f.terms.Pilot.CapitalBucketID).Scan(&until); err == nil {
		t.Fatal("expired history became authority")
	}
	assertCommissioningCounts(t, ctx, pool, f, 1, 1, 1, 1, 0, 0)
}
