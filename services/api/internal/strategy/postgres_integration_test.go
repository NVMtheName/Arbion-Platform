package strategy

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/automation"
	"github.com/arbion/platform/services/api/internal/risk"
	"github.com/arbion/platform/services/api/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func TestPostgresEvaluationCommitIsAtomicAndModeBound(t *testing.T) {
	databaseURL := os.Getenv("STRATEGY_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("STRATEGY_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	db, err := sql.Open("pgx", databaseURL)
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
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	const (
		userID       = "11111111-1111-4111-8111-111111111111"
		connectionID = "22222222-2222-4222-8222-222222222222"
		accountID    = "33333333-3333-4333-8333-333333333333"
		bucketID     = "44444444-4444-4444-8444-444444444444"
		mandateID    = "55555555-5555-4555-8555-555555555555"
	)
	statements := []string{
		`INSERT INTO users(id,email,normalized_email,display_name,email_verified_at) VALUES('` + userID + `','test@example.com','test@example.com','Test',now())`,
		`INSERT INTO provider_connections(id,user_id,provider_category,provider_name,display_name,status) VALUES('` + connectionID + `','` + userID + `','financial','schwab','Schwab','active')`,
		`INSERT INTO financial_accounts(id,user_id,provider_connection_id,provider_name,provider_account_id,display_name,account_type,base_currency,status,capabilities) VALUES('` + accountID + `','` + userID + `','` + connectionID + `','schwab','opaque','Schwab Test','brokerage','USD','active','{"options":"SUPPORTED","margin":"UNKNOWN"}')`,
		`INSERT INTO capital_buckets(id,user_id,financial_account_id,name,allocation_type,allocation_value,currency,protected_amount,status) VALUES('` + bucketID + `','` + userID + `','` + accountID + `','Paper','FIXED_AMOUNT',20000,'USD',0,'ACTIVE')`,
		`INSERT INTO automation_mandates(id,user_id,financial_account_id,automation_type,strategy_identifier,capital_bucket_id,autonomy_level,execution_mode,status,current_version,strategy_parameters,risk_parameters,allowed_universe,prohibited_universe,margin_allowed,options_allowed,schedule_conditions,capability_unverified) VALUES('` + mandateID + `','` + userID + `','` + accountID + `','STRATEGY','wheel','` + bucketID + `','STRATEGY_AUTONOMOUS','PAPER','READY',1,'{"symbols":["AAPL"],"minimum_dte":20,"maximum_dte":60,"target_delta":"0.30","target_delta_min":"0.20","target_delta_max":"0.40","maximum_contracts":1,"assignment_handling_policy":"continue_wheel"}','{}','{"symbols":["AAPL"],"universe_ids":[]}','{"symbols":[]}',false,true,'{}',false)`,
		`INSERT INTO automation_mandate_versions(mandate_id,version_number,created_by_user_id,source,snapshot,change_summary) VALUES('` + mandateID + `',1,'` + userID + `','UI','{}','{}')`,
	}
	for _, statement := range statements {
		if _, err = pool.Exec(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	wheel := "wheel"
	mandate := automation.Mandate{ID: mandateID, UserID: userID, FinancialAccountID: accountID, AutomationType: "STRATEGY", StrategyIdentifier: &wheel, CapitalBucketID: bucketID, ExecutionMode: "PAPER", CurrentVersion: 1}
	store := NewPostgresStore(pool)
	instance, err := store.Initialize(ctx, userID, mandate, "1.0000000000", ReadyForPut)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	action := proposedOption(instance, "manual:denied", "action:denied")
	deniedEvaluation := risk.RiskEvaluation{ID: "77777777-7777-4777-8777-777777777777", UserID: userID, AccountID: accountID, MandateID: action.MandateID, MandateVersion: action.MandateVersion, Timestamp: now, Decision: risk.Deny, Checks: []risk.RiskCheck{}, ReasonCodes: []risk.ReasonCode{risk.CapitalLimitExceeded}, Mode: "PAPER"}
	deniedDecision := Decision{ProposedAction: &action, ProposedState: PutProposed, CandidateCount: 1, Reason: "test", Rationale: []byte(`{"strategy":"wheel","candidate_count":1}`)}
	deniedResult := ExecutionResult{Status: RiskDenied, Reason: "risk_not_allowed"}
	if err = store.CommitEvaluation(ctx, instance, instance.StateVersion, deniedDecision, deniedEvaluation, deniedResult, now); err != nil {
		t.Fatal(err)
	}
	if err = store.CommitEvaluation(ctx, instance, instance.StateVersion, deniedDecision, deniedEvaluation, deniedResult, now); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("duplicate event was not rejected: %v", err)
	}
	assertCount(t, pool, `SELECT count(*) FROM risk_evaluations`, 1)
	assertCount(t, pool, `SELECT count(*) FROM nonlive_execution_records`, 1)
	assertCount(t, pool, `SELECT count(*) FROM decision_journal_entries`, 1)
	assertCount(t, pool, `SELECT count(*) FROM strategy_evaluation_events WHERE status='COMMITTED'`, 1)
	var state State
	var version int
	var cash string
	if err = pool.QueryRow(ctx, `SELECT current_state,state_version FROM strategy_instances WHERE id=$1`, instance.ID).Scan(&state, &version); err != nil || state != ReadyForPut || version != 1 {
		t.Fatalf("risk denial advanced state: %s v%d %v", state, version, err)
	}
	if err = pool.QueryRow(ctx, `SELECT cash::text FROM paper_portfolios WHERE strategy_instance_id=$1`, instance.ID).Scan(&cash); err != nil || cash != "1.0000000000" {
		t.Fatalf("risk denial mutated paper cash: %s %v", cash, err)
	}

	fillTime := now.Add(time.Minute)
	filledAction := proposedOption(instance, "manual:filled", "action:filled")
	allowedEvaluation := risk.RiskEvaluation{ID: "88888888-8888-4888-8888-888888888888", UserID: userID, AccountID: accountID, MandateID: filledAction.MandateID, MandateVersion: filledAction.MandateVersion, Timestamp: fillTime, Decision: risk.Allow, Checks: []risk.RiskCheck{}, ReasonCodes: []risk.ReasonCode{risk.Allowed}, Mode: "PAPER"}
	filledDecision := Decision{ProposedAction: &filledAction, ProposedState: PutProposed, CandidateCount: 1, Reason: "test", Rationale: []byte(`{"strategy":"wheel","candidate_count":1}`)}
	price, premium := "1.2500000000", "125.0000000000"
	filledResult := ExecutionResult{Status: SimulatedFilled, Price: &price, Notional: &premium, ExpectedState: ShortPutOpen}
	if err = store.CommitEvaluation(ctx, instance, instance.StateVersion, filledDecision, allowedEvaluation, filledResult, fillTime); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT current_state,state_version FROM strategy_instances WHERE id=$1`, instance.ID).Scan(&state, &version); err != nil || state != ShortPutOpen || version != 2 {
		t.Fatalf("paper fill did not advance state: %s v%d %v", state, version, err)
	}
	if err = pool.QueryRow(ctx, `SELECT cash::text FROM paper_portfolios WHERE strategy_instance_id=$1`, instance.ID).Scan(&cash); err != nil || cash != "126.0000000000" {
		t.Fatalf("paper premium was not isolated: %s %v", cash, err)
	}
	var quantity string
	if err = pool.QueryRow(ctx, `SELECT quantity::text FROM paper_positions`).Scan(&quantity); err != nil || quantity != "-1.0000000000" {
		t.Fatalf("paper option position missing: %s %v", quantity, err)
	}
	firstPage, err := store.Journal(ctx, userID, 1, nil)
	if err != nil || len(firstPage) != 1 {
		t.Fatalf("decision journal first page unavailable: %#v %v", firstPage, err)
	}
	first := firstPage[0]
	if !first.CreatedAt.Equal(fillTime) || first.AccountDisplayName != "Schwab Test" || first.RiskDecision == nil || *first.RiskDecision != string(risk.Allow) || first.ExecutionStatus == nil || *first.ExecutionStatus != string(SimulatedFilled) {
		t.Fatalf("decision journal projection is incomplete: %#v", first)
	}
	secondPage, err := store.Journal(ctx, userID, 1, &JournalCursor{CreatedAt: first.CreatedAt, ID: first.ID})
	if err != nil || len(secondPage) != 1 || !secondPage[0].CreatedAt.Equal(now) {
		t.Fatalf("decision journal cursor is unstable: %#v %v", secondPage, err)
	}
	foreignPage, err := store.Journal(ctx, "99999999-9999-4999-8999-999999999999", 10, nil)
	if err != nil || len(foreignPage) != 0 {
		t.Fatalf("decision journal crossed its owner boundary: %#v %v", foreignPage, err)
	}
	decisions, err := store.Decisions(ctx, userID, instance.ID)
	if err != nil || len(decisions) != 2 || decisions[0].CreatedAt.IsZero() {
		t.Fatalf("per-instance journal timestamps are missing: %#v %v", decisions, err)
	}
	assertCount(t, pool, `SELECT count(*) FROM strategy_state_transitions`, 2)
	facts, err := store.EvaluationFacts(ctx, Instance{ID: instance.ID, UserID: userID, FinancialAccountID: accountID, AutomationMandateID: mandateID, ExecutionMode: Paper}, fillTime)
	if err != nil || facts.Paper == nil || facts.Paper.CurrentExposure != "19000.0000000000" || facts.ActionsToday != 2 {
		t.Fatalf("paper evaluation facts are incomplete: %#v %v", facts, err)
	}
}

func proposedOption(instance Instance, eventID, actionID string) risk.ProposedAction {
	mandateID := instance.AutomationMandateID
	mandateVersion := instance.MandateVersion
	strategyIdentifier := instance.StrategyIdentifier
	strategyInstanceID := instance.ID
	strategyState := string(instance.CurrentState)
	price := "1.2500000000"
	return risk.ProposedAction{ID: actionID, CorrelationID: eventID, FinancialAccountID: instance.FinancialAccountID, Source: risk.SourceStrategy, ActionType: risk.ActionOpenOption, MandateID: &mandateID, MandateVersion: &mandateVersion, Instrument: "AAPL", Side: "SELL_TO_OPEN", Quantity: "1", Notional: "19000.0000000000", EstimatedPrice: &price, Option: &risk.OptionContract{Underlying: "AAPL", Expiration: "2026-01-31", PutCall: "PUT", Strike: "190", ContractMultiplier: 100}, StrategyIdentifier: &strategyIdentifier, StrategyInstanceID: &strategyInstanceID, StrategyState: &strategyState}
}

func assertCount(t *testing.T, pool *pgxpool.Pool, query string, expected int) {
	t.Helper()
	var count int
	if err := pool.QueryRow(context.Background(), query).Scan(&count); err != nil || count != expected {
		t.Fatalf("count mismatch for %q: %d %v", query, count, err)
	}
}
