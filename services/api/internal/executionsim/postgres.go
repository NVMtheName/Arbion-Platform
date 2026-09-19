package executionsim

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// This database name is an intentional laboratory barrier, not a configurable
// deployment setting. The schema exists only in the dedicated CI/test database.
const simulationDatabase = "arbion_execution_sim"

var ErrCommitUnknown = errors.New("simulation commit outcome unknown; recover saved evidence before continuing")

type SimulationDB interface {
	BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error)
}

// PostgresJournal exercises transactional recovery with fictional data only.
// It has no cached mutable projection, provider client, production table access,
// approval authority or schema-creation capability. A handle is safe to share;
// independent handles/processes serialize on the exact session's genesis row.
type PostgresJournal struct {
	db     SimulationDB
	config Config
}

// OpenPostgresJournal idempotently establishes an immutable fixture genesis.
// Changed configuration for the same scope is a conflict, never a new budget.
func OpenPostgresJournal(ctx context.Context, db SimulationDB, config Config, now time.Time) (*PostgresJournal, error) {
	engine, err := New(config, now)
	if err != nil || db == nil {
		return nil, ErrInvalid
	}
	j := &PostgresJournal{db: db, config: engine.config}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := j.begin(ctx, false)
	if err != nil {
		return nil, err
	}
	defer rollbackSimulation(tx)
	genesis := record{Sequence: 1, Config: &j.config}
	genesis.Hash = recordHash(genesis)
	data, err := json.Marshal(genesis)
	if err != nil {
		return nil, ErrInvalid
	}
	s := j.config.Scope
	_, err = tx.Exec(ctx, `INSERT INTO execution_sim_lab.sessions(owner_id,account_id,provider,run_id,genesis)
		VALUES($1,$2,$3,$4,$5) ON CONFLICT(owner_id,account_id,provider,run_id) DO NOTHING`, s.Owner, s.Account, s.Provider, s.Run, string(data))
	if err != nil {
		return nil, ErrJournal
	}
	if _, _, _, err = j.replay(ctx, tx, now, true); err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, ErrCommitUnknown
	}
	return j, nil
}

// Append locks, fully replays and validates, then appends exactly one new
// delivery in the same transaction. Economic aliases are saved without applying
// their settlement twice. Nothing is acknowledged or published before commit.
// A commit error is ambiguous: the caller must recover/replay the same identity,
// never generate a new send attempt. The engine exposes no resend transition.
func (j *PostgresJournal) Append(ctx context.Context, event Event, now time.Time) (bool, error) {
	if event.Scope != j.config.Scope {
		return false, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := j.begin(ctx, false)
	if err != nil {
		return false, err
	}
	defer rollbackSimulation(tx)
	engine, head, size, err := j.replay(ctx, tx, now, true)
	if err != nil {
		return false, err
	}
	_, known := engine.events[event.ID]
	applied, err := engine.Apply(event, now)
	if err != nil {
		return false, err
	}
	if !known {
		r := record{Sequence: head.Sequence + 1, Previous: head.Hash, Event: &event}
		r.Hash = recordHash(r)
		data, err := json.Marshal(r)
		if err != nil || len(data) > 64<<10 || size+len(data) > 16<<20 {
			return false, ErrJournal
		}
		s := j.config.Scope
		_, err = tx.Exec(ctx, `INSERT INTO execution_sim_lab.events(owner_id,account_id,provider,run_id,sequence,delivery_id,record)
			VALUES($1,$2,$3,$4,$5,$6,$7)`, s.Owner, s.Account, s.Provider, s.Run, r.Sequence, event.ID, string(data))
		if err != nil {
			return false, ErrJournal
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return false, ErrCommitUnknown
	}
	return applied, nil
}

// Snapshot uses one repeatable-read snapshot for genesis and every event. It
// never combines records from different committed views or serves cached state.
func (j *PostgresJournal) Snapshot(ctx context.Context, now time.Time) (Snapshot, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := j.begin(ctx, true)
	if err != nil {
		return Snapshot{}, err
	}
	defer rollbackSimulation(tx)
	engine, _, _, err := j.replay(ctx, tx, now, false)
	if err != nil {
		return Snapshot{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Snapshot{}, ErrJournal
	}
	return engine.Snapshot(), nil
}

func (j *PostgresJournal) begin(ctx context.Context, readOnly bool) (pgx.Tx, error) {
	options := pgx.TxOptions{IsoLevel: pgx.ReadCommitted}
	if readOnly {
		options.IsoLevel, options.AccessMode = pgx.RepeatableRead, pgx.ReadOnly
	}
	tx, err := j.db.BeginTx(ctx, options)
	if err != nil {
		return nil, ErrJournal
	}
	var database string
	if err := tx.QueryRow(ctx, `SELECT current_database()`).Scan(&database); err != nil || database != simulationDatabase {
		rollbackSimulation(tx)
		return nil, ErrJournal
	}
	if !readOnly {
		// Do not inherit asynchronous commit from a test server configuration.
		if _, err := tx.Exec(ctx, `SET LOCAL synchronous_commit = 'on'`); err != nil {
			rollbackSimulation(tx)
			return nil, ErrJournal
		}
	}
	return tx, nil
}

func rollbackSimulation(tx pgx.Tx) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = tx.Rollback(ctx)
}

func (j *PostgresJournal) replay(ctx context.Context, tx pgx.Tx, now time.Time, lock bool) (*Engine, record, int, error) {
	fail := func(err error) (*Engine, record, int, error) { return nil, record{}, 0, err }
	s := j.config.Scope
	query := `SELECT genesis FROM execution_sim_lab.sessions WHERE owner_id=$1 AND account_id=$2 AND provider=$3 AND run_id=$4`
	if lock {
		query += ` FOR UPDATE`
	}
	var raw string
	if err := tx.QueryRow(ctx, query, s.Owner, s.Account, s.Provider, s.Run).Scan(&raw); err != nil {
		return fail(ErrJournal)
	}
	head, err := decodeRecord([]byte(raw))
	if err != nil || head.Sequence != 1 || head.Previous != "" || head.Config == nil || head.Event != nil {
		return fail(ErrJournal)
	}
	want, _ := json.Marshal(j.config)
	got, _ := json.Marshal(head.Config)
	if string(want) != string(got) {
		return fail(ErrConflict)
	}
	engine, err := New(j.config, now)
	if err != nil {
		return fail(ErrJournal)
	}
	size := len(raw)
	rows, err := tx.Query(ctx, `SELECT sequence,delivery_id,record FROM execution_sim_lab.events
		WHERE owner_id=$1 AND account_id=$2 AND provider=$3 AND run_id=$4 ORDER BY sequence LIMIT 10001`, s.Owner, s.Account, s.Provider, s.Run)
	if err != nil {
		return fail(ErrJournal)
	}
	defer rows.Close()
	for rows.Next() {
		var sequence int
		var delivery string
		if err := rows.Scan(&sequence, &delivery, &raw); err != nil {
			return fail(ErrJournal)
		}
		size += len(raw)
		r, err := decodeRecord([]byte(raw))
		if err != nil || size > 16<<20 || sequence > 10001 || r.Sequence != sequence || sequence != head.Sequence+1 || r.Previous != head.Hash || r.Config != nil || r.Event == nil || r.Event.ID != delivery {
			return fail(ErrJournal)
		}
		if _, known := engine.events[delivery]; known {
			return fail(ErrJournal)
		}
		if _, err := engine.Apply(*r.Event, now); err != nil {
			return fail(ErrJournal)
		}
		head = r
	}
	if err := rows.Err(); err != nil {
		return fail(ErrJournal)
	}
	return engine, head, size, nil
}
