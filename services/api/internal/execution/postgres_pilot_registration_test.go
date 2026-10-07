package execution

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// These rows deliberately start unregistered. No provider request or real
// credentials are involved, and shared lifecycle fixtures remain unchanged.
func newPilotRegistrationFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool) (Request, PilotAllocation) {
	t.Helper()
	r := requestFixture()
	r.PilotLimits = &OwnerPilotLimits{MaximumOrderUSD: "60.60", ExpiresAt: time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)}
	if err := pool.QueryRow(ctx, `INSERT INTO users(external_id) VALUES($1) RETURNING id::text`, fmt.Sprintf("pilot-registration-%d", time.Now().UnixNano())).Scan(&r.OwnerID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO user_entitlements(user_id,entitlement_key) VALUES($1,'founder')`, r.OwnerID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO provider_connections(user_id,provider_category,provider_name,display_name,status) VALUES($1,'financial','coinbase','Synthetic registration','active') RETURNING id::text`, r.OwnerID).Scan(&r.ConnectionID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO financial_accounts(user_id,provider_connection_id,provider_name,provider_account_id,display_name,base_currency,status) VALUES($1,$2,'coinbase','portfolio:'||gen_random_uuid()::text,'Synthetic registration','USD','active') RETURNING id::text`, r.OwnerID, r.ConnectionID).Scan(&r.AccountID); err != nil {
		t.Fatal(err)
	}
	r.CapitalBucketID = pilotRegistrationBucket(t, ctx, pool, r.OwnerID, r.AccountID)
	pilotRegistrationNewKey(t, ctx, pool, &r)
	a := PilotAllocation{OwnerID: r.OwnerID, AccountID: r.AccountID, ConnectionID: r.ConnectionID, CapitalBucketID: r.CapitalBucketID, ProductID: r.ProductID, InitialCashUSD: "100", Limits: *r.PilotLimits}
	return r, a
}

func pilotRegistrationBucket(t *testing.T, ctx context.Context, pool *pgxpool.Pool, owner, account string) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(ctx, `INSERT INTO capital_buckets(user_id,financial_account_id,name,allocation_type,allocation_value,currency,status) VALUES($1,$2,'Synthetic registration','FIXED_AMOUNT',100,'USD','ACTIVE') RETURNING id::text`, owner, account).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func pilotRegistrationNewKey(t *testing.T, ctx context.Context, pool *pgxpool.Pool, r *Request) {
	t.Helper()
	if err := pool.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(&r.ClientOrderID); err != nil {
		t.Fatal(err)
	}
}

func assertPilotRegistrationAbsent(t *testing.T, ctx context.Context, pool *pgxpool.Pool, a PilotAllocation) {
	t.Helper()
	if _, err := NewPostgresStore(pool).ReadPilotBalance(ctx, a.OwnerID, a.AccountID); !errors.Is(err, ErrNotFound) {
		t.Fatal("denied registration left a readable allocation", err)
	}
}

func TestPostgresPilotRegistrationExactChangedAndLostCommitReplay(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	r, a := newPilotRegistrationFixture(t, ctx, pool)
	s := NewPostgresStore(pool)
	if _, err := NewPostgresStore(lostCommitDB{pool}).RegisterPilotAllocation(ctx, a); !errors.Is(err, ErrCommitUnknown) {
		t.Fatal("lost commit acknowledgement was not ambiguous", err)
	}
	before, err := s.ReadPilotBalance(ctx, a.OwnerID, a.AccountID)
	if err != nil || before.CashUSD != "100" || before.BaseQuantity != "0" || before.SettledOrderCount != 0 || before.Pending || before.RegisteredAt.IsZero() {
		t.Fatal("registration did not preserve the exact cash-only allocation", err, before)
	}
	for i := 0; i < 2; i++ {
		got, err := NewPostgresStore(pool).RegisterPilotAllocation(ctx, a)
		if err != nil || !reflect.DeepEqual(got, a) {
			t.Fatal("exact restart replay changed allocation", err, got)
		}
	}
	for name, mutate := range map[string]func(*PilotAllocation){
		"cash":    func(p *PilotAllocation) { p.InitialCashUSD = "99" },
		"cap":     func(p *PilotAllocation) { p.Limits.MaximumOrderUSD = "61" },
		"expiry":  func(p *PilotAllocation) { p.Limits.ExpiresAt = p.Limits.ExpiresAt.Add(time.Second) },
		"product": func(p *PilotAllocation) { p.ProductID = "ETH-USD" },
	} {
		t.Run(name, func(t *testing.T) {
			changed := a
			mutate(&changed)
			if _, err := s.RegisterPilotAllocation(ctx, changed); !errors.Is(err, ErrConflict) {
				t.Fatal("changed registration replay replaced original terms", err)
			}
		})
	}
	if _, err = s.Prepare(ctx, r); err != nil {
		t.Fatal("matching request denied after registration", err)
	}
	if _, err = s.RegisterPilotAllocation(ctx, a); err != nil {
		t.Fatal("saved exact registration replay blocked by later order history", err)
	}
	after, err := s.ReadPilotBalance(ctx, a.OwnerID, a.AccountID)
	if err != nil || after.CashUSD != before.CashUSD || after.BaseQuantity != before.BaseQuantity || !after.RegisteredAt.Equal(before.RegisteredAt) {
		t.Fatal("prepare or replay reset registered cash or time", err, after)
	}
	other, _ := newPilotRegistrationFixture(t, ctx, pool)
	if _, err = s.ReadPilotBalance(ctx, other.OwnerID, a.AccountID); !errors.Is(err, ErrNotFound) {
		t.Fatal("foreign owner could read pilot balance", err)
	}
}

func TestPostgresPilotRegistrationExpiryReplayIsReadOnly(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	_, a := newPilotRegistrationFixture(t, ctx, pool)
	a.Limits.ExpiresAt = pilotTestDeadline(t, ctx, pool)
	s := NewPostgresStore(pool)
	if _, err := s.RegisterPilotAllocation(ctx, a); err != nil {
		t.Fatal(err)
	}
	before, err := s.ReadPilotBalance(ctx, a.OwnerID, a.AccountID)
	if err != nil {
		t.Fatal(err)
	}
	waitPastPilotDeadline(t, ctx, pool, a.Limits.ExpiresAt)
	if got, err := s.RegisterPilotAllocation(ctx, a); err != nil || !reflect.DeepEqual(got, a) {
		t.Fatal("expired exact replay rewrote or lost immutable terms", err)
	}
	after, err := s.ReadPilotBalance(ctx, a.OwnerID, a.AccountID)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("expired replay changed saved balance", err)
	}
}

func TestPostgresPilotRegistrationRejectsInvalidInitialAuthority(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	for name, mutate := range map[string]func(*PilotAllocation){
		"over bucket":      func(a *PilotAllocation) { a.InitialCashUSD = "100.000000000000000001" },
		"cap over initial": func(a *PilotAllocation) { a.InitialCashUSD = "60.59" },
		"missing cash":     func(a *PilotAllocation) { a.InitialCashUSD = "" },
		"zero cash":        func(a *PilotAllocation) { a.InitialCashUSD = "0" },
		"nonfinite cash":   func(a *PilotAllocation) { a.InitialCashUSD = "NaN" },
		"USD-USD product":  func(a *PilotAllocation) { a.ProductID = "USD-USD" },
		"expired":          func(a *PilotAllocation) { a.Limits.ExpiresAt = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC) },
		"non UTC":          func(a *PilotAllocation) { a.Limits.ExpiresAt = a.Limits.ExpiresAt.In(time.FixedZone("offset", 3600)) },
	} {
		t.Run(name, func(t *testing.T) {
			_, a := newPilotRegistrationFixture(t, ctx, pool)
			mutate(&a)
			if _, err := NewPostgresStore(pool).RegisterPilotAllocation(ctx, a); err == nil {
				t.Fatal("invalid initial allocation accepted")
			}
			assertPilotRegistrationAbsent(t, ctx, pool, a)
		})
	}
	for name, statement := range map[string]string{
		"protected cash":     `UPDATE capital_buckets SET protected_amount=1 WHERE id=$1`,
		"allocation ceiling": `UPDATE capital_buckets SET allocation_limit=99 WHERE id=$1`,
		"percentage bucket":  `UPDATE capital_buckets SET allocation_type='PERCENT_OF_AVAILABLE_CASH' WHERE id=$1`,
	} {
		t.Run(name, func(t *testing.T) {
			_, a := newPilotRegistrationFixture(t, ctx, pool)
			if _, err := pool.Exec(ctx, statement, a.CapitalBucketID); err != nil {
				t.Fatal(err)
			}
			if _, err := NewPostgresStore(pool).RegisterPilotAllocation(ctx, a); !errors.Is(err, ErrNotAuthorized) {
				t.Fatal("registration ignored current usable bucket funds", err)
			}
			assertPilotRegistrationAbsent(t, ctx, pool, a)
		})
	}
	t.Run("non portfolio account", func(t *testing.T) {
		_, a := newPilotRegistrationFixture(t, ctx, pool)
		if _, err := pool.Exec(ctx, `UPDATE financial_accounts SET provider_account_id='fixture:not-a-portfolio' WHERE id=$1`, a.AccountID); err != nil {
			t.Fatal(err)
		}
		if _, err := NewPostgresStore(pool).RegisterPilotAllocation(ctx, a); err == nil {
			t.Fatal("non-portfolio identity gained allocation", err)
		}
		assertPilotRegistrationAbsent(t, ctx, pool, a)
	})
}

func TestPostgresPilotRegistrationCannotBeMutatedOrTruncated(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	_, a := newPilotRegistrationFixture(t, ctx, pool)
	s := NewPostgresStore(pool)
	if _, err := s.RegisterPilotAllocation(ctx, a); err != nil {
		t.Fatal(err)
	}
	before, err := s.ReadPilotBalance(ctx, a.OwnerID, a.AccountID)
	if err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{
		`UPDATE execution_pilot_allocations SET initial_cash_usd='1000' WHERE financial_account_id=$1`,
		`UPDATE execution_pilot_allocations SET maximum_order_usd='1000' WHERE financial_account_id=$1`,
		`UPDATE execution_pilot_allocations SET expires_at=expires_at+interval '1 day' WHERE financial_account_id=$1`,
		`DELETE FROM execution_pilot_allocations WHERE financial_account_id=$1`,
	} {
		if _, err = pool.Exec(ctx, query, a.AccountID); err == nil {
			t.Fatal("immutable pilot registry changed", query)
		}
	}
	// Always roll this isolated-test DDL back before asserting; even a broken
	// truncate guard must not delete fixtures needed by other acceptance tests.
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, truncateErr := tx.Exec(ctx, `TRUNCATE execution_pilot_allocations`)
	rollback(tx)
	if truncateErr == nil {
		t.Fatal("pilot registry could be truncated")
	}
	after, err := s.ReadPilotBalance(ctx, a.OwnerID, a.AccountID)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("failed mutation changed original allocation", err)
	}
}

func TestPostgresPilotRegistrationRejectsHistoricalPreparedOrder(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	r, a := newPilotRegistrationFixture(t, ctx, pool)
	s := NewPostgresStore(pool)
	o, err := s.Prepare(ctx, r)
	if err != nil {
		t.Fatal("legacy nonregistered preparation unavailable", err)
	}
	if _, err = s.RegisterPilotAllocation(ctx, a); !errors.Is(err, ErrNotAuthorized) {
		t.Fatal("historical execution account was reseeded", err)
	}
	assertPilotRegistrationAbsent(t, ctx, pool, a)
	if again, err := s.Prepare(ctx, r); err != nil || again.ID != o.ID {
		t.Fatal("denied registration broke historical order replay", err)
	}
}

func TestPostgresPilotRegistrationPinsScopeAndClaimFunds(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	for _, field := range []string{"bucket", "product", "connection", "cap", "expiry"} {
		t.Run(field, func(t *testing.T) {
			r, a := newPilotRegistrationFixture(t, ctx, pool)
			s := NewPostgresStore(pool)
			if _, err := s.RegisterPilotAllocation(ctx, a); err != nil {
				t.Fatal(err)
			}
			switch field {
			case "bucket":
				r.CapitalBucketID = pilotRegistrationBucket(t, ctx, pool, r.OwnerID, r.AccountID)
			case "product":
				r.ProductID = "ETH-USD"
			case "connection":
				if err := pool.QueryRow(ctx, `INSERT INTO provider_connections(user_id,provider_category,provider_name,display_name,status) VALUES($1,'financial','coinbase','Alternative fixture','active') RETURNING id::text`, r.OwnerID).Scan(&r.ConnectionID); err != nil {
					t.Fatal(err)
				}
			case "cap":
				r.PilotLimits.MaximumOrderUSD = "61"
			case "expiry":
				r.PilotLimits.ExpiresAt = r.PilotLimits.ExpiresAt.Add(time.Second)
			}
			if _, err := s.Prepare(ctx, r); !errors.Is(err, ErrNotAuthorized) {
				t.Fatal("registered account escaped pinned pilot scope", err)
			}
			var count int
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM execution_orders WHERE financial_account_id=$1`, a.AccountID).Scan(&count); err != nil || count != 0 {
				t.Fatal("rejected scope left an order", err, count)
			}
		})
	}
	for name, change := range map[string]string{
		"bucket shrink":   `UPDATE capital_buckets SET allocation_value=99 WHERE financial_account_id=$1`,
		"portfolio drift": `UPDATE financial_accounts SET provider_account_id='portfolio:'||gen_random_uuid()::text WHERE id=$1`,
	} {
		t.Run(name, func(t *testing.T) {
			r, a := newPilotRegistrationFixture(t, ctx, pool)
			s := NewPostgresStore(pool)
			if _, err := s.RegisterPilotAllocation(ctx, a); err != nil {
				t.Fatal(err)
			}
			o, err := s.Prepare(ctx, r)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = pool.Exec(ctx, change, a.AccountID); err != nil {
				t.Fatal(err)
			}
			if _, err = s.Claim(ctx, r.OwnerID, o.ID, fixtureAuthority); !errors.Is(err, ErrNotAuthorized) {
				t.Fatal("claim ignored current registered allocation authority", err)
			}
			if _, err = s.ReadAttempt(ctx, r.OwnerID, o.ID); !errors.Is(err, ErrNotFound) {
				t.Fatal("denied registered claim left an attempt", err)
			}
			if _, err = s.ReadCapitalReservation(ctx, r.OwnerID, o.ID); !errors.Is(err, ErrNotFound) {
				t.Fatal("denied registered claim reserved funds", err)
			}
		})
	}
}

func TestPostgresPilotRegistrationReplacementAccountCannotResetPortfolio(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	r, a := newPilotRegistrationFixture(t, ctx, pool)
	s := NewPostgresStore(pool)
	if _, err := s.RegisterPilotAllocation(ctx, a); err != nil {
		t.Fatal(err)
	}
	replacement := a
	var originalPortfolio string
	if err := pool.QueryRow(ctx, `SELECT provider_account_id FROM financial_accounts WHERE id=$1`, a.AccountID).Scan(&originalPortfolio); err != nil {
		t.Fatal(err)
	}
	// Retarget A before inserting B: the account identity uniqueness constraint
	// still applies, while the permanent registry retains A's original identity.
	if _, err := pool.Exec(ctx, `UPDATE financial_accounts SET provider_account_id='portfolio:'||gen_random_uuid()::text WHERE id=$1`, a.AccountID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO provider_connections(user_id,provider_category,provider_name,display_name,status) VALUES($1,'financial','coinbase','Reconnected fixture','active') RETURNING id::text`, a.OwnerID).Scan(&replacement.ConnectionID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO financial_accounts(user_id,provider_connection_id,provider_name,provider_account_id,display_name) VALUES($1,$2,'coinbase',$3,'Reconnected fixture') RETURNING id::text`, a.OwnerID, replacement.ConnectionID, originalPortfolio).Scan(&replacement.AccountID); err != nil {
		t.Fatal(err)
	}
	replacement.CapitalBucketID = pilotRegistrationBucket(t, ctx, pool, a.OwnerID, replacement.AccountID)
	// A replacement account must not escape the registry through legacy fallback,
	// even before anyone attempts to register that replacement account.
	r.AccountID, r.ConnectionID, r.CapitalBucketID = replacement.AccountID, replacement.ConnectionID, replacement.CapitalBucketID
	pilotRegistrationNewKey(t, ctx, pool, &r)
	if _, err := s.Prepare(ctx, r); !errors.Is(err, ErrNotAuthorized) {
		t.Fatal("replacement account bypassed registry through unregistered preparation", err)
	}
	var orders int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM execution_orders WHERE financial_account_id=$1`, replacement.AccountID).Scan(&orders); err != nil || orders != 0 {
		t.Fatal("replacement fallback persisted an order", err, orders)
	}
	if _, err := s.RegisterPilotAllocation(ctx, replacement); err == nil {
		t.Fatal("new connection/account reset the same provider portfolio allocation")
	}
	assertPilotRegistrationAbsent(t, ctx, pool, replacement)
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM execution_pilot_allocations WHERE owner_id=$1`, a.OwnerID).Scan(&count); err != nil || count != 1 {
		t.Fatal("replacement duplicated allocation", err, count)
	}
}

type pilotRegistrationIsolationDB struct {
	*pgxpool.Pool
	isolation pgx.TxIsoLevel
}

func (d pilotRegistrationIsolationDB) Begin(ctx context.Context) (pgx.Tx, error) {
	return d.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: d.isolation})
}

func TestPostgresPilotRegistrationRejectsNonCurrentSnapshots(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	for _, isolation := range []pgx.TxIsoLevel{pgx.RepeatableRead, pgx.Serializable} {
		t.Run(string(isolation), func(t *testing.T) {
			r, a := newPilotRegistrationFixture(t, ctx, pool)
			stale := NewPostgresStore(pilotRegistrationIsolationDB{pool, isolation})
			if _, err := stale.RegisterPilotAllocation(ctx, a); !errors.Is(err, ErrNotAuthorized) {
				t.Fatal("registration accepted a non-current snapshot", err)
			}
			assertPilotRegistrationAbsent(t, ctx, pool, a)
			if _, err := NewPostgresStore(pool).RegisterPilotAllocation(ctx, a); err != nil {
				t.Fatal(err)
			}
			if _, err := stale.Prepare(ctx, r); !errors.Is(err, ErrNotAuthorized) {
				t.Fatal("registered preparation accepted a non-current snapshot", err)
			}
			var count int
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM execution_orders WHERE financial_account_id=$1`, a.AccountID).Scan(&count); err != nil || count != 0 {
				t.Fatal("non-current snapshot inserted an order", err, count)
			}
		})
	}
}

type pilotRegistrationCommitBarrierDB struct {
	Database
	ready   chan struct{}
	release chan struct{}
}

type pilotRegistrationCommitBarrierTx struct {
	pgx.Tx
	ready   chan struct{}
	release chan struct{}
}

func (d pilotRegistrationCommitBarrierDB) Begin(ctx context.Context) (pgx.Tx, error) {
	tx, err := d.Database.Begin(ctx)
	if err != nil {
		return nil, err
	}
	return pilotRegistrationCommitBarrierTx{tx, d.ready, d.release}, nil
}

func (tx pilotRegistrationCommitBarrierTx) Commit(ctx context.Context) error {
	close(tx.ready)
	select {
	case <-tx.release:
		return tx.Tx.Commit(ctx)
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestPostgresPilotRegistrationAndPrepareSerializeBothOrderings(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	for _, stage := range []string{"registration before wrong product", "registration before wrong cap", "preparation before registration"} {
		t.Run(stage, func(t *testing.T) {
			registrationFirst := stage != "preparation before registration"
			r, a := newPilotRegistrationFixture(t, ctx, pool)
			waiterRequest := r
			if stage == "registration before wrong product" {
				waiterRequest.ProductID = "ETH-USD"
			} else if stage == "registration before wrong cap" {
				limits := *r.PilotLimits
				limits.MaximumOrderUSD = "61"
				waiterRequest.PilotLimits = &limits
			}
			ready, release := make(chan struct{}), make(chan struct{})
			defer func() {
				select {
				case <-release:
				default:
					close(release)
				}
			}()
			first := NewPostgresStore(pilotRegistrationCommitBarrierDB{pool, ready, release})
			firstDone := make(chan error, 1)
			go func() {
				var err error
				if registrationFirst {
					_, err = first.RegisterPilotAllocation(ctx, a)
				} else {
					_, err = first.Prepare(ctx, r)
				}
				firstDone <- err
			}()
			select {
			case <-ready:
			case err := <-firstDone:
				t.Fatal("first operation failed before holding its commit boundary", err)
			case <-time.After(5 * time.Second):
				t.Fatal("first operation never reached its commit boundary")
			}
			conn, err := pool.Acquire(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Release()
			secondDone := make(chan error, 1)
			go func() {
				var err error
				if registrationFirst {
					_, err = NewPostgresStore(conn).Prepare(ctx, waiterRequest)
				} else {
					_, err = NewPostgresStore(conn).RegisterPilotAllocation(ctx, a)
				}
				secondDone <- err
			}()
			// The second database session must actually wait on the first writer;
			// no goroutine scheduling or sleep is accepted as serialization proof.
			waitExecutionLock(t, ctx, pool, conn.Conn().PgConn().PID())
			close(release)
			if err = <-firstDone; err != nil {
				t.Fatal("first operation failed at commit", err)
			}
			err = <-secondDone
			if registrationFirst && !errors.Is(err, ErrNotAuthorized) {
				t.Fatal("mismatched preparation missed the allocation committed ahead of it", err)
			}
			if !registrationFirst && !errors.Is(err, ErrNotAuthorized) {
				t.Fatal("registration missed the order committed ahead of it", err)
			}
			var orders, allocations int
			if err = pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM execution_orders WHERE financial_account_id=$1),(SELECT count(*) FROM execution_pilot_allocations WHERE financial_account_id=$1)`, a.AccountID).Scan(&orders, &allocations); err != nil {
				t.Fatal(err)
			}
			wantOrders, wantAllocations := 1, 0
			if registrationFirst {
				wantOrders = 0
				wantAllocations = 1
			}
			if orders != wantOrders || allocations != wantAllocations {
				t.Fatal("race produced incorrect durable registration/order state", orders, allocations)
			}
		})
	}
}
