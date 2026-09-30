package credential

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/migrations"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func TestFinancialCredentialGenerationMigrationBoundary(t *testing.T) {
	body, err := fs.ReadFile(migrations.Files, "00052_financial_credential_generation.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"OLD.provider_category<>'financial' AND NEW.provider_category<>'financial'",
		"NEW.encrypted_credential_payload IS DISTINCT FROM OLD.encrypted_credential_payload",
		"NEW.credential_reference IS DISTINCT FROM OLD.credential_reference",
		"NEW.credential_generation=OLD.credential_generation+1",
		"NEW.credential_generation<OLD.credential_generation",
		"BEFORE UPDATE ON provider_connections",
	} {
		if !strings.Contains(string(body), required) {
			t.Errorf("missing credential version boundary: %s", required)
		}
	}
}

func TestPostgresFinancialCredentialGenerationTracksEffectiveMaterial(t *testing.T) {
	ctx, pool := setupFinancialGenerationTest(t)
	var userID, connectionID string
	if err := pool.QueryRow(ctx, `INSERT INTO users(external_id) VALUES($1) RETURNING id::text`, fmt.Sprintf("financial-generation-%d", time.Now().UnixNano())).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO provider_connections(user_id,provider_category,provider_name,display_name,status) VALUES($1,'financial','coinbase','Generation fixture','active') RETURNING id::text`, userID).Scan(&connectionID); err != nil {
		t.Fatal(err)
	}
	store := NewPostgresStore(pool)
	locator := Locator{ConnectionID: connectionID, UserID: userID, Class: Financial}
	assertGeneration := func(want int64) {
		t.Helper()
		var got int64
		if err := pool.QueryRow(ctx, `SELECT credential_generation FROM provider_connections WHERE id=$1`, connectionID).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("generation=%d, want %d", got, want)
		}
	}
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	assertGeneration(1)
	// Synthetic encrypted bytes only; the guard never decrypts or returns them.
	first, second := make([]byte, 32), make([]byte, 32)
	second[0] = 1
	if err := store.Put(ctx, locator, first, true); err != nil {
		t.Fatal(err)
	}
	assertGeneration(2)
	if err := store.Put(ctx, locator, first, false); err != nil {
		t.Fatal(err)
	}
	assertGeneration(2)
	if err := store.PutStaged(ctx, locator, second, strings.Repeat("a", 64)); err != nil {
		t.Fatal(err)
	}
	exec(`UPDATE provider_connections SET status='pending',last_verified_at=clock_timestamp(),credential_metadata='{"fixture":true}' WHERE id=$1`, connectionID)
	assertGeneration(2)
	if err := store.Put(ctx, locator, second, false); err != nil {
		t.Fatal(err)
	}
	assertGeneration(3)
	// A future financial writer that increments explicitly must not double-bump.
	exec(`UPDATE provider_connections SET encrypted_credential_payload=$2,credential_generation=credential_generation+1 WHERE id=$1`, connectionID, first)
	assertGeneration(4)
	if err := store.Delete(ctx, locator); err != nil {
		t.Fatal(err)
	}
	assertGeneration(5)
	exec(`UPDATE provider_connections SET credential_storage='managed_reference',credential_reference='fixture-reference-a' WHERE id=$1`, connectionID)
	assertGeneration(6)
	exec(`UPDATE provider_connections SET credential_reference='fixture-reference-b' WHERE id=$1`, connectionID)
	assertGeneration(7)
	exec(`UPDATE provider_connections SET credential_generation=credential_generation+1 WHERE id=$1`, connectionID)
	assertGeneration(8)
	_, err := pool.Exec(ctx, `UPDATE provider_connections SET credential_generation=1 WHERE id=$1`, connectionID)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.ConstraintName != "financial_credential_generation_monotonic" {
		t.Fatalf("generation rollback was not rejected: %v", err)
	}
	assertGeneration(8)
	// Changing category cannot bypass financial invalidation by a round trip.
	exec(`UPDATE provider_connections SET provider_category='ai' WHERE id=$1`, connectionID)
	assertGeneration(9)
	exec(`UPDATE provider_connections SET provider_category='financial' WHERE id=$1`, connectionID)
	assertGeneration(10)

	var aiID string
	if err = pool.QueryRow(ctx, `INSERT INTO provider_connections(user_id,provider_category,provider_name,display_name,status) VALUES($1,'ai','openai','Generation fixture','active') RETURNING id::text`, userID).Scan(&aiID); err != nil {
		t.Fatal(err)
	}
	exec(`UPDATE provider_connections SET encrypted_credential_payload=$2 WHERE id=$1`, aiID, first)
	var aiGeneration int64
	if err = pool.QueryRow(ctx, `SELECT credential_generation FROM provider_connections WHERE id=$1`, aiID).Scan(&aiGeneration); err != nil || aiGeneration != 1 {
		t.Fatalf("financial trigger changed AI rotation semantics: generation=%d err=%v", aiGeneration, err)
	}
}

func TestPostgresFinancialCredentialRotationWaitsForAuthorityLock(t *testing.T) {
	ctx, pool := setupFinancialGenerationTest(t)
	var userID, connectionID string
	if err := pool.QueryRow(ctx, `INSERT INTO users(external_id) VALUES($1) RETURNING id::text`, fmt.Sprintf("financial-generation-lock-%d", time.Now().UnixNano())).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO provider_connections(user_id,provider_category,provider_name,display_name,status,encrypted_credential_payload) VALUES($1,'financial','coinbase','Generation lock fixture','active',$2) RETURNING id::text`, userID, make([]byte, 32)).Scan(&connectionID); err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	var generation int64
	if err = tx.QueryRow(ctx, `SELECT credential_generation FROM provider_connections WHERE id=$1 FOR SHARE`, connectionID).Scan(&generation); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		result <- NewPostgresStore(pool).Put(ctx, Locator{ConnectionID: connectionID, UserID: userID, Class: Financial}, []byte(strings.Repeat("x", 32)), false)
	}()
	select {
	case err = <-result:
		t.Fatalf("credential changed while authority held its share lock: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err = <-result; err != nil {
		t.Fatal(err)
	}
	var current int64
	if err = pool.QueryRow(ctx, `SELECT credential_generation FROM provider_connections WHERE id=$1`, connectionID).Scan(&current); err != nil || current != generation+1 {
		t.Fatalf("unblocked rotation did not invalidate generation: current=%d prior=%d err=%v", current, generation, err)
	}
}

func setupFinancialGenerationTest(t *testing.T) (context.Context, *pgxpool.Pool) {
	t.Helper()
	url := os.Getenv("STRATEGY_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("STRATEGY_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	t.Cleanup(cancel)
	db, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	goose.SetBaseFS(migrations.Files)
	if err = goose.SetDialect("postgres"); err != nil {
		t.Fatal(err)
	}
	if err = goose.UpContext(ctx, db, "."); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return ctx, pool
}
