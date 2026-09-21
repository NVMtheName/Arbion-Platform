package strategy

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/arbion/platform/services/api/internal/aiconnection"
	"github.com/arbion/platform/services/api/internal/automation"
	"github.com/arbion/platform/services/api/internal/financial"
	"github.com/arbion/platform/services/api/internal/financialconnection"
	"github.com/jackc/pgx/v5/pgxpool"
)

// These synthetic fixtures are not yet initialized. Real persistence methods
// race only inside the isolated database; no provider, model, or broker is used.
func testInitializeConnectionSerialization(t *testing.T, parent context.Context, pool *pgxpool.Pool) {
	for _, mode := range []ExecutionMode{Paper, Shadow} {
		for _, retainTarget := range []bool{false, true} {
			name := "omitted target refuses initialization"
			if retainTarget {
				name = "retained target initializes after refresh"
			}
			t.Run(string(mode)+" sync first "+name, func(t *testing.T) {
				ctx, start := initializeRaceWorkers(t, parent)
				f := newInitializeConnectionFixture(t, ctx, pool, mode)
				gate, err := pool.Begin(ctx)
				if err != nil {
					t.Fatal(err)
				}
				defer gate.Rollback(ctx)
				if _, err = gate.Exec(ctx, `LOCK TABLE financial_account_sync_operations IN SHARE MODE`); err != nil {
					t.Fatal(err)
				}
				synced := start(func() error { return f.syncAccounts(ctx, retainTarget) })
				waitForPaperBindingLock(t, ctx, pool, gate.Conn().PgConn().PID())
				writerPID := resumeBlockedPID(t, ctx, pool, gate.Conn().PgConn().PID())
				initialized := start(func() error { return f.initialize(ctx) })
				blocked, initializeErr := waitForResumeLockOrCompletion(t, ctx, pool, writerPID, initialized)
				if !blocked {
					t.Error("initialization finished before the account refresh committed", initializeErr)
				}
				if err = gate.Commit(ctx); err != nil {
					t.Fatal(err)
				}
				syncErr := <-synced
				if blocked {
					initializeErr = <-initialized
				}
				if syncErr != nil {
					t.Fatal("real account refresh failed", syncErr)
				}
				want := 0
				if retainTarget {
					want = 1
					if initializeErr != nil {
						t.Error("still-active refreshed account could not initialize", initializeErr)
					}
				} else if !errors.Is(initializeErr, ErrMandateStale) {
					t.Error("initialization accepted the now-unavailable account", initializeErr)
				}
				f.assertArtifacts(t, want)
				f.assertSync(t, retainTarget)
			})
		}
		t.Run(string(mode)+" initialization first defers omitted-target refresh", func(t *testing.T) {
			ctx, start := initializeRaceWorkers(t, parent)
			f := newInitializeConnectionFixture(t, ctx, pool, mode)
			gate, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer gate.Rollback(ctx)
			if _, err = gate.Exec(ctx, `LOCK TABLE strategy_state_transitions IN SHARE MODE`); err != nil {
				t.Fatal(err)
			}
			initialized := start(func() error { return f.initialize(ctx) })
			waitForPaperBindingLock(t, ctx, pool, gate.Conn().PgConn().PID())
			initializerPID := resumeBlockedPID(t, ctx, pool, gate.Conn().PgConn().PID())
			synced := start(func() error { return f.syncAccounts(ctx, false) })
			blocked, syncErr := waitForResumeLockOrCompletion(t, ctx, pool, initializerPID, synced)
			if !blocked {
				t.Error("account refresh committed while initialization was unfinished", syncErr)
			}
			if err = gate.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			initializeErr := <-initialized
			if blocked {
				syncErr = <-synced
			}
			if initializeErr != nil || syncErr != nil {
				t.Fatal("ordered initialization and refresh failed", initializeErr, syncErr)
			}
			// This ACTIVE runtime was initialized before the later removal. It
			// is not permission for evaluation or execution after that removal.
			f.assertArtifacts(t, 1)
			f.assertSync(t, false)
		})
		for _, category := range []string{"financial", "ai"} {
			t.Run(string(mode)+" "+category+" status writer first refuses initialization", func(t *testing.T) {
				ctx, start := initializeRaceWorkers(t, parent)
				f := newInitializeConnectionFixture(t, ctx, pool, mode)
				gate, err := pool.Begin(ctx)
				if err != nil {
					t.Fatal(err)
				}
				defer gate.Rollback(ctx)
				// Real SetStatus is an autocommit UPDATE. Its explicit test
				// transaction lets us hold that same row change before commit.
				if _, err = gate.Exec(ctx, `UPDATE provider_connections SET status=$4,updated_at=now() WHERE id=$1 AND user_id=$2 AND provider_category=$3`, f.providerID(category), f.userID, category, initializeUnavailableStatus(category)); err != nil {
					t.Fatal(err)
				}
				initialized := start(func() error { return f.initialize(ctx) })
				blocked, initializeErr := waitForResumeLockOrCompletion(t, ctx, pool, gate.Conn().PgConn().PID(), initialized)
				if !blocked {
					t.Error("initialization finished before provider status committed", initializeErr)
				}
				if err = gate.Commit(ctx); err != nil {
					t.Fatal(err)
				}
				if blocked {
					initializeErr = <-initialized
				}
				if !errors.Is(initializeErr, ErrMandateStale) {
					t.Error("initialization accepted an unavailable provider", initializeErr)
				}
				f.assertArtifacts(t, 0)
				f.assertDisabledProvider(t, category)
			})
			t.Run(string(mode)+" initialization first defers "+category+" status writer", func(t *testing.T) {
				ctx, start := initializeRaceWorkers(t, parent)
				f := newInitializeConnectionFixture(t, ctx, pool, mode)
				gate, err := pool.Begin(ctx)
				if err != nil {
					t.Fatal(err)
				}
				defer gate.Rollback(ctx)
				if _, err = gate.Exec(ctx, `LOCK TABLE strategy_state_transitions IN SHARE MODE`); err != nil {
					t.Fatal(err)
				}
				initialized := start(func() error { return f.initialize(ctx) })
				waitForPaperBindingLock(t, ctx, pool, gate.Conn().PgConn().PID())
				initializerPID := resumeBlockedPID(t, ctx, pool, gate.Conn().PgConn().PID())
				disabled := start(func() error {
					if category == "financial" {
						_, err := financialconnection.NewPostgresStore(pool).SetStatus(ctx, f.userID, f.financialID, "error", nil)
						return err
					}
					_, err := aiconnection.NewPostgresStore(pool, aiconnection.DefaultRegistry()).SetStatus(ctx, f.userID, f.aiID, "disabled")
					return err
				})
				blocked, disableErr := waitForResumeLockOrCompletion(t, ctx, pool, initializerPID, disabled)
				if !blocked {
					t.Error("provider status committed while initialization was unfinished", disableErr)
				}
				if err = gate.Commit(ctx); err != nil {
					t.Fatal(err)
				}
				initializeErr := <-initialized
				if blocked {
					disableErr = <-disabled
				}
				if initializeErr != nil || disableErr != nil {
					t.Fatal("ordered initialization and status update failed", initializeErr, disableErr)
				}
				f.assertArtifacts(t, 1)
				f.assertDisabledProvider(t, category)
			})
		}
	}
}

// Buffered completion plus cancellation/worker joining also cleans up an
// unexpected assertion failure while a database gate is still held.
func initializeRaceWorkers(t *testing.T, parent context.Context) (context.Context, func(func() error) <-chan error) {
	t.Helper()
	ctx, cancel := context.WithCancel(parent)
	var workers sync.WaitGroup
	t.Cleanup(func() { cancel(); workers.Wait() })
	return ctx, func(work func() error) <-chan error {
		done := make(chan error, 1)
		workers.Add(1)
		go func() { defer workers.Done(); done <- work() }()
		return done
	}
}

type initializeConnectionFixture struct {
	pool                                 *pgxpool.Pool
	userID, financialID, aiID, accountID string
	siblingID, bucketID                  string
	mandate                              automation.Mandate
}

func newInitializeConnectionFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool, mode ExecutionMode) initializeConnectionFixture {
	t.Helper()
	f := initializeConnectionFixture{pool: pool, userID: paperBindingUUID(t, ctx, pool), financialID: paperBindingUUID(t, ctx, pool), aiID: paperBindingUUID(t, ctx, pool), accountID: paperBindingUUID(t, ctx, pool), siblingID: paperBindingUUID(t, ctx, pool), bucketID: paperBindingUUID(t, ctx, pool)}
	mandateID := paperBindingUUID(t, ctx, pool)
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO users(id,email,normalized_email,display_name,email_verified_at) VALUES($1,$2,$2,'Initialization serialization test',now())`, f.userID, f.userID+"@example.com")
	exec(`INSERT INTO user_entitlements(user_id,entitlement_key,source,billing_required) VALUES($1,'founder','bootstrap',false)`, f.userID)
	exec(`INSERT INTO provider_connections(id,user_id,provider_category,provider_name,display_name,status) VALUES($1,$2,'financial','coinbase',$4,'active'),($3,$2,'ai','openai',$5,'active')`, f.financialID, f.userID, f.aiID, "Test source "+f.financialID, "Test model "+f.aiID)
	for _, id := range []string{f.accountID, f.siblingID} {
		exec(`INSERT INTO financial_accounts(id,user_id,provider_connection_id,provider_name,provider_account_id,display_name,account_type,base_currency,status,capabilities) VALUES($1,$2,$3,'coinbase',$4,'Synthetic account','crypto','USD','active','{}')`, id, f.userID, f.financialID, "fixture:"+id)
	}
	exec(`INSERT INTO capital_buckets(id,user_id,financial_account_id,name,allocation_type,allocation_value,currency,protected_amount,status) VALUES($1,$2,$3,'Test budget','FIXED_AMOUNT',1000,'USD',0,'ACTIVE')`, f.bucketID, f.userID, f.accountID)
	schedule := json.RawMessage(`{"enabled":true,"interval_minutes":60,"session":"CONTINUOUS"}`)
	exec(`INSERT INTO automation_mandates(id,user_id,financial_account_id,automation_type,ai_provider_connection_id,ai_model_id,capital_bucket_id,autonomy_level,execution_mode,status,current_version,strategy_parameters,risk_parameters,allowed_universe,prohibited_universe,margin_allowed,options_allowed,schedule_conditions,capability_unverified) VALUES($1,$2,$3,'AI_AUTONOMOUS',$4,'gpt-5.6-sol',$5,'FULL_AUTONOMOUS',$6,'READY',1,'{"objective":"Simulation only.","max_proposal_notional":"100"}','{}','{"symbols":["BTC"]}','{"symbols":[]}',false,false,$7,false)`, mandateID, f.userID, f.accountID, f.aiID, f.bucketID, mode, schedule)
	exec(`INSERT INTO automation_mandate_versions(mandate_id,version_number,created_by_user_id,source,snapshot,change_summary) SELECT id,1,user_id,'UI',to_jsonb(m),'{}' FROM automation_mandates m WHERE id=$1`, mandateID)
	f.mandate = automation.Mandate{ID: mandateID, UserID: f.userID, FinancialAccountID: f.accountID, CapitalBucketID: f.bucketID, AIProviderConnectionID: &f.aiID, AutomationType: "AI_AUTONOMOUS", ExecutionMode: string(mode), Status: "READY", CurrentVersion: 1, ScheduleConditions: schedule}
	return f
}

func (f initializeConnectionFixture) initialize(ctx context.Context) error {
	_, err := NewPostgresStore(f.pool).Initialize(ctx, f.userID, f.mandate, "1000", AIMonitoring)
	return err
}

func (f initializeConnectionFixture) syncAccounts(ctx context.Context, retainTarget bool) error {
	ids := []string{f.siblingID}
	if retainTarget {
		ids = append(ids, f.accountID)
	}
	accounts := make([]financial.FinancialAccount, 0, len(ids))
	for _, id := range ids {
		accounts = append(accounts, financial.FinancialAccount{Provider: "coinbase", ProviderAccountID: "fixture:" + id, DisplayName: "Synthetic refreshed account", AccountType: "crypto", BaseCurrency: "USD", Capabilities: financial.Capabilities{}})
	}
	return financialconnection.NewPostgresStore(f.pool).SyncAccounts(ctx, f.userID, f.financialID, accounts)
}

func (f initializeConnectionFixture) providerID(category string) string {
	if category == "financial" {
		return f.financialID
	}
	return f.aiID
}

func initializeUnavailableStatus(category string) string {
	if category == "financial" {
		return "error"
	}
	return "disabled"
}

func (f initializeConnectionFixture) assertDisabledProvider(t *testing.T, category string) {
	t.Helper()
	assertCount(t, f.pool, `SELECT count(*) FROM provider_connections WHERE user_id='`+f.userID+`' AND id='`+f.providerID(category)+`' AND provider_category='`+category+`' AND status='`+initializeUnavailableStatus(category)+`'`, 1)
	assertCount(t, f.pool, `SELECT count(*) FROM provider_connections WHERE user_id='`+f.userID+`' AND id<>'`+f.providerID(category)+`' AND status='active'`, 1)
	assertCount(t, f.pool, `SELECT count(*) FROM financial_accounts WHERE user_id='`+f.userID+`' AND status='active'`, 2)
}

func (f initializeConnectionFixture) assertSync(t *testing.T, retained bool) {
	t.Helper()
	status, count := "unavailable", 1
	if retained {
		status, count = "active", 2
	}
	assertCount(t, f.pool, `SELECT count(*) FROM financial_accounts WHERE id='`+f.accountID+`' AND user_id='`+f.userID+`' AND provider_connection_id='`+f.financialID+`' AND status='`+status+`'`, 1)
	assertCount(t, f.pool, `SELECT count(*) FROM financial_accounts WHERE id='`+f.siblingID+`' AND user_id='`+f.userID+`' AND provider_connection_id='`+f.financialID+`' AND status='active'`, 1)
	assertCount(t, f.pool, `SELECT count(*) FROM financial_accounts WHERE user_id='`+f.userID+`'`, 2)
	assertCount(t, f.pool, `SELECT count(*) FROM financial_account_sync_operations WHERE user_id='`+f.userID+`' AND provider_connection_id='`+f.financialID+`' AND source_operation='PROVIDER_ACCOUNT_DISCOVERY' AND outcome='SAVED'`, 1)
	assertCount(t, f.pool, `SELECT count(*) FROM financial_account_sync_checkpoints WHERE user_id='`+f.userID+`' AND provider_connection_id='`+f.financialID+`'`, count)
	assertCount(t, f.pool, `SELECT count(*) FROM provider_connections WHERE user_id='`+f.userID+`' AND status='active'`, 2)
}

func (f initializeConnectionFixture) assertArtifacts(t *testing.T, want int) {
	t.Helper()
	owner := ` WHERE user_id='` + f.userID + `'`
	for _, table := range []string{"strategy_instances", "strategy_capital_reservations", "nonlive_strategy_schedules"} {
		assertCount(t, f.pool, `SELECT count(*) FROM `+table+owner, want)
	}
	for _, table := range []string{"ai_paper_spot_fills", "nonlive_execution_records", "decision_journal_entries", "risk_evaluations", "order_intents", "nonlive_schedule_runs"} {
		assertCount(t, f.pool, `SELECT count(*) FROM `+table+owner, 0)
	}
	instances := `SELECT id FROM strategy_instances` + owner
	assertCount(t, f.pool, `SELECT count(*) FROM strategy_evaluation_events WHERE strategy_instance_id IN (`+instances+`)`, 0)
	assertCount(t, f.pool, `SELECT count(*) FROM strategy_state_transitions WHERE strategy_instance_id IN (`+instances+`)`, want)
	assertCount(t, f.pool, `SELECT count(*) FROM paper_positions WHERE paper_portfolio_id IN (SELECT id FROM paper_portfolios`+owner+`)`, 0)
	paperCount := 0
	if f.mandate.ExecutionMode == string(Paper) {
		paperCount = want
	}
	assertCount(t, f.pool, `SELECT count(*) FROM paper_portfolios`+owner, paperCount)
	assertCount(t, f.pool, `SELECT count(*) FROM capital_buckets`+owner+` AND id='`+f.bucketID+`' AND financial_account_id='`+f.accountID+`' AND allocation_type='FIXED_AMOUNT' AND allocation_value=1000 AND protected_amount=0 AND currency='USD' AND status='ACTIVE'`, 1)
	assertCount(t, f.pool, `SELECT count(*) FROM automation_mandates`+owner+` AND id='`+f.mandate.ID+`' AND status='READY' AND current_version=1`, 1)
	assertCount(t, f.pool, `SELECT count(*) FROM automation_mandate_versions WHERE mandate_id='`+f.mandate.ID+`'`, 1)
	if want == 0 {
		return
	}
	assertCount(t, f.pool, `SELECT count(*) FROM strategy_instances`+owner+` AND automation_mandate_id='`+f.mandate.ID+`' AND mandate_version=1 AND financial_account_id='`+f.accountID+`' AND capital_bucket_id='`+f.bucketID+`' AND strategy_identifier='ai_shadow' AND strategy_definition_version=1 AND execution_mode='`+f.mandate.ExecutionMode+`' AND current_state='AI_MONITORING' AND state_version=1 AND status='ACTIVE'`, 1)
	basis := "BUCKET_FIXED_CAPACITY"
	if f.mandate.ExecutionMode == string(Paper) {
		basis = "PAPER_STARTING_CASH"
		assertCount(t, f.pool, `SELECT count(*) FROM paper_portfolios`+owner+` AND strategy_instance_id IN (`+instances+`) AND currency='USD' AND starting_cash=1000 AND cash=1000 AND version=1`, 1)
	}
	assertCount(t, f.pool, `SELECT count(*) FROM strategy_capital_reservations`+owner+` AND strategy_instance_id IN (`+instances+`) AND financial_account_id='`+f.accountID+`' AND capital_bucket_id='`+f.bucketID+`' AND execution_mode='`+f.mandate.ExecutionMode+`' AND reservation_amount=1000 AND currency='USD' AND reservation_basis='`+basis+`' AND released_at IS NULL`, 1)
	assertCount(t, f.pool, `SELECT count(*) FROM nonlive_strategy_schedules`+owner+` AND strategy_instance_id IN (`+instances+`) AND mandate_id='`+f.mandate.ID+`' AND mandate_version=1 AND interval_minutes=60 AND session='CONTINUOUS' AND next_run_at>created_at AND last_status IS NULL AND consecutive_failures=0`, 1)
	assertCount(t, f.pool, `SELECT count(*) FROM strategy_state_transitions WHERE strategy_instance_id IN (`+instances+`) AND trigger='INITIALIZED' AND state_version=1 AND previous_state='AI_MONITORING' AND new_state='AI_MONITORING'`, 1)
}
