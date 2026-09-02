package financialconnection

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/financial"
	"github.com/arbion/platform/services/api/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func TestPostgresConnectionLifecycleIsAccountScoped(t *testing.T) {
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
		userID      = "c1111111-1111-4111-8111-111111111111"
		connectionA = "c2222222-2222-4222-8222-222222222222"
		connectionB = "c3333333-3333-4333-8333-333333333333"
		accountA    = "c4444444-4444-4444-8444-444444444444"
		accountB    = "c5555555-5555-4555-8555-555555555555"
		bucketB     = "c6666666-6666-4666-8666-666666666666"
		mandateB    = "c7777777-7777-4777-8777-777777777777"
		providerIDA = "portfolio:portfolio-a"
		providerIDB = "portfolio:portfolio-b"
	)
	statements := []string{
		`INSERT INTO users(id,email,normalized_email,display_name,email_verified_at) VALUES('` + userID + `','connection-isolation@example.com','connection-isolation@example.com','Connection Isolation',now())`,
		`INSERT INTO provider_connections(id,user_id,provider_category,provider_name,display_name,status) VALUES('` + connectionA + `','` + userID + `','financial','coinbase','Coinbase A','active')`,
		`INSERT INTO provider_connections(id,user_id,provider_category,provider_name,display_name,status) VALUES('` + connectionB + `','` + userID + `','financial','coinbase','Coinbase B','active')`,
		`INSERT INTO financial_accounts(id,user_id,provider_connection_id,provider_name,provider_account_id,display_name,account_type,base_currency,status,capabilities) VALUES('` + accountA + `','` + userID + `','` + connectionA + `','coinbase','` + providerIDA + `','Portfolio A','digital_asset_portfolio','USD','active','{}')`,
		`INSERT INTO financial_accounts(id,user_id,provider_connection_id,provider_name,provider_account_id,display_name,account_type,base_currency,status,capabilities) VALUES('` + accountB + `','` + userID + `','` + connectionB + `','coinbase','` + providerIDB + `','Portfolio B','digital_asset_portfolio','USD','active','{}')`,
		`INSERT INTO capital_buckets(id,user_id,financial_account_id,name,allocation_type,allocation_value,currency,status) VALUES('` + bucketB + `','` + userID + `','` + accountB + `','Protected automation','FIXED_AMOUNT',100,'USD','ACTIVE')`,
		`INSERT INTO automation_mandates(id,user_id,financial_account_id,automation_type,strategy_identifier,capital_bucket_id,autonomy_level,execution_mode,status,current_version,strategy_parameters,risk_parameters,allowed_universe,prohibited_universe,margin_allowed,options_allowed,schedule_conditions,capability_unverified) VALUES('` + mandateB + `','` + userID + `','` + accountB + `','STRATEGY','wheel','` + bucketB + `','STRATEGY_AUTONOMOUS','PAPER','READY',1,'{}','{}','{"symbols":[],"universe_ids":[]}','{"symbols":[]}',false,false,'{}',false)`,
	}
	for _, statement := range statements {
		if _, err = pool.Exec(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}

	store := NewPostgresStore(pool)
	if err = store.SyncAccounts(ctx, userID, connectionA, []financial.FinancialAccount{{Provider: "coinbase", ProviderAccountID: providerIDA, DisplayName: "Portfolio A refreshed", AccountType: "digital_asset_portfolio", BaseCurrency: "USD", Capabilities: financial.Capabilities{}}}); err != nil {
		t.Fatal(err)
	}
	checkpoints, err := store.ListAccountSyncCheckpoints(ctx, userID, accountA, 10, "")
	if err != nil || len(checkpoints) != 1 {
		t.Fatalf("first immutable sync checkpoint was not saved: %#v err=%v", checkpoints, err)
	}
	firstCheckpoint := checkpoints[0]
	if firstCheckpoint.ID == "" || firstCheckpoint.OperationID == "" || firstCheckpoint.FinancialAccountID != accountA || firstCheckpoint.ProviderConnectionID != connectionA || firstCheckpoint.Provider != "coinbase" || firstCheckpoint.SourceOperation != "PROVIDER_ACCOUNT_DISCOVERY" || firstCheckpoint.Outcome != "SAVED" || firstCheckpoint.AccountCount != 1 || firstCheckpoint.CompletedAt.Before(firstCheckpoint.ObservedAt) {
		t.Fatalf("immutable sync checkpoint identity was incomplete: %#v", firstCheckpoint)
	}
	if err = store.SyncAccounts(ctx, userID, connectionA, []financial.FinancialAccount{{Provider: "coinbase", ProviderAccountID: providerIDA, DisplayName: "Portfolio A refreshed again", AccountType: "digital_asset_portfolio", BaseCurrency: "USD", Capabilities: financial.Capabilities{}}}); err != nil {
		t.Fatal(err)
	}
	checkpoints, err = store.ListAccountSyncCheckpoints(ctx, userID, accountA, 1, "")
	if err != nil || len(checkpoints) != 1 || checkpoints[0].OperationID == firstCheckpoint.OperationID {
		t.Fatalf("second sync did not create a distinct bounded checkpoint: %#v err=%v", checkpoints, err)
	}
	newestCheckpoint := checkpoints[0]
	checkpoints, err = store.ListAccountSyncCheckpoints(ctx, userID, accountA, 2, newestCheckpoint.ID)
	if err != nil || len(checkpoints) != 1 || checkpoints[0].ID != firstCheckpoint.ID {
		t.Fatalf("sync checkpoint cursor did not return older evidence: %#v err=%v", checkpoints, err)
	}
	if _, err = store.ListAccountSyncCheckpoints(ctx, userID, accountB, 2, newestCheckpoint.ID); !errors.Is(err, ErrInvalidSyncCheckpointHistory) {
		t.Fatalf("cross-account sync checkpoint cursor was accepted: %v", err)
	}
	if _, err = pool.Exec(ctx, `UPDATE financial_account_sync_checkpoints SET outcome='SAVED' WHERE id=$1`, newestCheckpoint.ID); err == nil {
		t.Fatal("immutable financial account sync checkpoint was updateable")
	}
	if _, err = pool.Exec(ctx, `DELETE FROM financial_account_sync_operations WHERE id=$1`, newestCheckpoint.OperationID); err == nil {
		t.Fatal("immutable financial account sync operation was deleteable")
	}
	var bStatus, bName string
	if err = pool.QueryRow(ctx, `SELECT status,display_name FROM financial_accounts WHERE id=$1`, accountB).Scan(&bStatus, &bName); err != nil || bStatus != "active" || bName != "Portfolio B" {
		t.Fatalf("sync crossed connection boundary: status=%q name=%q err=%v", bStatus, bName, err)
	}

	reused, err := store.UpsertConnectionForAccount(ctx, userID, "coinbase", "replacement label", providerIDB, nil, nil)
	if err != nil || reused.ID != connectionB {
		t.Fatalf("reauthorization did not preserve provider-account identity: %#v %v", reused, err)
	}
	created, err := store.UpsertConnectionForAccount(ctx, userID, "coinbase", "Coinbase C", "portfolio:portfolio-c", nil, nil)
	if err != nil || created.ID == connectionA || created.ID == connectionB {
		t.Fatalf("a distinct provider account did not receive an isolated connection: %#v %v", created, err)
	}

	inUse, err := store.ConnectionInUse(ctx, userID, connectionB)
	if err != nil || !inUse {
		t.Fatalf("active automation did not protect its connection: in_use=%v err=%v", inUse, err)
	}
	connections, err := store.ListConnections(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	for _, connection := range connections {
		switch connection.ID {
		case connectionA:
			if connection.RuntimeProtected || connection.ProtectedMandateCount != 0 || connection.ActiveStrategyCount != 0 {
				t.Fatalf("unbound connection reported a protected dependency: %#v", connection)
			}
		case connectionB:
			if !connection.RuntimeProtected || connection.ProtectedMandateCount != 1 || connection.ActiveStrategyCount != 0 {
				t.Fatalf("protected connection continuity facts were incorrect: %#v", connection)
			}
		}
	}
	reconciliation, err := store.CreateReconciliation(ctx, userID, PortfolioReconciliation{
		FinancialAccountID: accountA, Provider: "coinbase", ComparisonStatus: "BASELINE",
		BalancesStatus: "READY", PositionsStatus: "READY", PerformanceStatus: "UNAVAILABLE",
		RealizedPerformanceStatus: "UNAVAILABLE", AutonomySignal: "INSUFFICIENT_EVIDENCE",
		AutonomyEnforcementActive: true, BlocksNewActions: true,
		ObservedPositionCount: 1, Changes: []ReconciliationChange{},
		Balances:   financial.Balances{Cash: &financial.Money{Amount: "25", Currency: "USD"}},
		Positions:  []ReconciliationPosition{{Symbol: "BTC", InstrumentType: "CRYPTO", Direction: "long", Quantity: "0.1", PerformanceStatus: "UNAVAILABLE"}},
		ObservedAt: time.Now().UTC(),
	}, make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := store.LatestReconciliation(ctx, userID, accountA)
	if err != nil || loaded.ID != reconciliation.ID || !loaded.AutonomyEnforcementActive || !loaded.BlocksNewActions || loaded.BlockingChangeCount != 0 || len(loaded.Positions) != 1 || loaded.Positions[0].Quantity != "0.100000000000000000" || loaded.Balances.Cash == nil || loaded.Balances.Cash.Amount != "25.000000000000000000" {
		t.Fatalf("immutable reconciliation evidence did not round-trip: %#v err=%v", loaded, err)
	}
	matched, err := store.CreateReconciliation(ctx, userID, PortfolioReconciliation{
		FinancialAccountID: accountA, Provider: "coinbase", ComparisonStatus: "MATCHED",
		BalancesStatus: "READY", PositionsStatus: "READY", PerformanceStatus: "UNAVAILABLE",
		RealizedPerformanceStatus: "UNAVAILABLE", AutonomySignal: "CLEAR",
		AutonomyEnforcementActive: true, BlocksNewActions: false,
		ObservedPositionCount: 1, PreviousReconciliationID: &reconciliation.ID, Changes: []ReconciliationChange{},
		Balances:   financial.Balances{Cash: &financial.Money{Amount: "25", Currency: "USD"}},
		Positions:  []ReconciliationPosition{{Symbol: "BTC", InstrumentType: "CRYPTO", Direction: "long", Quantity: "0.1", PerformanceStatus: "UNAVAILABLE"}},
		ObservedAt: reconciliation.ObservedAt.Add(time.Minute),
	}, make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	history, err := store.ListReconciliations(ctx, userID, accountA, 1, "")
	if err != nil || len(history) != 1 || history[0].ID != matched.ID || len(history[0].Positions) != 0 {
		t.Fatalf("bounded history did not return newest summary evidence: %#v err=%v", history, err)
	}
	history, err = store.ListReconciliations(ctx, userID, accountA, 2, matched.ID)
	if err != nil || len(history) != 1 || history[0].ID != reconciliation.ID {
		t.Fatalf("history cursor did not remain account-scoped: %#v err=%v", history, err)
	}
	if _, err = store.ListReconciliations(ctx, userID, accountB, 2, matched.ID); !errors.Is(err, ErrInvalidReconciliationHistory) {
		t.Fatalf("cross-account history cursor was accepted: %v", err)
	}
	if _, err = pool.Exec(ctx, `UPDATE portfolio_reconciliations SET autonomy_signal='CLEAR' WHERE id=$1`, reconciliation.ID); err == nil {
		t.Fatal("immutable portfolio reconciliation was updateable")
	}
	if _, err = pool.Exec(ctx, `DELETE FROM portfolio_reconciliation_positions WHERE reconciliation_id=$1`, reconciliation.ID); err == nil {
		t.Fatal("immutable portfolio reconciliation positions were deleteable")
	}
	if err = store.Retire(ctx, userID, connectionA); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT status FROM financial_accounts WHERE id=$1`, accountB).Scan(&bStatus); err != nil || bStatus != "active" {
		t.Fatalf("retiring one connection changed another account: status=%q err=%v", bStatus, err)
	}
}
