package credential

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestPostgresFinancialVersionRetainsCredentialSnapshotGeneration(t *testing.T) {
	ctx, pool := setupFinancialGenerationTest(t)
	var ownerID, connectionID string
	if err := pool.QueryRow(ctx, `INSERT INTO users(external_id) VALUES($1) RETURNING id::text`, fmt.Sprintf("financial-version-snapshot-%d", time.Now().UnixNano())).Scan(&ownerID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO provider_connections(user_id,provider_category,provider_name,display_name,status)
		VALUES($1,'financial','coinbase','Versioned snapshot fixture','active') RETURNING id::text`, ownerID).Scan(&connectionID); err != nil {
		t.Fatal(err)
	}
	l := Locator{ConnectionID: connectionID, UserID: ownerID, Class: Financial}
	key := make([]byte, 32)
	current, err := NewEncryptedVault(key, NewPostgresStore(pool))
	if err != nil {
		t.Fatal(err)
	}
	if err = current.Store(ctx, l, []byte("synthetic-old-material")); err != nil {
		t.Fatal(err)
	}
	old, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		t.Fatal(err)
	}
	defer old.Rollback(ctx)
	snapshot, err := NewEncryptedVault(key, NewPostgresStore(old))
	if err != nil {
		t.Fatal(err)
	}
	initial, initialGeneration, err := snapshot.RetrieveFinancialVersion(ctx, l)
	defer clear(initial)
	if err != nil || string(initial) != "synthetic-old-material" || initialGeneration <= 0 {
		t.Fatalf("initial snapshot unavailable: generation=%d err=%v", initialGeneration, err)
	}
	if err = current.Replace(ctx, l, []byte("synthetic-new-material")); err != nil {
		t.Fatal(err)
	}
	fresh, freshGeneration, err := current.RetrieveFinancialVersion(ctx, l)
	defer clear(fresh)
	if err != nil || string(fresh) != "synthetic-new-material" || freshGeneration != initialGeneration+1 {
		t.Fatalf("fresh material did not advance atomically: generation=%d prior=%d err=%v", freshGeneration, initialGeneration, err)
	}
	stale, staleGeneration, err := snapshot.RetrieveFinancialVersion(ctx, l)
	defer clear(stale)
	if err != nil || string(stale) != "synthetic-old-material" || staleGeneration != initialGeneration || staleGeneration == freshGeneration {
		t.Fatalf("old material was relabeled with current generation: stale=%d fresh=%d err=%v", staleGeneration, freshGeneration, err)
	}
	// The caller can now reject this older snapshot by comparing its returned
	// generation with the current controls, even though no rotation occurs
	// during that caller's own before/after generation checks.
	otherOwner := l
	otherOwner.UserID = "00000000-0000-0000-0000-000000000001"
	if material, generation, err := current.RetrieveFinancialVersion(ctx, otherOwner); !errors.Is(err, ErrNotFound) || material != nil || generation != 0 {
		t.Fatalf("foreign owner credential exposed: generation=%d err=%v", generation, err)
	}
}
