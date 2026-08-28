package automation

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func TestPostgresAutomationFactsReadProviderNames(t *testing.T) {
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
		userID         = "f1111111-1111-4111-8111-111111111111"
		connectionID   = "f2222222-2222-4222-8222-222222222222"
		accountID      = "f3333333-3333-4333-8333-333333333333"
		aiConnectionID = "f4444444-4444-4444-8444-444444444444"
		bucketID       = "f5555555-5555-4555-8555-555555555555"
	)
	if _, err = pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, userID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id=$1`, userID) })
	statements := []string{
		`INSERT INTO users(id,email,normalized_email,display_name,email_verified_at) VALUES('` + userID + `','automation-account-facts@example.com','automation-account-facts@example.com','Automation Account Facts',now())`,
		`INSERT INTO provider_connections(id,user_id,provider_category,provider_name,display_name,status) VALUES('` + connectionID + `','` + userID + `','financial','coinbase','Coinbase','active')`,
		`INSERT INTO provider_connections(id,user_id,provider_category,provider_name,display_name,status) VALUES('` + aiConnectionID + `','` + userID + `','ai','openai','OpenAI','active')`,
		`INSERT INTO financial_accounts(id,user_id,provider_connection_id,provider_name,provider_account_id,display_name,account_type,base_currency,status,capabilities) VALUES('` + accountID + `','` + userID + `','` + connectionID + `','coinbase','portfolio:test','Coinbase Test','digital_asset_portfolio','USD','active','{"options":"UNSUPPORTED","margin":"UNSUPPORTED"}')`,
		`INSERT INTO capital_buckets(id,user_id,financial_account_id,name,allocation_type,allocation_value,currency,protected_amount,status) VALUES('` + bucketID + `','` + userID + `','` + accountID + `','Connection lock test','FIXED_AMOUNT',100,'USD',0,'ACTIVE')`,
	}
	for _, statement := range statements {
		if _, err = pool.Exec(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}

	store := NewPostgresStore(pool)
	facts, err := store.AccountFacts(ctx, userID, accountID)
	if err != nil {
		t.Fatal(err)
	}
	if !facts.Owned || facts.Provider != "coinbase" || facts.Options != "UNSUPPORTED" || facts.Margin != "UNSUPPORTED" {
		t.Fatalf("financial account facts were not loaded from provider_name and capabilities: %#v", facts)
	}
	aiFacts, err := store.AIFacts(ctx, userID, aiConnectionID, "gpt-5.6-sol")
	if err != nil {
		t.Fatal(err)
	}
	if !aiFacts.Owned || !aiFacts.Active || !aiFacts.ModelValid || aiFacts.Provider != "openai" {
		t.Fatalf("AI connection facts were not loaded from provider_name: %#v", aiFacts)
	}

	aiConnection := aiConnectionID
	aiModel := "gpt-5.6-sol"
	command := MandateCommand{
		FinancialAccountID:     accountID,
		AutomationType:         "AI_AUTONOMOUS",
		AIProviderConnectionID: &aiConnection,
		AIModelID:              &aiModel,
		CapitalBucketID:        bucketID,
		AutonomyLevel:          "FULL_AUTONOMOUS",
		ExecutionMode:          "SHADOW",
		StrategyParameters:     []byte(`{"objective":"Test lifecycle serialization.","max_proposal_notional":"1"}`),
		AllowedUniverse:        Universe{Symbols: []string{"BTC"}},
	}
	assertSerializedConnectionChange := func(connectionToLock string) {
		t.Helper()
		lockConnection, acquireErr := pool.Acquire(ctx)
		if acquireErr != nil {
			t.Fatal(acquireErr)
		}
		defer lockConnection.Release()
		lockTx, beginErr := lockConnection.Begin(ctx)
		if beginErr != nil {
			t.Fatal(beginErr)
		}
		defer lockTx.Rollback(ctx)
		if _, lockErr := lockTx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, connectionToLock); lockErr != nil {
			t.Fatal(lockErr)
		}
		result := make(chan error, 1)
		go func() {
			_, createErr := store.CreateMandate(context.Background(), userID, command, false)
			result <- createErr
		}()
		select {
		case createErr := <-result:
			t.Fatalf("mandate write bypassed the connection lifecycle lock: %v", createErr)
		case <-time.After(200 * time.Millisecond):
		}
		if _, updateErr := lockTx.Exec(ctx, `UPDATE provider_connections SET status='disabled' WHERE id=$1 AND user_id=$2`, connectionToLock, userID); updateErr != nil {
			t.Fatal(updateErr)
		}
		if commitErr := lockTx.Commit(ctx); commitErr != nil {
			t.Fatal(commitErr)
		}
		select {
		case createErr := <-result:
			if !errors.Is(createErr, ErrConflict) {
				t.Fatalf("stale mandate attachment did not fail closed: %v", createErr)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("serialized mandate write did not finish after lifecycle commit")
		}
		if _, resetErr := pool.Exec(ctx, `UPDATE provider_connections SET status='active' WHERE id=$1 AND user_id=$2`, connectionToLock, userID); resetErr != nil {
			t.Fatal(resetErr)
		}
	}
	assertSerializedConnectionChange(connectionID)
	assertSerializedConnectionChange(aiConnectionID)
}
