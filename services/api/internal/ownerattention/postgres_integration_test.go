package ownerattention

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func TestPostgresAttentionIsOwnerScopedCurrentAndCredentialFree(t *testing.T) {
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
	var ownerID, foreignID string
	if err = pool.QueryRow(ctx, `INSERT INTO users(email,normalized_email,display_name,status) VALUES($1,$1,'Attention Owner','active') RETURNING id::text`, fmt.Sprintf("attention-owner-%d@example.com", unique)).Scan(&ownerID); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `INSERT INTO users(email,normalized_email,display_name,status) VALUES($1,$1,'Foreign Owner','active') RETURNING id::text`, fmt.Sprintf("attention-foreign-%d@example.com", unique)).Scan(&foreignID); err != nil {
		t.Fatal(err)
	}

	var financialConnectionID, aiConnectionID, accountID string
	if err = pool.QueryRow(ctx, `INSERT INTO provider_connections(user_id,provider_category,provider_name,display_name,status,encrypted_credential_payload,credential_metadata) VALUES($1,'financial','coinbase','Owner Coinbase','error',decode('736563726574','hex'),'{"private":"never return"}') RETURNING id::text`, ownerID).Scan(&financialConnectionID); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `INSERT INTO provider_connections(user_id,provider_category,provider_name,display_name,status,encrypted_credential_payload,credential_metadata) VALUES($1,'ai','openai','Owner AI','expired',decode('746f6b656e','hex'),'{"provider_response":"never return"}') RETURNING id::text`, ownerID).Scan(&aiConnectionID); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `INSERT INTO financial_accounts(user_id,provider_connection_id,provider_name,provider_account_id,display_name,account_type,base_currency,status,capabilities) VALUES($1,$2,'coinbase',$3,'Private portfolio name','digital_asset_portfolio','USD','unavailable','{}') RETURNING id::text`, ownerID, financialConnectionID, fmt.Sprintf("attention-owner-%d", unique)).Scan(&accountID); err != nil {
		t.Fatal(err)
	}

	var bucketID, mandateID, instanceID string
	if err = pool.QueryRow(ctx, `INSERT INTO capital_buckets(user_id,financial_account_id,name,allocation_type,allocation_value,currency,protected_amount,status) VALUES($1,$2,'Attention test bucket','FIXED_AMOUNT',1,'USD',0,'ACTIVE') RETURNING id::text`, ownerID, accountID).Scan(&bucketID); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `INSERT INTO automation_mandates(user_id,financial_account_id,automation_type,strategy_identifier,capital_bucket_id,autonomy_level,execution_mode,status,current_version,strategy_parameters,risk_parameters,allowed_universe,prohibited_universe,margin_allowed,options_allowed,schedule_conditions,capability_unverified) VALUES($1,$2,'STRATEGY','wheel',$3,'STRATEGY_AUTONOMOUS','SHADOW','READY',1,'{}','{}','{"symbols":["PRIVATE"],"universe_ids":[]}','{"symbols":[]}',false,false,'{"enabled":true,"interval_minutes":60,"session":"US_EQUITIES_REGULAR"}',false) RETURNING id::text`, ownerID, accountID, bucketID).Scan(&mandateID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO automation_mandate_versions(mandate_id,version_number,created_by_user_id,source,snapshot,change_summary) SELECT id,1,user_id,'SYSTEM',to_jsonb(m) || '{"execution_capable":false}'::jsonb,'{}'::jsonb FROM automation_mandates m WHERE id=$1`, mandateID); err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.QueryRow(ctx, `INSERT INTO strategy_instances(user_id,automation_mandate_id,mandate_version,financial_account_id,capital_bucket_id,strategy_identifier,execution_mode,current_state,status) VALUES($1,$2,1,$3,$4,'wheel','SHADOW','MONITORING','ACTIVE') RETURNING id::text`, ownerID, mandateID, accountID, bucketID).Scan(&instanceID); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO strategy_capital_reservations(user_id,financial_account_id,capital_bucket_id,strategy_instance_id,execution_mode,reservation_amount,currency,reservation_basis,reserved_at) SELECT $1,$2,$3,$4,'SHADOW',1,'USD','BUCKET_FIXED_CAPACITY',started_at FROM strategy_instances WHERE id=$4`, ownerID, accountID, bucketID, instanceID); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO nonlive_strategy_schedules(strategy_instance_id,user_id,mandate_id,mandate_version,interval_minutes,session,next_run_at,last_completed_at,last_status,last_error_code,consecutive_failures) VALUES($1,$2,$3,1,60,'US_EQUITIES_REGULAR',now()+interval '1 hour',now(),'FAILED','SAFE_TEST_FAILURE',2)`, instanceID, ownerID, mandateID); err != nil {
		t.Fatal(err)
	}

	var reconciliationID, ownerBreakerID string
	if err = pool.QueryRow(ctx, `INSERT INTO portfolio_reconciliations(user_id,financial_account_id,provider_name,comparison_status,balances_status,positions_status,performance_status,realized_performance_status,autonomy_signal,autonomy_enforcement_active,blocks_new_actions,observed_position_count,performance_position_count,change_count,blocking_change_count,changes,evidence_hash,observed_at) VALUES($1,$2,'coinbase','DRIFT_DETECTED','READY','READY','UNAVAILABLE','UNAVAILABLE','REVIEW_RECOMMENDED',true,true,0,0,1,1,'[{"symbol":"PRIVATE","instrument_type":"CRYPTO","direction":"long","change_type":"POSITION_APPEARED","control_impact":"TRADABLE_INVENTORY","current_quantity":"98765432101234567890"}]',decode(repeat('ab',32),'hex'),now()) RETURNING id::text`, ownerID, accountID).Scan(&reconciliationID); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `INSERT INTO risk_circuit_breakers(scope,scope_id,state,reason,source,engaged_by_user_id) VALUES('USER',$1,'OPEN','private stop reason','SYSTEM',$1) RETURNING id::text`, ownerID).Scan(&ownerBreakerID); err != nil {
		t.Fatal(err)
	}

	var foreignConnectionID, foreignAccountID, foreignBreakerID string
	if err = pool.QueryRow(ctx, `INSERT INTO provider_connections(user_id,provider_category,provider_name,display_name,status,encrypted_credential_payload) VALUES($1,'financial','coinbase','Foreign Coinbase','revoked',decode('666f726569676e','hex')) RETURNING id::text`, foreignID).Scan(&foreignConnectionID); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `INSERT INTO financial_accounts(user_id,provider_connection_id,provider_name,provider_account_id,display_name,account_type,base_currency,status,capabilities) VALUES($1,$2,'coinbase',$3,'Foreign private portfolio','digital_asset_portfolio','USD','unavailable','{}') RETURNING id::text`, foreignID, foreignConnectionID, fmt.Sprintf("attention-foreign-%d", unique)).Scan(&foreignAccountID); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `INSERT INTO risk_circuit_breakers(scope,scope_id,state,reason,source,engaged_by_user_id) VALUES('USER',$1,'OPEN','foreign private reason','SYSTEM',$1) RETURNING id::text`, foreignID).Scan(&foreignBreakerID); err != nil {
		t.Fatal(err)
	}

	items, err := NewPostgresStore(pool).Items(ctx, ownerID, 50)
	if err != nil {
		t.Fatalf("attention query did not compile against the migrated schema: %v", err)
	}
	if len(items) != 6 || items[0].Code != "OWNER_SAFETY_STOP" || items[0].Severity != SeverityStopped {
		t.Fatalf("unexpected active attention projection: %#v", items)
	}
	wantCodes := map[string]bool{
		"SCHEDULE_FAILURE":                false,
		"PORTFOLIO_DRIFT_REVIEW_REQUIRED": false,
		"AI_CONNECTION_ATTENTION":         false,
		"FINANCIAL_CONNECTION_ATTENTION":  false,
		"FINANCIAL_ACCOUNT_UNAVAILABLE":   false,
		"OWNER_SAFETY_STOP":               false,
	}
	foreignIDs := map[string]bool{foreignConnectionID: true, foreignAccountID: true, foreignBreakerID: true}
	for _, item := range items {
		if _, known := wantCodes[item.Code]; !known {
			t.Fatalf("unexpected attention code: %#v", item)
		}
		wantCodes[item.Code] = true
		if foreignIDs[item.ID] {
			t.Fatalf("foreign owner condition crossed the attention boundary: %#v", item)
		}
	}
	for code, found := range wantCodes {
		if !found {
			t.Fatalf("active attention code %s was missing: %#v", code, items)
		}
	}
	if _, err = pool.Exec(ctx, `UPDATE provider_connections SET provider_name='schwab' WHERE id=$1`, financialConnectionID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE financial_accounts SET provider_name='schwab' WHERE id=$1`, accountID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE nonlive_strategy_schedules SET last_error_code='MARKET_DATA_REALTIME_UNCONFIRMED' WHERE strategy_instance_id=$1`, instanceID); err != nil {
		t.Fatal(err)
	}
	classifiedItems, err := NewPostgresStore(pool).Items(ctx, ownerID, 50)
	if err != nil {
		t.Fatalf("Schwab attention classification did not compile: %v", err)
	}
	var schwabAttentionFound bool
	for _, item := range classifiedItems {
		if item.ID == instanceID && item.Code == "SCHWAB_MARKET_DATA_ATTENTION" {
			schwabAttentionFound = true
		}
	}
	if !schwabAttentionFound {
		t.Fatalf("Schwab market-data attention was not classified exactly: %#v", classifiedItems)
	}
	projectedJSON, err := json.Marshal(items)
	if err != nil {
		t.Fatal(err)
	}
	projected := string(projectedJSON)
	for _, privateValue := range []string{"PRIVATE", "98765432101234567890", "secret", "token", "private stop reason", "Owner Coinbase", "OpenAI"} {
		if strings.Contains(projected, privateValue) {
			t.Fatalf("private value escaped the bounded projection: %q in %s", privateValue, projected)
		}
	}
	if !containsAttentionID(items, instanceID) || !containsAttentionID(items, reconciliationID) || !containsAttentionID(items, ownerBreakerID) || !containsAttentionID(items, financialConnectionID) || !containsAttentionID(items, aiConnectionID) || !containsAttentionID(items, accountID) {
		t.Fatalf("owner resources were not represented exactly: %#v", items)
	}
}

func containsAttentionID(items []Item, id string) bool {
	for _, item := range items {
		if item.ID == id {
			return true
		}
	}
	return false
}
