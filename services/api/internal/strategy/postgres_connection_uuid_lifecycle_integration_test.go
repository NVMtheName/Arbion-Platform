package strategy

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/arbion/platform/services/api/internal/authorization"
	"github.com/arbion/platform/services/api/internal/automation"
	"github.com/arbion/platform/services/api/internal/financialconnection"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SQL UUID identity accepts case variants; lifecycle locks must serialize that
// same identity before checking current dependencies. These tests use the real
// disable service and mandate persistence, without providers or credentials.
func testConnectionUUIDLifecycleSerialization(t *testing.T, parent context.Context, originalPool *pgxpool.Pool) {
	// Disable holds an advisory lock while its callback uses a different
	// connection. Gate, callback, READY writer, and inspector need five in total.
	config := originalPool.Config().Copy()
	if config.MaxConns < 6 {
		config.MaxConns = 6
	}
	pool, err := pgxpool.NewWithConfig(parent, config)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	for _, mode := range []ExecutionMode{Paper, Shadow} {
		for _, uppercase := range []bool{false, true} {
			spelling := "canonical"
			if uppercase {
				spelling = "uppercase"
			}
			t.Run(string(mode)+" READY first protects "+spelling+" financial connection", func(t *testing.T) {
				ctx, start := initializeRaceWorkers(t, parent)
				f := newConnectionUUIDDraftFixture(t, ctx, pool, mode)
				id := f.financialID
				if uppercase {
					id = strings.ToUpper(id)
				}
				gate, err := pool.Begin(ctx)
				if err != nil {
					t.Fatal(err)
				}
				defer gate.Rollback(ctx)
				if _, err = gate.Exec(ctx, `LOCK TABLE automation_mandate_versions IN SHARE MODE`); err != nil {
					t.Fatal(err)
				}
				ready := start(func() error {
					_, err := automation.NewPostgresStore(pool).Transition(ctx, f.userID, f.mandate.ID, f.mandate.CurrentVersion, "READY", "UI")
					return err
				})
				waitForPaperBindingLock(t, ctx, pool, gate.Conn().PgConn().PID())
				readyPID := resumeBlockedPID(t, ctx, pool, gate.Conn().PgConn().PID())
				disabled := start(func() error { return disableUUIDFixtureConnection(ctx, f, id) })
				blocked, disableErr := waitForResumeLockOrCompletion(t, ctx, pool, readyPID, disabled)
				if !blocked {
					t.Error("connection disable finished before READY committed", disableErr)
				}
				if err = gate.Commit(ctx); err != nil {
					t.Fatal(err)
				}
				readyErr := <-ready
				if blocked {
					disableErr = <-disabled
				}
				if readyErr != nil {
					t.Error("winning READY transition failed", readyErr)
				}
				if !errors.Is(disableErr, financialconnection.ErrConnectionInUse) {
					t.Error("disable failed to recheck the now-visible READY dependency", disableErr)
				}
				assertConnectionUUIDLifecycleState(t, f, "READY", 3, "active")
			})
			t.Run(string(mode)+" "+spelling+" disable first refuses READY", func(t *testing.T) {
				ctx, start := initializeRaceWorkers(t, parent)
				f := newConnectionUUIDDraftFixture(t, ctx, pool, mode)
				id := f.financialID
				if uppercase {
					id = strings.ToUpper(id)
				}
				gate, err := pool.Begin(ctx)
				if err != nil {
					t.Fatal(err)
				}
				defer gate.Rollback(ctx)
				// SHARE allows SELECT/FOR SHARE, but stops the real disable
				// UPDATE after its dependency check and advisory acquisition.
				if _, err = gate.Exec(ctx, `LOCK TABLE provider_connections IN SHARE MODE`); err != nil {
					t.Fatal(err)
				}
				disabled := start(func() error { return disableUUIDFixtureConnection(ctx, f, id) })
				waitForPaperBindingLock(t, ctx, pool, gate.Conn().PgConn().PID())
				lockPID := connectionUUIDAdvisoryOwner(t, ctx, pool, f.financialID, id)
				ready := start(func() error {
					_, err := automation.NewPostgresStore(pool).Transition(ctx, f.userID, f.mandate.ID, f.mandate.CurrentVersion, "READY", "UI")
					return err
				})
				blocked, readyErr := waitForResumeLockOrCompletion(t, ctx, pool, lockPID, ready)
				if !blocked {
					t.Error("READY bypassed an in-flight equivalent-ID disable lock", readyErr)
				}
				if err = gate.Commit(ctx); err != nil {
					t.Fatal(err)
				}
				disableErr := <-disabled
				if blocked {
					readyErr = <-ready
				}
				if disableErr != nil {
					t.Error("winning disable failed", disableErr)
				}
				if !errors.Is(readyErr, automation.ErrConflict) {
					t.Error("READY accepted a connection whose disable won", readyErr)
				}
				assertConnectionUUIDLifecycleState(t, f, "DRAFT", 2, "disabled")
			})
		}
	}
}

func newConnectionUUIDDraftFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool, mode ExecutionMode) initializeConnectionFixture {
	t.Helper()
	f := newInitializeConnectionFixture(t, ctx, pool, mode)
	// A randomly generated all-digit UUID has no distinct uppercase spelling.
	// Generate another independent fixture rather than making that rare event
	// a flaky regression test or mutating referenced identity rows.
	for strings.ToUpper(f.financialID) == f.financialID {
		f = newInitializeConnectionFixture(t, ctx, pool, mode)
	}
	store := automation.NewPostgresStore(pool)
	m, err := store.GetMandate(ctx, f.userID, f.mandate.ID)
	if err != nil {
		t.Fatal(err)
	}
	// Preserve READY v1 immutably and use the actual editing operation to make
	// a new DRAFT v2. A committed READY dependency would correctly stop disable
	// even with mismatched advisory keys, so it is not a valid race fixture.
	f.mandate, err = store.UpdateMandate(ctx, f.userID, m.ID, m.CurrentVersion, automation.MandateCommand{
		FinancialAccountID: m.FinancialAccountID, AutomationType: m.AutomationType,
		CapitalBucketID: m.CapitalBucketID, AutonomyLevel: m.AutonomyLevel, ExecutionMode: m.ExecutionMode,
		StrategyIdentifier: m.StrategyIdentifier, AIProviderConnectionID: m.AIProviderConnectionID, AIModelID: m.AIModelID,
		StrategyParameters: m.StrategyParameters, Risk: m.Risk, AllowedUniverse: m.AllowedUniverse, ProhibitedUniverse: m.ProhibitedUniverse,
		MarginAllowed: m.MarginAllowed, OptionsAllowed: m.OptionsAllowed, PaperOptionsSimulationAttested: m.PaperOptionsSimulationAttested,
		ScheduleConditions: m.ScheduleConditions, EffectiveFrom: &m.EffectiveFrom, EffectiveUntil: m.EffectiveUntil,
	}, m.CapabilityUnverified, "UI")
	if err != nil || f.mandate.Status != "DRAFT" || f.mandate.CurrentVersion != 2 {
		t.Fatal("could not prepare a fresh stored DRAFT", err)
	}
	inUse, err := financialconnection.NewPostgresStore(pool).ConnectionInUse(ctx, f.userID, f.financialID)
	if err != nil || inUse {
		t.Fatal("DRAFT fixture is unexpectedly runtime-protected", inUse, err)
	}
	return f
}

func disableUUIDFixtureConnection(ctx context.Context, f initializeConnectionFixture, id string) error {
	service := financialconnection.NewService(financialconnection.NewPostgresStore(f.pool), nil, nil, nil, nil)
	_, err := service.SetEnabled(ctx, authorization.Principal{UserID: f.userID, Entitlement: authorization.EntitlementFounder}, id, false)
	return err
}

func connectionUUIDAdvisoryOwner(t *testing.T, ctx context.Context, pool *pgxpool.Pool, canonical, supplied string) uint32 {
	t.Helper()
	var pid uint32
	// Accept either historical raw spelling or the corrected canonical key so
	// this test compiles and identifies the actual holder before and after fix.
	err := pool.QueryRow(ctx, `WITH keys AS (
		SELECT hashtextextended($1,0) AS value UNION SELECT hashtextextended($2,0)
	) SELECT DISTINCT l.pid FROM pg_locks l JOIN keys k
		ON l.classid::bigint=((k.value >> 32) & 4294967295)
		AND l.objid::bigint=(k.value & 4294967295)
		WHERE l.locktype='advisory' AND l.granted AND l.objsubid=1`, canonical, supplied).Scan(&pid)
	if err != nil {
		t.Fatal("could not identify the real lifecycle advisory owner", err)
	}
	return pid
}

func assertConnectionUUIDLifecycleState(t *testing.T, f initializeConnectionFixture, status string, version int, connectionStatus string) {
	t.Helper()
	owner := ` WHERE user_id='` + f.userID + `'`
	for _, table := range []string{"strategy_instances", "strategy_capital_reservations", "paper_portfolios", "nonlive_strategy_schedules", "nonlive_schedule_runs", "ai_paper_spot_fills", "nonlive_execution_records", "decision_journal_entries", "risk_evaluations", "order_intents"} {
		assertCount(t, f.pool, `SELECT count(*) FROM `+table+owner, 0)
	}
	assertCount(t, f.pool, `SELECT count(*) FROM automation_mandates`+owner+` AND id='`+f.mandate.ID+`' AND status='`+status+`' AND financial_account_id='`+f.accountID+`' AND capital_bucket_id='`+f.bucketID+`' AND ai_provider_connection_id='`+f.aiID+`' AND execution_mode='`+f.mandate.ExecutionMode+`'`, 1)
	assertCount(t, f.pool, `SELECT current_version FROM automation_mandates WHERE id='`+f.mandate.ID+`'`, version)
	assertCount(t, f.pool, `SELECT count(*) FROM automation_mandate_versions WHERE mandate_id='`+f.mandate.ID+`'`, version)
	assertCount(t, f.pool, `SELECT count(*) FROM automation_mandate_versions WHERE mandate_id='`+f.mandate.ID+`' AND version_number=1 AND snapshot->>'status'='READY'`, 1)
	assertCount(t, f.pool, `SELECT count(*) FROM automation_mandate_versions WHERE mandate_id='`+f.mandate.ID+`' AND version_number=2 AND snapshot->>'status'='DRAFT'`, 1)
	assertCount(t, f.pool, `SELECT count(*) FROM provider_connections`+owner+` AND id='`+f.financialID+`' AND provider_category='financial' AND status='`+connectionStatus+`'`, 1)
	assertCount(t, f.pool, `SELECT count(*) FROM provider_connections`+owner+` AND id='`+f.aiID+`' AND provider_category='ai' AND status='active'`, 1)
	assertCount(t, f.pool, `SELECT count(*) FROM financial_accounts`+owner+` AND provider_connection_id='`+f.financialID+`' AND status='active'`, 2)
	assertCount(t, f.pool, `SELECT count(*) FROM capital_buckets`+owner+` AND id='`+f.bucketID+`' AND financial_account_id='`+f.accountID+`' AND allocation_value=1000 AND protected_amount=0 AND status='ACTIVE'`, 1)
}
