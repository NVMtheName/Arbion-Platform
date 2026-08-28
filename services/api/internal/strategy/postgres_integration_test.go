package strategy

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/automation"
	"github.com/arbion/platform/services/api/internal/neural"
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
		aiConnectionID  = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
		aiAccountID     = "dddddddd-dddd-4ddd-8ddd-dddddddddddd"
		aiBucketID      = "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee"
		aiMandateID     = "ffffffff-ffff-4fff-8fff-ffffffffffff"
	)
	statements := []string{
		`INSERT INTO users(id,email,normalized_email,display_name,email_verified_at) VALUES('` + userID + `','test@example.com','test@example.com','Test',now())`,
		`INSERT INTO user_entitlements(user_id,entitlement_key,source,billing_required) VALUES('` + userID + `','founder','bootstrap',false)`,
		`INSERT INTO provider_connections(id,user_id,provider_category,provider_name,display_name,status) VALUES('` + connectionID + `','` + userID + `','financial','schwab','Schwab','active')`,
		`INSERT INTO financial_accounts(id,user_id,provider_connection_id,provider_name,provider_account_id,display_name,account_type,base_currency,status,capabilities) VALUES('` + accountID + `','` + userID + `','` + connectionID + `','schwab','opaque','Schwab Test','brokerage','USD','active','{"options":"SUPPORTED","margin":"UNKNOWN"}')`,
		`INSERT INTO capital_buckets(id,user_id,financial_account_id,name,allocation_type,allocation_value,currency,protected_amount,status) VALUES('` + bucketID + `','` + userID + `','` + accountID + `','Paper','FIXED_AMOUNT',20000,'USD',0,'ACTIVE')`,
		`INSERT INTO capital_buckets(id,user_id,financial_account_id,name,allocation_type,allocation_value,currency,protected_amount,status) VALUES('` + secondBucketID + `','` + userID + `','` + accountID + `','Second paper bucket','FIXED_AMOUNT',1000,'USD',0,'ACTIVE')`,
		`INSERT INTO automation_mandates(id,user_id,financial_account_id,automation_type,strategy_identifier,capital_bucket_id,autonomy_level,execution_mode,status,current_version,strategy_parameters,risk_parameters,allowed_universe,prohibited_universe,margin_allowed,options_allowed,schedule_conditions,capability_unverified) VALUES('` + mandateID + `','` + userID + `','` + accountID + `','STRATEGY','wheel','` + bucketID + `','STRATEGY_AUTONOMOUS','PAPER','READY',1,'{"symbols":["AAPL"],"minimum_dte":20,"maximum_dte":60,"target_delta":"0.30","target_delta_min":"0.20","target_delta_max":"0.40","maximum_contracts":1,"assignment_handling_policy":"continue_wheel"}','{}','{"symbols":["AAPL"],"universe_ids":[]}','{"symbols":[]}',false,true,'{"enabled":true,"interval_minutes":60,"session":"US_EQUITIES_REGULAR","notifications":{"evaluation_completed":true,"lifecycle_required":true,"first_failure":true}}',false)`,
		`INSERT INTO automation_mandate_versions(mandate_id,version_number,created_by_user_id,source,snapshot,change_summary) SELECT id,1,user_id,'UI',to_jsonb(m) || '{"execution_capable":false}'::jsonb,'{}'::jsonb FROM automation_mandates m WHERE id='` + mandateID + `'`,
		`INSERT INTO automation_mandates(id,user_id,financial_account_id,automation_type,strategy_identifier,capital_bucket_id,autonomy_level,execution_mode,status,current_version,strategy_parameters,risk_parameters,allowed_universe,prohibited_universe,margin_allowed,options_allowed,schedule_conditions,capability_unverified) VALUES('` + secondMandateID + `','` + userID + `','` + accountID + `','STRATEGY','wheel','` + secondBucketID + `','RESEARCH_ONLY','PAPER','READY',1,'{}','{}','{"symbols":[],"universe_ids":[]}','{"symbols":[]}',false,true,'{}',false)`,
		`INSERT INTO automation_mandate_versions(mandate_id,version_number,created_by_user_id,source,snapshot,change_summary) SELECT id,1,user_id,'UI',to_jsonb(m) || '{"execution_capable":false}'::jsonb,'{}'::jsonb FROM automation_mandates m WHERE id='` + secondMandateID + `'`,
		`INSERT INTO provider_connections(id,user_id,provider_category,provider_name,display_name,status) VALUES('` + aiConnectionID + `','` + userID + `','ai','openai','OpenAI','active')`,
		`INSERT INTO financial_accounts(id,user_id,provider_connection_id,provider_name,provider_account_id,display_name,account_type,base_currency,status,capabilities) VALUES('` + aiAccountID + `','` + userID + `','` + connectionID + `','schwab','opaque-ai','Schwab AI Test','brokerage','USD','active','{"options":"UNSUPPORTED","margin":"UNSUPPORTED"}')`,
		`INSERT INTO capital_buckets(id,user_id,financial_account_id,name,allocation_type,allocation_value,currency,protected_amount,status) VALUES('` + aiBucketID + `','` + userID + `','` + aiAccountID + `','AI shadow budget','FIXED_AMOUNT',10,'USD',0,'ACTIVE')`,
		`INSERT INTO automation_mandates(id,user_id,financial_account_id,automation_type,ai_provider_connection_id,ai_model_id,capital_bucket_id,autonomy_level,execution_mode,status,current_version,strategy_parameters,risk_parameters,allowed_universe,prohibited_universe,margin_allowed,options_allowed,schedule_conditions,capability_unverified) VALUES('` + aiMandateID + `','` + userID + `','` + aiAccountID + `','AI_AUTONOMOUS','` + aiConnectionID + `','gpt-5.6-sol','` + aiBucketID + `','FULL_AUTONOMOUS','SHADOW','READY',1,'{"objective":"Preserve capital.","max_proposal_notional":"1"}','{}','{"symbols":["AAPL"],"universe_ids":[]}','{"symbols":[]}',false,false,'{"enabled":true,"interval_minutes":60,"session":"US_EQUITIES_REGULAR","notifications":{"reconciliation_review_required":true}}',false)`,
		`INSERT INTO automation_mandate_versions(mandate_id,version_number,created_by_user_id,source,snapshot,change_summary) SELECT id,1,user_id,'UI',to_jsonb(m) || '{"execution_capable":false}'::jsonb,'{}'::jsonb FROM automation_mandates m WHERE id='` + aiMandateID + `'`,
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
	if _, err = pool.Exec(ctx, `UPDATE automation_mandates SET status='DRAFT',current_version=2,autonomy_level='RESEARCH_ONLY',schedule_conditions='{"enabled":false}' WHERE id=$1`, mandateID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO automation_mandate_versions(mandate_id,version_number,created_by_user_id,source,snapshot,change_summary) SELECT id,2,user_id,'UI',to_jsonb(m) || '{"execution_capable":false}'::jsonb,'{"change":"replacement_draft"}'::jsonb FROM automation_mandates m WHERE id=$1`, mandateID); err != nil {
		t.Fatal(err)
	}
	secondMandate := mandate
	secondMandate.ID = secondMandateID
	secondMandate.CapitalBucketID = secondBucketID
	secondMandate.ScheduleConditions = json.RawMessage(`{}`)
	if _, err = store.Initialize(ctx, userID, secondMandate, "1000", ReadyForPut); !errors.Is(err, ErrAccountInUse) {
		t.Fatalf("financial account was reused by a second active strategy: %v", err)
	}
	assertCount(t, pool, `SELECT count(*) FROM strategy_instances`, 1)
	earlyClaimAt := time.Now().UTC().Add(59 * time.Minute)
	if scheduled, claimErr := store.ClaimDueSchedule(ctx, earlyClaimAt, scheduleLeaseDuration); claimErr != nil || scheduled != nil {
		t.Fatalf("new schedule ran before its configured interval: %#v %v", scheduled, claimErr)
	}
	claimAt := time.Now().UTC().Add(61 * time.Minute).Truncate(time.Microsecond)
	scheduled, err := store.ClaimDueSchedule(ctx, claimAt, scheduleLeaseDuration)
	if err != nil || scheduled == nil {
		t.Fatalf("guarded schedule was not claimed: %#v %v", scheduled, err)
	}
	if scheduled.FinancialAccountID != accountID || scheduled.OwnerEmail != "test@example.com" || !scheduled.OwnerEmailVerified || !scheduled.NotifyEvaluation || !scheduled.NotifyLifecycle || !scheduled.NotifyFirstFailure || scheduled.PreviousErrorCode != nil || scheduled.ConsecutiveFailures != 0 {
		t.Fatalf("notification preferences crossed the schedule boundary: %#v", scheduled)
	}
	if !scheduled.StartedAt.Equal(claimAt) {
		t.Fatalf("schedule claim start time was not preserved: got=%s want=%s", scheduled.StartedAt, claimAt)
	}
	invalidCompletion := ScheduleCompletion{
		CompletedAt: claimAt,
		NextRunAt:   claimAt.Add(24 * time.Hour),
		Status:      "SUCCEEDED",
		AIDecision:  "INVALID",
	}
	if err = store.CompleteSchedule(ctx, *scheduled, invalidCompletion); err == nil {
		t.Fatal("invalid immutable schedule evidence advanced the current schedule")
	}
	var retainedLease string
	var retainedStatus *string
	var retainedNextRunAt time.Time
	if err = pool.QueryRow(ctx, `SELECT lease_token::text,last_status,next_run_at FROM nonlive_strategy_schedules WHERE strategy_instance_id=$1`, instance.ID).Scan(&retainedLease, &retainedStatus, &retainedNextRunAt); err != nil {
		t.Fatal(err)
	}
	if retainedLease != scheduled.LeaseToken || retainedStatus != nil || !retainedNextRunAt.Equal(scheduled.ScheduledFor) {
		t.Fatalf("failed history insert was not atomic: lease=%q status=%v next=%s", retainedLease, retainedStatus, retainedNextRunAt)
	}
	if err = store.CompleteSchedule(ctx, *scheduled, ScheduleCompletion{CompletedAt: claimAt, NextRunAt: claimAt.Add(24 * time.Hour), Status: "SKIPPED", ErrorCode: "OUTSIDE_SESSION"}); err != nil {
		t.Fatal(err)
	}
	scheduleRuns, err := store.ScheduleRuns(ctx, userID, instance.ID, 10, nil)
	if err != nil || len(scheduleRuns) != 1 || scheduleRuns[0].Status != "SKIPPED" || scheduleRuns[0].ErrorCode == nil || *scheduleRuns[0].ErrorCode != "OUTSIDE_SESSION" || scheduleRuns[0].ExecutionMode != Paper || scheduleRuns[0].ConsecutiveFailures != 0 {
		t.Fatalf("immutable schedule run was not recorded: %#v %v", scheduleRuns, err)
	}
	foreignScheduleRuns, err := store.ScheduleRuns(ctx, "99999999-9999-4999-8999-999999999999", instance.ID, 10, nil)
	if err != nil || len(foreignScheduleRuns) != 0 {
		t.Fatalf("schedule run history crossed its owner boundary: %#v %v", foreignScheduleRuns, err)
	}
	if _, err = pool.Exec(ctx, `UPDATE nonlive_schedule_runs SET status='SUCCEEDED' WHERE id=$1`, scheduleRuns[0].ID); err == nil {
		t.Fatal("immutable schedule run was updated")
	}
	if _, err = pool.Exec(ctx, `DELETE FROM nonlive_schedule_runs WHERE id=$1`, scheduleRuns[0].ID); err == nil {
		t.Fatal("immutable schedule run was deleted")
	}
	if _, err = pool.Exec(ctx, `UPDATE automation_mandates SET status='DISABLED' WHERE id=$1`, mandateID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE nonlive_strategy_schedules SET next_run_at=$1 WHERE strategy_instance_id=$2`, claimAt, instance.ID); err != nil {
		t.Fatal(err)
	}
	if blocked, claimErr := store.ClaimDueSchedule(ctx, claimAt.Add(time.Minute), scheduleLeaseDuration); claimErr != nil || blocked != nil {
		t.Fatalf("explicitly disabled mandate still produced scheduled work: %#v %v", blocked, claimErr)
	}
	if _, err = pool.Exec(ctx, `UPDATE automation_mandates SET status='DRAFT' WHERE id=$1`, mandateID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE nonlive_strategy_schedules SET next_run_at=$1 WHERE strategy_instance_id=$2`, claimAt.Add(24*time.Hour), instance.ID); err != nil {
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
	var status string
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
	if _, err = store.Finish(ctx, userID, instance.ID, 2, fillTime.Add(time.Second)); !errors.Is(err, ErrOpenExposure) {
		t.Fatalf("open PAPER exposure released the account claim: %v", err)
	}
	if err = pool.QueryRow(ctx, `SELECT status,state_version FROM strategy_instances WHERE id=$1`, instance.ID).Scan(&status, &version); err != nil || status != "ACTIVE" || version != 2 {
		t.Fatalf("rejected finish changed the strategy instance: %s v%d %v", status, version, err)
	}
	pausedWithExposure, err := store.Pause(ctx, userID, instance.ID, 2, fillTime.Add(2*time.Second))
	if err != nil || pausedWithExposure.Status != "PAUSED" || pausedWithExposure.StateVersion != 3 {
		t.Fatalf("open PAPER exposure could not be paused safely: %#v %v", pausedWithExposure, err)
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
	decisions, err := store.StrategyDecisionEntries(ctx, userID, instance.ID, 10, nil)
	if err != nil || len(decisions) != 2 || decisions[0].CreatedAt.IsZero() {
		t.Fatalf("per-instance journal timestamps are missing: %#v %v", decisions, err)
	}
	latestDecision := decisions[0]
	if latestDecision.RiskDecision == nil || *latestDecision.RiskDecision != string(risk.Allow) ||
		latestDecision.ExecutionStatus == nil || *latestDecision.ExecutionStatus != string(SimulatedFilled) ||
		latestDecision.Symbol == nil || *latestDecision.Symbol != "AAPL" ||
		latestDecision.Side == nil || *latestDecision.Side != "SELL_TO_OPEN" ||
		latestDecision.Quantity == nil || *latestDecision.Quantity != "1.0000000000" ||
		latestDecision.Price == nil || *latestDecision.Price != price ||
		latestDecision.Notional == nil || *latestDecision.Notional != premium {
		t.Fatalf("per-instance journal omitted linked risk or execution evidence: %#v", latestDecision)
	}
	transitionPage, err := store.StrategyTransitionEntries(ctx, userID, instance.ID, 2, nil)
	if err != nil || len(transitionPage) != 2 || transitionPage[0].StateVersion != 3 || transitionPage[1].StateVersion != 2 {
		t.Fatalf("bounded state history was not newest-first: %#v %v", transitionPage, err)
	}
	olderTransitions, err := store.StrategyTransitionEntries(ctx, userID, instance.ID, 2, &StrategyTransitionCursor{StateVersion: transitionPage[1].StateVersion, ID: transitionPage[1].ID})
	if err != nil || len(olderTransitions) != 1 || olderTransitions[0].StateVersion != 1 || olderTransitions[0].Trigger != "INITIALIZED" {
		t.Fatalf("state-history cursor was unstable: %#v %v", olderTransitions, err)
	}
	foreignTransitions, err := store.StrategyTransitionEntries(ctx, "99999999-9999-4999-8999-999999999999", instance.ID, 10, nil)
	if err != nil || len(foreignTransitions) != 0 {
		t.Fatalf("state history crossed its owner boundary: %#v %v", foreignTransitions, err)
	}
	executionPage, err := store.StrategyExecutionEntries(ctx, userID, instance.ID, 1, nil)
	if err != nil || len(executionPage) != 1 || executionPage[0].Status != SimulatedFilled || !executionPage[0].CreatedAt.Equal(fillTime) || executionPage[0].Price == nil || *executionPage[0].Price != price {
		t.Fatalf("bounded execution evidence was incomplete: %#v %v", executionPage, err)
	}
	olderExecutions, err := store.StrategyExecutionEntries(ctx, userID, instance.ID, 1, &StrategyExecutionCursor{CreatedAt: executionPage[0].CreatedAt, ID: executionPage[0].ID})
	if err != nil || len(olderExecutions) != 1 || olderExecutions[0].Status != RiskDenied || !olderExecutions[0].CreatedAt.Equal(now) {
		t.Fatalf("execution-history cursor was unstable: %#v %v", olderExecutions, err)
	}
	foreignExecutions, err := store.StrategyExecutionEntries(ctx, "99999999-9999-4999-8999-999999999999", instance.ID, 10, nil)
	if err != nil || len(foreignExecutions) != 0 {
		t.Fatalf("execution history crossed its owner boundary: %#v %v", foreignExecutions, err)
	}
	assertCount(t, pool, `SELECT count(*) FROM strategy_state_transitions`, 3)
	facts, err := store.EvaluationFacts(ctx, Instance{ID: instance.ID, UserID: userID, FinancialAccountID: accountID, AutomationMandateID: mandateID, ExecutionMode: Paper}, fillTime)
	if err != nil || facts.Paper == nil || facts.Paper.CurrentExposure != "19000.0000000000" || facts.ActionsToday != 2 {
		t.Fatalf("paper evaluation facts are incomplete: %#v %v", facts, err)
	}

	lifecycleTime := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)
	lifecycleCommand := LifecycleCommand{EventID: "manual-lifecycle:expired-1", EventType: ExpireWorthless, ExpectedStateVersion: 3, ConfirmPaperSimulation: true}
	lifecycle, err := store.RecordLifecycle(ctx, userID, instance.ID, lifecycleCommand, lifecycleTime)
	if err != nil || lifecycle.Duplicate || lifecycle.PreviousState != ShortPutOpen || lifecycle.NewState != ReadyForPut || lifecycle.StateVersion != 4 {
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
	if err = pool.QueryRow(ctx, `SELECT current_state,state_version,status FROM strategy_instances WHERE id=$1`, instance.ID).Scan(&state, &version, &status); err != nil || state != ReadyForPut || version != 4 || status != "PAUSED" {
		t.Fatalf("paper expiration did not advance state once: %s v%d %v", state, version, err)
	}
	resumedAfterLifecycle, err := store.Resume(ctx, userID, instance.ID, 4, lifecycleTime.Add(time.Second))
	if err != nil || resumedAfterLifecycle.Status != "ACTIVE" || resumedAfterLifecycle.StateVersion != 5 {
		t.Fatalf("resolved PAPER lifecycle did not resume under its ready mandate: %#v %v", resumedAfterLifecycle, err)
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
	assertCount(t, pool, `SELECT count(*) FROM strategy_state_transitions`, 5)
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
	assignmentCommand := LifecycleCommand{EventID: "manual-lifecycle:assigned-1", EventType: Assignment, ExpectedStateVersion: 6, ConfirmPaperSimulation: true}
	assignment, err := store.RecordLifecycle(ctx, userID, instance.ID, assignmentCommand, assignmentTime)
	if err != nil || assignment.NewState != LongShares || assignment.StateVersion != 7 {
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
	callAwayCommand := LifecycleCommand{EventID: "manual-lifecycle:called-away-1", EventType: CallAway, ExpectedStateVersion: 8, ConfirmPaperSimulation: true}
	callAway, err := store.RecordLifecycle(ctx, userID, instance.ID, callAwayCommand, callAwayTime)
	if err != nil || callAway.NewState != Cash || callAway.StateVersion != 9 {
		t.Fatalf("paper called-away event was not applied: %#v %v", callAway, err)
	}
	if err = pool.QueryRow(ctx, `SELECT quantity::text FROM paper_positions WHERE instrument='EQUITY' AND symbol='AAPL'`).Scan(&shares); err != nil || shares != "0.0000000000" {
		t.Fatalf("paper called-away event did not remove shares: %s %v", shares, err)
	}
	if err = pool.QueryRow(ctx, `SELECT cash::text FROM paper_portfolios WHERE strategy_instance_id=$1`, instance.ID).Scan(&cash); err != nil || cash != "21450.0000000000" {
		t.Fatalf("paper called-away cash is wrong: %s %v", cash, err)
	}
	assertCount(t, pool, `SELECT count(*) FROM strategy_lifecycle_events`, 3)
	assertCount(t, pool, `SELECT count(*) FROM strategy_state_transitions`, 9)
	assertCount(t, pool, `SELECT count(*) FROM decision_journal_entries`, 7)

	finishedAt := callAwayTime.Add(time.Minute)
	if !finishedAt.After(instance.StartedAt) {
		finishedAt = instance.StartedAt.Add(time.Minute)
	}
	finished, err := store.Finish(ctx, userID, instance.ID, 9, finishedAt)
	if err != nil || finished.Status != "COMPLETED" || finished.StateVersion != 10 || finished.CompletedAt == nil {
		t.Fatalf("finishing did not release the account claim: %#v %v", finished, err)
	}
	secondInstance, err := store.Initialize(ctx, userID, secondMandate, "1000", ReadyForPut)
	if err != nil || secondInstance.CapitalBucketID != secondBucketID || secondInstance.FinancialAccountID != accountID {
		t.Fatalf("completed strategy did not release its financial account: %#v %v", secondInstance, err)
	}
	pauseTime := finishedAt.Add(time.Minute)
	paused, err := store.Pause(ctx, userID, secondInstance.ID, 1, pauseTime)
	if err != nil || paused.Status != "PAUSED" || paused.StateVersion != 2 || paused.PausedAt == nil {
		t.Fatalf("non-live pause did not preserve the account claim: %#v %v", paused, err)
	}
	if _, err = store.Initialize(ctx, userID, mandate, "20000", ReadyForPut); !errors.Is(err, ErrMandateStale) {
		t.Fatalf("stale mandate initialized after its immutable version was replaced: %v", err)
	}
	if _, err = pool.Exec(ctx, `UPDATE automation_mandates SET status='PAUSED' WHERE id=$1`, secondMandateID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Resume(ctx, userID, secondInstance.ID, 2, pauseTime.Add(time.Minute)); !errors.Is(err, ErrMandateStale) {
		t.Fatalf("paused strategy resumed under a non-ready mandate: %v", err)
	}
	if err = pool.QueryRow(ctx, `SELECT status,state_version FROM strategy_instances WHERE id=$1`, secondInstance.ID).Scan(&status, &version); err != nil || status != "PAUSED" || version != 2 {
		t.Fatalf("rejected resume changed the strategy instance: %s v%d %v", status, version, err)
	}
	if _, err = pool.Exec(ctx, `UPDATE automation_mandates SET status='READY' WHERE id=$1`, secondMandateID); err != nil {
		t.Fatal(err)
	}
	resumed, err := store.Resume(ctx, userID, secondInstance.ID, 2, pauseTime.Add(2*time.Minute))
	if err != nil || resumed.Status != "ACTIVE" || resumed.StateVersion != 3 || resumed.PausedAt != nil {
		t.Fatalf("exact ready mandate did not resume safely: %#v %v", resumed, err)
	}
	assertCount(t, pool, `SELECT count(*) FROM strategy_instances`, 2)
	assertCount(t, pool, `SELECT count(*) FROM strategy_state_transitions`, 13)

	aiConnection, aiModel := aiConnectionID, "gpt-5.6-sol"
	aiMandate := automation.Mandate{
		ID: aiMandateID, UserID: userID, FinancialAccountID: aiAccountID,
		AutomationType: "AI_AUTONOMOUS", AIProviderConnectionID: &aiConnection,
		AIModelID: &aiModel, CapitalBucketID: aiBucketID, AutonomyLevel: "FULL_AUTONOMOUS",
		ExecutionMode: "SHADOW", Status: "READY", CurrentVersion: 1,
		ScheduleConditions: json.RawMessage(`{"enabled":true,"interval_minutes":60,"session":"US_EQUITIES_REGULAR","notifications":{"reconciliation_review_required":true}}`),
	}
	if _, err = pool.Exec(ctx, `UPDATE provider_connections SET status='disabled' WHERE id=$1`, aiConnectionID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Initialize(ctx, userID, aiMandate, "0", AIMonitoring); !errors.Is(err, ErrMandateStale) {
		t.Fatalf("AI runtime initialized with an inactive provider connection: %v", err)
	}
	if _, err = pool.Exec(ctx, `UPDATE provider_connections SET status='active' WHERE id=$1`, aiConnectionID); err != nil {
		t.Fatal(err)
	}
	aiInstance, err := store.Initialize(ctx, userID, aiMandate, "0", AIMonitoring)
	if err != nil || aiInstance.StrategyIdentifier != "ai_shadow" || aiInstance.ExecutionMode != Shadow {
		t.Fatalf("AI shadow instance was not initialized safely: %#v %v", aiInstance, err)
	}
	aiEvaluationTime := time.Date(2026, 8, 25, 18, 0, 0, 0, time.UTC)
	if _, err = pool.Exec(ctx, `INSERT INTO portfolio_reconciliations(user_id,financial_account_id,provider_name,comparison_status,balances_status,positions_status,performance_status,realized_performance_status,autonomy_signal,autonomy_enforcement_active,blocks_new_actions,observed_position_count,performance_position_count,change_count,changes,evidence_hash,observed_at) VALUES($1,$2,'schwab','MATCHED','READY','READY','UNAVAILABLE','UNAVAILABLE','CLEAR',true,false,0,0,0,'[]',decode(repeat('ab',32),'hex'),$3)`, userID, aiAccountID, aiEvaluationTime.Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	aiActionID := "ai-shadow:integration:proposal"
	aiEventID := "scheduled:ai-shadow-integration"
	aiState := string(aiInstance.CurrentState)
	aiPrice, aiNotional := "2.0000000000", "1.0000000000"
	aiAction := risk.ProposedAction{
		ID: aiActionID, CorrelationID: aiEventID, FinancialAccountID: aiAccountID,
		Source: risk.SourceAI, ActionType: risk.ActionSell, MandateID: &aiInstance.AutomationMandateID,
		MandateVersion: &aiInstance.MandateVersion, Instrument: "AAPL", Side: "SELL",
		Quantity: "0.5000000000", Notional: aiNotional, EstimatedPrice: &aiPrice,
		StrategyInstanceID: &aiInstance.ID, StrategyState: &aiState, CreatedAt: aiEvaluationTime,
	}
	aiRisk := risk.RiskEvaluation{
		ID: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", UserID: userID, AccountID: aiAccountID,
		MandateID: aiAction.MandateID, MandateVersion: aiAction.MandateVersion,
		Timestamp: aiEvaluationTime, Decision: risk.Allow, Checks: []risk.RiskCheck{},
		ReasonCodes: []risk.ReasonCode{risk.Allowed}, Mode: "SHADOW",
	}
	aiDecision := Decision{
		ProposedAction: &aiAction, Source: "AI", InstrumentType: "EQUITY",
		ProposedState: AIMonitoring, CandidateCount: 1, Reason: "integration",
		Rationale: []byte(`{"decision":"PROPOSE","symbol":"AAPL","side":"SELL","ai_provider":"openai","model_id":"gpt-5.6-sol","profile":"deep","input_usage":30,"output_usage":45,"latency_ms":120}`),
	}
	aiResult := ExecutionResult{
		Status: WouldHaveSubmitted, Price: &aiPrice, Notional: &aiNotional,
		ExpectedState: AIMonitoring, Reason: "shadow_only_no_order_was_sent",
	}
	if err = store.CommitEvaluation(ctx, aiInstance, aiInstance.StateVersion, aiDecision, aiRisk, aiResult, aiEvaluationTime); err != nil {
		t.Fatal(err)
	}
	aiFacts, err := store.EvaluationFacts(ctx, aiInstance, aiEvaluationTime.Add(30*time.Minute))
	if err != nil || len(aiFacts.RecentActions) != 1 || aiFacts.RecentActions[0].Instrument != "AAPL" || aiFacts.RecentActions[0].Side != "SELL" || !aiFacts.RecentActions[0].OccurredAt.Equal(aiEvaluationTime) {
		t.Fatalf("AI repeat-action evidence was not reconstructed safely: %#v %v", aiFacts.RecentActions, err)
	}
	if aiFacts.Reconciliation == nil || aiFacts.Reconciliation.AccountID != aiAccountID || aiFacts.Reconciliation.ComparisonStatus != "MATCHED" || !aiFacts.Reconciliation.AutonomyEnforcementActive || aiFacts.Reconciliation.BlocksNewActions {
		t.Fatalf("AI reconciliation evidence was not reconstructed safely: %#v", aiFacts.Reconciliation)
	}
	if len(aiFacts.RecentDecisions) != 1 || aiFacts.RecentDecisions[0].Decision != "PROPOSE" || aiFacts.RecentDecisions[0].Symbol != "AAPL" || aiFacts.RecentDecisions[0].Side != "SELL" || aiFacts.RecentDecisions[0].Disposition != "WOULD_HAVE_SUBMITTED" || !aiFacts.RecentDecisions[0].OccurredAt.Equal(aiEvaluationTime) {
		t.Fatalf("AI decision memory was not reconstructed safely: %#v", aiFacts.RecentDecisions)
	}
	aiFacts, err = store.EvaluationFacts(ctx, aiInstance, aiEvaluationTime.Add(risk.AIRepeatActionCooldown+time.Second))
	if err != nil || len(aiFacts.RecentActions) != 0 {
		t.Fatalf("expired AI repeat-action evidence remained active: %#v %v", aiFacts.RecentActions, err)
	}
	markTime := aiEvaluationTime.Add(2 * time.Hour)
	due, err := store.DueShadowOutcomes(ctx, aiInstance, markTime)
	if err != nil || len(due) != 1 || due[0].Horizon != ShadowOutcomeOneHour {
		t.Fatalf("one-hour AI shadow mark was not due exactly once: %#v %v", due, err)
	}
	mark, err := buildAIShadowOutcome(due[0], neural.ShadowMarketFact{
		Symbol: "AAPL", Bid: "1.9000000000", Ask: "2.1000000000", Mark: "2.0000000000",
		Last: "2.0000000000", Feed: "schwab_market_data", Quality: "BROKER_REALTIME", ObservedAt: markTime,
	}, markTime)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.RecordShadowOutcome(ctx, aiInstance, mark); err != nil {
		t.Fatal(err)
	}
	if err = store.RecordShadowOutcome(ctx, aiInstance, mark); err != nil {
		t.Fatalf("AI shadow outcome retry was not idempotent: %v", err)
	}
	conflictingMark := mark
	conflictingMark.ObservedPrice = "2.2000000000"
	conflictingMark.DirectionalChangeUSD = "-0.1000000000"
	conflictingMark.DirectionalChangePercent = "-10.0000000000"
	if err = store.RecordShadowOutcome(ctx, aiInstance, conflictingMark); !errors.Is(err, ErrConflict) {
		t.Fatalf("conflicting AI shadow outcome retry was not rejected: %v", err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO decision_journal_entries(user_id,financial_account_id,mandate_id,mandate_version,strategy_instance_id,strategy_state,source,decision_type,structured_rationale,resulting_state,created_at) VALUES($1,$2,$3,$4,$5,$6,'AI','ABSTAIN',$7,$6,$8)`, userID, aiAccountID, aiMandateID, 1, aiInstance.ID, AIMonitoring, json.RawMessage(`{"decision":"ABSTAIN","ai_provider":"anthropic"}`), aiEvaluationTime.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	marks, err := store.ShadowOutcomes(ctx, userID, aiInstance.ID)
	if err != nil || len(marks) != 1 || marks[0].PricingBasis != "ASK_TO_CLOSE" || marks[0].DirectionalChangeUSD != "-0.0500000000" || marks[0].DirectionalChangePercent != "-5.0000000000" {
		t.Fatalf("AI shadow outcome projection was incomplete: %#v %v", marks, err)
	}
	foreignMarks, err := store.ShadowOutcomes(ctx, "99999999-9999-4999-8999-999999999999", aiInstance.ID)
	if err != nil || len(foreignMarks) != 0 {
		t.Fatalf("AI shadow outcomes crossed their owner boundary: %#v %v", foreignMarks, err)
	}
	newestAIDecisions, err := store.StrategyDecisionEntries(ctx, userID, aiInstance.ID, 1, nil)
	if err != nil || len(newestAIDecisions) != 1 || newestAIDecisions[0].DecisionType != "ABSTAIN" || newestAIDecisions[0].ExecutionRecordID != nil {
		t.Fatalf("newest bounded AI decision page was incorrect: %#v %v", newestAIDecisions, err)
	}
	olderAIDecisions, err := store.StrategyDecisionEntries(ctx, userID, aiInstance.ID, 1, &StrategyDecisionCursor{CreatedAt: newestAIDecisions[0].CreatedAt, ID: newestAIDecisions[0].ID})
	if err != nil || len(olderAIDecisions) != 1 || olderAIDecisions[0].DecisionType != "ALLOW_WOULD_HAVE_SUBMITTED" || olderAIDecisions[0].ExecutionRecordID == nil {
		t.Fatalf("older bounded AI decision page was unstable: %#v %v", olderAIDecisions, err)
	}
	matchedMarks, err := store.ShadowOutcomesForExecutions(ctx, userID, aiInstance.ID, []string{*olderAIDecisions[0].ExecutionRecordID})
	if err != nil || len(matchedMarks) != 1 || matchedMarks[0].ID != marks[0].ID {
		t.Fatalf("decision-page outcome evidence was not matched exactly: %#v %v", matchedMarks, err)
	}
	foreignDecisionPage, err := store.StrategyDecisionEntries(ctx, "99999999-9999-4999-8999-999999999999", aiInstance.ID, 10, nil)
	if err != nil || len(foreignDecisionPage) != 0 {
		t.Fatalf("bounded AI decision history crossed its owner boundary: %#v %v", foreignDecisionPage, err)
	}
	foreignMatchedMarks, err := store.ShadowOutcomesForExecutions(ctx, "99999999-9999-4999-8999-999999999999", aiInstance.ID, []string{*olderAIDecisions[0].ExecutionRecordID})
	if err != nil || len(foreignMatchedMarks) != 0 {
		t.Fatalf("matched outcome evidence crossed its owner boundary: %#v %v", foreignMatchedMarks, err)
	}
	scorecard, err := store.ShadowScorecard(ctx, userID, aiInstance.ID)
	if err != nil || scorecard.TotalMarks != 1 || len(scorecard.Horizons) != 2 {
		t.Fatalf("AI shadow scorecard was incomplete: %#v %v", scorecard, err)
	}
	oneHourScore, twentyFourHourScore := scorecard.Horizons[0], scorecard.Horizons[1]
	if oneHourScore.Horizon != ShadowOutcomeOneHour || oneHourScore.SampleSize != 1 || oneHourScore.FavorableMarks != 0 || oneHourScore.UnfavorableMarks != 1 || oneHourScore.FlatMarks != 0 || oneHourScore.FavorableRatePercent == nil || *oneHourScore.FavorableRatePercent != "0.0000000000" || oneHourScore.AverageDirectionalChangePercent == nil || *oneHourScore.AverageDirectionalChangePercent != "-5.0000000000" || oneHourScore.MedianDirectionalChangePercent == nil || *oneHourScore.MedianDirectionalChangePercent != "-5.0000000000" || oneHourScore.BestDirectionalChangePercent == nil || *oneHourScore.BestDirectionalChangePercent != "-5.0000000000" || oneHourScore.WorstDirectionalChangePercent == nil || *oneHourScore.WorstDirectionalChangePercent != "-5.0000000000" || oneHourScore.AverageDirectionalChangeUSD == nil || *oneHourScore.AverageDirectionalChangeUSD != "-0.0500000000" || oneHourScore.CumulativeDirectionalChangeUSD == nil || *oneHourScore.CumulativeDirectionalChangeUSD != "-0.0500000000" || oneHourScore.Interpretation != "INSUFFICIENT_SAMPLE" || oneHourScore.MinimumSampleForObservationalLabel != ShadowScorecardMinimumSample || oneHourScore.FirstEvaluatedAt == nil || oneHourScore.LastEvaluatedAt == nil {
		t.Fatalf("one-hour AI shadow score was incorrect: %#v", oneHourScore)
	}
	if twentyFourHourScore.Horizon != ShadowOutcomeTwentyFourHours || twentyFourHourScore.SampleSize != 0 || twentyFourHourScore.FavorableRatePercent != nil || twentyFourHourScore.AverageDirectionalChangePercent != nil || twentyFourHourScore.MedianDirectionalChangePercent != nil || twentyFourHourScore.BestDirectionalChangePercent != nil || twentyFourHourScore.WorstDirectionalChangePercent != nil || twentyFourHourScore.AverageDirectionalChangeUSD != nil || twentyFourHourScore.CumulativeDirectionalChangeUSD != nil || twentyFourHourScore.Interpretation != "INSUFFICIENT_SAMPLE" {
		t.Fatalf("pending 24-hour AI shadow score was incorrect: %#v", twentyFourHourScore)
	}
	gate := scorecard.EvidenceGate
	if gate.Status != ShadowEvidenceCollecting || gate.OneHourSampleSize != 1 || gate.TwentyFourHourSampleSize != 0 || gate.EvidenceWindowHours != 0 || gate.ScheduleHealthy || gate.LiveExecutionAvailable || len(gate.Blockers) != 4 || gate.Blockers[3] != ShadowEvidenceScheduleNotVerified {
		t.Fatalf("AI shadow evidence gate was not conservative: %#v", gate)
	}
	behavior := scorecard.Behavior
	if behavior.TotalAIDecisions != 2 || behavior.Abstentions != 1 || behavior.ProposedDecisions != 1 || behavior.RiskHeldDecisions != 0 || behavior.RepeatActionCooldownHolds != 0 || behavior.WouldHaveSubmittedDecisions != 1 || behavior.AttributedDecisions != 1 || behavior.UnattributedLegacyDecisions != 1 || behavior.AbstentionRatePercent == nil || *behavior.AbstentionRatePercent != "50.0000000000" || behavior.ProposalRatePercent == nil || *behavior.ProposalRatePercent != "50.0000000000" || behavior.AverageDecisionIntervalMins == nil || *behavior.AverageDecisionIntervalMins != "1.00" || behavior.FirstDecisionAt == nil || behavior.LastDecisionAt == nil || len(behavior.Routes) != 2 || len(behavior.Symbols) != 1 {
		t.Fatalf("AI shadow behavior summary was incomplete: %#v", behavior)
	}
	var explicitRoute, legacyRoute *ShadowRouteBehavior
	for index := range behavior.Routes {
		route := &behavior.Routes[index]
		if route.ProvenanceStatus == ShadowRouteProvenanceExplicit {
			explicitRoute = route
		} else if route.ProvenanceStatus == ShadowRouteProvenanceLegacy {
			legacyRoute = route
		}
	}
	if explicitRoute == nil || explicitRoute.AIProvider != "openai" || explicitRoute.ModelID != "gpt-5.6-sol" || explicitRoute.Profile != "deep" || explicitRoute.TotalDecisions != 1 || explicitRoute.ProposedDecisions != 1 || explicitRoute.WouldHaveSubmittedDecisions != 1 || explicitRoute.OneHourOutcomeMarks != 1 || explicitRoute.TwentyFourHourOutcomeMarks != 0 || explicitRoute.MeasuredLatencyDecisions != 1 || explicitRoute.AverageLatencyMilliseconds == nil || *explicitRoute.AverageLatencyMilliseconds != "120.00" || explicitRoute.MeteredUsageDecisions != 1 || explicitRoute.RecordedInputTokens != 30 || explicitRoute.RecordedOutputTokens != 45 {
		t.Fatalf("explicit AI route behavior was incorrect: %#v", explicitRoute)
	}
	if legacyRoute == nil || legacyRoute.AIProvider != "" || legacyRoute.ModelID != "" || legacyRoute.Profile != "" || legacyRoute.TotalDecisions != 1 || legacyRoute.Abstentions != 1 || legacyRoute.MeasuredLatencyDecisions != 0 || legacyRoute.AverageLatencyMilliseconds != nil {
		t.Fatalf("legacy AI route provenance was inferred: %#v", legacyRoute)
	}
	symbolBehavior := behavior.Symbols[0]
	if symbolBehavior.Symbol != "AAPL" || symbolBehavior.ProposedDecisions != 1 || symbolBehavior.RiskHeldDecisions != 0 || symbolBehavior.WouldHaveSubmittedDecisions != 1 || symbolBehavior.OneHourOutcomeMarks != 1 || symbolBehavior.TwentyFourHourOutcomeMarks != 0 {
		t.Fatalf("AI symbol behavior was incorrect: %#v", symbolBehavior)
	}
	if _, err = store.ShadowScorecard(ctx, "99999999-9999-4999-8999-999999999999", aiInstance.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("AI shadow scorecard crossed its owner boundary: %v", err)
	}
	if _, err = pool.Exec(ctx, `UPDATE shadow_execution_outcomes SET observed_price=3 WHERE id=$1`, marks[0].ID); err == nil {
		t.Fatal("immutable AI shadow outcome was updated")
	}
	if _, err = pool.Exec(ctx, `DELETE FROM shadow_execution_outcomes WHERE id=$1`, marks[0].ID); err == nil {
		t.Fatal("immutable AI shadow outcome was deleted")
	}
	due, err = store.DueShadowOutcomes(ctx, aiInstance, aiEvaluationTime.Add(25*time.Hour))
	if err != nil || len(due) != 1 || due[0].Horizon != ShadowOutcomeTwentyFourHours {
		t.Fatalf("24-hour AI shadow mark was not independently due: %#v %v", due, err)
	}
	aiClaimAt := time.Now().UTC().Add(62 * time.Minute).Truncate(time.Microsecond)
	aiScheduled, err := store.ClaimDueSchedule(ctx, aiClaimAt, scheduleLeaseDuration)
	if err != nil || aiScheduled == nil || aiScheduled.StrategyInstanceID != aiInstance.ID || !aiScheduled.NotifyReconciliationReview || aiScheduled.LastReconciliationNotificationID != nil {
		t.Fatalf("AI drift-review preference did not cross the durable claim boundary: %#v %v", aiScheduled, err)
	}
	if err = store.CompleteSchedule(ctx, *aiScheduled, ScheduleCompletion{CompletedAt: aiClaimAt, NextRunAt: aiClaimAt.Add(time.Hour), Status: "SUCCEEDED", AIDecision: "ABSTAIN", ExecutionStatus: ExecutionCanceled}); err != nil {
		t.Fatal(err)
	}
	aiScheduleRuns, err := store.ScheduleRuns(ctx, userID, aiInstance.ID, 10, nil)
	if err != nil || len(aiScheduleRuns) != 1 || aiScheduleRuns[0].AIDecision == nil || *aiScheduleRuns[0].AIDecision != "ABSTAIN" || aiScheduleRuns[0].ExecutionStatus == nil || *aiScheduleRuns[0].ExecutionStatus != string(ExecutionCanceled) || aiScheduleRuns[0].ExecutionMode != Shadow {
		t.Fatalf("AI schedule disposition was not recorded safely: %#v %v", aiScheduleRuns, err)
	}
	var driftID string
	if err = pool.QueryRow(ctx, `INSERT INTO portfolio_reconciliations(user_id,financial_account_id,provider_name,comparison_status,balances_status,positions_status,performance_status,realized_performance_status,autonomy_signal,autonomy_enforcement_active,blocks_new_actions,observed_position_count,performance_position_count,change_count,blocking_change_count,changes,evidence_hash,observed_at) VALUES($1,$2,'schwab','DRIFT_DETECTED','READY','READY','UNAVAILABLE','UNAVAILABLE','REVIEW_RECOMMENDED',true,true,0,0,1,1,'[{"symbol":"SPY","instrument_type":"EQUITY","direction":"long","change_type":"POSITION_APPEARED","control_impact":"TRADABLE_INVENTORY","current_quantity":"1"}]',decode(repeat('cd',32),'hex'),$3) RETURNING id::text`, userID, aiAccountID, markTime).Scan(&driftID); err != nil {
		t.Fatal(err)
	}
	deliveredAt := aiClaimAt.Add(time.Minute).Truncate(time.Microsecond)
	run := ScheduledRun{StrategyInstanceID: aiInstance.ID, UserID: userID, FinancialAccountID: aiAccountID}
	if err = store.RecordReconciliationNotification(ctx, run, driftID, deliveredAt); err != nil {
		t.Fatal(err)
	}
	if err = store.RecordReconciliationNotification(ctx, run, driftID, deliveredAt.Add(time.Hour)); err != nil {
		t.Fatalf("same immutable drift marker was not idempotent: %v", err)
	}
	var storedDriftID string
	var storedDeliveredAt time.Time
	if err = pool.QueryRow(ctx, `SELECT last_reconciliation_notification_id::text,last_reconciliation_notification_at FROM nonlive_strategy_schedules WHERE strategy_instance_id=$1`, aiInstance.ID).Scan(&storedDriftID, &storedDeliveredAt); err != nil || storedDriftID != driftID || !storedDeliveredAt.Equal(deliveredAt) {
		t.Fatalf("drift delivery marker was not durable and stable: id=%q at=%s err=%v", storedDriftID, storedDeliveredAt, err)
	}
	var foreignDriftID string
	if err = pool.QueryRow(ctx, `INSERT INTO portfolio_reconciliations(user_id,financial_account_id,provider_name,comparison_status,balances_status,positions_status,performance_status,realized_performance_status,autonomy_signal,autonomy_enforcement_active,blocks_new_actions,observed_position_count,performance_position_count,change_count,blocking_change_count,changes,evidence_hash,observed_at) VALUES($1,$2,'schwab','DRIFT_DETECTED','READY','READY','UNAVAILABLE','UNAVAILABLE','REVIEW_RECOMMENDED',true,true,0,0,1,1,'[{"symbol":"AAPL","instrument_type":"EQUITY","direction":"long","change_type":"POSITION_APPEARED","control_impact":"TRADABLE_INVENTORY","current_quantity":"1"}]',decode(repeat('ef',32),'hex'),$3) RETURNING id::text`, userID, accountID, markTime).Scan(&foreignDriftID); err != nil {
		t.Fatal(err)
	}
	if err = store.RecordReconciliationNotification(ctx, run, foreignDriftID, deliveredAt.Add(2*time.Hour)); err == nil {
		t.Fatal("cross-account drift evidence was accepted as a notification marker")
	}
	reviewedAt := aiClaimAt.Add(3 * time.Hour).Truncate(time.Microsecond)
	review, err := store.CreateShadowEvidenceReview(ctx, userID, ShadowEvidenceReview{
		StrategyInstanceID:          aiInstance.ID,
		MandateID:                   aiMandateID,
		MandateVersion:              1,
		EvidenceFingerprint:         strings.Repeat("ab", 32),
		GateStatus:                  ShadowEvidenceReviewable,
		OneHourSampleSize:           20,
		TwentyFourHourSampleSize:    20,
		EvidenceWindowHours:         168,
		ScheduleHealthy:             true,
		LastScheduleStatus:          "SUCCEEDED",
		ConsecutiveScheduleFailures: 0,
		ExecutionBoundary:           ShadowExecutionBoundary,
		LiveExecutionAvailable:      false,
		ReviewScope:                 ShadowEvidenceReviewScope,
		MFAMethod:                   "totp",
		ReviewedAt:                  reviewedAt,
	})
	if err != nil || review.ID == "" || review.EvidenceFingerprint != strings.Repeat("ab", 32) || !review.ReviewedAt.Equal(reviewedAt) || review.LiveExecutionAvailable {
		t.Fatalf("Shadow evidence review was not persisted safely: %#v %v", review, err)
	}
	latestReview, err := store.LatestShadowEvidenceReview(ctx, userID, aiInstance.ID)
	if err != nil || latestReview == nil || latestReview.ID != review.ID || latestReview.ReviewScope != ShadowEvidenceReviewScope {
		t.Fatalf("owner Shadow evidence review was not projected safely: %#v %v", latestReview, err)
	}
	foreignReview, err := store.LatestShadowEvidenceReview(ctx, "99999999-9999-4999-8999-999999999999", aiInstance.ID)
	if err != nil || foreignReview != nil {
		t.Fatalf("Shadow evidence review crossed its owner boundary: %#v %v", foreignReview, err)
	}
	newerReview := review
	newerReview.ID = ""
	newerReview.EvidenceFingerprint = strings.Repeat("ef", 32)
	newerReview.ReviewedAt = reviewedAt.Add(time.Hour)
	newerReview.CreatedAt = time.Time{}
	newerReview, err = store.CreateShadowEvidenceReview(ctx, userID, newerReview)
	if err != nil {
		t.Fatal(err)
	}
	firstReviewPage, err := store.ShadowEvidenceReviews(ctx, userID, aiInstance.ID, 1, nil)
	if err != nil || len(firstReviewPage) != 1 || firstReviewPage[0].ID != newerReview.ID {
		t.Fatalf("newest Shadow review page was unstable: %#v %v", firstReviewPage, err)
	}
	secondReviewPage, err := store.ShadowEvidenceReviews(ctx, userID, aiInstance.ID, 1, &ShadowEvidenceReviewCursor{ReviewedAt: firstReviewPage[0].ReviewedAt, ID: firstReviewPage[0].ID})
	if err != nil || len(secondReviewPage) != 1 || secondReviewPage[0].ID != review.ID {
		t.Fatalf("older Shadow review page was unstable: %#v %v", secondReviewPage, err)
	}
	foreignReviewPage, err := store.ShadowEvidenceReviews(ctx, "99999999-9999-4999-8999-999999999999", aiInstance.ID, 10, nil)
	if err != nil || len(foreignReviewPage) != 0 {
		t.Fatalf("Shadow review ledger crossed its owner boundary: %#v %v", foreignReviewPage, err)
	}
	invalidReview := review
	invalidReview.ID = ""
	invalidReview.MandateID = secondMandateID
	invalidReview.EvidenceFingerprint = strings.Repeat("cd", 32)
	if _, err = store.CreateShadowEvidenceReview(ctx, userID, invalidReview); err == nil {
		t.Fatal("cross-mandate Shadow evidence review was accepted")
	}
	if _, err = pool.Exec(ctx, `UPDATE shadow_evidence_reviews SET one_hour_sample_size=21 WHERE id=$1`, review.ID); err == nil {
		t.Fatal("immutable Shadow evidence review was updated")
	}
	if _, err = pool.Exec(ctx, `DELETE FROM shadow_evidence_reviews WHERE id=$1`, review.ID); err == nil {
		t.Fatal("immutable Shadow evidence review was deleted")
	}
	assertCount(t, pool, `SELECT count(*) FROM shadow_execution_outcomes`, 1)
	assertCount(t, pool, `SELECT count(*) FROM shadow_evidence_reviews`, 2)
}

func TestPostgresCapitalReservationsAllowOnlyExactAggregateSharing(t *testing.T) {
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
		userID       = "12121212-1212-4121-8121-121212121212"
		connectionID = "13131313-1313-4131-8131-131313131313"
		accountID    = "14141414-1414-4141-8141-141414141414"
		bucketOne    = "15151515-1515-4151-8151-151515151515"
		bucketTwo    = "16161616-1616-4161-8161-161616161616"
		bucketThree  = "17171717-1717-4171-8171-171717171717"
		bucketFour   = "21212121-2121-4212-8212-212121212121"
		bucketFive   = "22222222-2121-4212-8212-212121212121"
		mandateOne   = "18181818-1818-4181-8181-181818181818"
		mandateTwo   = "19191919-1919-4191-8191-191919191919"
		mandateThree = "20202020-2020-4202-8202-202020202020"
		mandateFour  = "23232323-2323-4232-8232-232323232323"
		mandateFive  = "24242424-2424-4242-8242-242424242424"
	)
	for _, statement := range []struct {
		query string
		args  []any
	}{
		{`INSERT INTO users(id,email,normalized_email,display_name,email_verified_at) VALUES($1,'reservations@example.com','reservations@example.com','Reservations',now())`, []any{userID}},
		{`INSERT INTO user_entitlements(user_id,entitlement_key,source,billing_required) VALUES($1,'founder','bootstrap',false)`, []any{userID}},
		{`INSERT INTO provider_connections(id,user_id,provider_category,provider_name,display_name,status) VALUES($1,$2,'financial','schwab','Schwab','active')`, []any{connectionID, userID}},
		{`INSERT INTO financial_accounts(id,user_id,provider_connection_id,provider_name,provider_account_id,display_name,account_type,base_currency,status,capabilities) VALUES($1,$2,$3,'schwab','aggregate-test','Aggregate Test','brokerage','USD','active','{"options":"SUPPORTED","margin":"UNKNOWN"}')`, []any{accountID, userID, connectionID}},
	} {
		if _, err = pool.Exec(ctx, statement.query, statement.args...); err != nil {
			t.Fatal(err)
		}
	}
	for _, bucket := range []struct {
		id, name, amount string
	}{{bucketOne, "First", "1000"}, {bucketTwo, "Second", "1500"}, {bucketThree, "Third", "1000"}, {bucketFour, "Concurrent first", "2000"}, {bucketFive, "Concurrent second", "2000"}} {
		if _, err = pool.Exec(ctx, `INSERT INTO capital_buckets(id,user_id,financial_account_id,name,allocation_type,allocation_value,currency,protected_amount,allocation_limit,status) VALUES($1,$2,$3,$4,'FIXED_AMOUNT',$5,'USD',0,3000,'ACTIVE')`, bucket.id, userID, accountID, bucket.name, bucket.amount); err != nil {
			t.Fatal(err)
		}
	}
	for _, binding := range []struct{ mandateID, bucketID string }{{mandateOne, bucketOne}, {mandateTwo, bucketTwo}, {mandateThree, bucketThree}, {mandateFour, bucketFour}, {mandateFive, bucketFive}} {
		if _, err = pool.Exec(ctx, `INSERT INTO automation_mandates(id,user_id,financial_account_id,automation_type,strategy_identifier,capital_bucket_id,autonomy_level,execution_mode,status,current_version,strategy_parameters,risk_parameters,allowed_universe,prohibited_universe,margin_allowed,options_allowed,schedule_conditions,capability_unverified) VALUES($1,$2,$3,'STRATEGY','wheel',$4,'RESEARCH_ONLY','PAPER','READY',1,'{}','{}','{"symbols":[],"universe_ids":[]}','{"symbols":[]}',false,true,'{}',false)`, binding.mandateID, userID, accountID, binding.bucketID); err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(ctx, `INSERT INTO automation_mandate_versions(mandate_id,version_number,created_by_user_id,source,snapshot,change_summary) SELECT id,1,user_id,'UI',to_jsonb(m) || '{"execution_capable":false}'::jsonb,'{}'::jsonb FROM automation_mandates m WHERE id=$1`, binding.mandateID); err != nil {
			t.Fatal(err)
		}
	}

	wheel := "wheel"
	mandate := func(id, bucket string) automation.Mandate {
		return automation.Mandate{ID: id, UserID: userID, FinancialAccountID: accountID, AutomationType: "STRATEGY", StrategyIdentifier: &wheel, CapitalBucketID: bucket, ExecutionMode: "PAPER", Status: "READY", CurrentVersion: 1, ScheduleConditions: json.RawMessage(`{}`)}
	}
	store := NewPostgresStore(pool)
	first, err := store.Initialize(ctx, userID, mandate(mandateOne, bucketOne), "1000", ReadyForPut)
	if err != nil {
		t.Fatal(err)
	}
	firstReservation, err := store.CapitalReservation(ctx, userID, first.ID)
	if err != nil || firstReservation.Status != "ACTIVE" || firstReservation.ReservationAmount == nil || *firstReservation.ReservationAmount != "1000.0000000000" || firstReservation.AccountAllocationLimit == nil || *firstReservation.AccountAllocationLimit != "3000.0000000000" {
		t.Fatalf("active reservation projection was incomplete: %#v %v", firstReservation, err)
	}
	second, err := store.Initialize(ctx, userID, mandate(mandateTwo, bucketTwo), "1500", ReadyForPut)
	if err != nil {
		t.Fatalf("compatible fixed reservations did not share one account: %v", err)
	}
	reservationInventory, err := store.CapitalReservations(ctx, userID)
	if err != nil || len(reservationInventory) != 2 || reservationInventory[0].Status != "ACTIVE" || reservationInventory[1].Status != "ACTIVE" {
		t.Fatalf("owner reservation inventory was incomplete: %#v %v", reservationInventory, err)
	}
	otherInventory, err := store.CapitalReservations(ctx, "99999999-9999-4999-8999-999999999999")
	if err != nil || len(otherInventory) != 0 {
		t.Fatalf("reservation inventory was not owner scoped: %#v %v", otherInventory, err)
	}
	var activeCount int
	var activeAmount, ceiling string
	if err = pool.QueryRow(ctx, `SELECT count(*),sum(reservation_amount)::text,min(account_allocation_limit)::text FROM strategy_capital_reservations WHERE user_id=$1 AND financial_account_id=$2 AND released_at IS NULL`, userID, accountID).Scan(&activeCount, &activeAmount, &ceiling); err != nil || activeCount != 2 || activeAmount != "2500.0000000000" || ceiling != "3000.0000000000" {
		t.Fatalf("aggregate reservation snapshot changed: count=%d amount=%s ceiling=%s err=%v", activeCount, activeAmount, ceiling, err)
	}
	if _, err = store.Initialize(ctx, userID, mandate(mandateThree, bucketThree), "600", ReadyForPut); !errors.Is(err, ErrAccountInUse) {
		t.Fatalf("reservation exceeding the shared account ceiling was accepted: %v", err)
	}
	if _, err = pool.Exec(ctx, `UPDATE capital_buckets SET allocation_value=900 WHERE id=$1`, bucketOne); err == nil {
		t.Fatal("active reservation allowed its capital policy to change")
	}
	if _, err = pool.Exec(ctx, `UPDATE capital_buckets SET name='First renamed' WHERE id=$1`, bucketOne); err != nil {
		t.Fatalf("active reservation blocked a cosmetic rename: %v", err)
	}
	if _, err = store.Pause(ctx, userID, second.ID, 1, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Initialize(ctx, userID, mandate(mandateThree, bucketThree), "600", ReadyForPut); !errors.Is(err, ErrAccountInUse) {
		t.Fatalf("paused reservation stopped counting toward the account ceiling: %v", err)
	}
	finishedAt := time.Now().UTC().Add(time.Minute).Truncate(time.Microsecond)
	if _, err = store.Finish(ctx, userID, first.ID, 1, finishedAt); err != nil {
		t.Fatalf("completed strategy did not release its reservation: %v", err)
	}
	reservationInventory, err = store.CapitalReservations(ctx, userID)
	if err != nil || len(reservationInventory) != 1 || reservationInventory[0].StrategyInstanceID != second.ID {
		t.Fatalf("active reservation inventory retained a released claim: %#v %v", reservationInventory, err)
	}
	firstReservation, err = store.CapitalReservation(ctx, userID, first.ID)
	if err != nil || firstReservation.Status != "RELEASED" || firstReservation.ReleasedAt == nil || !firstReservation.ReleasedAt.Equal(finishedAt) || firstReservation.ReleaseReason == nil || *firstReservation.ReleaseReason != "COMPLETED" {
		t.Fatalf("released reservation projection was incomplete: %#v %v", firstReservation, err)
	}
	third, err := store.Initialize(ctx, userID, mandate(mandateThree, bucketThree), "600", ReadyForPut)
	if err != nil || third.ID == "" {
		t.Fatalf("released capital was not reusable within the shared ceiling: %#v %v", third, err)
	}
	if _, err = pool.Exec(ctx, `UPDATE strategy_capital_reservations SET reservation_amount=1 WHERE strategy_instance_id=$1`, second.ID); err == nil {
		t.Fatal("active reservation evidence was mutable")
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM strategy_capital_reservations WHERE strategy_instance_id=$1 AND released_at=$2 AND release_reason='COMPLETED'`, first.ID, finishedAt).Scan(&activeCount); err != nil || activeCount != 1 {
		t.Fatalf("completion release evidence was not retained: count=%d err=%v", activeCount, err)
	}
	if _, err = store.Finish(ctx, userID, second.ID, 2, finishedAt.Add(time.Minute)); err != nil {
		t.Fatalf("paused strategy reservation did not release on completion: %v", err)
	}
	if _, err = store.Finish(ctx, userID, third.ID, 1, finishedAt.Add(2*time.Minute)); err != nil {
		t.Fatalf("active strategy reservation did not release on completion: %v", err)
	}

	start := make(chan struct{})
	results := make(chan error, 2)
	var concurrent sync.WaitGroup
	for _, candidate := range []automation.Mandate{mandate(mandateFour, bucketFour), mandate(mandateFive, bucketFive)} {
		concurrent.Add(1)
		go func(candidate automation.Mandate) {
			defer concurrent.Done()
			<-start
			_, initializeErr := store.Initialize(ctx, userID, candidate, "2000", ReadyForPut)
			results <- initializeErr
		}(candidate)
	}
	close(start)
	concurrent.Wait()
	close(results)
	var accepted, rejected int
	for result := range results {
		if result == nil {
			accepted++
		} else if errors.Is(result, ErrAccountInUse) {
			rejected++
		} else {
			t.Fatalf("concurrent reservation returned an unexpected error: %v", result)
		}
	}
	if accepted != 1 || rejected != 1 {
		t.Fatalf("concurrent account ceiling was not serialized: accepted=%d rejected=%d", accepted, rejected)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*),sum(reservation_amount)::text FROM strategy_capital_reservations WHERE user_id=$1 AND financial_account_id=$2 AND released_at IS NULL`, userID, accountID).Scan(&activeCount, &activeAmount); err != nil || activeCount != 1 || activeAmount != "2000.0000000000" {
		t.Fatalf("concurrent reservation aggregate changed: count=%d amount=%s err=%v", activeCount, activeAmount, err)
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
