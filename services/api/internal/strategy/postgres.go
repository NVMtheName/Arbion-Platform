package strategy

import (
	"context"
	"encoding/json"

	"github.com/arbion/platform/services/api/internal/automation"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresStore struct{ db *pgxpool.Pool }

func NewPostgresStore(db *pgxpool.Pool) *PostgresStore { return &PostgresStore{db} }

const instanceColumns = `id::text,user_id::text,automation_mandate_id::text,mandate_version,financial_account_id::text,strategy_identifier,strategy_definition_version,execution_mode,current_state,state_version,status,started_at,updated_at,paused_at,completed_at,last_evaluated_at`

func scanInstance(r pgx.Row) (i Instance, e error) {
	e = r.Scan(&i.ID, &i.UserID, &i.AutomationMandateID, &i.MandateVersion, &i.FinancialAccountID, &i.StrategyIdentifier, &i.DefinitionVersion, &i.ExecutionMode, &i.CurrentState, &i.StateVersion, &i.Status, &i.StartedAt, &i.UpdatedAt, &i.PausedAt, &i.CompletedAt, &i.LastEvaluatedAt)
	return
}
func (s *PostgresStore) Initialize(c context.Context, u string, m automation.Mandate, cash string, state State) (Instance, error) {
	tx, e := s.db.Begin(c)
	if e != nil {
		return Instance{}, e
	}
	defer tx.Rollback(c)
	i, e := scanInstance(tx.QueryRow(c, `INSERT INTO strategy_instances(user_id,automation_mandate_id,mandate_version,financial_account_id,strategy_identifier,strategy_definition_version,execution_mode,current_state) VALUES($1,$2,$3,$4,$5,1,$6,$7) RETURNING `+instanceColumns, u, m.ID, m.CurrentVersion, m.FinancialAccountID, *m.StrategyIdentifier, m.ExecutionMode, state))
	if e != nil {
		return i, e
	}
	if m.ExecutionMode == "PAPER" {
		if _, e = tx.Exec(c, `INSERT INTO paper_portfolios(user_id,strategy_instance_id,currency,starting_cash,cash) SELECT $1,$2,b.currency,$3,$3 FROM capital_buckets b WHERE b.id=$4`, u, i.ID, cash, m.CapitalBucketID); e != nil {
			return i, e
		}
	}
	meta, _ := json.Marshal(map[string]any{"mandate_version": m.CurrentVersion, "definition_version": 1})
	_, e = tx.Exec(c, `INSERT INTO strategy_state_transitions(strategy_instance_id,previous_state,new_state,state_version,trigger,metadata) VALUES($1,$2,$2,1,'INITIALIZED',$3)`, i.ID, state, meta)
	if e != nil {
		return i, e
	}
	return i, tx.Commit(c)
}
func (s *PostgresStore) List(c context.Context, u string) ([]Instance, error) {
	rows, e := s.db.Query(c, `SELECT `+instanceColumns+` FROM strategy_instances WHERE user_id=$1 ORDER BY updated_at DESC`, u)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Instance{}
	for rows.Next() {
		i, e := scanInstance(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, i)
	}
	return out, rows.Err()
}
func (s *PostgresStore) Get(c context.Context, u, id string) (Instance, error) {
	return scanInstance(s.db.QueryRow(c, `SELECT `+instanceColumns+` FROM strategy_instances WHERE id=$1 AND user_id=$2`, id, u))
}
func (s *PostgresStore) History(c context.Context, u, id string) ([]Transition, error) {
	rows, e := s.db.Query(c, `SELECT t.id::text,t.strategy_instance_id::text,t.previous_state,t.new_state,t.state_version,t.trigger,t.proposed_action_id,t.risk_evaluation_id::text,t.execution_record_id::text,t.metadata,t.occurred_at FROM strategy_state_transitions t JOIN strategy_instances i ON i.id=t.strategy_instance_id WHERE i.id=$1 AND i.user_id=$2 ORDER BY t.state_version`, id, u)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Transition{}
	for rows.Next() {
		var x Transition
		if e = rows.Scan(&x.ID, &x.StrategyInstanceID, &x.PreviousState, &x.NewState, &x.StateVersion, &x.Trigger, &x.ProposedActionID, &x.RiskEvaluationID, &x.ExecutionRecordID, &x.Metadata, &x.OccurredAt); e != nil {
			return nil, e
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
func (s *PostgresStore) Decisions(c context.Context, u, id string) ([]DecisionJournalEntry, error) {
	rows, e := s.db.Query(c, `SELECT d.id::text,d.strategy_instance_id::text,d.strategy_state,d.source,d.decision_type,d.structured_rationale,d.proposed_action_id,d.risk_evaluation_id::text,d.execution_record_id::text,d.resulting_state FROM decision_journal_entries d JOIN strategy_instances i ON i.id=d.strategy_instance_id WHERE i.id=$1 AND i.user_id=$2 ORDER BY d.created_at DESC`, id, u)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []DecisionJournalEntry{}
	for rows.Next() {
		var x DecisionJournalEntry
		if e = rows.Scan(&x.ID, &x.StrategyInstanceID, &x.StrategyState, &x.Source, &x.DecisionType, &x.StructuredRationale, &x.ProposedActionID, &x.RiskEvaluationID, &x.ExecutionRecordID, &x.ResultingState); e != nil {
			return nil, e
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
func (s *PostgresStore) Executions(c context.Context, u, id string) ([]ExecutionRecord, error) {
	rows, e := s.db.Query(c, `SELECT x.id::text,x.idempotency_key,x.user_id::text,x.strategy_instance_id::text,x.mandate_id::text,x.mandate_version,x.proposed_action_id,x.risk_evaluation_id::text,x.mode,x.status,x.symbol,x.instrument,x.side,x.quantity::text,x.price::text,x.notional::text,x.metadata,x.created_at FROM nonlive_execution_records x WHERE x.strategy_instance_id=$1 AND x.user_id=$2 ORDER BY x.created_at DESC`, id, u)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []ExecutionRecord{}
	for rows.Next() {
		var x ExecutionRecord
		if e = rows.Scan(&x.ID, &x.IdempotencyKey, &x.UserID, &x.StrategyInstanceID, &x.MandateID, &x.MandateVersion, &x.ProposedActionID, &x.RiskEvaluationID, &x.Mode, &x.Status, &x.Symbol, &x.Instrument, &x.Side, &x.Quantity, &x.Price, &x.Notional, &x.Metadata, &x.CreatedAt); e != nil {
			return nil, e
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

var _ Persistence = (*PostgresStore)(nil)
