package liveexecution

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/migrations"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func TestPostgresRegistryIsOwnerScopedIdempotentAndImmutable(t *testing.T) {
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
	setupTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer setupTx.Rollback(ctx)

	unique := time.Now().UTC().UnixNano()
	var ownerID, foreignOwnerID string
	if err = setupTx.QueryRow(ctx, `INSERT INTO users(email,normalized_email,display_name,status) VALUES($1,$1,'Safety Owner','active') RETURNING id::text`, fmt.Sprintf("live-safety-owner-%d@example.com", unique)).Scan(&ownerID); err != nil {
		t.Fatal(err)
	}
	if err = setupTx.QueryRow(ctx, `INSERT INTO users(email,normalized_email,display_name,status) VALUES($1,$1,'Foreign Safety Owner','active') RETURNING id::text`, fmt.Sprintf("live-safety-foreign-%d@example.com", unique)).Scan(&foreignOwnerID); err != nil {
		t.Fatal(err)
	}

	var connectionID, accountID, bucketID, mandateID, instanceID, reservationID string
	if err = setupTx.QueryRow(ctx, `INSERT INTO provider_connections(user_id,provider_category,provider_name,display_name,status,credential_storage,credential_reference) VALUES($1,'financial','coinbase',$2,'active','managed_reference',$3) RETURNING id::text`, ownerID, fmt.Sprintf("Safety Coinbase %d", unique), fmt.Sprintf("safety-reference-%d", unique)).Scan(&connectionID); err != nil {
		t.Fatal(err)
	}
	if err = setupTx.QueryRow(ctx, `INSERT INTO financial_accounts(user_id,provider_connection_id,provider_name,provider_account_id,display_name,account_type,base_currency,status,capabilities) VALUES($1,$2,'coinbase',$3,'Safety portfolio','digital_asset_portfolio','USD','active','{}') RETURNING id::text`, ownerID, connectionID, fmt.Sprintf("safety-account-%d", unique)).Scan(&accountID); err != nil {
		t.Fatal(err)
	}
	if err = setupTx.QueryRow(ctx, `INSERT INTO capital_buckets(user_id,financial_account_id,name,allocation_type,allocation_value,currency,is_reserve,protected_amount,allocation_limit,status) VALUES($1,$2,'Safety evidence only','FIXED_AMOUNT',100,'USD',false,0,100,'ACTIVE') RETURNING id::text`, ownerID, accountID).Scan(&bucketID); err != nil {
		t.Fatal(err)
	}
	if err = setupTx.QueryRow(ctx, `INSERT INTO automation_mandates(user_id,financial_account_id,automation_type,strategy_identifier,capital_bucket_id,autonomy_level,execution_mode,status,current_version,strategy_parameters,risk_parameters,allowed_universe,prohibited_universe,margin_allowed,options_allowed,schedule_conditions,capability_unverified) VALUES($1,$2,'STRATEGY','wheel',$3,'STRATEGY_AUTONOMOUS','SHADOW','READY',1,'{}','{}','{"symbols":["SPY"],"universe_ids":[]}','{"symbols":[]}',false,false,'{"enabled":true,"interval_minutes":60,"session":"US_EQUITIES_REGULAR"}',false) RETURNING id::text`, ownerID, accountID, bucketID).Scan(&mandateID); err != nil {
		t.Fatal(err)
	}
	if _, err = setupTx.Exec(ctx, `INSERT INTO automation_mandate_versions(mandate_id,version_number,created_by_user_id,source,snapshot,change_summary) SELECT id,1,user_id,'SYSTEM',to_jsonb(m) || '{"execution_capable":false}'::jsonb,'{}'::jsonb FROM automation_mandates m WHERE id=$1`, mandateID); err != nil {
		t.Fatal(err)
	}
	if err = setupTx.QueryRow(ctx, `INSERT INTO strategy_instances(user_id,automation_mandate_id,mandate_version,financial_account_id,capital_bucket_id,strategy_identifier,execution_mode,current_state,status) VALUES($1,$2,1,$3,$4,'wheel','SHADOW','MONITORING','ACTIVE') RETURNING id::text`, ownerID, mandateID, accountID, bucketID).Scan(&instanceID); err != nil {
		t.Fatal(err)
	}
	if err = setupTx.QueryRow(ctx, `INSERT INTO strategy_capital_reservations(user_id,financial_account_id,capital_bucket_id,strategy_instance_id,execution_mode,reservation_amount,currency,reservation_basis,account_allocation_limit,reserved_at) SELECT $1,$2,$3,$4,'SHADOW',100,'USD','BUCKET_FIXED_CAPACITY',100,started_at FROM strategy_instances WHERE id=$4 RETURNING id::text`, ownerID, accountID, bucketID, instanceID).Scan(&reservationID); err != nil {
		t.Fatal(err)
	}
	if _, err = setupTx.Exec(ctx, `SET CONSTRAINTS ALL IMMEDIATE`); err != nil {
		t.Fatal(err)
	}
	if err = setupTx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	observedAt := time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond)
	input := validRegistryInput(observedAt)
	input.FinancialAccountID = accountID
	input.ProviderConnectionID = connectionID
	input.StrategyInstanceID = instanceID
	input.MandateID = mandateID
	input.CapitalBucketID = bucketID
	input.CapitalReservationID = reservationID
	input.AssessmentKey = fmt.Sprintf("safety-case-%d", unique)
	store := NewRegistryStore(pool)
	first, err := store.Record(ctx, ownerID, input)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := store.Record(ctx, ownerID, input)
	if err != nil || replay.ID != first.ID || replay.ContentDigest != first.ContentDigest {
		t.Fatalf("exact replay did not return immutable row: %#v %v", replay, err)
	}
	conflict := input
	conflict.ActionDigest = strings.Repeat("c", 64)
	if _, err = store.Record(ctx, ownerID, conflict); !errors.Is(err, ErrRegistryConflict) {
		t.Fatalf("conflicting replay should fail closed, got %v", err)
	}
	crossStrategyConflict := input
	crossStrategyConflict.StrategyInstanceID = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	if _, err = store.Record(ctx, ownerID, crossStrategyConflict); !errors.Is(err, ErrRegistryConflict) {
		t.Fatalf("cross-strategy replay should fail closed, got %v", err)
	}

	ownerWindow, err := store.List(ctx, ownerID, instanceID, 10)
	if err != nil || len(ownerWindow.Records) != 1 || ownerWindow.Records[0].ID != first.ID ||
		ownerWindow.VerificationStatus != ChainVerifiedToGenesis || ownerWindow.EarlierEvidenceOmitted {
		t.Fatalf("owner chain window mismatch: %#v %v", ownerWindow, err)
	}
	foreignWindow, err := store.List(ctx, foreignOwnerID, instanceID, 10)
	if err != nil || len(foreignWindow.Records) != 0 || foreignWindow.VerificationStatus != ChainEmpty {
		t.Fatalf("cross-owner evidence leaked: %#v %v", foreignWindow, err)
	}

	const concurrentAppends = 8
	results := make(chan RegistryRecord, concurrentAppends)
	errorsFound := make(chan error, concurrentAppends)
	var wait sync.WaitGroup
	for index := 0; index < concurrentAppends; index++ {
		wait.Add(1)
		go func(sequence int) {
			defer wait.Done()
			concurrentInput := input
			concurrentInput.AssessmentKey = fmt.Sprintf("safety-case-%d-parallel-%02d", unique, sequence)
			concurrentInput.ActionDigest = fmt.Sprintf("%064x", sequence+1)
			record, recordErr := store.Record(ctx, ownerID, concurrentInput)
			if recordErr != nil {
				errorsFound <- recordErr
				return
			}
			results <- record
		}(index)
	}
	wait.Wait()
	close(results)
	close(errorsFound)
	for recordErr := range errorsFound {
		t.Fatalf("concurrent append failed: %v", recordErr)
	}
	sequences := map[int64]bool{first.ChainSequence: true}
	for record := range results {
		if sequences[record.ChainSequence] {
			t.Fatalf("duplicate concurrent chain position: %d", record.ChainSequence)
		}
		sequences[record.ChainSequence] = true
	}
	for sequence := int64(1); sequence <= int64(concurrentAppends+1); sequence++ {
		if !sequences[sequence] {
			t.Fatalf("concurrent chain has a gap at %d: %#v", sequence, sequences)
		}
	}
	completeWindow, err := store.List(ctx, ownerID, instanceID, MaxRegistryList)
	if err != nil || len(completeWindow.Records) != concurrentAppends+1 || completeWindow.VerificationStatus != ChainVerifiedToGenesis || completeWindow.EarlierEvidenceOmitted {
		t.Fatalf("complete concurrent chain did not verify to genesis: %#v %v", completeWindow, err)
	}
	boundedWindow, err := store.List(ctx, ownerID, instanceID, 3)
	if err != nil || len(boundedWindow.Records) != 3 || boundedWindow.VerificationStatus != ChainVerifiedBounded || !boundedWindow.EarlierEvidenceOmitted || boundedWindow.NewestSequence != concurrentAppends+1 || boundedWindow.OldestSequence != concurrentAppends-1 {
		t.Fatalf("bounded chain did not disclose omitted evidence: %#v %v", boundedWindow, err)
	}

	assertRegistryStatementRejected(t, ctx, pool, `UPDATE live_safety_case_evidence SET assessment_state='UNAVAILABLE' WHERE id=$1`, first.ID)
	assertRegistryStatementRejected(t, ctx, pool, `DELETE FROM live_safety_case_evidence WHERE id=$1`, first.ID)

	assertRegistryStatementRejected(t, ctx, pool, `INSERT INTO live_safety_case_evidence(user_id,assessment_key,financial_account_id,provider_connection_id,provider_name,strategy_instance_id,mandate_id,mandate_version,capital_bucket_id,capital_reservation_id,action_digest_sha256,contract_version,assessment_state,structural_blocker,reason_codes,evidence_manifest,evidence_sha256,content_sha256,observed_at) VALUES($1,$2,$3,$4,'coinbase',$5,$6,1,$7,$8,$9,$10,'READY',true,'["LIVE_RUNTIME_UNIMPLEMENTED"]',$11,$9,$9,$12)`, ownerID, fmt.Sprintf("ready-case-%d", unique), accountID, connectionID, instanceID, mandateID, bucketID, reservationID, strings.Repeat("a", 64), ContractVersion, `{"items":[{"id":"88888888-8888-4888-8888-888888888888","kind":"DESIGN_CONTRACT","digest_sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","source":"DESIGN_CONTRACT","observed_at":"2026-09-07T15:00:00Z"}]}`, observedAt)

	assertRegistryStatementRejected(t, ctx, pool, `INSERT INTO live_safety_case_evidence(user_id,assessment_key,financial_account_id,provider_connection_id,provider_name,strategy_instance_id,mandate_id,mandate_version,capital_bucket_id,capital_reservation_id,action_digest_sha256,contract_version,assessment_state,structural_blocker,reason_codes,evidence_manifest,evidence_sha256,content_sha256,chain_sequence,previous_chain_sha256,chain_sha256,observed_at) VALUES($1,$2,$3,$4,'coinbase',$5,$6,1,$7,$8,$9,$10,'BLOCKED_UNIMPLEMENTED',true,'["LIVE_RUNTIME_UNIMPLEMENTED"]',$11,$9,$9,1,$12,$13,$14)`, foreignOwnerID, fmt.Sprintf("foreign-case-%d", unique), accountID, connectionID, instanceID, mandateID, bucketID, reservationID, strings.Repeat("a", 64), ContractVersion, `{"items":[{"id":"88888888-8888-4888-8888-888888888888","kind":"DESIGN_CONTRACT","digest_sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","source":"DESIGN_CONTRACT","observed_at":"2026-09-07T15:00:00Z"}]}`, GenesisChainDigest, strings.Repeat("b", 64), observedAt)

	assertRegistryStatementRejected(t, ctx, pool, `INSERT INTO live_safety_case_evidence(user_id,assessment_key,financial_account_id,provider_connection_id,provider_name,strategy_instance_id,mandate_id,mandate_version,capital_bucket_id,capital_reservation_id,action_digest_sha256,contract_version,assessment_state,structural_blocker,reason_codes,evidence_manifest,evidence_sha256,content_sha256,chain_sequence,previous_chain_sha256,chain_sha256,observed_at) SELECT user_id,assessment_key||'-gap',financial_account_id,provider_connection_id,provider_name,strategy_instance_id,mandate_id,mandate_version,capital_bucket_id,capital_reservation_id,action_digest_sha256,contract_version,assessment_state,structural_blocker,reason_codes,evidence_manifest,evidence_sha256,content_sha256,999,chain_sha256,$2,observed_at FROM live_safety_case_evidence WHERE id=$1`, first.ID, strings.Repeat("d", 64))
}

type registryTxBeginner interface {
	Begin(context.Context) (pgx.Tx, error)
}

func assertRegistryStatementRejected(t *testing.T, ctx context.Context, db registryTxBeginner, query string, arguments ...any) {
	t.Helper()
	probe, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = probe.Exec(ctx, query, arguments...); err == nil {
		_ = probe.Rollback(ctx)
		t.Fatal("database accepted forbidden live safety evidence mutation")
	}
	if err = probe.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
}
