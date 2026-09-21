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
	"github.com/pressly/goose/v3"
)

func TestPostgresAIPaperCommitBindings(t *testing.T) {
	dsn := os.Getenv("STRATEGY_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("STRATEGY_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var database string
	if err = pool.QueryRow(ctx, `SELECT current_database()`).Scan(&database); err != nil || database != "arbion_strategy" {
		t.Fatal("dedicated arbion_strategy test database required", err)
	}
	db, err := sql.Open("pgx", dsn)
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

	for _, status := range []string{"PAUSED", "DISABLED", "ARCHIVED"} {
		t.Run("mandate "+status, func(t *testing.T) {
			f := newPaperBindingFixture(t, ctx, pool)
			if _, err := automation.NewPostgresStore(pool).Transition(ctx, f.instance.UserID, f.instance.AutomationMandateID, 1, status, "UI"); err != nil {
				t.Fatal(err)
			}
			if err := f.commit(ctx); !errors.Is(err, ErrEvaluationConfigurationChanged) {
				t.Fatal("revoked mandate accepted", err)
			}
			f.assertEmpty(t, ctx)
		})
	}
	t.Run("newer draft retains approved version", func(t *testing.T) {
		f := newPaperBindingFixture(t, ctx, pool)
		if _, err := pool.Exec(ctx, `UPDATE automation_mandates SET status='DRAFT',current_version=2 WHERE id=$1`, f.instance.AutomationMandateID); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO automation_mandate_versions(mandate_id,version_number,created_by_user_id,source,snapshot,change_summary) SELECT id,2,user_id,'UI',to_jsonb(m),'{}' FROM automation_mandates m WHERE id=$1`, f.instance.AutomationMandateID); err != nil {
			t.Fatal(err)
		}
		if err := f.commit(ctx); err != nil {
			t.Fatal("unrelated draft interrupted pinned instance", err)
		}
		// Lost-response delivery remains an exact duplicate even after revocation.
		if _, err := automation.NewPostgresStore(pool).Transition(ctx, f.instance.UserID, f.instance.AutomationMandateID, 2, "DISABLED", "UI"); err != nil {
			t.Fatal(err)
		}
		if err := f.commit(ctx); !errors.Is(err, ErrDuplicate) {
			t.Fatal("completed event was not duplicate safe", err)
		}
		assertCount(t, pool, `SELECT count(*) FROM ai_paper_spot_fills WHERE strategy_instance_id='`+f.instance.ID+`'`, 1)
	})
	t.Run("wrong pinned version", func(t *testing.T) {
		f := newPaperBindingFixture(t, ctx, pool)
		f.instance.MandateVersion = 2
		f.decision.ProposedAction.MandateVersion = &f.instance.MandateVersion
		f.evaluation.MandateVersion = &f.instance.MandateVersion
		if err := f.commit(ctx); !errors.Is(err, ErrEvaluationConfigurationChanged) {
			t.Fatal("unpersisted mandate version accepted", err)
		}
		f.assertEmpty(t, ctx)
	})
	t.Run("cross account caller", func(t *testing.T) {
		f := newPaperBindingFixture(t, ctx, pool)
		f.instance.FinancialAccountID = f.instance.UserID // Valid UUID, not this account.
		f.decision.ProposedAction.FinancialAccountID = f.instance.FinancialAccountID
		f.evaluation.AccountID = f.instance.FinancialAccountID
		if err := f.commit(ctx); !errors.Is(err, ErrCommitConnectionUnavailable) {
			t.Fatal("cross-account binding accepted", err)
		}
		f.assertEmpty(t, ctx)
	})
	t.Run("starting cash must match reservation", func(t *testing.T) {
		f := newPaperBindingFixture(t, ctx, pool)
		if _, err := pool.Exec(ctx, `UPDATE paper_portfolios SET starting_cash=999 WHERE strategy_instance_id=$1`, f.instance.ID); err != nil {
			t.Fatal(err)
		}
		if err := f.commit(ctx); !errors.Is(err, ErrCapitalReservation) {
			t.Fatal("inconsistent reservation accepted", err)
		}
		f.assertEmpty(t, ctx)
	})
	t.Run("finished instance and released claim", func(t *testing.T) {
		f := newPaperBindingFixture(t, ctx, pool)
		if _, err := f.store.Finish(ctx, f.instance.UserID, f.instance.ID, 1, f.now); err != nil {
			t.Fatal(err)
		}
		if err := f.commit(ctx); !errors.Is(err, ErrConflict) {
			t.Fatal("completed instance accepted", err)
		}
		f.assertEmpty(t, ctx)
	})
	for _, target := range []string{"mandate", "instance"} {
		t.Run("concurrent "+target+" pause wins", func(t *testing.T) {
			f := newPaperBindingFixture(t, ctx, pool)
			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			if target == "mandate" {
				_, err = tx.Exec(ctx, `UPDATE automation_mandates SET status='PAUSED',current_version=2 WHERE id=$1`, f.instance.AutomationMandateID)
			} else {
				_, err = tx.Exec(ctx, `UPDATE strategy_instances SET status='PAUSED',state_version=2 WHERE id=$1`, f.instance.ID)
			}
			if err != nil {
				t.Fatal(err)
			}
			result := make(chan error, 1)
			go func() { result <- f.commit(ctx) }()
			waitForPaperBindingLock(t, ctx, pool, tx.Conn().PgConn().PID())
			if err = tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			want := ErrConflict
			if target == "mandate" {
				want = ErrEvaluationConfigurationChanged
			}
			if err = <-result; !errors.Is(err, want) {
				t.Fatal("mid-cycle pause lost", err)
			}
			f.assertEmpty(t, ctx)
		})
	}
	t.Run("binding locks survive until transaction end", func(t *testing.T) {
		f := newPaperBindingFixture(t, ctx, pool)
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		if _, err = lockAIPaperCommitBindings(ctx, tx, f.instance); err != nil {
			t.Fatal(err)
		}
		result := make(chan error, 1)
		go func() {
			_, err := automation.NewPostgresStore(pool).Transition(ctx, f.instance.UserID, f.instance.AutomationMandateID, 1, "DISABLED", "UI")
			result <- err
		}()
		waitForPaperBindingLock(t, ctx, pool, tx.Conn().PgConn().PID())
		if err = tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		if err = <-result; err != nil {
			t.Fatal(err)
		}
		f.assertEmpty(t, ctx)
	})
	t.Run("atomic emergency stops", func(t *testing.T) {
		testPaperCommitEmergencyStops(t, ctx, pool)
	})
	t.Run("current commit access", func(t *testing.T) {
		testNonLiveCommitAccess(t, ctx, pool)
	})
	t.Run("commit quote freshness", func(t *testing.T) {
		testAICommitMarketTime(t, ctx, pool)
	})
	t.Run("commit mandate window", func(t *testing.T) {
		testAICommitMandateWindow(t, ctx, pool)
	})
	t.Run("Shadow commit bindings", func(t *testing.T) {
		testAIShadowCommitBindings(t, ctx, pool)
	})
	t.Run("current commit activity", func(t *testing.T) {
		testAICommitActivity(t, ctx, pool)
	})
	t.Run("current commit reconciliation", func(t *testing.T) {
		testAICommitReconciliation(t, ctx, pool)
	})
	t.Run("accepted Shadow risk binding", func(t *testing.T) {
		testAIShadowCommitRiskBinding(t, ctx, pool)
	})
	t.Run("concurrent different deliveries cannot double spend", func(t *testing.T) {
		f := newPaperBindingFixture(t, ctx, pool)
		other := f
		action := *f.decision.ProposedAction
		action.ID += ":other"
		action.CorrelationID += ":other"
		other.decision.ProposedAction = &action
		other.evaluation.ID = paperBindingUUID(t, ctx, pool)
		result := make(chan error, 2)
		go func() { result <- f.commit(ctx) }()
		go func() { result <- other.commit(ctx) }()
		first, second := <-result, <-result
		if !((first == nil && errors.Is(second, ErrConflict)) || (second == nil && errors.Is(first, ErrConflict))) {
			t.Fatal("expected one commit and one stale-cash refusal, not a deadlock", first, second)
		}
		assertCount(t, pool, `SELECT count(*) FROM ai_paper_spot_fills WHERE strategy_instance_id='`+f.instance.ID+`'`, 1)
		assertCount(t, pool, `SELECT count(*) FROM strategy_evaluation_events WHERE strategy_instance_id='`+f.instance.ID+`'`, 1)
	})
}

type paperBindingFixture struct {
	store      *PostgresStore
	instance   Instance
	decision   Decision
	evaluation risk.RiskEvaluation
	fill       AIPaperFill
	now        time.Time
}

func (f paperBindingFixture) commit(ctx context.Context) error {
	return f.store.CommitAIPaperEvaluation(ctx, f.instance, f.instance.StateVersion, f.decision, f.evaluation, f.fill, f.now)
}

func (f paperBindingFixture) assertEmpty(t *testing.T, ctx context.Context) {
	t.Helper()
	for _, table := range []string{"ai_paper_spot_fills", "nonlive_execution_records", "decision_journal_entries", "risk_evaluations", "order_intents"} {
		assertCount(t, f.store.db, `SELECT count(*) FROM `+table+` WHERE user_id='`+f.instance.UserID+`'`, 0)
	}
	assertCount(t, f.store.db, `SELECT count(*) FROM strategy_evaluation_events WHERE strategy_instance_id='`+f.instance.ID+`'`, 0)
	assertCount(t, f.store.db, `SELECT count(*) FROM paper_positions WHERE paper_portfolio_id=(SELECT id FROM paper_portfolios WHERE strategy_instance_id='`+f.instance.ID+`')`, 0)
	if f.instance.ExecutionMode != Paper {
		assertCount(t, f.store.db, `SELECT count(*) FROM paper_portfolios WHERE strategy_instance_id='`+f.instance.ID+`'`, 0)
		return
	}
	var cash string
	if err := f.store.db.QueryRow(ctx, `SELECT cash::text FROM paper_portfolios WHERE strategy_instance_id=$1`, f.instance.ID).Scan(&cash); err != nil || !sameAIPaperDecimal(cash, "1000") {
		t.Fatal("rejected commit changed cash", cash, err)
	}
}

func paperBindingUUID(t *testing.T, ctx context.Context, pool *pgxpool.Pool) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func newPaperBindingFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool, ownerIDs ...string) paperBindingFixture {
	t.Helper()
	return newNonLiveBindingFixture(t, ctx, pool, Paper, json.RawMessage(`{}`), ownerIDs...)
}

// Snapshot overrides are applied only at insertion into the isolated test DB;
// immutable history is never updated or its protections disabled.
func newNonLiveBindingFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool, mode ExecutionMode, snapshotOverride json.RawMessage, ownerIDs ...string) paperBindingFixture {
	t.Helper()
	return newNonLiveBindingFixtureWithBucket(t, ctx, pool, mode, snapshotOverride, nil, ownerIDs...)
}

func newNonLiveBindingFixtureWithBucket(t *testing.T, ctx context.Context, pool *pgxpool.Pool, mode ExecutionMode, snapshotOverride json.RawMessage, bucketOverride *automation.CapitalBucket, ownerIDs ...string) paperBindingFixture {
	t.Helper()
	return newNonLiveBindingFixtureWithReconciliation(t, ctx, pool, mode, snapshotOverride, bucketOverride, true, ownerIDs...)
}

func newNonLiveBindingFixtureWithReconciliation(t *testing.T, ctx context.Context, pool *pgxpool.Pool, mode ExecutionMode, snapshotOverride json.RawMessage, bucketOverride *automation.CapitalBucket, seedReconciliation bool, ownerIDs ...string) paperBindingFixture {
	t.Helper()
	u, financial, ai, account, bucket, mandate := paperBindingUUID(t, ctx, pool), paperBindingUUID(t, ctx, pool), paperBindingUUID(t, ctx, pool), paperBindingUUID(t, ctx, pool), paperBindingUUID(t, ctx, pool), paperBindingUUID(t, ctx, pool)
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	if len(ownerIDs) > 0 {
		u = ownerIDs[0]
	} else {
		exec(`INSERT INTO users(id,email,normalized_email,display_name,email_verified_at) VALUES($1,$2,$2,'Paper binding test',now())`, u, u+"@example.com")
		exec(`INSERT INTO user_entitlements(user_id,entitlement_key,source,billing_required) VALUES($1,'founder','bootstrap',false)`, u)
	}
	exec(`INSERT INTO provider_connections(id,user_id,provider_category,provider_name,display_name,status) VALUES($1,$2,'financial','coinbase',$4,'active'),($3,$2,'ai','openai',$5,'active')`, financial, u, ai, "Test source "+financial, "Test model "+ai)
	exec(`INSERT INTO financial_accounts(id,user_id,provider_connection_id,provider_name,provider_account_id,display_name,account_type,base_currency,status,capabilities) VALUES($1,$2,$3,'coinbase',$4,'Test account','crypto','USD','active','{}')`, account, u, financial, "fixture:"+account)
	exec(`INSERT INTO capital_buckets(id,user_id,financial_account_id,name,allocation_type,allocation_value,currency,protected_amount,status) VALUES($1,$2,$3,'Test budget','FIXED_AMOUNT',1000,'USD',0,'ACTIVE')`, bucket, u, account)
	if bucketOverride != nil {
		exec(`UPDATE capital_buckets SET allocation_type=$2,allocation_value=$3,currency=$4,protected_amount=$5,allocation_limit=$6 WHERE id=$1`, bucket, bucketOverride.AllocationType, bucketOverride.AllocationValue, bucketOverride.Currency, bucketOverride.ProtectedAmount, bucketOverride.AllocationLimit)
	}
	exec(`INSERT INTO automation_mandates(id,user_id,financial_account_id,automation_type,ai_provider_connection_id,ai_model_id,capital_bucket_id,autonomy_level,execution_mode,status,current_version,strategy_parameters,risk_parameters,allowed_universe,prohibited_universe,margin_allowed,options_allowed,schedule_conditions,capability_unverified) VALUES($1,$2,$3,'AI_AUTONOMOUS',$4,'gpt-5.6-sol',$5,'FULL_AUTONOMOUS','PAPER','READY',1,'{"objective":"Simulation only.","max_proposal_notional":"100"}','{}','{"symbols":["BTC"]}','{"symbols":[]}',false,false,'{"enabled":false}',false)`, mandate, u, account, ai, bucket)
	exec(`UPDATE automation_mandates SET execution_mode=$2 WHERE id=$1`, mandate, mode)
	exec(`INSERT INTO automation_mandate_versions(mandate_id,version_number,created_by_user_id,source,snapshot,change_summary) SELECT id,1,user_id,'UI',to_jsonb(m) || $2::jsonb, '{}' FROM automation_mandates m WHERE id=$1`, mandate, snapshotOverride)
	store := NewPostgresStore(pool)
	i, err := store.Initialize(ctx, u, automation.Mandate{ID: mandate, UserID: u, FinancialAccountID: account, CapitalBucketID: bucket, AIProviderConnectionID: &ai, AutomationType: "AI_AUTONOMOUS", ExecutionMode: string(mode), Status: "READY", CurrentVersion: 1, ScheduleConditions: json.RawMessage(`{"enabled":false}`)}, "1000", AIMonitoring)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	price, state := "100", string(AIMonitoring)
	a := risk.ProposedAction{ID: "test:" + i.ID, CorrelationID: "test:" + i.ID, Source: risk.SourceAI, FinancialAccountID: account, MandateID: &mandate, MandateVersion: &i.MandateVersion, ActionType: risk.ActionBuy, Instrument: "BTC", Side: "BUY", Quantity: "0.5", Notional: "50", EstimatedPrice: &price, StrategyInstanceID: &i.ID, StrategyState: &state, CreatedAt: now}
	e := risk.RiskEvaluation{ID: paperBindingUUID(t, ctx, pool), UserID: u, AccountID: account, MandateID: &mandate, MandateVersion: &i.MandateVersion, Decision: risk.Allow, Mode: "PAPER", Timestamp: now, Checks: []risk.RiskCheck{}, ReasonCodes: []risk.ReasonCode{risk.Allowed}}
	quote := &AIProposalQuoteReference{Symbol: "BTC", Side: "BUY", Price: price, Basis: "ASK", Provider: "coinbase", Feed: "rest_ticker", Quality: "REAL_TIME_SINGLE_VENUE", ObservedAt: now}
	rationale, err := json.Marshal(map[string]any{"decision": "PROPOSE", "quote_reference": quote})
	if err != nil {
		t.Fatal(err)
	}
	d := Decision{ProposedAction: &a, Source: "AI", InstrumentType: "CRYPTO", ProposedState: AIMonitoring, QuoteReference: quote, Rationale: rationale}
	fill := SimulateAIPaperSpotFill(a, e, "CRYPTO", AIPaperPortfolioSnapshot{Currency: "USD", Cash: "1000", Positions: map[string]string{}}, AIPaperMarketReference{Symbol: "BTC", Price: price, Basis: "ASK", Provider: quote.Provider, Feed: quote.Feed, Quality: quote.Quality, ObservedAt: now}, AIPaperSimulationConfig{}, now)
	if fill.Status != SimulatedFilled {
		t.Fatal("invalid test simulation", fill)
	}
	e.Mode = string(mode)
	f := paperBindingFixture{store: store, instance: i, decision: d, evaluation: e, fill: fill, now: now}
	if mode == Shadow && seedReconciliation {
		if err := saveCommitReconciliation(ctx, pool, f, "MATCHED", now.Add(-time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	return f
}

// Wait for an actual PostgreSQL lock dependency rather than assuming a worker
// reached the commit boundary after an arbitrary sleep.
func waitForPaperBindingLock(t *testing.T, ctx context.Context, pool *pgxpool.Pool, blocker uint32) {
	t.Helper()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	for {
		var blocked bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE $1::int=ANY(pg_blocking_pids(pid)))`, int32(blocker)).Scan(&blocked); err != nil {
			t.Fatal("worker did not reach the database lock", err)
		}
		if blocked {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal("worker did not reach the database lock")
		case <-time.After(10 * time.Millisecond):
		}
	}
}
