package executionsim

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// No production migration embeds or installs this schema.
//
//go:embed postgres_schema_test.sql
var postgresFixtureSchema string

type commitFaultDB struct {
	SimulationDB
	mode string
}

func (db commitFaultDB) BeginTx(ctx context.Context, options pgx.TxOptions) (pgx.Tx, error) {
	tx, err := db.SimulationDB.BeginTx(ctx, options)
	if err != nil {
		return nil, err
	}
	return commitFaultTx{Tx: tx, mode: db.mode}, nil
}

type commitFaultTx struct {
	pgx.Tx
	mode string
}

func (tx commitFaultTx) Commit(ctx context.Context) error {
	switch tx.mode {
	case "exit-before":
		os.Exit(0) // Simulated worker death with an uncommitted insert.
	case "rollback":
		_ = tx.Tx.Rollback(ctx)
		return errors.New("fictional lost commit response")
	}
	if err := tx.Tx.Commit(ctx); err != nil {
		return err
	}
	if tx.mode == "exit-after" {
		os.Exit(0) // Durable commit, worker dies before acknowledging it.
	}
	return errors.New("fictional lost commit response")
}

func TestPostgresSimulationRecovery(t *testing.T) {
	dsn := os.Getenv("EXECUTION_SIM_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("dedicated EXECUTION_SIM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal("simulation database connection configuration unavailable")
	}
	defer pool.Close()
	var database string
	if err := pool.QueryRow(ctx, `SELECT current_database()`).Scan(&database); err != nil || database != simulationDatabase {
		t.Fatal("refusing schema creation: dedicated arbion_execution_sim database required")
	}
	config := func(run string) Config {
		c := fixtureConfig()
		c.Scope.Run = run
		return c
	}
	event := func(c Config, v Event) Event { v.Scope = c.Scope; return v }
	open := func(db SimulationDB, c Config) *PostgresJournal {
		t.Helper()
		j, err := OpenPostgresJournal(ctx, db, c, fixtureTime)
		if err != nil {
			t.Fatal("open", err)
		}
		return j
	}
	appendEvent := func(j *PostgresJournal, v Event) {
		t.Helper()
		if applied, err := j.Append(ctx, event(j.config, v), fixtureTime); err != nil || !applied {
			t.Fatal("append", v.Kind, applied, err)
		}
	}
	snapshot := func(j *PostgresJournal) Snapshot {
		t.Helper()
		s, err := j.Snapshot(ctx, fixtureTime)
		if err != nil {
			t.Fatal("snapshot", err)
		}
		return s
	}
	if mode := os.Getenv("ARBION_PG_SIM_EXIT_MODE"); mode != "" {
		if mode != "exit-before" && mode != "exit-after" {
			t.Fatal("invalid crash fixture mode")
		}
		j := open(pool, config("crash-"+mode))
		j.db = commitFaultDB{SimulationDB: pool, mode: mode}
		appendEvent(j, fixtureEvent("send", Send, 2))
		t.Fatal("fixture child did not exit at commit boundary")
	}
	if _, err := pool.Exec(ctx, postgresFixtureSchema); err != nil {
		t.Fatal("dedicated simulation schema", err)
	}

	t.Run("concurrent genesis and exact delivery", func(t *testing.T) {
		c := config("concurrent")
		var wg sync.WaitGroup
		results := make(chan bool, 20)
		failures := make(chan error, 20)
		for i := 0; i < 20; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				j, err := OpenPostgresJournal(ctx, pool, c, fixtureTime)
				if err != nil {
					failures <- err
					return
				}
				v := Event{Scope: c.Scope, ID: "deposit", Kind: Deposit, TransactionID: "transfer", At: fixtureTime, Amount: "25"}
				applied, err := j.Append(ctx, v, fixtureTime)
				failures <- err
				results <- applied
			}()
		}
		wg.Wait()
		close(failures)
		close(results)
		for err := range failures {
			if err != nil {
				t.Fatal(err)
			}
		}
		count := 0
		for applied := range results {
			if applied {
				count++
			}
		}
		j := open(pool, c)
		s := snapshot(j)
		if count != 1 || s.Cash != "1025.0000000000" || s.MatchedTransactions != 1 || s.AppliedEvents != 1 {
			t.Fatal("duplicate money", count, s)
		}
		alias := Event{Scope: c.Scope, ID: "alias", Kind: Deposit, TransactionID: "transfer", At: fixtureTime, Amount: "25"}
		if applied, err := j.Append(ctx, alias, fixtureTime); err != nil || applied || !reflect.DeepEqual(s, snapshot(j)) {
			t.Fatal("economic alias settled again", applied, err)
		}
		j = open(pool, c)
		alias.Amount = "26"
		if _, err := j.Append(ctx, alias, fixtureTime); !errors.Is(err, ErrConflict) {
			t.Fatal("alias conflict lost on recovery", err)
		}
	})

	t.Run("competing claims cannot double allocate", func(t *testing.T) {
		j := open(pool, config("claims"))
		var wg sync.WaitGroup
		outcomes := make(chan error, 3)
		for _, id := range []string{"first", "second", "third"} {
			wg.Add(1)
			go func(id string) {
				defer wg.Done()
				v := event(j.config, opening())
				v.ID, v.OrderID = id, id
				_, err := j.Append(ctx, v, fixtureTime)
				outcomes <- err
			}(id)
		}
		wg.Wait()
		close(outcomes)
		allowed, denied := 0, 0
		for err := range outcomes {
			switch {
			case err == nil:
				allowed++
			case errors.Is(err, ErrLimits):
				denied++
			default:
				t.Fatal(err)
			}
		}
		if s := snapshot(j); allowed != 2 || denied != 1 || s.ReservedCash != "170.0000000000" || s.Cash != "1000.0000000000" {
			t.Fatal("concurrent claims bypassed fixed budget", allowed, denied, s)
		}
	})

	t.Run("scope partition and immutable genesis", func(t *testing.T) {
		c := config("binding")
		j := open(pool, c)
		appendEvent(j, opening())
		before := snapshot(j)
		for _, change := range []func(*Config){
			func(c *Config) { c.Scope.Owner = "other-owner" },
			func(c *Config) { c.Scope.Account = "other-account" },
			func(c *Config) { c.Scope.Provider = "schwab" },
			func(c *Config) { c.Scope.Run = "other-run" },
		} {
			other := c
			change(&other)
			v := event(other, fixtureEvent("send", Send, 2))
			if _, err := j.Append(ctx, v, fixtureTime); !errors.Is(err, ErrInvalid) {
				t.Fatal("foreign identity accepted", err)
			}
			isolated := open(pool, other)
			if len(snapshot(isolated).Orders) != 0 {
				t.Fatal("foreign scope read another session")
			}
			if _, err := isolated.Append(ctx, v, fixtureTime); !errors.Is(err, ErrTransition) {
				t.Fatal("foreign scope recovered another scope's order", err)
			}
		}
		changed := c
		changed.BuySpendCeiling = "9999"
		if _, err := OpenPostgresJournal(ctx, pool, changed, fixtureTime); !errors.Is(err, ErrConflict) || !reflect.DeepEqual(before, snapshot(j)) {
			t.Fatal("reopen expanded configuration", err)
		}
	})

	t.Run("partial cancellation recovery", func(t *testing.T) {
		for _, provider := range []string{"coinbase", "schwab"} {
			c := config("partial-" + provider)
			c.Scope.Provider = provider
			j := open(pool, c)
			appendEvent(j, opening())
			appendEvent(j, fixtureEvent("send", Send, 2))
			j = open(pool, c)
			if snapshot(j).Orders["order-a"].State != "OUTCOME_UNKNOWN" {
				t.Fatal("uncertain attempt not recovered")
			}
			ack := fixtureEvent("ack", Acknowledge, 3)
			ack.SimulatedOrderID = "sim-order-a"
			appendEvent(j, ack)
			appendEvent(j, filling())
			appendEvent(j, fixtureEvent("cancel-requested", RequestCancel, 5))
			terminal := event(c, cancellation(6))
			terminal.TerminalSettlement = &SettlementTotals{FilledQuantity: "1.5", GrossNotional: "60", Fees: "0.3"}
			before := snapshot(j)
			if applied, err := j.Append(ctx, terminal, fixtureTime); applied || !errors.Is(err, ErrSettlement) {
				t.Fatal("premature cancel released claim", applied, err)
			}
			j = open(pool, c)
			if !reflect.DeepEqual(before, snapshot(j)) || before.ReservedCash != "44.8000000000" {
				t.Fatal("unmatched cancellation lost on restart")
			}
			late := filling()
			late.ID, late.TransactionID, late.OrderVersion, late.Quantity, late.Fee = "late", "late-trade", 6, "0.5", "0.1"
			appendEvent(j, late)
			terminal.OrderVersion = 7
			appendEvent(j, terminal)
			j = open(pool, c)
			s := snapshot(j)
			if s.Cash != "939.7000000000" || s.ReservedCash != zero() || s.Orders["order-a"].State != "CANCELLED" || s.Orders["order-a"].FeesPaid != "0.3000000000" {
				t.Fatal(s)
			}
			late.ID = "late-alias"
			if applied, err := j.Append(ctx, event(c, late), fixtureTime); applied || err != nil || !reflect.DeepEqual(s, snapshot(j)) {
				t.Fatal("late duplicate changed terminal settlement", applied, err)
			}
		}
	})

	t.Run("commit error never publishes speculative state", func(t *testing.T) {
		for _, mode := range []string{"rollback", "lost-response"} {
			j := open(pool, config(mode))
			appendEvent(j, opening())
			faulty := *j
			faulty.db = commitFaultDB{SimulationDB: pool, mode: mode}
			v := event(j.config, fixtureEvent("send", Send, 2))
			if applied, err := faulty.Append(ctx, v, fixtureTime); applied || !errors.Is(err, ErrCommitUnknown) {
				t.Fatal("uncertain commit acknowledged", applied, err)
			}
			recovered := open(pool, j.config)
			want := "OUTCOME_UNKNOWN"
			if mode == "rollback" {
				want = "REGISTERED"
			}
			if s := snapshot(recovered); s.Orders["order-a"].State != want || s.ReservedCash != "85.0000000000" {
				t.Fatal("bad recovery", mode, s)
			}
			applied, err := recovered.Append(ctx, v, fixtureTime)
			if err != nil || applied != (mode == "rollback") {
				t.Fatal("same identity retry did not match durable outcome", applied, err)
			}
		}
	})

	t.Run("actual process death around commit", func(t *testing.T) {
		for _, mode := range []string{"exit-before", "exit-after"} {
			c := config("crash-" + mode)
			j := open(pool, c)
			appendEvent(j, opening())
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestPostgresSimulationRecovery$")
			cmd.Env = append(os.Environ(), "ARBION_PG_SIM_EXIT_MODE="+mode)
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("fictional worker: %v %s", err, output)
			}
			freshPool, err := pgxpool.New(ctx, dsn)
			if err != nil {
				t.Fatal("new worker pool unavailable")
			}
			defer freshPool.Close()
			recovered := open(freshPool, c)
			want := "OUTCOME_UNKNOWN"
			if mode == "exit-before" {
				want = "REGISTERED"
			}
			if s := snapshot(recovered); s.Orders["order-a"].State != want || s.ReservedCash != "85.0000000000" {
				t.Fatal("worker death changed durable claim", mode, s)
			}
			applied, err := recovered.Append(ctx, event(c, fixtureEvent("send", Send, 2)), fixtureTime)
			if err != nil || applied != (mode == "exit-before") {
				t.Fatal("response-lost recovery did not reuse identity", applied, err)
			}
			if _, err := recovered.Append(ctx, event(c, fixtureEvent("new-send-id", Send, 3)), fixtureTime); !errors.Is(err, ErrTransition) {
				t.Fatal("worker death enabled blind resend", err)
			}
		}
	})

	t.Run("busy session does not block another account", func(t *testing.T) {
		c := config("lock")
		j := open(pool, c)
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		if _, err := tx.Exec(ctx, `SELECT 1 FROM execution_sim_lab.sessions WHERE run_id='lock' FOR UPDATE`); err != nil {
			t.Fatal(err)
		}
		bounded, stop := context.WithTimeout(ctx, 50*time.Millisecond)
		defer stop()
		if _, err := j.Append(bounded, event(c, opening()), fixtureTime); !errors.Is(err, ErrJournal) {
			t.Fatal("locked writer did not fail closed at deadline", err)
		}
		c.Scope.Account = "unblocked-account"
		other := open(pool, c)
		appendEvent(other, opening())
		if err := tx.Rollback(ctx); err != nil {
			t.Fatal(err)
		}
		if len(snapshot(j).Orders) != 0 {
			t.Fatal("timed-out writer committed")
		}
		appendEvent(j, opening())
	})

	t.Run("SQL immutability scope and sequence guards", func(t *testing.T) {
		j := open(pool, config("sql-guards"))
		appendEvent(j, opening())
		for _, statement := range []string{
			`UPDATE execution_sim_lab.sessions SET genesis=genesis WHERE run_id='sql-guards'`,
			`DELETE FROM execution_sim_lab.sessions WHERE run_id='sql-guards'`,
			`UPDATE execution_sim_lab.events SET record=record WHERE run_id='sql-guards'`,
			`DELETE FROM execution_sim_lab.events WHERE run_id='sql-guards'`,
			`TRUNCATE execution_sim_lab.events`,
			`TRUNCATE execution_sim_lab.sessions,execution_sim_lab.events`,
		} {
			if _, err := pool.Exec(ctx, statement); err == nil {
				t.Fatal("immutable test evidence accepted mutation")
			}
		}
		var head string
		if err := pool.QueryRow(ctx, `SELECT record FROM execution_sim_lab.events WHERE run_id='sql-guards' AND sequence=2`).Scan(&head); err != nil {
			t.Fatal(err)
		}
		previous, _ := decodeRecord([]byte(head))
		v := event(j.config, fixtureEvent("send", Send, 2))
		r := record{Sequence: 3, Previous: previous.Hash, Event: &v}
		insertRaw := func(r record, account string) error {
			r.Hash = recordHash(r)
			data, _ := json.Marshal(r)
			_, err := pool.Exec(ctx, `INSERT INTO execution_sim_lab.events(owner_id,account_id,provider,run_id,sequence,delivery_id,record) VALUES($1,$2,$3,$4,$5,$6,$7)`, j.config.Scope.Owner, account, j.config.Scope.Provider, j.config.Scope.Run, r.Sequence, r.Event.ID, string(data))
			return err
		}
		if err := insertRaw(r, "another-account"); err == nil {
			t.Fatal("cross-account raw event accepted")
		}
		r.Sequence = 4
		if err := insertRaw(r, j.config.Scope.Account); err == nil {
			t.Fatal("sequence gap accepted")
		}
		r.Sequence, r.Previous = 3, "0000000000000000000000000000000000000000000000000000000000000000"
		if err := insertRaw(r, j.config.Scope.Account); err == nil {
			t.Fatal("wrong chain link accepted")
		}
		// Bypass only application validation, not triggers: a hash-valid but
		// impossible revision must poison replay, never be silently repaired.
		r.Previous, v.OrderVersion = previous.Hash, 99
		if err := insertRaw(r, j.config.Scope.Account); err != nil {
			t.Fatal("could not install deliberate semantic corruption fixture", err)
		}
		if _, err := j.Snapshot(ctx, fixtureTime); !errors.Is(err, ErrJournal) {
			t.Fatal("corrupt lifecycle replayed", err)
		}
		if _, err := OpenPostgresJournal(ctx, pool, j.config, fixtureTime); !errors.Is(err, ErrJournal) {
			t.Fatal("reopen repaired corrupt lifecycle", err)
		}
		var count int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM execution_sim_lab.events WHERE run_id='sql-guards'`).Scan(&count); err != nil || count != 2 {
			t.Fatal("original corrupt evidence not retained", count, err)
		}
	})

	t.Run("other database is refused before schema access", func(t *testing.T) {
		configuration, err := pgxpool.ParseConfig(dsn)
		if err != nil {
			t.Fatal("test configuration unavailable")
		}
		configuration.ConnConfig.Database = "postgres"
		other, err := pgxpool.NewWithConfig(ctx, configuration)
		if err != nil {
			t.Fatal("control test database unavailable")
		}
		defer other.Close()
		if _, err := OpenPostgresJournal(ctx, other, config("wrong-database"), fixtureTime); !errors.Is(err, ErrJournal) {
			t.Fatal("database safety boundary accepted another database", err)
		}
		var exists bool
		if err := other.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_namespace WHERE nspname='execution_sim_lab')`).Scan(&exists); err != nil || exists {
			t.Fatal("journal created a schema outside its dedicated test database", exists, err)
		}
	})
}
