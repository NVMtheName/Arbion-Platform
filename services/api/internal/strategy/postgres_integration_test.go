package strategy

import (
	"context"
	"database/sql"
	"encoding/json"
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
		userID          = "11111111-1111-4111-8111-111111111111"
		connectionID    = "22222222-2222-4222-8222-222222222222"
		accountID       = "33333333-3333-4333-8333-333333333333"
		bucketID        = "44444444-4444-4444-8444-444444444444"
		secondBucketID  = "77777777-7777-4777-8777-777777777777"
		mandateID       = "55555555-5555-4555-8555-555555555555"
		secondMandateID = "66666666-6666-4666-8666-666666666666"
	)
	statements := []string{
		`INSERT INTO users(id,email,normalized_email,display_name,email_verified_at) VALUES('` + userID + `','test@example.com','test@example.com','Test',now())`,
		`INSERT INTO user_entitlements(user_id,entitlement_key,source,billing_required) VALUES('` + userID + `','founder','bootstrap',false)`,
		`INSERT INTO provider_connections(id,user_id,provider_category,provider_name,display_name,status) VALUES('` + connectionID + `','` + userID + `','financial','schwab','Schwab','active')`,
		`INSERT INTO financial_accounts(id,user_id,provider_connection_id,provider_name,provider_account_id,display_name,account_type,base_currency,status,capabilities) VALUES('` + accountID + `','` + userID + `','` + connectionID + `','schwab','opaque','Schwab Test','brokerage','USD','active','{"options":"SUPPORTED","margin":"UNKNOWN"}')`,
		`INSERT INTO capital_buckets(id,user_id,financial_account_id,name,allocation_type,allocation_value,currency,protected_amount,status) VALUES('` + bucketID + `','` + userID + `','` + accountID + `','Paper','FIXED_AMOUNT',20000,'USD',0,'ACTIVE')`,
		`INSERT INTO capital_buckets(id,user_id,financial_account_id,name,allocation_type,allocation_value,currency,protected_amount,status) VALUES('` + secondBucketID + `','` + userID + `','` + accountID + `','Second paper bucket','FIXED_AMOUNT',1000,'USD',0,'ACTIVE')`,
		`INSERT INTO automation_mandates(id,user_id,financial_account_id,automation_type,strategy_identifier,capital_bucket_id,autonomy_level,execution_mode,status,current_version,strategy_parameters,risk_parameters,allowed_universe,prohibited_universe,margin_allowed,options_allowed,schedule_conditions,capability_unverified) VALUES('` + mandateID + `','` + userID + `','` + accountID + `','STRATEGY','wheel','` + bucketID + `','STRATEGY_AUTONOMOUS','PAPER','READY',1,'{"symbols":["AAPL"],"minimum_dte":20,"maximum_dte":60,"target_delta":"0.30","target_delta_min":"0.20","target_delta_max":"0.40","maximum_contracts":1,"assignment_handling_policy":"continue_wheel"}','{}','{"symbols":["AAPL"],"universe_ids":[]}','{"symbols":[]}',false,true,'{"enabled":true,"interval_minutes":60,"session":"US_EQUITIES_REGULAR","notifications":{"evaluation_completed":true,"lifecycle_required":true,"first_failure":true}}',false)`,
		`INSERT INTO automation_mandate_versions(mandate_id,version_number,created_by_user_id,source,snapshot,change_summary) VALUES('` + mandateID + `',1,'` + userID + `','UI','{}','{}')`,
		`INSERT INTO automation_mandates(id,user_id,financial_account_id,automation_type,strategy_identifier,capital_bucket_id,autonomy_level,execution_mode,status,current_version,strategy_parameters,risk_parameters,allowed_universe,prohibited_universe,margin_allowed,options_allowed,schedule_conditions,capability_unverified) VALUES('` + secondMandateID + `','` + userID + `','` + accountID + `','STRATEGY','wheel','` + secondBucketID + `','RESEARCH_ONLY','PAPER','READY',1,'{}','{}','{"symbols":[],"universe_ids":[]}','{"symbols":[]}',false,true,'{}',false)`,
		`INSERT INTO automation_mandate_versions(mandate_id,version_number,created_by_user_id,source,snapshot,change_summary) VALUES('` + secondMandateID + `',1,'` + userID + `','UI','{}','{}')`,
	}
	for _, statement := range statements {
		if _, err = pool.Exec(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	wheel := "wheel"
	mandate := automation.Mandate{
		ID: mandateID, UserID: userID, FinancialAccountID: accountID, AutomationType: "STRATEGY",
		StrategyIdentifier: &wheel, CapitalBucketID: bucketID, ExecutionMode: "PAPER", CurrentVersion: 1,
		ScheduleConditions: json.RawMessage(`{"enabled":true,"interval_minutes":60,"session":"US_EQUITIES_REGULAR","notifications":{"evaluation_completed":true,"lifecycle_required":true,"first_failure":true}}`),
	}
	store := NewPostgresStore(pool)
	instance, err := store.Initialize(ctx, userID, mandate, "20000.0000000000", ReadyForPut)
	if err != nil {
		t.Fatal(err)
	}
	if instance.CapitalBucketID != bucketID {
		t.Fatalf("strategy instance lost its capital bucket binding: %#v", instance)
	}
	secondMandate := mandate
	secondMandate.ID = secondMandateID
	secondMandate.CapitalBucketID = secondBucketID
	secondMandate.ScheduleConditions = json.RawMessage(`{}`)
	if _, err = store.Initialize(ctx, userID, secondMandate, "1000", ReadyForPut); !errors.Is(err, ErrAccountInUse) {
		t.Fatalf("financial account was reused by a second active strategy: %v", err)
	}
	assertCount(t, pool, `SELECT count(*) FROM strategy_instances`, 1)
	claimAt := time.Now().UTC().Add(time.Minute)
	scheduled, err := store.ClaimDueSchedule(ctx, claimAt, scheduleLeaseDuration)
	if err != nil || scheduled == nil {
		t.Fatalf("guarded schedule was not claimed: %#v %v", scheduled, err)
	}
	if scheduled.OwnerEmail != "test@example.com" || !scheduled.OwnerEmailVerified || !scheduled.NotifyEvaluation || !scheduled.NotifyLifecycle || !scheduled.NotifyFirstFailure || scheduled.PreviousErrorCode != nil || scheduled.ConsecutiveFailures != 0 {
		t.Fatalf("notification preferences crossed the schedule boundary: %#v", scheduled)
	}
	if err = store.CompleteSchedule(ctx, *scheduled, ScheduleCompletion{CompletedAt: claimAt, NextRunAt: claimAt.Add(24 * time.Hour), Status: "SKIPPED", ErrorCode: "OUTSIDE_SESSION"}); err != nil {
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
	if err = pool.QueryRow(ctx, `SELECT cash::text FROM paper_portfolios WHERE strategy_instance_id=$1`, instance.ID).Scan(&cash); err != nil || cash != "20000.0000000000" {
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
	if err = pool.QueryRow(ctx, `SELECT cash::text FROM paper_portfolios WHERE strategy_instance_id=$1`, instance.ID).Scan(&cash); err != nil || cash != "20125.0000000000" {
		t.Fatalf("paper premium was not isolated: %s %v", cash, err)
	}
	var quantity string
	if err = pool.QueryRow(ctx, `SELECT quantity::text FROM paper_positions`).Scan(&quantity); err != nil || quantity != "-1.0000000000" {
		t.Fatalf("paper option position missing: %s %v", quantity, err)
	}
	portfolio, err := store.PaperPortfolio(ctx, userID, instance.ID)
	if err != nil || portfolio.StartingCash != "20000.0000000000" || portfolio.Cash != "20125.0000000000" || portfolio.Currency != "USD" || portfolio.Version != 2 || len(portfolio.Positions) != 1 {
		t.Fatalf("paper portfolio projection is incomplete: %#v %v", portfolio, err)
	}
	position := portfolio.Positions[0]
	if !position.IsOpen || position.Symbol != "AAPL" || position.Instrument != "OPTION" || position.OptionType != "PUT" || position.Strike != "190.0000000000" || position.Expiration != "2026-01-31" || position.Quantity != "-1.0000000000" || position.AveragePrice != "1.2500000000" {
		t.Fatalf("paper position projection is incomplete: %#v", position)
	}
	if _, err = store.PaperPortfolio(ctx, "99999999-9999-4999-8999-999999999999", instance.ID); err == nil {
		t.Fatal("paper portfolio crossed its owner boundary")
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

	lifecycleTime := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)
	lifecycleCommand := LifecycleCommand{EventID: "manual-lifecycle:expired-1", EventType: ExpireWorthless, ExpectedStateVersion: 2, ConfirmPaperSimulation: true}
	lifecycle, err := store.RecordLifecycle(ctx, userID, instance.ID, lifecycleCommand, lifecycleTime)
	if err != nil || lifecycle.Duplicate || lifecycle.PreviousState != ShortPutOpen || lifecycle.NewState != ReadyForPut || lifecycle.StateVersion != 3 {
		t.Fatalf("paper expiration was not recorded atomically: %#v %v", lifecycle, err)
	}
	duplicate, err := store.RecordLifecycle(ctx, userID, instance.ID, lifecycleCommand, lifecycleTime.Add(time.Minute))
	if err != nil || !duplicate.Duplicate || duplicate.ID != lifecycle.ID {
		t.Fatalf("paper lifecycle retry was not idempotent: %#v %v", duplicate, err)
	}
	conflictingCommand := lifecycleCommand
	conflictingCommand.EventType = Assignment
	if _, err = store.RecordLifecycle(ctx, userID, instance.ID, conflictingCommand, lifecycleTime.Add(2*time.Minute)); !errors.Is(err, ErrConflict) {
		t.Fatalf("conflicting lifecycle identity was accepted: %v", err)
	}
	if err = pool.QueryRow(ctx, `SELECT current_state,state_version FROM strategy_instances WHERE id=$1`, instance.ID).Scan(&state, &version); err != nil || state != ReadyForPut || version != 3 {
		t.Fatalf("paper expiration did not advance state once: %s v%d %v", state, version, err)
	}
	if err = pool.QueryRow(ctx, `SELECT quantity::text FROM paper_positions WHERE instrument='OPTION'`).Scan(&quantity); err != nil || quantity != "0.0000000000" {
		t.Fatalf("expired paper option remained open: %s %v", quantity, err)
	}
	portfolio, err = store.PaperPortfolio(ctx, userID, instance.ID)
	if err != nil || len(portfolio.Positions) != 1 || portfolio.Positions[0].IsOpen {
		t.Fatalf("closed paper position was not projected: %#v %v", portfolio, err)
	}
	if err = pool.QueryRow(ctx, `SELECT cash::text FROM paper_portfolios WHERE strategy_instance_id=$1`, instance.ID).Scan(&cash); err != nil || cash != "20125.0000000000" {
		t.Fatalf("worthless expiration changed paper cash: %s %v", cash, err)
	}
	assertCount(t, pool, `SELECT count(*) FROM strategy_lifecycle_events`, 1)
	assertCount(t, pool, `SELECT count(*) FROM strategy_state_transitions`, 3)
	assertCount(t, pool, `SELECT count(*) FROM decision_journal_entries`, 3)
	lifecyclePage, err := store.Journal(ctx, userID, 1, nil)
	if err != nil || len(lifecyclePage) != 1 || lifecyclePage[0].Source != "LIFECYCLE" || lifecyclePage[0].DecisionType != string(ExpireWorthless) || lifecyclePage[0].RiskDecision != nil || lifecyclePage[0].ExecutionStatus != nil {
		t.Fatalf("paper lifecycle journal projection is incomplete: %#v %v", lifecyclePage, err)
	}

	readyInstance, err := store.Get(ctx, userID, instance.ID)
	if err != nil {
		t.Fatal(err)
	}
	secondPutTime := lifecycleTime.Add(time.Minute)
	secondPut := proposedOption(readyInstance, "manual:filled-2", "action:filled-2")
	secondPut.Option.Expiration = "2026-02-28"
	secondPutEvaluation := risk.RiskEvaluation{ID: "99999999-9999-4999-8999-999999999998", UserID: userID, AccountID: accountID, MandateID: secondPut.MandateID, MandateVersion: secondPut.MandateVersion, Timestamp: secondPutTime, Decision: risk.Allow, Checks: []risk.RiskCheck{}, ReasonCodes: []risk.ReasonCode{risk.Allowed}, Mode: "PAPER"}
	secondPutDecision := Decision{ProposedAction: &secondPut, ProposedState: PutProposed, CandidateCount: 1, Reason: "test", Rationale: []byte(`{"strategy":"wheel","candidate_count":1}`)}
	if err = store.CommitEvaluation(ctx, readyInstance, readyInstance.StateVersion, secondPutDecision, secondPutEvaluation, filledResult, secondPutTime); err != nil {
		t.Fatal(err)
	}
	assignmentTime := secondPutTime.Add(time.Minute)
	assignmentCommand := LifecycleCommand{EventID: "manual-lifecycle:assigned-1", EventType: Assignment, ExpectedStateVersion: 4, ConfirmPaperSimulation: true}
	assignment, err := store.RecordLifecycle(ctx, userID, instance.ID, assignmentCommand, assignmentTime)
	if err != nil || assignment.NewState != LongShares || assignment.StateVersion != 5 {
		t.Fatalf("paper assignment was not applied: %#v %v", assignment, err)
	}
	var shares string
	if err = pool.QueryRow(ctx, `SELECT quantity::text FROM paper_positions WHERE instrument='EQUITY' AND symbol='AAPL'`).Scan(&shares); err != nil || shares != "100.0000000000" {
		t.Fatalf("paper assignment did not create shares: %s %v", shares, err)
	}
	if err = pool.QueryRow(ctx, `SELECT cash::text FROM paper_portfolios WHERE strategy_instance_id=$1`, instance.ID).Scan(&cash); err != nil || cash != "1250.0000000000" {
		t.Fatalf("paper assignment cash is wrong: %s %v", cash, err)
	}

	sharesInstance, err := store.Get(ctx, userID, instance.ID)
	if err != nil {
		t.Fatal(err)
	}
	callTime := assignmentTime.Add(time.Minute)
	callAction := proposedCall(sharesInstance, "manual:call-filled", "action:call-filled")
	callEvaluation := risk.RiskEvaluation{ID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", UserID: userID, AccountID: accountID, MandateID: callAction.MandateID, MandateVersion: callAction.MandateVersion, Timestamp: callTime, Decision: risk.Allow, Checks: []risk.RiskCheck{}, ReasonCodes: []risk.ReasonCode{risk.Allowed}, Mode: "PAPER"}
	callDecision := Decision{ProposedAction: &callAction, ProposedState: CallProposed, CandidateCount: 1, Reason: "test", Rationale: []byte(`{"strategy":"wheel","candidate_count":1}`)}
	callPrice, callPremium := "2.0000000000", "200.0000000000"
	callResult := ExecutionResult{Status: SimulatedFilled, Price: &callPrice, Notional: &callPremium, ExpectedState: ShortCallOpen}
	if err = store.CommitEvaluation(ctx, sharesInstance, sharesInstance.StateVersion, callDecision, callEvaluation, callResult, callTime); err != nil {
		t.Fatal(err)
	}
	callAwayTime := callTime.Add(time.Minute)
	callAwayCommand := LifecycleCommand{EventID: "manual-lifecycle:called-away-1", EventType: CallAway, ExpectedStateVersion: 6, ConfirmPaperSimulation: true}
	callAway, err := store.RecordLifecycle(ctx, userID, instance.ID, callAwayCommand, callAwayTime)
	if err != nil || callAway.NewState != Cash || callAway.StateVersion != 7 {
		t.Fatalf("paper called-away event was not applied: %#v %v", callAway, err)
	}
	if err = pool.QueryRow(ctx, `SELECT quantity::text FROM paper_positions WHERE instrument='EQUITY' AND symbol='AAPL'`).Scan(&shares); err != nil || shares != "0.0000000000" {
		t.Fatalf("paper called-away event did not remove shares: %s %v", shares, err)
	}
	if err = pool.QueryRow(ctx, `SELECT cash::text FROM paper_portfolios WHERE strategy_instance_id=$1`, instance.ID).Scan(&cash); err != nil || cash != "21450.0000000000" {
		t.Fatalf("paper called-away cash is wrong: %s %v", cash, err)
	}
	assertCount(t, pool, `SELECT count(*) FROM strategy_lifecycle_events`, 3)
	assertCount(t, pool, `SELECT count(*) FROM strategy_state_transitions`, 7)
	assertCount(t, pool, `SELECT count(*) FROM decision_journal_entries`, 7)

	finished, err := store.Finish(ctx, userID, instance.ID, 7, callAwayTime.Add(time.Minute))
	if err != nil || finished.Status != "COMPLETED" || finished.StateVersion != 8 || finished.CompletedAt == nil {
		t.Fatalf("finishing did not release the account claim: %#v %v", finished, err)
	}
	secondInstance, err := store.Initialize(ctx, userID, secondMandate, "1000", ReadyForPut)
	if err != nil || secondInstance.CapitalBucketID != secondBucketID || secondInstance.FinancialAccountID != accountID {
		t.Fatalf("completed strategy did not release its financial account: %#v %v", secondInstance, err)
	}
	assertCount(t, pool, `SELECT count(*) FROM strategy_instances`, 2)
	assertCount(t, pool, `SELECT count(*) FROM strategy_state_transitions`, 9)
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

func proposedCall(instance Instance, eventID, actionID string) risk.ProposedAction {
	action := proposedOption(instance, eventID, actionID)
	action.Notional = "20000.0000000000"
	action.EstimatedPrice = ptrString("2.0000000000")
	action.Option.PutCall = "CALL"
	action.Option.Strike = "200"
	action.Option.Expiration = "2026-03-31"
	return action
}

func ptrString(value string) *string { return &value }

func assertCount(t *testing.T, pool *pgxpool.Pool, query string, expected int) {
	t.Helper()
	var count int
	if err := pool.QueryRow(context.Background(), query).Scan(&count); err != nil || count != expected {
		t.Fatalf("count mismatch for %q: %d %v", query, count, err)
	}
}
