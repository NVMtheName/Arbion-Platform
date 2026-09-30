package execution

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

// Direct inserts below test storage guards using a real, successfully committed
// owner-authorized Claim. They are not a public proof or resolution writer.
const noSendGuardInsert = `INSERT INTO execution_no_send_resolutions(order_id,owner_id,financial_account_id,capital_bucket_id,authorization_id,request_digest,credential_generation,claimed_at)
	VALUES($1,$2,$3,$4,$5,$6,$7,$8)`

func noSendGuardArgs(f sendFixture, a Attempt) []any {
	r := f.order.Request
	return []any{f.order.ID, r.OwnerID, r.AccountID, r.CapitalBucketID, a.AuthorizationID, f.order.RequestDigest, a.CredentialGeneration, a.ClaimedAt}
}

func TestPostgresNoSendGuardRejectsMismatchedClaim(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	s := NewPostgresStore(pool)
	f := newSendFixture(t, ctx, pool, "BUY")
	a, err := s.Claim(ctx, f.order.Request.OwnerID, f.order.ID, NewOwnerAuthority(NewSavedPreflightVerifier(f.evidence)))
	if err != nil {
		t.Fatal(err)
	}
	other := newSendFixture(t, ctx, pool, "BUY")
	otherAttempt, err := s.Claim(ctx, other.order.Request.OwnerID, other.order.ID, NewOwnerAuthority(NewSavedPreflightVerifier(other.evidence)))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name  string
		index int
		value any
	}{
		{"order", 0, other.order.ID},
		{"owner", 1, other.order.Request.OwnerID},
		{"account", 2, other.order.Request.AccountID},
		{"bucket", 3, other.order.Request.CapitalBucketID},
		{"authorization", 4, otherAttempt.AuthorizationID},
		{"digest", 5, strings.Repeat("0", 64)},
		{"generation", 6, a.CredentialGeneration + 1},
		{"claim time", 7, a.ClaimedAt.Add(time.Microsecond)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := noSendGuardArgs(f, a)
			args[tc.index] = tc.value
			_, err := pool.Exec(ctx, noSendGuardInsert, args...)
			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) || pgErr.Code != "23514" || pgErr.ConstraintName != "execution_no_send_evidence" {
				t.Fatal("exact claim mismatch did not fail the resolution guard", err)
			}
			assertSendHeld(t, ctx, pool, f.order, "")
			assertSendHeld(t, ctx, pool, other.order, "")
		})
	}
	// The same original claim still closes with all exact fields after every
	// rejected insert; no failed candidate consumed or released either hold.
	if _, err = pool.Exec(ctx, noSendGuardInsert, noSendGuardArgs(f, a)...); err != nil {
		t.Fatal("exact original claim could not close", err)
	}
	assertNoSendClosed(t, ctx, pool, f)
	assertSendHeld(t, ctx, pool, other.order, "")
}

func TestPostgresNoSendGuardSerializesBrokerReceipts(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	for _, kind := range []string{"ack", "rejection"} {
		for _, noSendWins := range []bool{false, true} {
			t.Run(kind+map[bool]string{false: " wins", true: " loses"}[noSendWins], func(t *testing.T) {
				f := newSendFixture(t, ctx, pool, "BUY")
				a, err := NewPostgresStore(pool).Claim(ctx, f.order.Request.OwnerID, f.order.ID, NewOwnerAuthority(NewSavedPreflightVerifier(f.evidence)))
				if err != nil {
					t.Fatal(err)
				}
				brokerSQL := `INSERT INTO execution_submission_rejections(order_id,owner_id,error_code) VALUES($1,$2,$3)`
				brokerArgs := []any{f.order.ID, f.order.Request.OwnerID, "REJECTED"}
				if kind == "ack" {
					brokerSQL = `INSERT INTO execution_broker_acknowledgements(order_id,owner_id,provider_order_id) VALUES($1,$2,$3)`
					brokerArgs[2] = f.ack.ProviderOrderID
				}
				winner, err := pool.Begin(ctx)
				if err != nil {
					t.Fatal(err)
				}
				defer rollback(winner)
				var winnerPID uint32
				if err = winner.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&winnerPID); err != nil {
					t.Fatal(err)
				}
				waitSQL, waitArgs := noSendGuardInsert, noSendGuardArgs(f, a)
				if noSendWins {
					if _, err = winner.Exec(ctx, noSendGuardInsert, waitArgs...); err != nil {
						t.Fatal(err)
					}
					waitSQL, waitArgs = brokerSQL, brokerArgs
				} else {
					// The winner holds ONLY the original attempt row when closure
					// reaches its lock wait, not an account/advisory/receipt lock.
					if _, err = winner.Exec(ctx, `SELECT order_id FROM execution_dispatch_attempts WHERE order_id=$1 FOR UPDATE`, f.order.ID); err != nil {
						t.Fatal(err)
					}
				}
				waiter, err := pool.Acquire(ctx)
				if err != nil {
					t.Fatal(err)
				}
				waiterPID := waiter.Conn().PgConn().PID()
				result := make(chan error, 1)
				go func() {
					defer waiter.Release()
					_, err := waiter.Exec(ctx, waitSQL, waitArgs...)
					result <- err
				}()
				waitExecutionLock(t, ctx, pool, waiterPID)
				var blockedByWinner bool
				if err = pool.QueryRow(ctx, `SELECT $2::integer=ANY(pg_blocking_pids($1))`, waiterPID, winnerPID).Scan(&blockedByWinner); err != nil || !blockedByWinner {
					t.Fatal("writer did not wait on the original-attempt holder", err, blockedByWinner)
				}
				if !noSendWins {
					if _, err = winner.Exec(ctx, brokerSQL, brokerArgs...); err != nil {
						t.Fatal(err)
					}
				}
				if err = winner.Commit(ctx); err != nil {
					t.Fatal(err)
				}
				var pgErr *pgconn.PgError
				wantConstraint := "execution_no_send_evidence"
				if noSendWins {
					wantConstraint = "execution_receipt_conflict"
				}
				if err = <-result; !errors.As(err, &pgErr) || pgErr.Code != "23514" || pgErr.ConstraintName != wantConstraint {
					t.Fatal("waiting writer accepted contradictory evidence", err)
				}
				if noSendWins {
					assertNoSendClosed(t, ctx, pool, f)
				} else {
					providerID := ""
					if kind == "ack" {
						providerID = f.ack.ProviderOrderID
					} else if receipt, err := NewPostgresStore(pool).ReadSubmissionRejection(ctx, f.order.Request.OwnerID, f.order.ID); err != nil || receipt.Code != "REJECTED" {
						t.Fatal("winning rejection was not preserved", err)
					}
					assertSendHeld(t, ctx, pool, f.order, providerID)
				}
			})
		}
	}
}
