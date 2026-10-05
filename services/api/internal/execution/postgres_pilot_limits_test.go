package execution

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/financial"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type pilotQueryRower interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func pilotRequestJSON(t *testing.T, r Request) map[string]any {
	t.Helper()
	encoded, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var request map[string]any
	if err = json.Unmarshal(encoded, &request); err != nil {
		t.Fatal(err)
	}
	return request
}

func insertPilotOrderSQL(ctx context.Context, db pilotQueryRower, r Request, request map[string]any) (string, error) {
	digest, err := requestDigest(r)
	if err != nil {
		return "", err
	}
	body, err := json.Marshal(request)
	if err != nil {
		return "", err
	}
	var id string
	err = db.QueryRow(ctx, `INSERT INTO execution_orders(owner_id,financial_account_id,provider_connection_id,capital_bucket_id,client_order_id,request_digest,request)
	 VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id::text`, r.OwnerID, r.AccountID, r.ConnectionID, r.CapitalBucketID, r.ClientOrderID, digest, body).Scan(&id)
	return id, err
}

func requirePilotDenial(t *testing.T, err error) {
	t.Helper()
	var postgres *pgconn.PgError
	if !errors.As(err, &postgres) || postgres.Code != "23514" {
		t.Fatalf("expected storage pilot denial, got %v", err)
	}
}

func requireCurrentPilotDenial(t *testing.T, err error) {
	t.Helper()
	requirePilotDenial(t, err)
	var postgres *pgconn.PgError
	_ = errors.As(err, &postgres)
	if postgres.ConstraintName != "execution_current_controls" {
		t.Fatalf("denial did not use the current-control boundary: %v", err)
	}
}

func pilotTestDeadline(t *testing.T, ctx context.Context, pool *pgxpool.Pool) time.Time {
	t.Helper()
	var deadline time.Time
	if err := pool.QueryRow(ctx, `SELECT clock_timestamp()+interval '3 seconds'`).Scan(&deadline); err != nil {
		t.Fatal(err)
	}
	return deadline.UTC()
}

func waitPastPilotDeadline(t *testing.T, ctx context.Context, pool *pgxpool.Pool, deadline time.Time) {
	t.Helper()
	// Synchronize with the same database wall clock as the guard, not a guessed
	// runner sleep. Every fixture deadline is bounded to three seconds.
	if _, err := pool.Exec(ctx, `SELECT pg_sleep(GREATEST(0,extract(epoch FROM ($1::timestamptz-clock_timestamp())))+0.01)`, deadline); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresPilotLimitsRejectDirectSQLBypass(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	for name, mutate := range map[string]func(map[string]any){
		"missing": func(r map[string]any) { delete(r, "PilotLimits") },
		"null":    func(r map[string]any) { r["PilotLimits"] = nil },
		"array":   func(r map[string]any) { r["PilotLimits"] = []any{} },
		"string":  func(r map[string]any) { r["PilotLimits"] = "limits" },
		"extra field": func(r map[string]any) {
			r["PilotLimits"].(map[string]any)["EnableLive"] = true
		},
		"missing cap": func(r map[string]any) { delete(r["PilotLimits"].(map[string]any), "MaximumOrderUSD") },
		"numeric cap": func(r map[string]any) { r["PilotLimits"].(map[string]any)["MaximumOrderUSD"] = 100 },
		"zero cap":    func(r map[string]any) { r["PilotLimits"].(map[string]any)["MaximumOrderUSD"] = "0" },
		"signed cap":  func(r map[string]any) { r["PilotLimits"].(map[string]any)["MaximumOrderUSD"] = "+100" },
		"negative cap": func(r map[string]any) {
			r["PilotLimits"].(map[string]any)["MaximumOrderUSD"] = "-100"
		},
		"nonfinite cap": func(r map[string]any) { r["PilotLimits"].(map[string]any)["MaximumOrderUSD"] = "NaN" },
		"exponent cap":  func(r map[string]any) { r["PilotLimits"].(map[string]any)["MaximumOrderUSD"] = "1e2" },
		"leading zero":  func(r map[string]any) { r["PilotLimits"].(map[string]any)["MaximumOrderUSD"] = "0100" },
		"cap overflow": func(r map[string]any) {
			r["PilotLimits"].(map[string]any)["MaximumOrderUSD"] = "1000000000000000000"
		},
		"cap precision": func(r map[string]any) {
			r["PilotLimits"].(map[string]any)["MaximumOrderUSD"] = "100.0000000000000000001"
		},
		"missing expiry": func(r map[string]any) { delete(r["PilotLimits"].(map[string]any), "ExpiresAt") },
		"relative expiry": func(r map[string]any) {
			r["PilotLimits"].(map[string]any)["ExpiresAt"] = "3 hours"
		},
		"invalid date": func(r map[string]any) {
			r["PilotLimits"].(map[string]any)["ExpiresAt"] = "2099-02-30T00:00:00Z"
		},
		"expiry offset": func(r map[string]any) {
			r["PilotLimits"].(map[string]any)["ExpiresAt"] = "2099-01-01T00:00:00+00:00"
		},
		"expiry precision": func(r map[string]any) {
			r["PilotLimits"].(map[string]any)["ExpiresAt"] = "2099-01-01T00:00:00.0000001Z"
		},
		"expiry rollover": func(r map[string]any) {
			r["PilotLimits"].(map[string]any)["ExpiresAt"] = "2099-01-01T24:00:00Z"
		},
		"expired":          func(r map[string]any) { r["PilotLimits"].(map[string]any)["ExpiresAt"] = "2000-01-01T00:00:00Z" },
		"hidden buy debit": func(r map[string]any) { r["MaximumDebitUSD"] = "1" },
		"numeric size":     func(r map[string]any) { r["BaseSize"] = 1 },
		"negative fee":     func(r map[string]any) { r["FeeAllowanceUSD"] = "-1" },
	} {
		t.Run(name, func(t *testing.T) {
			r := newExecutionFixture(t, ctx, pool)
			request := pilotRequestJSON(t, r)
			mutate(request)
			_, err := insertPilotOrderSQL(ctx, pool, r, request)
			requirePilotDenial(t, err)
			var count int
			if err = pool.QueryRow(ctx, `SELECT count(*) FROM execution_orders WHERE client_order_id=$1`, r.ClientOrderID).Scan(&count); err != nil || count != 0 {
				t.Fatal("invalid SQL request left an order", err, count)
			}
		})
	}
}

func TestPostgresPilotLimitsExactBuyAndSellFeeBounds(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	for _, side := range []string{"BUY", "SELL"} {
		for _, cap := range []string{"60.60", "60.599999999999999999"} {
			t.Run(side+"/"+cap, func(t *testing.T) {
				r := newExecutionFixture(t, ctx, pool)
				r.Side = side
				if side == "SELL" {
					r.MaximumDebitUSD = "0"
				}
				r.PilotLimits.MaximumOrderUSD = "60.60"
				request := pilotRequestJSON(t, r)
				// Mutate only the SQL payload for the over-cap bypass attempt so
				// Go validation cannot satisfy this storage-boundary assertion.
				request["PilotLimits"].(map[string]any)["MaximumOrderUSD"] = cap
				id, err := insertPilotOrderSQL(ctx, pool, r, request)
				if cap != "60.60" {
					requireCurrentPilotDenial(t, err)
					return
				}
				if err != nil {
					t.Fatal("exact boundary was rejected", err)
				}
				var expiry time.Time
				if err = pool.QueryRow(ctx, `SELECT check_execution_pilot_limits($1)`, id).Scan(&expiry); err != nil || !expiry.Equal(r.PilotLimits.ExpiresAt) {
					t.Fatal("saved pilot deadline changed", err, expiry)
				}
				if _, err = pool.Exec(ctx, `UPDATE execution_orders SET request=jsonb_set(request,'{PilotLimits,MaximumOrderUSD}','"1000"') WHERE id=$1`, id); err == nil {
					t.Fatal("saved pilot cap was mutable")
				}
			})
		}
	}
}

func TestPostgresPilotLimitsNewAdmissionsExpire(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	f := newAuthorityFixture(t, ctx, pool, "BUY")
	r := f.order.Request
	if err := pool.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(&r.ClientOrderID); err != nil {
		t.Fatal(err)
	}
	r.PilotLimits = &OwnerPilotLimits{MaximumOrderUSD: "60.60", ExpiresAt: pilotTestDeadline(t, ctx, pool)}
	s := NewPostgresStore(pool)
	o, err := s.Prepare(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE financial_accounts SET provider_account_id='portfolio:'||id::text WHERE id=$1`, r.AccountID); err != nil {
		t.Fatal(err)
	}
	waitPastPilotDeadline(t, ctx, pool, r.PilotLimits.ExpiresAt)
	if replay, err := s.Prepare(ctx, r); err != nil || !reflect.DeepEqual(replay, o) {
		t.Fatal("expired exact preparation replay lost saved identity", err, replay)
	}
	newRequest := r
	if err = pool.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(&newRequest.ClientOrderID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Prepare(ctx, newRequest); !errors.Is(err, ErrNotAuthorized) {
		t.Fatal("expired pilot prepared a new order", err)
	}
	var deadline time.Time
	err = pool.QueryRow(ctx, `SELECT check_execution_pilot_limits($1)`, o.ID).Scan(&deadline)
	requireCurrentPilotDenial(t, err)
	_, err = pool.Exec(ctx, `WITH verified AS (
	 UPDATE auth_totp_factors SET updated_at=clock_timestamp(),last_used_step=floor(extract(epoch FROM clock_timestamp())/30)
	 WHERE user_id=$2 RETURNING updated_at,enabled_at)
	 INSERT INTO execution_owner_approvals(order_id,owner_id,request_digest,credential_generation,mfa_method,mfa_verified_at,mfa_enabled_at,approved_at,expires_at)
	 SELECT $1,$2,$3,$4,'totp',updated_at,enabled_at,clock_timestamp(),clock_timestamp()+interval '1 minute' FROM verified`, o.ID, r.OwnerID, o.RequestDigest, f.approval.CredentialGeneration)
	requireCurrentPilotDenial(t, err)
	_, err = pool.Exec(ctx, `WITH observed AS (SELECT clock_timestamp() AS at)
	 INSERT INTO execution_provider_preflights(order_id,owner_id,financial_account_id,provider_connection_id,credential_generation,request_digest,portfolio_id,reconciliation_id,observed_at,expires_at,evidence)
	 SELECT $1::uuid,$2::uuid,$3::uuid,$4::uuid,$5::bigint,$6::text,$3::uuid,$7::uuid,at,at+interval '30 seconds',
	 jsonb_build_object('RequestDigest',$6::text,'PortfolioID',$3::uuid::text,'StartedAt',at) FROM observed`, o.ID, r.OwnerID, r.AccountID, r.ConnectionID, f.approval.CredentialGeneration, o.RequestDigest, f.proof.ReconciliationID)
	requireCurrentPilotDenial(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO execution_dispatch_attempts(order_id,owner_id,financial_account_id,authorization_id,credential_generation,claimed_at,expires_at)
	 VALUES($1,$2,$3,gen_random_uuid(),$4,clock_timestamp(),clock_timestamp()+interval '1 second')`, o.ID, r.OwnerID, r.AccountID, f.approval.CredentialGeneration)
	requireCurrentPilotDenial(t, err)
	var admissions int
	if err = pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM execution_owner_approvals WHERE order_id=$1)+(SELECT count(*) FROM execution_provider_preflights WHERE order_id=$1)+(SELECT count(*) FROM execution_dispatch_attempts WHERE order_id=$1)`, o.ID).Scan(&admissions); err != nil || admissions != 0 {
		t.Fatal("expired admission persisted", err, admissions)
	}
}

func TestPostgresPilotLimitsClaimAndCommitWaitsFailClosed(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	for _, stage := range []string{"insertion lock", "deferred commit", "deadline extension"} {
		t.Run(stage, func(t *testing.T) {
			r := newExecutionFixture(t, ctx, pool)
			r.PilotLimits = &OwnerPilotLimits{MaximumOrderUSD: "60.60", ExpiresAt: pilotTestDeadline(t, ctx, pool)}
			o, err := NewPostgresStore(pool).Prepare(ctx, r)
			if err != nil {
				t.Fatal(err)
			}
			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer rollback(tx)
			insert := func(db interface {
				Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
			}, expiry time.Time) error {
				_, err := db.Exec(ctx, `INSERT INTO execution_dispatch_attempts(order_id,owner_id,financial_account_id,authorization_id,credential_generation,claimed_at,expires_at)
				 VALUES($1,$2,$3,gen_random_uuid(),1,clock_timestamp(),$4)`, o.ID, r.OwnerID, r.AccountID, expiry)
				return err
			}
			switch stage {
			case "deadline extension":
				requireCurrentPilotDenial(t, insert(tx, r.PilotLimits.ExpiresAt.Add(time.Microsecond)))
			case "deferred commit":
				if err = insert(tx, r.PilotLimits.ExpiresAt); err != nil {
					t.Fatal("fresh exact-boundary claim denied", err)
				}
				waitPastPilotDeadline(t, ctx, pool, r.PilotLimits.ExpiresAt)
				requireCurrentPilotDenial(t, tx.Commit(ctx))
			case "insertion lock":
				if _, err = tx.Exec(ctx, `SELECT id FROM financial_accounts WHERE id=$1 FOR UPDATE`, r.AccountID); err != nil {
					t.Fatal(err)
				}
				conn, err := pool.Acquire(ctx)
				if err != nil {
					t.Fatal(err)
				}
				defer conn.Release()
				defer rollback(tx) // Release the blocker before releasing an in-use connection on failure.
				done := make(chan error, 1)
				// Keep the attempt's own window current past the wait: only the
				// new pilot guard, not the existing attempt expiry, can deny it.
				go func() { done <- insert(conn, r.PilotLimits.ExpiresAt.Add(5*time.Second)) }()
				waitExecutionLock(t, ctx, pool, conn.Conn().PgConn().PID())
				waitPastPilotDeadline(t, ctx, pool, r.PilotLimits.ExpiresAt)
				if err = tx.Commit(ctx); err != nil {
					t.Fatal(err)
				}
				requireCurrentPilotDenial(t, <-done)
			}
			var count int
			if err = pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM execution_dispatch_attempts WHERE order_id=$1)+(SELECT count(*) FROM execution_account_holds WHERE order_id=$1)+(SELECT count(*) FROM execution_capital_reservations WHERE order_id=$1)`, o.ID).Scan(&count); err != nil || count != 0 {
				t.Fatal("denied claim retained an attempt or holds", err, count)
			}
		})
	}
}

func TestPostgresPilotLimitsHistoricalOrderRemainsReadableButCannotClaim(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	r := newExecutionFixture(t, ctx, pool)
	r.PilotLimits = nil
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rollback(tx)
	// Isolated fixture only: reproduce a pre-migration immutable row. The guard
	// is restored in the SAME transaction; failures roll the DDL and row back.
	if _, err = tx.Exec(ctx, `ALTER TABLE execution_orders DISABLE TRIGGER execution_order_guard`); err != nil {
		t.Fatal(err)
	}
	id, err := insertPilotOrderSQL(ctx, tx, r, pilotRequestJSON(t, r))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `ALTER TABLE execution_orders ENABLE TRIGGER execution_order_guard`); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	read, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rollback(read)
	o, err := readOrder(ctx, read, r.OwnerID, "id", id)
	if err != nil || !reflect.DeepEqual(o.Request, r) {
		t.Fatal("historical request was rewritten or became unreadable", err)
	}
	if _, err = NewPostgresStore(pool).Claim(ctx, r.OwnerID, id, fixtureAuthority); !errors.Is(err, ErrNotAuthorized) {
		t.Fatal("historical order gained new pilot authority", err)
	}
}

func TestPostgresPilotLimitsExpireBeforeFinalSenderEntry(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	f := newSendFixture(t, ctx, pool, "BUY", OwnerPilotLimits{MaximumOrderUSD: "60.60", ExpiresAt: pilotTestDeadline(t, ctx, pool)})
	waited, called := false, false
	db := &sendBoundaryDB{Database: pool, beforeBegin: func(c context.Context, n int) error {
		if n == 3 {
			waited = true
			waitPastPilotDeadline(t, c, pool, f.order.Request.PilotLimits.ExpiresAt)
		}
		return nil
	}}
	_, err := NewPostgresStore(db).SendConfirmed(ctx, f.order.Request.OwnerID, f.order.ID, f.evidence, f.vault, sendFunc(func(context.Context, *financial.Credentials, ConfirmedSubmission) (SubmissionAcknowledgement, error) {
		called = true
		return f.ack, nil
	}))
	if !waited || called || !errors.Is(err, ErrNotAuthorized) || !errors.Is(err, ErrSubmissionNotSent) {
		t.Fatal("pilot expired between durable claim and final sender entry", err, waited, called)
	}
	assertNoSendClosed(t, ctx, pool, f)
}

func TestPostgresPilotLimitsFinalSendExpiryPreservesRecoveryAndSettlement(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	f := newSendFixture(t, ctx, pool, "BUY", OwnerPilotLimits{MaximumOrderUSD: "60.60", ExpiresAt: pilotTestDeadline(t, ctx, pool)})
	r := f.order.Request
	called := false
	_, err := NewPostgresStore(pool).SendConfirmed(ctx, r.OwnerID, f.order.ID, f.evidence, f.vault, sendFunc(func(c context.Context, _ *financial.Credentials, _ ConfirmedSubmission) (SubmissionAcknowledgement, error) {
		called = true
		deadline, ok := c.Deadline()
		if !ok || !deadline.After(time.Now()) || deadline.After(r.PilotLimits.ExpiresAt) {
			t.Error("final sender deadline escaped the immutable pilot expiry")
		}
		<-c.Done()
		// A late success cannot turn expired send authority into an acknowledgement.
		return f.ack, nil
	}))
	if !errors.Is(err, ErrSubmissionUnknown) || !called {
		t.Fatal("pilot expiry did not bound the synthetic sender", err, called)
	}
	waitPastPilotDeadline(t, ctx, pool, r.PilotLimits.ExpiresAt)
	assertSendHeld(t, ctx, pool, f.order, "")
	// Restart and all remaining callbacks below are local synthetic adapters.
	s := NewPostgresStore(pool)
	a, err := s.RecoverSubmission(ctx, r.OwnerID, f.order.ID, f.vault, lookupFunc(func(_ context.Context, _ *financial.Credentials, sub ConfirmedSubmission, a Attempt) (SubmissionAcknowledgement, error) {
		if !reflect.DeepEqual(sub.Order, f.order) || a.ClientOrderID != r.ClientOrderID {
			t.Error("post-expiry recovery changed the original order")
		}
		return f.ack, nil
	}))
	if err != nil || a.ProviderOrderID != f.ack.ProviderOrderID {
		t.Fatal("expired pilot stranded original-identity recovery", err, a)
	}
	if _, err = s.CancelBrokerOrder(ctx, r.OwnerID, f.order.ID, f.vault, cancellationFunc(func(context.Context, *financial.Credentials, ConfirmedSubmission, Attempt) (CancellationAcknowledgement, error) {
		return CancellationAcknowledgement{ProviderOrderID: f.ack.ProviderOrderID, Accepted: true}, nil
	})); err != nil {
		t.Fatal("expired pilot blocked risk-reducing cancellation", err)
	}
	assertSendHeld(t, ctx, pool, f.order, f.ack.ProviderOrderID)
	var now time.Time
	if err = pool.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		t.Fatal(err)
	}
	b := BrokerObservation{BrokerIdentity: BrokerIdentity{r.OwnerID, f.order.ID, r.AccountID, r.ConnectionID, r.ClientOrderID, a.ProviderOrderID, r.ProductID, r.Side},
		Status: "CANCELLED", CompleteFills: true, Totals: Totals{0, "0", "0", "0"}, Fills: []Fill{}, StartedAt: now, ObservedAt: now}
	if _, err = s.ReconcileBrokerOrder(ctx, r.OwnerID, f.order.ID, f.vault, savedObservation(b)); err != nil {
		t.Fatal("expired pilot blocked terminal reconciliation", err)
	}
	if err = pool.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		t.Fatal(err)
	}
	b.StartedAt, b.ObservedAt = now, now
	e := AccountSettlementEvidence{PortfolioID: f.preflight.PortfolioID, Observation: b, CashUSD: "1000", AvailableCashUSD: "1000", TotalBase: "1", AvailableBase: "1", Complete: true, NoOpenOrders: true, StartedAt: now, CompletedAt: now}
	settled, err := s.SettleBrokerAccount(ctx, r.OwnerID, f.order.ID, f.vault, savedSettlement(e))
	if err != nil {
		t.Fatal("expired pilot blocked verified account settlement", err)
	}
	assertSettlementReceipt(t, ctx, pool, f, e, settled)
	assertSendCannotRetry(t, ctx, pool, f)
}
