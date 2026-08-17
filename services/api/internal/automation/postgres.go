package automation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresStore struct{ db *pgxpool.Pool }

func NewPostgresStore(db *pgxpool.Pool) *PostgresStore { return &PostgresStore{db} }
func (s *PostgresStore) AccountFacts(c context.Context, u, id string) (AccountFacts, error) {
	var caps []byte
	var f AccountFacts
	e := s.db.QueryRow(c, `SELECT capabilities FROM financial_accounts WHERE id=$1 AND user_id=$2 AND status='active'`, id, u).Scan(&caps)
	if e != nil {
		return f, e
	}
	f.Owned = true
	var m map[string]string
	_ = json.Unmarshal(caps, &m)
	f.Options = m["options"]
	f.Margin = m["margin"]
	return f, nil
}
func (s *PostgresStore) AIFacts(c context.Context, u, id, model string) (AIFacts, error) {
	var f AIFacts
	e := s.db.QueryRow(c, `SELECT status='active', EXISTS(SELECT 1 FROM neural_engine_preferences n WHERE n.user_id=$2 AND n.provider_connection_id=p.id AND n.model_id=$3) FROM provider_connections p WHERE p.id=$1 AND p.user_id=$2 AND p.provider_category='ai'`, id, u, model).Scan(&f.Active, &f.ModelValid)
	f.Owned = e == nil
	return f, e
}

const bucketCols = `id::text,user_id::text,financial_account_id::text,name,allocation_type,allocation_value::text,currency,is_reserve,protected_amount::text,allocation_limit::text,status,created_at,updated_at`

func scanBucket(r pgx.Row) (b CapitalBucket, e error) {
	var lim *string
	e = r.Scan(&b.ID, &b.UserID, &b.FinancialAccountID, &b.Name, &b.AllocationType, &b.AllocationValue, &b.Currency, &b.IsReserve, &b.ProtectedAmount, &lim, &b.Status, &b.CreatedAt, &b.UpdatedAt)
	b.AllocationLimit = lim
	return
}
func (s *PostgresStore) CreateBucket(c context.Context, u string, x CreateBucketCommand) (CapitalBucket, error) {
	return scanBucket(s.db.QueryRow(c, `INSERT INTO capital_buckets(user_id,financial_account_id,name,allocation_type,allocation_value,currency,is_reserve,protected_amount,allocation_limit) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING `+bucketCols, u, x.FinancialAccountID, x.Name, x.AllocationType, x.AllocationValue, x.Currency, x.IsReserve, x.ProtectedAmount, x.AllocationLimit))
}
func (s *PostgresStore) FixedAllocated(c context.Context, u, a string) (*big.Rat, error) {
	var v string
	e := s.db.QueryRow(c, `SELECT COALESCE(sum(allocation_value),0)::text FROM capital_buckets WHERE user_id=$1 AND financial_account_id=$2 AND status='ACTIVE' AND allocation_type='FIXED_AMOUNT'`, u, a).Scan(&v)
	r, _ := new(big.Rat).SetString(v)
	return r, e
}
func (s *PostgresStore) ListBuckets(c context.Context, u string) ([]CapitalBucket, error) {
	rows, e := s.db.Query(c, `SELECT `+bucketCols+` FROM capital_buckets WHERE user_id=$1 ORDER BY created_at`, u)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []CapitalBucket
	for rows.Next() {
		b, e := scanBucket(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, b)
	}
	return out, rows.Err()
}
func (s *PostgresStore) GetBucket(c context.Context, u, id string) (CapitalBucket, error) {
	return scanBucket(s.db.QueryRow(c, `SELECT `+bucketCols+` FROM capital_buckets WHERE id=$1 AND user_id=$2`, id, u))
}
func (s *PostgresStore) UpdateBucket(c context.Context, u, id string, x CreateBucketCommand) (CapitalBucket, error) {
	return scanBucket(s.db.QueryRow(c, `UPDATE capital_buckets SET name=$3,allocation_type=$4,allocation_value=$5,currency=$6,is_reserve=$7,protected_amount=$8,allocation_limit=$9,updated_at=now() WHERE id=$1 AND user_id=$2 AND status='ACTIVE' RETURNING `+bucketCols, id, u, x.Name, x.AllocationType, x.AllocationValue, x.Currency, x.IsReserve, x.ProtectedAmount, x.AllocationLimit))
}
func (s *PostgresStore) DeleteBucket(c context.Context, u, id string) error {
	tag, e := s.db.Exec(c, `UPDATE capital_buckets SET status='ARCHIVED',updated_at=now() WHERE id=$1 AND user_id=$2 AND status='ACTIVE' AND NOT EXISTS(SELECT 1 FROM automation_mandates m WHERE m.capital_bucket_id=$1 AND m.status<>'ARCHIVED')`, id, u)
	if e == nil && tag.RowsAffected() == 0 {
		return ErrConflict
	}
	return e
}

const mandateCols = `id::text,user_id::text,financial_account_id::text,automation_type,strategy_identifier,ai_provider_connection_id::text,ai_model_id,capital_bucket_id::text,autonomy_level,execution_mode,status,current_version,strategy_parameters,risk_parameters,allowed_universe,prohibited_universe,margin_allowed,options_allowed,capability_unverified,schedule_conditions,effective_from,effective_until,created_at,updated_at`

func scanMandate(r pgx.Row) (m Mandate, e error) {
	var risk, allow, deny []byte
	e = r.Scan(&m.ID, &m.UserID, &m.FinancialAccountID, &m.AutomationType, &m.StrategyIdentifier, &m.AIProviderConnectionID, &m.AIModelID, &m.CapitalBucketID, &m.AutonomyLevel, &m.ExecutionMode, &m.Status, &m.CurrentVersion, &m.StrategyParameters, &risk, &allow, &deny, &m.MarginAllowed, &m.OptionsAllowed, &m.CapabilityUnverified, &m.ScheduleConditions, &m.EffectiveFrom, &m.EffectiveUntil, &m.CreatedAt, &m.UpdatedAt)
	_ = json.Unmarshal(risk, &m.Risk)
	_ = json.Unmarshal(allow, &m.AllowedUniverse)
	_ = json.Unmarshal(deny, &m.ProhibitedUniverse)
	m.ExecutionCapable = false
	return
}
func normalizeJSON(v json.RawMessage) json.RawMessage {
	if len(v) == 0 {
		return json.RawMessage(`{}`)
	}
	return v
}
func args(u string, x MandateCommand, unverified bool) []any {
	risk, _ := json.Marshal(x.Risk)
	allow, _ := json.Marshal(x.AllowedUniverse)
	deny, _ := json.Marshal(x.ProhibitedUniverse)
	from := x.EffectiveFrom
	return []any{u, x.FinancialAccountID, x.AutomationType, x.StrategyIdentifier, x.AIProviderConnectionID, x.AIModelID, x.CapitalBucketID, x.AutonomyLevel, x.ExecutionMode, normalizeJSON(x.StrategyParameters), risk, allow, deny, x.MarginAllowed, x.OptionsAllowed, normalizeJSON(x.ScheduleConditions), unverified, from, x.EffectiveUntil}
}
func snapshot(m Mandate) ([]byte, error) {
	return json.Marshal(map[string]any{"financial_account_id": m.FinancialAccountID, "automation_type": m.AutomationType, "strategy_identifier": m.StrategyIdentifier, "ai_provider_connection_id": m.AIProviderConnectionID, "ai_model_id": m.AIModelID, "capital_bucket_id": m.CapitalBucketID, "autonomy_level": m.AutonomyLevel, "execution_mode": m.ExecutionMode, "status": m.Status, "strategy_parameters": m.StrategyParameters, "risk_parameters": m.Risk, "allowed_universe": m.AllowedUniverse, "prohibited_universe": m.ProhibitedUniverse, "margin_allowed": m.MarginAllowed, "options_allowed": m.OptionsAllowed, "schedule_conditions": m.ScheduleConditions, "capability_unverified": m.CapabilityUnverified, "effective_from": m.EffectiveFrom, "effective_until": m.EffectiveUntil, "execution_capable": false})
}
func versionChangeSummary(previous int) ([]byte, error) {
	return json.Marshal(map[string]int{"previous_version": previous})
}
func (s *PostgresStore) CreateMandate(c context.Context, u string, x MandateCommand, unverified bool) (Mandate, error) {
	tx, e := s.db.Begin(c)
	if e != nil {
		return Mandate{}, e
	}
	defer tx.Rollback(c)
	a := args(u, x, unverified)
	m, e := scanMandate(tx.QueryRow(c, `INSERT INTO automation_mandates(user_id,financial_account_id,automation_type,strategy_identifier,ai_provider_connection_id,ai_model_id,capital_bucket_id,autonomy_level,execution_mode,strategy_parameters,risk_parameters,allowed_universe,prohibited_universe,margin_allowed,options_allowed,schedule_conditions,capability_unverified,effective_from,effective_until,current_version) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,COALESCE($18,now()),$19,1) RETURNING `+mandateCols, a...))
	if e != nil {
		return m, e
	}
	snap, _ := snapshot(m)
	_, e = tx.Exec(c, `INSERT INTO automation_mandate_versions(mandate_id,version_number,created_by_user_id,source,snapshot,change_summary) VALUES($1,1,$2,'UI',$3,'{"change":"created"}')`, m.ID, u, snap)
	if e != nil {
		return m, e
	}
	return m, tx.Commit(c)
}
func (s *PostgresStore) ListMandates(c context.Context, u string) ([]Mandate, error) {
	rows, e := s.db.Query(c, `SELECT `+mandateCols+` FROM automation_mandates WHERE user_id=$1 ORDER BY updated_at DESC`, u)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []Mandate
	for rows.Next() {
		m, e := scanMandate(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
func (s *PostgresStore) GetMandate(c context.Context, u, id string) (Mandate, error) {
	return scanMandate(s.db.QueryRow(c, `SELECT `+mandateCols+` FROM automation_mandates WHERE id=$1 AND user_id=$2`, id, u))
}
func (s *PostgresStore) UpdateMandate(c context.Context, u, id string, expected int, x MandateCommand, unverified bool, source string) (Mandate, error) {
	a := args(u, x, unverified)
	q := `UPDATE automation_mandates SET financial_account_id=$3,automation_type=$4,strategy_identifier=$5,ai_provider_connection_id=$6,ai_model_id=$7,capital_bucket_id=$8,autonomy_level=$9,execution_mode=$10,strategy_parameters=$11,risk_parameters=$12,allowed_universe=$13,prohibited_universe=$14,margin_allowed=$15,options_allowed=$16,schedule_conditions=$17,capability_unverified=$18,effective_from=COALESCE($19,effective_from),effective_until=$20,current_version=current_version+1,updated_at=now() WHERE id=$1 AND user_id=$2 AND current_version=$21 RETURNING ` + mandateCols
	return s.versionedUpdate(c, u, id, expected, source, q, append([]any{id, u}, append(a[1:], expected)...)...)
}
func (s *PostgresStore) Transition(c context.Context, u, id string, expected int, status, source string) (Mandate, error) {
	if _, ok := map[string]bool{"READY": true, "PAUSED": true, "DISABLED": true, "ARCHIVED": true}[status]; !ok {
		return Mandate{}, ErrInvalid
	}
	q := `UPDATE automation_mandates SET status=$4,current_version=current_version+1,updated_at=now() WHERE id=$1 AND user_id=$2 AND current_version=$3 AND status<>'ARCHIVED' RETURNING ` + mandateCols
	return s.versionedUpdate(c, u, id, expected, source, q, id, u, expected, status)
}
func (s *PostgresStore) versionedUpdate(c context.Context, u, id string, expected int, source, q string, a ...any) (Mandate, error) {
	tx, e := s.db.Begin(c)
	if e != nil {
		return Mandate{}, e
	}
	defer tx.Rollback(c)
	m, e := scanMandate(tx.QueryRow(c, q, a...))
	if errors.Is(e, pgx.ErrNoRows) {
		return m, ErrConflict
	}
	if e != nil {
		return m, e
	}
	snap, e := snapshot(m)
	if e != nil {
		return m, e
	}
	summary, e := versionChangeSummary(expected)
	if e != nil {
		return m, e
	}
	_, e = tx.Exec(c, `INSERT INTO automation_mandate_versions(mandate_id,version_number,created_by_user_id,source,snapshot,change_summary) VALUES($1,$2,$3,$4,$5,$6)`, id, m.CurrentVersion, u, source, snap, summary)
	if e != nil {
		return m, e
	}
	return m, tx.Commit(c)
}
func (s *PostgresStore) Versions(c context.Context, u, id string) ([]Version, error) {
	rows, e := s.db.Query(c, `SELECT v.id::text,v.mandate_id::text,v.version_number,v.created_at,v.created_by_user_id::text,v.source,v.snapshot,v.change_summary FROM automation_mandate_versions v JOIN automation_mandates m ON m.id=v.mandate_id WHERE v.mandate_id=$1 AND m.user_id=$2 ORDER BY version_number`, id, u)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []Version
	for rows.Next() {
		var v Version
		if e = rows.Scan(&v.ID, &v.MandateID, &v.VersionNumber, &v.CreatedAt, &v.CreatedByUserID, &v.Source, &v.Snapshot, &v.ChangeSummary); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *PostgresStore) Version(c context.Context, u, id string, n int) (Version, error) {
	var v Version
	e := s.db.QueryRow(c, `SELECT v.id::text,v.mandate_id::text,v.version_number,v.created_at,v.created_by_user_id::text,v.source,v.snapshot,v.change_summary FROM automation_mandate_versions v JOIN automation_mandates m ON m.id=v.mandate_id WHERE v.mandate_id=$1 AND m.user_id=$2 AND v.version_number=$3`, id, u, n).Scan(&v.ID, &v.MandateID, &v.VersionNumber, &v.CreatedAt, &v.CreatedByUserID, &v.Source, &v.Snapshot, &v.ChangeSummary)
	return v, e
}

var _ Store = (*PostgresStore)(nil)
var _ = fmt.Sprintf
