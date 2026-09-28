package execution

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/migrations"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

type authorityFunc func(context.Context, pgx.Tx, Order, time.Time) (Authorization, error)

func (f authorityFunc) AuthorizeDispatch(c context.Context, tx pgx.Tx, o Order, now time.Time) (Authorization, error) {
	return f(c, tx, o, now)
}

// Test-only authority; never imported by a production caller.
var fixtureAuthority = authorityFunc(func(ctx context.Context, tx pgx.Tx, o Order, now time.Time) (Authorization, error) {
	var id string
	if err := tx.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(&id); err != nil {
		return Authorization{}, err
	}
	return Authorization{ID: id, RequestDigest: o.RequestDigest, CredentialGeneration: 1, ExpiresAt: now.Add(30 * time.Second)}, nil
})

type lostCommitDB struct{ *pgxpool.Pool }
type lostCommitTx struct{ pgx.Tx }

func (d lostCommitDB) Begin(ctx context.Context) (pgx.Tx, error) {
	tx, err := d.Pool.Begin(ctx)
	return lostCommitTx{tx}, err
}
func (tx lostCommitTx) Commit(ctx context.Context) error {
	if err := tx.Tx.Commit(ctx); err != nil {
		return err
	}
	return errors.New("fixture lost commit acknowledgement")
}

func TestPostgresDispatchIsDurableScopedAndSingleAttempt(t *testing.T) {
	url := os.Getenv("STRATEGY_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("STRATEGY_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	db, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	goose.SetBaseFS(migrations.Files)
	if err = goose.SetDialect("postgres"); err != nil {
		t.Fatal(err)
	}
	if err = goose.UpContext(ctx, db, "."); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	store := NewPostgresStore(pool)
	newFixture := func() Request {
		t.Helper()
		r := requestFixture()
		if err := pool.QueryRow(ctx, `INSERT INTO users(external_id) VALUES($1) RETURNING id::text`, fmt.Sprintf("dispatch-test-%d", time.Now().UnixNano())).Scan(&r.OwnerID); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, `INSERT INTO provider_connections(user_id,provider_category,provider_name,display_name,status) VALUES($1,'financial','coinbase','Dispatch fixture','active') RETURNING id::text`, r.OwnerID).Scan(&r.ConnectionID); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, `INSERT INTO financial_accounts(user_id,provider_connection_id,provider_name,provider_account_id,display_name,base_currency,status) VALUES($1,$2,'coinbase',$3,'Dispatch fixture','USD','active') RETURNING id::text`, r.OwnerID, r.ConnectionID, "fixture:"+r.ConnectionID).Scan(&r.AccountID); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, `INSERT INTO capital_buckets(user_id,financial_account_id,name,allocation_type,allocation_value,currency,status) VALUES($1,$2,'Dispatch fixture','FIXED_AMOUNT',100,'USD','ACTIVE') RETURNING id::text`, r.OwnerID, r.AccountID).Scan(&r.CapitalBucketID); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(&r.ClientOrderID); err != nil {
			t.Fatal(err)
		}
		return r
	}
	prepare := func(r Request) Order {
		t.Helper()
		o, err := store.Prepare(ctx, r)
		if err != nil {
			t.Fatal(err)
		}
		return o
	}
	r := newFixture()
	o := prepare(r)
	other := newFixture()
	if again := prepare(r); again.ID != o.ID {
		t.Fatal("replay changed identity")
	}
	changed := r
	changed.LimitPrice = "59999"
	if _, err := store.Prepare(ctx, changed); !errors.Is(err, ErrConflict) {
		t.Fatalf("changed replay: %v", err)
	}
	bad := other
	bad.ClientOrderID = r.ClientOrderID
	if _, err := store.Prepare(ctx, bad); !errors.Is(err, ErrConflict) {
		t.Fatalf("cross-owner replay: %v", err)
	}
	for _, mutate := range []func(*Request){func(r *Request) { r.AccountID = other.AccountID }, func(r *Request) { r.ConnectionID = other.ConnectionID }, func(r *Request) { r.CapitalBucketID = other.CapitalBucketID }} {
		bad = r
		bad.ClientOrderID = other.ClientOrderID
		mutate(&bad)
		if _, err := store.Prepare(ctx, bad); !errors.Is(err, ErrInvalid) {
			t.Fatalf("cross binding accepted: %v", err)
		}
	}
	if _, err := store.Claim(ctx, other.OwnerID, o.ID, fixtureAuthority); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross owner claim: %v", err)
	}
	for name, gate := range map[string]Authority{
		"missing": nil,
		"denied": authorityFunc(func(context.Context, pgx.Tx, Order, time.Time) (Authorization, error) {
			return Authorization{}, ErrNotAuthorized
		}),
		"expired": authorityFunc(func(c context.Context, tx pgx.Tx, o Order, n time.Time) (Authorization, error) {
			a, e := fixtureAuthority(c, tx, o, n)
			a.ExpiresAt = n.Add(-time.Second)
			return a, e
		}),
		"digest": authorityFunc(func(c context.Context, tx pgx.Tx, o Order, n time.Time) (Authorization, error) {
			a, e := fixtureAuthority(c, tx, o, n)
			a.RequestDigest = "wrong"
			return a, e
		}),
		"unbounded": authorityFunc(func(c context.Context, tx pgx.Tx, o Order, n time.Time) (Authorization, error) {
			a, e := fixtureAuthority(c, tx, o, n)
			a.ExpiresAt = n.Add(time.Hour)
			return a, e
		}),
		"expires during gate": authorityFunc(func(c context.Context, tx pgx.Tx, o Order, n time.Time) (Authorization, error) {
			a, e := fixtureAuthority(c, tx, o, n)
			a.ExpiresAt = n.Add(10 * time.Millisecond)
			_, e2 := tx.Exec(c, `SELECT pg_sleep(0.03)`)
			if e2 != nil {
				return a, e2
			}
			return a, e
		}),
	} {
		t.Run(name, func(t *testing.T) {
			if _, e := store.Claim(ctx, r.OwnerID, o.ID, gate); !errors.Is(e, ErrNotAuthorized) {
				t.Fatalf("gate: %v", e)
			}
			if _, e := store.ReadAttempt(ctx, r.OwnerID, o.ID); !errors.Is(e, ErrNotFound) {
				t.Fatalf("denial left an attempt: %v", e)
			}
		})
	}

	// Separate DB handles represent independent workers, not a process-local mutex.
	secondPool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer secondPool.Close()
	var wg sync.WaitGroup
	results := make(chan error, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			s := store
			if i%2 == 1 {
				s = NewPostgresStore(secondPool)
			}
			_, e := s.Claim(ctx, r.OwnerID, o.ID, fixtureAuthority)
			results <- e
		}(i)
	}
	wg.Wait()
	close(results)
	wins := 0
	for err := range results {
		if err == nil {
			wins++
		} else if !errors.Is(err, ErrAlreadyAttempted) {
			t.Fatal(err)
		}
	}
	if wins != 1 {
		t.Fatalf("expected exactly one claim, got %d", wins)
	}
	a, err := store.ReadAttempt(ctx, r.OwnerID, o.ID)
	if err != nil || a.ProviderOrderID != "" || a.ClientOrderID != r.ClientOrderID {
		t.Fatalf("recover unknown: %#v %v", a, err)
	}
	if _, err = store.ReadAttempt(ctx, other.OwnerID, o.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-owner read: %v", err)
	}
	// A fresh OS process sees the same hold and cannot claim the order again.
	body, _ := json.Marshal([]string{r.OwnerID, o.ID})
	child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestPostgresDispatchRecoveryChild$", "-test.count=1")
	child.Env = append(os.Environ(), "ARBION_DISPATCH_RECOVERY_FIXTURE="+string(body))
	if output, err := child.CombinedOutput(); err != nil {
		t.Fatalf("process recovery: %v %s", err, output)
	}
	var brokerID, changedBrokerID string
	if err = pool.QueryRow(ctx, `SELECT gen_random_uuid()::text,gen_random_uuid()::text`).Scan(&brokerID, &changedBrokerID); err != nil {
		t.Fatal(err)
	}
	if err = store.RecordAcknowledgement(ctx, r.OwnerID, o.ID, brokerID); err != nil {
		t.Fatal(err)
	}
	if err = store.RecordAcknowledgement(ctx, r.OwnerID, o.ID, brokerID); err != nil {
		t.Fatal(err)
	}
	if err = store.RecordAcknowledgement(ctx, r.OwnerID, o.ID, changedBrokerID); !errors.Is(err, ErrConflict) {
		t.Fatalf("changed broker identity: %v", err)
	}
	if err = store.RecordAcknowledgement(ctx, other.OwnerID, o.ID, brokerID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-owner ack: %v", err)
	}
	if a, err = NewPostgresStore(secondPool).ReadAttempt(ctx, r.OwnerID, o.ID); err != nil || a.ProviderOrderID != brokerID {
		t.Fatalf("ack recovery: %#v %v", a, err)
	}
	// Acknowledgement does not release the account or create settlement authority.
	r2 := r
	r2.ClientOrderID = changedBrokerID
	o2 := prepare(r2)
	if _, err = store.Claim(ctx, r.OwnerID, o2.ID, fixtureAuthority); !errors.Is(err, ErrAccountHeld) {
		t.Fatalf("account reused: %v", err)
	}
	for _, query := range []string{`UPDATE execution_orders SET request_digest=repeat('0',64) WHERE id=$1`, `DELETE FROM execution_orders WHERE id=$1`, `UPDATE execution_dispatch_attempts SET credential_generation=2 WHERE order_id=$1`, `DELETE FROM execution_dispatch_attempts WHERE order_id=$1`, `DELETE FROM execution_broker_acknowledgements WHERE order_id=$1`} {
		if _, err = pool.Exec(ctx, query, o.ID); err == nil {
			t.Fatalf("mutable evidence: %s", query)
		}
	}

	// The DB commits but the response is lost. No dispatch receipt escapes; both
	// same-order and different-order retries remain blocked after recovery.
	r3 := newFixture()
	o3 := prepare(r3)
	claimed, err := NewPostgresStore(lostCommitDB{pool}).Claim(ctx, r3.OwnerID, o3.ID, fixtureAuthority)
	if !errors.Is(err, ErrCommitUnknown) || claimed.OrderID != "" {
		t.Fatalf("ambiguous commit permitted dispatch: %#v %v", claimed, err)
	}
	if _, err = store.ReadAttempt(ctx, r3.OwnerID, o3.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Claim(ctx, r3.OwnerID, o3.ID, fixtureAuthority); !errors.Is(err, ErrAlreadyAttempted) {
		t.Fatalf("commit loss retried: %v", err)
	}

	// Different concurrent intents also share the single account hold.
	r4 := newFixture()
	o4 := prepare(r4)
	r5 := r4
	r5.ClientOrderID = other.ClientOrderID
	o5 := prepare(r5)
	results = make(chan error, 2)
	for _, id := range []string{o4.ID, o5.ID} {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			_, e := store.Claim(ctx, r4.OwnerID, id, fixtureAuthority)
			results <- e
		}(id)
	}
	wg.Wait()
	close(results)
	wins = 0
	for e := range results {
		if e == nil {
			wins++
		} else if !errors.Is(e, ErrAccountHeld) {
			t.Fatal(e)
		}
	}
	if wins != 1 {
		t.Fatalf("concurrent account claims: %d", wins)
	}
}

func TestPostgresDispatchRecoveryChild(t *testing.T) {
	fixture := os.Getenv("ARBION_DISPATCH_RECOVERY_FIXTURE")
	if fixture == "" {
		t.Skip("subprocess fixture only")
	}
	var ids []string
	if err := json.Unmarshal([]byte(fixture), &ids); err != nil || len(ids) != 2 {
		t.Fatal("invalid recovery fixture")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, os.Getenv("STRATEGY_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	store := NewPostgresStore(pool)
	if _, err = store.ReadAttempt(ctx, ids[0], ids[1]); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Claim(ctx, ids[0], ids[1], fixtureAuthority); !errors.Is(err, ErrAlreadyAttempted) {
		t.Fatalf("restart allowed resend: %v", err)
	}
}
