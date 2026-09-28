package orderintent

import (
	"context"
	"errors"
	"testing"

	"github.com/arbion/platform/services/api/internal/execution"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Reuse the real manual-preview fixture to prove both directions of exclusion.
// No production Authority or provider is involved; SQL exercises schema guards.
func testExecutionCapitalFence(t *testing.T, ctx context.Context, pool *pgxpool.Pool, input draft, connectionID string) {
	t.Helper()
	if _, err := pool.Exec(ctx, `INSERT INTO user_entitlements(user_id,entitlement_key) VALUES($1,'founder')`, input.UserID); err != nil {
		t.Fatal(err)
	}
	prepare := func(account, bucket string) execution.Order {
		t.Helper()
		var client string
		if err := pool.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(&client); err != nil {
			t.Fatal(err)
		}
		o, err := execution.NewPostgresStore(pool).Prepare(ctx, execution.Request{OwnerID: input.UserID, AccountID: account, ConnectionID: connectionID, CapitalBucketID: bucket, ClientOrderID: client, ProductID: "BTC-USD", Side: "BUY", BaseSize: "0.001", LimitPrice: "60000", FeeAllowanceUSD: "0.60", MaximumDebitUSD: "60.60"})
		if err != nil {
			t.Fatal(err)
		}
		return o
	}
	claim := func(o execution.Order) error {
		_, err := pool.Exec(ctx, `INSERT INTO execution_dispatch_attempts(order_id,owner_id,financial_account_id,authorization_id,credential_generation,claimed_at,expires_at) VALUES($1,$2,$3,gen_random_uuid(),1,clock_timestamp(),clock_timestamp()+interval '30 seconds')`, o.ID, input.UserID, o.Request.AccountID)
		return err
	}
	o := prepare(input.FinancialAccountID, input.CapitalBucketID)
	var pe *pgconn.PgError
	if err := claim(o); !errors.As(err, &pe) || pe.ConstraintName != "execution_capital_held" {
		t.Fatal("dispatch bypassed active manual reservation", err)
	}
	var account, bucket string
	if err := pool.QueryRow(ctx, `INSERT INTO financial_accounts(user_id,provider_connection_id,provider_name,provider_account_id,display_name) VALUES($1,$2,'coinbase',gen_random_uuid()::text,'Isolated fixture') RETURNING id::text`, input.UserID, connectionID).Scan(&account); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO capital_buckets(user_id,financial_account_id,name,allocation_type,allocation_value,currency,protected_amount,status) VALUES($1,$2,'Coinbase manual','FIXED_AMOUNT',100,'USD',10,'ACTIVE') RETURNING id::text`, input.UserID, account).Scan(&bucket); err != nil {
		t.Fatal(err)
	}
	next := prepare(account, bucket)
	stale, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		t.Fatal(err)
	}
	defer stale.Rollback(ctx)
	if _, err = stale.Exec(ctx, `SELECT count(*) FROM execution_capital_reservations WHERE financial_account_id=$1`, account); err != nil {
		t.Fatal(err)
	}
	if err = claim(next); err != nil {
		t.Fatal(err)
	}
	// Waiting on an advisory fence does not refresh a SERIALIZABLE snapshot.
	// The shared fence-row write must reject that snapshot, not miss this claim.
	_, err = stale.Exec(ctx, `SELECT lock_execution_capital_fence($1)`, account)
	if !errors.As(err, &pe) || pe.Code != "40001" {
		t.Fatal("stale snapshot bypassed capital fence", err)
	}
	_ = stale.Rollback(ctx)
	input.FinancialAccountID = account
	input.CapitalBucketID = bucket
	input.IdempotencyKey = next.Request.ClientOrderID
	risk := *input.Risk
	input.Risk = &risk
	input.Risk.CapitalBucketID = bucket
	if err = pool.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(&input.Risk.EvaluationID); err != nil {
		t.Fatal(err)
	}
	if _, _, err = NewPostgresStore(pool).Create(ctx, input); !errors.Is(err, ErrReservationConflict) {
		t.Fatal("manual reservation bypassed durable execution capital", err)
	}
}
