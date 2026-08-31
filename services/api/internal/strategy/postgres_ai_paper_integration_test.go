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

func TestPostgresAIPaperFillIsAtomicImmutableAndBrokerDisconnected(t *testing.T) {
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
		userID                = "a1000000-0000-4000-8000-000000000001"
		financialConnectionID = "a1000000-0000-4000-8000-000000000002"
		aiConnectionID        = "a1000000-0000-4000-8000-000000000003"
		accountID             = "a1000000-0000-4000-8000-000000000004"
		bucketID              = "a1000000-0000-4000-8000-000000000005"
		mandateID             = "a1000000-0000-4000-8000-000000000006"
	)
	statements := []string{
		`INSERT INTO users(id,email,normalized_email,display_name,email_verified_at) VALUES('` + userID + `','ai-paper@example.com','ai-paper@example.com','AI Paper',now())`,
		`INSERT INTO user_entitlements(user_id,entitlement_key,source,billing_required) VALUES('` + userID + `','founder','bootstrap',false)`,
		`INSERT INTO provider_connections(id,user_id,provider_category,provider_name,display_name,status) VALUES('` + financialConnectionID + `','` + userID + `','financial','coinbase','Coinbase','active')`,
		`INSERT INTO provider_connections(id,user_id,provider_category,provider_name,display_name,status) VALUES('` + aiConnectionID + `','` + userID + `','ai','openai','OpenAI','active')`,
		`INSERT INTO financial_accounts(id,user_id,provider_connection_id,provider_name,provider_account_id,display_name,account_type,base_currency,status,capabilities) VALUES('` + accountID + `','` + userID + `','` + financialConnectionID + `','coinbase','paper-source','Coinbase Paper Source','crypto','USD','active','{"trade":"SUPPORTED","transfer":"UNSUPPORTED"}')`,
		`INSERT INTO capital_buckets(id,user_id,financial_account_id,name,allocation_type,allocation_value,currency,protected_amount,status) VALUES('` + bucketID + `','` + userID + `','` + accountID + `','AI paper budget','FIXED_AMOUNT',1000,'USD',0,'ACTIVE')`,
		`INSERT INTO automation_mandates(id,user_id,financial_account_id,automation_type,ai_provider_connection_id,ai_model_id,capital_bucket_id,autonomy_level,execution_mode,status,current_version,strategy_parameters,risk_parameters,allowed_universe,prohibited_universe,margin_allowed,options_allowed,schedule_conditions,capability_unverified) VALUES('` + mandateID + `','` + userID + `','` + accountID + `','AI_AUTONOMOUS','` + aiConnectionID + `','gpt-5.6-sol','` + bucketID + `','FULL_AUTONOMOUS','PAPER','READY',1,'{"objective":"Paper only.","max_proposal_notional":"100"}','{}','{"symbols":["BTC"],"universe_ids":[]}','{"symbols":[]}',false,false,'{"enabled":false}',false)`,
		`INSERT INTO automation_mandate_versions(mandate_id,version_number,created_by_user_id,source,snapshot,change_summary) SELECT id,1,user_id,'UI',to_jsonb(m) || '{"execution_capable":false}'::jsonb,'{}'::jsonb FROM automation_mandates m WHERE id='` + mandateID + `'`,
	}
	for _, statement := range statements {
		if _, err = pool.Exec(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}

	aiConnection, model := aiConnectionID, "gpt-5.6-sol"
	mandate := automation.Mandate{
		ID: mandateID, UserID: userID, FinancialAccountID: accountID,
		AutomationType: "AI_AUTONOMOUS", AIProviderConnectionID: &aiConnection,
		AIModelID: &model, CapitalBucketID: bucketID, AutonomyLevel: "FULL_AUTONOMOUS",
		ExecutionMode: "PAPER", Status: "READY", CurrentVersion: 1,
		ScheduleConditions: json.RawMessage(`{"enabled":false}`),
	}
	store := NewPostgresStore(pool)
	instance, err := store.Initialize(ctx, userID, mandate, "1000.0000000000", AIMonitoring)
	if err != nil || instance.ExecutionMode != Paper || instance.StrategyIdentifier != "ai_shadow" {
		t.Fatalf("AI PAPER instance was not initialized safely: %#v %v", instance, err)
	}

	now := time.Date(2026, 8, 28, 17, 0, 0, 0, time.UTC)
	price := "100.0000000000"
	state := string(AIMonitoring)
	action := risk.ProposedAction{
		ID: "ai-paper:buy:1", CorrelationID: "scheduled:ai-paper:buy:1", FinancialAccountID: accountID,
		Source: risk.SourceAI, ActionType: risk.ActionBuy, MandateID: &instance.AutomationMandateID,
		MandateVersion: &instance.MandateVersion, Instrument: "BTC", Side: "BUY", Quantity: "1.0000000000",
		Notional: "100", EstimatedPrice: &price, StrategyInstanceID: &instance.ID,
		StrategyState: &state, CreatedAt: now,
	}
	evaluation := risk.RiskEvaluation{
		ID: "a1000000-0000-4000-8000-000000000007", UserID: userID, AccountID: accountID,
		MandateID: action.MandateID, MandateVersion: action.MandateVersion, Timestamp: now,
		Decision: risk.Allow, Checks: []risk.RiskCheck{}, ReasonCodes: []risk.ReasonCode{risk.Allowed}, Mode: "PAPER",
	}
	decision := Decision{
		ProposedAction: &action, Source: "AI", InstrumentType: "CRYPTO", ProposedState: AIMonitoring,
		CandidateCount: 1, Reason: "integration", Rationale: json.RawMessage(`{"decision":"PROPOSE","symbol":"BTC","side":"BUY","ai_provider":"openai","model_id":"gpt-5.6-sol","profile":"deep"}`),
	}
	fill := SimulateAIPaperSpotFill(
		action, evaluation, "CRYPTO",
		AIPaperPortfolioSnapshot{Currency: "USD", Cash: "1000.0000000000", Positions: map[string]string{}},
		AIPaperMarketReference{Symbol: "BTC", Price: price, Basis: "ASK", Provider: "coinbase", Feed: "advanced_trade", Quality: "BROKER_REALTIME", ObservedAt: now.Add(-time.Second)},
		AIPaperSimulationConfig{FeeBasisPoints: 50, SlippageBasisPoints: 25}, now,
	)
	if err = store.CommitAIPaperEvaluation(ctx, instance, instance.StateVersion, decision, evaluation, fill, now); err != nil {
		t.Fatal(err)
	}
	if err = store.CommitAIPaperEvaluation(ctx, instance, instance.StateVersion, decision, evaluation, fill, now); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("duplicate AI PAPER event was not rejected: %v", err)
	}

	assertCount(t, pool, `SELECT count(*) FROM ai_paper_spot_fills WHERE user_id='`+userID+`'`, 1)
	assertCount(t, pool, `SELECT count(*) FROM nonlive_execution_records WHERE user_id='`+userID+`' AND mode='PAPER' AND status='SIMULATED_FILLED'`, 1)
	assertCount(t, pool, `SELECT count(*) FROM decision_journal_entries WHERE user_id='`+userID+`' AND source='AI' AND decision_type='ALLOW_SIMULATED_FILLED'`, 1)
	assertCount(t, pool, `SELECT count(*) FROM risk_evaluations WHERE user_id='`+userID+`' AND platform_execution_available=false`, 1)

	var cash, quantity, averagePrice, simulationOnly string
	if err = pool.QueryRow(ctx, `SELECT cash::text FROM paper_portfolios WHERE strategy_instance_id=$1`, instance.ID).Scan(&cash); err != nil || cash != fill.ResultingCash {
		t.Fatalf("paper cash projection is wrong: %q want %q err=%v", cash, fill.ResultingCash, err)
	}
	if err = pool.QueryRow(ctx, `SELECT quantity::text,average_price::text FROM paper_positions WHERE paper_portfolio_id=(SELECT id FROM paper_portfolios WHERE strategy_instance_id=$1) AND symbol='BTC' AND instrument='CRYPTO'`, instance.ID).Scan(&quantity, &averagePrice); err != nil || quantity != fill.ResultingPositionQuantity || averagePrice != "100.7512500000" {
		t.Fatalf("paper position projection is wrong: quantity=%q average=%q err=%v", quantity, averagePrice, err)
	}
	if err = pool.QueryRow(ctx, `SELECT simulation_only::text FROM ai_paper_spot_fills WHERE user_id=$1`, userID).Scan(&simulationOnly); err != nil || simulationOnly != "true" {
		t.Fatalf("paper fill lost its simulation-only marker: %q %v", simulationOnly, err)
	}
	fills, err := store.AIPaperSpotFills(ctx, userID, instance.ID, 25, nil)
	if err != nil || len(fills) != 1 || fills[0].ExecutionRecordID == "" || fills[0].MarketProvider != "coinbase" || fills[0].MarketFeed != "advanced_trade" || !fills[0].SimulationOnly {
		t.Fatalf("owner AI Paper fill projection is wrong: fills=%#v err=%v", fills, err)
	}
	portfolio, err := store.PaperPortfolio(ctx, userID, instance.ID)
	if err != nil || portfolio.RealizedOutcome.Status != PaperRealizedNoSales || portfolio.RealizedOutcome.TotalRealizedProfitLoss != "0.0000000000" || portfolio.RealizedOutcome.FillCount != 1 || portfolio.RealizedOutcome.SellFillCount != 0 || len(portfolio.RealizedOutcome.Symbols) != 1 {
		t.Fatalf("exact realized projection is wrong: portfolio=%#v err=%v", portfolio, err)
	}
	realizedBTC := portfolio.RealizedOutcome.Symbols[0]
	if realizedBTC.Symbol != "BTC" || realizedBTC.RealizedProfitLoss != "0.0000000000" || realizedBTC.TotalFees != fill.Fee || realizedBTC.EndingPositionQuantity != fill.ResultingPositionQuantity || realizedBTC.EndingAverageCost != "100.7512500000" {
		t.Fatalf("exact realized symbol attribution is wrong: %#v", realizedBTC)
	}
	otherOwnerFills, err := store.AIPaperSpotFills(ctx, "99999999-9999-4999-8999-999999999999", instance.ID, 25, nil)
	if err != nil || len(otherOwnerFills) != 0 {
		t.Fatalf("AI Paper fill projection crossed owners: fills=%#v err=%v", otherOwnerFills, err)
	}

	staleAction := action
	staleAction.ID = "ai-paper:buy:stale"
	staleAction.CorrelationID = "scheduled:ai-paper:buy:stale"
	staleEvaluation := evaluation
	staleEvaluation.ID = "a1000000-0000-4000-8000-000000000008"
	staleDecision := decision
	staleDecision.ProposedAction = &staleAction
	staleFill := fill
	staleFill.SimulatedAt = now.Add(time.Minute)
	staleFill.MarketObservedAt = now.Add(time.Minute - time.Second)
	if err = store.CommitAIPaperEvaluation(ctx, instance, instance.StateVersion, staleDecision, staleEvaluation, staleFill, staleFill.SimulatedAt); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale paper snapshot was not rejected atomically: %v", err)
	}
	assertCount(t, pool, `SELECT count(*) FROM strategy_evaluation_events WHERE strategy_instance_id='`+instance.ID+`'`, 1)
	assertCount(t, pool, `SELECT count(*) FROM risk_evaluations WHERE user_id='`+userID+`'`, 1)

	if _, err = pool.Exec(ctx, `UPDATE ai_paper_spot_fills SET fee=0 WHERE user_id=$1`, userID); err == nil {
		t.Fatal("immutable AI PAPER fill was updated")
	}
	if _, err = pool.Exec(ctx, `DELETE FROM ai_paper_spot_fills WHERE user_id=$1`, userID); err == nil {
		t.Fatal("immutable AI PAPER fill was deleted")
	}
	assertCount(t, pool, `SELECT count(*) FROM ai_paper_spot_fills WHERE user_id='`+userID+`'`, 1)
}
