package automation

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/arbion/platform/services/api/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func TestPostgresAccountFactsReadsProviderName(t *testing.T) {
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
		userID       = "f1111111-1111-4111-8111-111111111111"
		connectionID = "f2222222-2222-4222-8222-222222222222"
		accountID    = "f3333333-3333-4333-8333-333333333333"
	)
	if _, err = pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, userID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id=$1`, userID) })
	statements := []string{
		`INSERT INTO users(id,email,normalized_email,display_name,email_verified_at) VALUES('` + userID + `','automation-account-facts@example.com','automation-account-facts@example.com','Automation Account Facts',now())`,
		`INSERT INTO provider_connections(id,user_id,provider_category,provider_name,display_name,status) VALUES('` + connectionID + `','` + userID + `','financial','coinbase','Coinbase','active')`,
		`INSERT INTO financial_accounts(id,user_id,provider_connection_id,provider_name,provider_account_id,display_name,account_type,base_currency,status,capabilities) VALUES('` + accountID + `','` + userID + `','` + connectionID + `','coinbase','portfolio:test','Coinbase Test','digital_asset_portfolio','USD','active','{"options":"UNSUPPORTED","margin":"UNSUPPORTED"}')`,
	}
	for _, statement := range statements {
		if _, err = pool.Exec(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}

	facts, err := NewPostgresStore(pool).AccountFacts(ctx, userID, accountID)
	if err != nil {
		t.Fatal(err)
	}
	if !facts.Owned || facts.Provider != "coinbase" || facts.Options != "UNSUPPORTED" || facts.Margin != "UNSUPPORTED" {
		t.Fatalf("financial account facts were not loaded from provider_name and capabilities: %#v", facts)
	}
}
