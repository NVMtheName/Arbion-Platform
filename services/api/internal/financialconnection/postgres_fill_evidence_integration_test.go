package financialconnection

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/financial"
	"github.com/arbion/platform/services/api/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func TestPostgresPrivateFillEvidenceIsImmutableScopedAndDuplicateSafe(t *testing.T) {
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
	const user = "e1111111-1111-4111-8111-111111111111"
	const foreign = "e9999999-9999-4999-8999-999999999999"
	const connection = "e2222222-2222-4222-8222-222222222222"
	const account = "e3333333-3333-4333-8333-333333333333"
	const other = "e4444444-4444-4444-8444-444444444444"
	for _, statement := range []string{
		`INSERT INTO users(id,email,normalized_email,display_name,email_verified_at) VALUES('` + user + `','fill-owner@example.com','fill-owner@example.com','Fill Owner',now()),('` + foreign + `','fill-foreign@example.com','fill-foreign@example.com','Foreign',now())`,
		`INSERT INTO provider_connections(id,user_id,provider_category,provider_name,display_name,status) VALUES('` + connection + `','` + user + `','financial','coinbase','Evidence','active')`,
		`INSERT INTO financial_accounts(id,user_id,provider_connection_id,provider_name,provider_account_id,display_name,account_type,base_currency,status,capabilities) VALUES('` + account + `','` + user + `','` + connection + `','coinbase','portfolio:test-a','Evidence A','digital_asset_portfolio','USD','active','{}'),('` + other + `','` + user + `','` + connection + `','coinbase','portfolio:test-b','Evidence B','digital_asset_portfolio','USD','active','{}')`,
	} {
		if _, err = pool.Exec(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	store := NewPostgresStore(pool)
	now := time.Now().UTC().Add(-time.Second)
	page := privateTestPage("portfolio:test-a", now)
	page.Fills[0].Fill.TradeTime = now.Add(-2 * time.Minute).Truncate(time.Second).Add(123456789 * time.Nanosecond)
	if err = store.CaptureFillEvidence(ctx, user, account, page); err != nil {
		t.Fatal(err)
	}
	page.Fills[0].Fill.Price = "60000.0"
	if err = store.CaptureFillEvidence(ctx, user, account, page); err != nil {
		t.Fatal("equivalent decimal did not match", err)
	}
	var count, receipts, matched int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM financial_fill_observations WHERE financial_account_id=$1`, account).Scan(&count); err != nil || count != 1 {
		t.Fatal("duplicate observation", count, err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*),sum(matched_count) FROM financial_fill_capture_receipts WHERE financial_account_id=$1`, account).Scan(&receipts, &matched); err != nil || receipts != 2 || matched != 1 {
		t.Fatal("capture receipts do not match", receipts, matched, err)
	}
	var exactTime string
	if err = pool.QueryRow(ctx, `SELECT trade_time FROM financial_fill_observations WHERE financial_account_id=$1`, account).Scan(&exactTime); err != nil || exactTime != page.Fills[0].Fill.TradeTime.Format(time.RFC3339Nano) {
		t.Fatal("nanosecond evidence rounded", exactTime, err)
	}
	if err = store.CaptureFillEvidence(ctx, foreign, account, page); !errors.Is(err, ErrNotFound) {
		t.Fatal("foreign owner captured", err)
	}
	if err = store.CaptureFillEvidence(ctx, user, other, page); !errors.Is(err, financial.ErrInvalidFillEvidence) {
		t.Fatal("cross-account evidence captured", err)
	}
	// A conflict after a new row must roll back the entire incoming batch.
	conflict := page.Fills[0]
	conflict.Fill.Commission.Amount = "0.02"
	newFill := page.Fills[0]
	newFill.EntryReference, _ = financial.CorrelationReference("coinbase", "portfolio:test-a", "entry", "new-before-conflict")
	bad := page
	bad.Fills = []financial.FillEvidence{newFill, conflict}
	if err = store.CaptureFillEvidence(ctx, user, account, bad); !errors.Is(err, ErrFillEvidenceConflict) {
		t.Fatal("changed economic fact did not fail closed", err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM financial_fill_observations WHERE financial_account_id=$1`, account).Scan(&count); err != nil || count != 1 {
		t.Fatal("partial batch survived conflict", count, err)
	}
	for _, statement := range []string{`UPDATE financial_fill_observations SET commission=1 WHERE financial_account_id=$1`, `DELETE FROM financial_fill_observations WHERE financial_account_id=$1`, `UPDATE financial_fill_capture_receipts SET has_more=true WHERE financial_account_id=$1`, `DELETE FROM financial_fill_capture_receipts WHERE financial_account_id=$1`} {
		if _, err = pool.Exec(ctx, statement, account); err == nil {
			t.Fatal("immutable evidence mutated")
		}
	}
	// Database validation must reject unsupported precision, never silently
	// round first, and must not accept PostgreSQL numeric's NaN special value.
	for _, invalid := range []string{"NaN", "1e40", "0.000000000000000000000000000000001", "-1", "0"} {
		_, err = pool.Exec(ctx, `INSERT INTO financial_fill_observations(user_id,financial_account_id,provider_name,entry_reference,trade_reference,order_reference,evidence_digest,product_id,base_asset,quote_currency,side,price,size,size_unit,commission,liquidity,trade_time,sequence_time,observed_at)
		SELECT user_id,financial_account_id,provider_name,$2,trade_reference,order_reference,evidence_digest,product_id,base_asset,quote_currency,side,$3::numeric,size,size_unit,commission,liquidity,trade_time,sequence_time,observed_at FROM financial_fill_observations WHERE financial_account_id=$1`, account, strings.Repeat("f", 64), invalid)
		if err == nil {
			t.Fatal("database accepted inexact price", invalid)
		}
	}
	if _, err = pool.Exec(ctx, `INSERT INTO financial_fill_capture_receipts(user_id,financial_account_id,provider_name,observed_at,reported_count,unique_count,inserted_count,matched_count,has_more,complete_account_history) VALUES($1,$2,'coinbase',now(),0,0,0,0,false,true)`, user, account); err == nil {
		t.Fatal("receipt claimed complete account history")
	}
	// Different accounts retain different identities even for identical fills.
	if err = store.CaptureFillEvidence(ctx, user, other, privateTestPage("portfolio:test-b", now)); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- store.CaptureFillEvidence(ctx, user, account, page) }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal("concurrent duplicate failed", err)
		}
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM financial_fill_observations WHERE financial_account_id=$1`, account).Scan(&count); err != nil || count != 1 {
		t.Fatal("concurrent duplicate inserted", count, err)
	}
	var forbidden int
	if err = pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM order_intents WHERE user_id=$1)+(SELECT count(*) FROM strategy_instances WHERE user_id=$1)+(SELECT count(*) FROM financial_fill_capture_receipts WHERE user_id=$1 AND complete_account_history)`, user).Scan(&forbidden); err != nil || forbidden != 0 {
		t.Fatal("capture created authority or false coverage", forbidden, err)
	}
}
