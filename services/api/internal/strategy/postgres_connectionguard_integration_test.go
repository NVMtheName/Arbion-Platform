package strategy

import (
	"context"
	"errors"
	"testing"

	"github.com/arbion/platform/services/api/internal/connectionguard"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Exercise the shared guard directly so a later Initialize mandate-binding
// check cannot hide a missing owner or provider-category check here.
func testConnectionGuardPolicy(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	f := newInitializeConnectionFixture(t, ctx, pool, Shadow)
	foreign := newInitializeConnectionFixture(t, ctx, pool, Shadow)
	for _, tt := range []struct {
		name      string
		accountID string
		aiID      *string
		wantErr   error
	}{
		{"foreign-owner account", foreign.accountID, &f.aiID, connectionguard.ErrUnavailable},
		{"foreign-owner AI", f.accountID, &foreign.aiID, connectionguard.ErrUnavailable},
		{"financial-as-AI same-ID deduplication", f.accountID, &f.financialID, connectionguard.ErrUnavailable},
		{"valid AI", f.accountID, &f.aiID, nil},
		{"nil AI", f.accountID, nil, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			guardErr := connectionguard.LockActive(ctx, tx, f.userID, tt.accountID, tt.aiID)
			if !errors.Is(guardErr, tt.wantErr) {
				t.Errorf("LockActive returned %v, want %v", guardErr, tt.wantErr)
			}
			if err := tx.Rollback(ctx); err != nil {
				t.Fatal("guard transaction rollback failed", err)
			}
			f.assertArtifacts(t, 0)
			foreign.assertArtifacts(t, 0)
		})
	}
}
