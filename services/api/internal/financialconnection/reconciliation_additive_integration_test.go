package financialconnection

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/financial"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Runs inside the existing isolated PostgreSQL lifecycle CI test, never against
// production. Tests both forward inserts and the database's independent guard.
func testAdditiveReconciliationStore(t *testing.T, store *PostgresStore, pool *pgxpool.Pool, userID, accountID, otherAccount string, prior PortfolioReconciliation) {
	t.Helper()
	ctx := context.Background()
	position := additivePosition("USDC", "100", "100", "0")
	appeared := prior
	appeared.PreviousReconciliationID = &prior.ID
	appeared.ObservedAt = prior.ObservedAt.Add(time.Minute)
	appeared.Positions = append(append([]ReconciliationPosition{}, prior.Positions...), position)
	appeared.ObservedPositionCount = len(appeared.Positions)
	appeared.Changes = compareReconciliationPositions("coinbase", prior.Positions, appeared.Positions)
	appeared.ChangeCount = len(appeared.Changes)
	first, err := store.CreateReconciliation(ctx, userID, appeared, make([]byte, 32))
	if err != nil {
		t.Fatalf("valid new additive inventory rejected: %v", err)
	}
	next := first
	next.PreviousReconciliationID = &first.ID
	next.ObservedAt = first.ObservedAt.Add(time.Minute)
	next.Positions = []ReconciliationPosition{prior.Positions[0], additivePosition("USDC", "125", "125", "0")}
	next.Changes = compareReconciliationPositions("coinbase", first.Positions, next.Positions)
	for _, test := range []struct {
		name   string
		mutate func(*PortfolioReconciliation)
	}{
		{"missing prior", func(r *PortfolioReconciliation) { r.PreviousReconciliationID = nil }},
		{"other account", func(r *PortfolioReconciliation) { r.FinancialAccountID = otherAccount }},
		{"other provider", func(r *PortfolioReconciliation) { r.Provider = "schwab" }},
		{"old timestamp", func(r *PortfolioReconciliation) { r.ObservedAt = prior.ObservedAt }},
		{"cash decline", func(r *PortfolioReconciliation) { r.Balances.Cash = &financial.Money{Amount: "24", Currency: "USD"} }},
		{"cash missing", func(r *PortfolioReconciliation) { r.Balances.Cash = nil }},
		{"bad sum", func(r *PortfolioReconciliation) {
			v := financial.Decimal("126")
			r.Changes[0].CurrentAvailableQuantity = &v
		}},
		{"negative quantity", func(r *PortfolioReconciliation) {
			v := financial.Decimal("-1")
			r.Changes[0].CurrentUnavailableQuantity = &v
		}},
		{"missing quantity", func(r *PortfolioReconciliation) { r.Changes[0].CurrentAvailableQuantity = nil }},
		{"invalid decimal", func(r *PortfolioReconciliation) { r.Changes[0].CurrentQuantity = "NaN" }},
		{"short inventory", func(r *PortfolioReconciliation) { r.Changes[0].Direction = "short" }},
		{"forged prior quantity", func(r *PortfolioReconciliation) { r.Changes[0].PreviousQuantity = "99" }},
		{"not really appeared", func(r *PortfolioReconciliation) {
			r.Changes[0].ChangeType = "POSITION_APPEARED"
			r.Changes[0].PreviousQuantity = ""
			r.Changes[0].PreviousAvailableQuantity = nil
			r.Changes[0].PreviousUnavailableQuantity = nil
		}},
		{"zero increase", func(r *PortfolioReconciliation) {
			v := financial.Decimal("100")
			r.Changes[0].CurrentQuantity = v
			r.Changes[0].CurrentAvailableQuantity = &v
		}},
		{"component decrease", func(r *PortfolioReconciliation) {
			a, u := financial.Decimal("99"), financial.Decimal("26")
			r.Changes[0].CurrentAvailableQuantity = &a
			r.Changes[0].CurrentUnavailableQuantity = &u
		}},
		{"incorrect count", func(r *PortfolioReconciliation) { r.BlockingChangeCount = 1 }},
		{"missing current position", func(r *PortfolioReconciliation) { r.Positions = r.Positions[:1] }},
		{"different saved current quantity", func(r *PortfolioReconciliation) { r.Positions[1].Quantity = "126" }},
		{"duplicate change identity", func(r *PortfolioReconciliation) { r.Changes = append(r.Changes, r.Changes[0]); r.ChangeCount++ }},
	} {
		t.Run("database additive guard/"+test.name, func(t *testing.T) {
			encoded, err := json.Marshal(next)
			if err != nil {
				t.Fatal(err)
			}
			var candidate PortfolioReconciliation
			if err = json.Unmarshal(encoded, &candidate); err != nil {
				t.Fatal(err)
			}
			test.mutate(&candidate)
			if _, err = store.CreateReconciliation(ctx, userID, candidate, make([]byte, 32)); err == nil {
				t.Fatal("database accepted unsafe additive evidence")
			}
		})
	}
	saved, err := store.CreateReconciliation(ctx, userID, next, make([]byte, 32))
	if err != nil {
		t.Fatalf("valid repeated addition rejected: %v", err)
	}
	loaded, err := store.LatestReconciliation(ctx, userID, accountID)
	if err != nil || loaded.ID != saved.ID || loaded.BlocksNewActions || loaded.BlockingChangeCount != 0 || len(loaded.Changes) != 1 || loaded.Changes[0].ControlImpact != reconciliationControlAdditiveOnly {
		t.Fatalf("additive round-trip failed: %#v %v", loaded, err)
	}
	if _, err = pool.Exec(ctx, `UPDATE portfolio_reconciliations SET change_count=0 WHERE id=$1`, saved.ID); err == nil {
		t.Fatal("additive history was mutable")
	}
}
