package orderintent

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/financial"
	"github.com/arbion/platform/services/api/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func TestPostgresOrderIntentIsIdempotentOwnerScopedImmutableAndNonExecuting(t *testing.T) {
	databaseURL := os.Getenv("STRATEGY_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("STRATEGY_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	database, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	goose.SetBaseFS(migrations.Files)
	if err = goose.SetDialect("postgres"); err != nil {
		t.Fatal(err)
	}
	if err = goose.UpContext(ctx, database, "."); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	unique := time.Now().UTC().UnixNano()
	var userID, otherUserID, connectionID, accountID string
	if err = pool.QueryRow(ctx, `INSERT INTO users(external_id) VALUES($1) RETURNING id::text`, fmt.Sprintf("order-intent-owner-%d", unique)).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `INSERT INTO users(external_id) VALUES($1) RETURNING id::text`, fmt.Sprintf("order-intent-other-%d", unique)).Scan(&otherUserID); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `INSERT INTO provider_connections(user_id,provider_category,provider_name,display_name,status) VALUES($1,'financial','coinbase','Coinbase','active') RETURNING id::text`, userID).Scan(&connectionID); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `INSERT INTO financial_accounts(user_id,provider_connection_id,provider_name,provider_account_id,display_name,account_type,base_currency,status,capabilities) VALUES($1,$2,'coinbase',$3,'Coinbase Portfolio','crypto','USD','active','{"order_preview":"SUPPORTED","provider_trade_authorization":"SUPPORTED","transfers":"UNSUPPORTED"}') RETURNING id::text`, userID, connectionID, fmt.Sprintf("portfolio:%d", unique)).Scan(&accountID); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC().Add(time.Second)
	requestDigest := sha256.Sum256([]byte("request"))
	evidenceDigest := sha256.Sum256([]byte("evidence"))
	input := draft{
		UserID: userID, IdempotencyKey: fmt.Sprintf("intent-key-%d", unique), RequestHash: requestDigest[:], EvidenceHash: evidenceDigest[:],
		Intent: Intent{
			FinancialAccountID: accountID, Source: SourceUI, Provider: "coinbase", ProductID: "BTC-USD", BaseAsset: "BTC", QuoteCurrency: "USD", Side: "BUY", OrderType: "MARKET_IOC",
			RequestedSize: financial.Money{Amount: "25.50", Currency: "USD"}, Status: ReviewRequired, Version: 1,
			Preview:   PreviewEvidence{Provider: "coinbase", Feed: "advanced_trade_order_preview", PreviewState: "READY", BaseSize: "0.0004249", QuoteSize: "25.50", OrderTotal: financial.Money{Amount: "25.50", Currency: "USD"}, CommissionTotal: financial.Money{Amount: "0.15", Currency: "USD"}, ProviderTradingAuthorized: true, BlockReasons: []string{}, Warnings: []string{"SMALL_ORDER"}, PreviewedAt: now, ExpiresAt: now.Add(time.Minute)},
			CreatedAt: now, UpdatedAt: now,
		},
	}
	store := NewPostgresStore(pool)
	created, storedHash, err := store.Create(ctx, input)
	if err != nil || created.ID == "" || created.Status != ReviewRequired || created.Version != 1 || created.RequestedSize.Amount != "25.5" || !equalHash(storedHash, requestDigest[:]) {
		t.Fatalf("unexpected stored intent: %#v hash=%x err=%v", created, storedHash, err)
	}
	replayed, replayHash, err := store.Create(ctx, input)
	if err != nil || replayed.ID != created.ID || !equalHash(replayHash, requestDigest[:]) {
		t.Fatalf("idempotent store replay failed: %#v %x %v", replayed, replayHash, err)
	}
	changed := input
	changedDigest := sha256.Sum256([]byte("changed"))
	changed.RequestHash = changedDigest[:]
	if _, _, err = store.Create(ctx, changed); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("changed idempotency payload returned %v", err)
	}
	if _, _, err = store.Get(ctx, otherUserID, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-owner intent read returned %v", err)
	}
	reviewedAt := now.Add(10 * time.Second)
	reviewed, err := store.Review(ctx, reviewEvidence{IntentID: created.ID, UserID: userID, ExpectedVersion: 1, EvidenceHash: evidenceDigest[:], MFAMethod: "totp", ReviewedAt: reviewedAt})
	if err != nil || reviewed.Status != UserApprovedNonExecutable || reviewed.Version != 2 || reviewed.SubmissionAvailable || reviewed.RiskApprovalAvailable || reviewed.LiveExecutionAvailable {
		t.Fatalf("non-executing review failed: %#v %v", reviewed, err)
	}
	if _, err = store.Review(ctx, reviewEvidence{IntentID: created.ID, UserID: userID, ExpectedVersion: 1, EvidenceHash: evidenceDigest[:], MFAMethod: "totp", ReviewedAt: reviewedAt}); !errors.Is(err, ErrConflict) {
		t.Fatalf("review replay returned %v", err)
	}
	if _, err = pool.Exec(ctx, `UPDATE order_intents SET product_id='ETH-USD',updated_at=now()+interval '1 second' WHERE id=$1`, created.ID); err == nil {
		t.Fatal("immutable order intent proposal was modified")
	}
	if _, err = pool.Exec(ctx, `DELETE FROM order_intent_previews WHERE order_intent_id=$1`, created.ID); err == nil {
		t.Fatal("immutable preview evidence was deleted")
	}
	_, err = pool.Exec(ctx, `INSERT INTO order_intents(user_id,financial_account_id,source,idempotency_key,request_hash,provider_name,product_id,base_asset,quote_currency,side,order_type,requested_size,requested_size_currency,status,version) VALUES($1,$2,'UI',$3,$4,'coinbase','BTC-USD','BTC','USD','BUY','MARKET_IOC',1,'USD','USER_APPROVED_NONEXECUTABLE',2)`, userID, accountID, fmt.Sprintf("unsafe-intent-%d", unique), requestDigest[:])
	if err == nil {
		t.Fatal("order intent was inserted directly into an approved state")
	}
	var reviews, events, attempts int
	if err = pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM order_intent_reviews WHERE order_intent_id=$1),(SELECT count(*) FROM order_intent_events WHERE order_intent_id=$1),(SELECT count(*) FROM information_schema.tables WHERE table_schema='public' AND table_name IN ('provider_orders','order_execution_attempts'))`, created.ID).Scan(&reviews, &events, &attempts); err != nil || reviews != 1 || events != 2 || attempts != 0 {
		t.Fatalf("unexpected evidence or execution tables: reviews=%d events=%d attempts=%d err=%v", reviews, events, attempts, err)
	}
}
