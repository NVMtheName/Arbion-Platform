package aiconnection

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/credential"
	"github.com/arbion/platform/services/api/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func TestPostgresAIConnectionDependenciesTrackPreferencesMandatesAndPinnedRuntime(t *testing.T) {
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
		userID                = "a1111111-1111-4111-8111-111111111111"
		aiConnectionA         = "a2222222-2222-4222-8222-222222222222"
		aiConnectionB         = "a3333333-3333-4333-8333-333333333333"
		financialConnectionID = "a4444444-4444-4444-8444-444444444444"
		financialAccountID    = "a5555555-5555-4555-8555-555555555555"
		capitalBucketID       = "a6666666-6666-4666-8666-666666666666"
		mandateID             = "a7777777-7777-4777-8777-777777777777"
		instanceID            = "a8888888-8888-4888-8888-888888888888"
		foreignUserID         = "b1111111-1111-4111-8111-111111111111"
	)
	statements := []string{
		`INSERT INTO users(id,email,normalized_email,display_name,email_verified_at) VALUES('` + userID + `','ai-lifecycle@example.com','ai-lifecycle@example.com','AI Lifecycle',now())`,
		`INSERT INTO users(id,email,normalized_email,display_name,email_verified_at) VALUES('` + foreignUserID + `','ai-lifecycle-foreign@example.com','ai-lifecycle-foreign@example.com','Foreign AI Lifecycle',now())`,
		`INSERT INTO provider_connections(id,user_id,provider_category,provider_name,display_name,status) VALUES('` + aiConnectionA + `','` + userID + `','ai','openai','OpenAI A','active')`,
		`INSERT INTO provider_connections(id,user_id,provider_category,provider_name,display_name,status) VALUES('` + aiConnectionB + `','` + userID + `','ai','openai','OpenAI B','active')`,
		`INSERT INTO provider_connections(id,user_id,provider_category,provider_name,display_name,status) VALUES('` + financialConnectionID + `','` + userID + `','financial','coinbase','Coinbase','active')`,
		`INSERT INTO financial_accounts(id,user_id,provider_connection_id,provider_name,provider_account_id,display_name,account_type,base_currency,status,capabilities) VALUES('` + financialAccountID + `','` + userID + `','` + financialConnectionID + `','coinbase','portfolio:ai-lifecycle','Coinbase AI Lifecycle','digital_asset_portfolio','USD','active','{}')`,
		`INSERT INTO capital_buckets(id,user_id,financial_account_id,name,allocation_type,allocation_value,currency,protected_amount,status) VALUES('` + capitalBucketID + `','` + userID + `','` + financialAccountID + `','AI lifecycle shadow budget','FIXED_AMOUNT',1000,'USD',0,'ACTIVE')`,
	}
	for _, statement := range statements {
		if _, err = pool.Exec(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	store := NewPostgresStore(pool, DefaultRegistry())
	assertAIConnectionDependency(t, store, userID, aiConnectionA, false, false)
	assertAIConnectionProjection(t, store, userID, aiConnectionA, false, false, 0, 0, 0, false)
	if _, err = pool.Exec(ctx, `UPDATE provider_connections SET encrypted_credential_payload=$2 WHERE id=$1`, aiConnectionA, []byte("current-ciphertext")); err != nil {
		t.Fatal(err)
	}
	credentialStore := credential.NewPostgresStore(pool)
	locator := credential.Locator{ConnectionID: aiConnectionA, UserID: userID, Class: credential.AI}
	candidateCiphertext := bytes.Repeat([]byte{0x5a}, 32)
	stagingToken := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if err = credentialStore.PutStaged(ctx, locator, candidateCiphertext, stagingToken); err != nil {
		t.Fatal(err)
	}
	rotated, err := store.CommitStagedCredential(ctx, userID, aiConnectionA, stagingToken, "••••••••test", "active", "active", 1, true)
	if err != nil || rotated.Status != "active" || rotated.CredentialGeneration != 2 || rotated.LastVerifiedAt == nil || rotated.CredentialHint != "••••••••test" {
		t.Fatalf("staged credential did not activate atomically: %#v %v", rotated, err)
	}
	var currentCiphertext, pendingCiphertext []byte
	var pendingToken *string
	if err = pool.QueryRow(ctx, `SELECT encrypted_credential_payload,pending_encrypted_credential_payload,pending_credential_token FROM provider_connections WHERE id=$1`, aiConnectionA).Scan(&currentCiphertext, &pendingCiphertext, &pendingToken); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(currentCiphertext, candidateCiphertext) || pendingCiphertext != nil || pendingToken != nil {
		t.Fatal("atomic activation did not promote and clear the exact staged candidate")
	}
	if _, err = store.CommitStagedCredential(ctx, userID, aiConnectionA, stagingToken, "••••••••stale", "active", "active", 1, true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("stale generation or staging token activated a credential: %v", err)
	}

	if _, err = pool.Exec(ctx, `INSERT INTO neural_engine_preferences(user_id,provider_connection_id,model_id) VALUES($1,$2,'gpt-5.6-sol')`, userID, aiConnectionA); err != nil {
		t.Fatal(err)
	}
	assertAIConnectionDependency(t, store, userID, aiConnectionA, true, false)
	assertAIConnectionProjection(t, store, userID, aiConnectionA, false, true, 0, 0, 0, true)
	if _, err = pool.Exec(ctx, `DELETE FROM neural_engine_preferences WHERE user_id=$1`, userID); err != nil {
		t.Fatal(err)
	}

	if _, err = pool.Exec(ctx, `INSERT INTO automation_mandates(
		id,user_id,financial_account_id,automation_type,ai_provider_connection_id,ai_model_id,capital_bucket_id,
		autonomy_level,execution_mode,status,current_version,strategy_parameters,risk_parameters,allowed_universe,
		prohibited_universe,margin_allowed,options_allowed,schedule_conditions,capability_unverified
	) VALUES($1,$2,$3,'AI_AUTONOMOUS',$4,'gpt-5.6-sol',$5,'FULL_AUTONOMOUS','SHADOW','DRAFT',1,
		'{"objective":"Protect connection continuity.","max_proposal_notional":"1000","max_trades_per_day":1}',
		'{}','{"symbols":["BTC"],"universe_ids":[]}','{"symbols":[]}',false,false,
		'{"enabled":true,"interval_minutes":60,"session":"CONTINUOUS"}',false)`, mandateID, userID, financialAccountID, aiConnectionA, capitalBucketID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO automation_mandate_versions(mandate_id,version_number,created_by_user_id,source,snapshot,change_summary)
		SELECT id,1,user_id,'UI',to_jsonb(m) || '{"execution_capable":false}'::jsonb,'{}'::jsonb
		FROM automation_mandates m WHERE id=$1`, mandateID); err != nil {
		t.Fatal(err)
	}
	assertAIConnectionDependency(t, store, userID, aiConnectionA, true, false)
	assertAIConnectionProjection(t, store, userID, aiConnectionA, false, true, 0, 0, 1, false)

	if _, err = pool.Exec(ctx, `UPDATE automation_mandates SET status='READY' WHERE id=$1`, mandateID); err != nil {
		t.Fatal(err)
	}
	assertAIConnectionDependency(t, store, userID, aiConnectionA, true, true)
	assertAIConnectionProjection(t, store, userID, aiConnectionA, true, true, 1, 0, 1, false)

	if _, err = pool.Exec(ctx, `UPDATE automation_mandates SET status='DRAFT',current_version=2,ai_provider_connection_id=$2 WHERE id=$1`, mandateID, aiConnectionB); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO automation_mandate_versions(mandate_id,version_number,created_by_user_id,source,snapshot,change_summary)
		SELECT id,2,user_id,'UI',to_jsonb(m) || '{"execution_capable":false}'::jsonb,'{"change":"new_ai_connection"}'::jsonb
		FROM automation_mandates m WHERE id=$1`, mandateID); err != nil {
		t.Fatal(err)
	}
	assertAIConnectionDependency(t, store, userID, aiConnectionA, true, false)
	assertAIConnectionDependency(t, store, userID, aiConnectionB, true, false)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO strategy_instances(
		id,user_id,automation_mandate_id,mandate_version,financial_account_id,capital_bucket_id,
		strategy_identifier,execution_mode,current_state,status
	) VALUES($1,$2,$3,1,$4,$5,'ai_shadow','SHADOW','AI_MONITORING','ACTIVE')`, instanceID, userID, mandateID, financialAccountID, capitalBucketID); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO strategy_capital_reservations(user_id,financial_account_id,capital_bucket_id,strategy_instance_id,execution_mode,reservation_amount,currency,reservation_basis,reserved_at) SELECT $1,$2,$3,$4,'SHADOW',1000,'USD','BUCKET_FIXED_CAPACITY',started_at FROM strategy_instances WHERE id=$4`, userID, financialAccountID, capitalBucketID, instanceID); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	assertAIConnectionDependency(t, store, userID, aiConnectionA, true, true)
	assertAIConnectionDependency(t, store, userID, aiConnectionB, true, false)
	assertAIConnectionProjection(t, store, userID, aiConnectionA, true, true, 0, 1, 1, false)
	assertAIConnectionProjection(t, store, userID, aiConnectionB, false, true, 0, 0, 1, false)
	assertAIConnectionDependency(t, store, foreignUserID, aiConnectionA, false, false)

	completedAt := time.Now().UTC().Add(time.Second).Truncate(time.Microsecond)
	tx, err = pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `UPDATE strategy_instances SET status='COMPLETED',completed_at=$2 WHERE id=$1`, instanceID, completedAt); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `UPDATE strategy_capital_reservations SET released_at=$2,release_reason='COMPLETED' WHERE strategy_instance_id=$1`, instanceID, completedAt); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	assertAIConnectionDependency(t, store, userID, aiConnectionA, true, false)
	if _, err = pool.Exec(ctx, `UPDATE automation_mandates SET status='READY' WHERE id=$1`, mandateID); err != nil {
		t.Fatal(err)
	}
	assertAIConnectionDependency(t, store, userID, aiConnectionB, true, true)
	assertAIConnectionProjection(t, store, userID, aiConnectionB, true, true, 1, 0, 1, false)
}

func assertAIConnectionDependency(t *testing.T, store *PostgresStore, user, connection string, wantDurable, wantInUse bool) {
	t.Helper()
	durable, err := store.HasDependencies(context.Background(), user, connection)
	if err != nil {
		t.Fatal(err)
	}
	inUse, err := store.ConnectionInUse(context.Background(), user, connection)
	if err != nil {
		t.Fatal(err)
	}
	if durable != wantDurable || inUse != wantInUse {
		t.Fatalf("dependency mismatch for %s: durable=%t/%t in_use=%t/%t", connection, durable, wantDurable, inUse, wantInUse)
	}
}

func assertAIConnectionProjection(t *testing.T, store *PostgresStore, user, connection string, wantRuntime, wantRemoval bool, wantMandates, wantStrategies, wantRetained int, wantDefault bool) {
	t.Helper()
	connections, err := store.List(context.Background(), user)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range connections {
		if item.ID != connection {
			continue
		}
		if item.RuntimeProtected != wantRuntime || item.RemovalProtected != wantRemoval || item.ProtectedMandateCount != wantMandates || item.ActiveStrategyCount != wantStrategies || item.RetainedAutomationCount != wantRetained || item.DefaultModelSelected != wantDefault {
			t.Fatalf("continuity projection mismatch for %s: %#v", connection, item)
		}
		return
	}
	t.Fatalf("connection %s was absent from continuity projection", connection)
}
