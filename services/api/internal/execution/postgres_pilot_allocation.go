package execution

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// PilotAllocation permanently attributes an initial cash amount and zero initial
// inventory to one account/bucket/product. It is not an autonomous mandate, a
// broker transfer, or permission to submit. No runtime caller registers pilots.
type PilotAllocation struct {
	OwnerID         string
	AccountID       string
	ConnectionID    string
	CapitalBucketID string
	ProductID       string
	InitialCashUSD  string
	Limits          OwnerPilotLimits
}

// PilotBalance is a snapshot of settled pilot funds, never broker buying power
// or a claim authorization. Pending capital cannot be reused until existing
// lifecycle controls prove settlement or the narrow positive no-send closure.
type PilotBalance struct {
	PilotAllocation
	CashUSD           string
	BaseQuantity      string
	SettledOrderCount int64
	RegisteredAt      time.Time
	Pending           bool
}

func validPilotAllocation(p PilotAllocation) bool {
	for _, id := range []string{p.OwnerID, p.AccountID, p.ConnectionID, p.CapitalBucketID} {
		if !validUUID(id) {
			return false
		}
	}
	initial, ok := decimal(p.InitialCashUSD, true)
	if !ok || !validPilotLimits(&p.Limits) || !productPattern.MatchString(p.ProductID) || p.ProductID == "USD-USD" {
		return false
	}
	cap, _ := decimal(p.Limits.MaximumOrderUSD, true)
	return cap.Cmp(initial) <= 0
}

func samePilotAllocation(a, b PilotAllocation) bool {
	return a.OwnerID == b.OwnerID && a.AccountID == b.AccountID && a.ConnectionID == b.ConnectionID &&
		a.CapitalBucketID == b.CapitalBucketID && a.ProductID == b.ProductID && a.InitialCashUSD == b.InitialCashUSD &&
		samePilotLimits(&a.Limits, b.Limits)
}

// RegisterPilotAllocation must precede every execution order on the account.
// Registration and Prepare serialize on the same database account fence, even
// for direct SQL. Exact retries read the immutable original; changed terms never
// replenish cash or extend the pilot. A commit error is recovered by reading or
// exact retry, not by creating a replacement account or allocation.
func (s *PostgresStore) RegisterPilotAllocation(ctx context.Context, p PilotAllocation) (PilotAllocation, error) {
	if !validPilotAllocation(p) {
		return PilotAllocation{}, ErrInvalid
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return PilotAllocation{}, err
	}
	defer rollback(tx)
	if _, err = tx.Exec(ctx, `SELECT lock_execution_pilot_account($1,$2)`, p.OwnerID, p.AccountID); err != nil {
		return PilotAllocation{}, mapError(err)
	}
	existing, err := readPilotAllocation(ctx, tx, p.OwnerID, p.AccountID)
	if err == nil {
		if !samePilotAllocation(existing, p) {
			return PilotAllocation{}, ErrConflict
		}
		return existing, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return PilotAllocation{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO execution_pilot_allocations
		(owner_id,financial_account_id,provider_connection_id,capital_bucket_id,product_id,initial_cash_usd,maximum_order_usd,expires_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, p.OwnerID, p.AccountID, p.ConnectionID, p.CapitalBucketID,
		p.ProductID, p.InitialCashUSD, p.Limits.MaximumOrderUSD, p.Limits.ExpiresAt)
	if err != nil {
		return PilotAllocation{}, mapError(err)
	}
	if err = tx.Commit(ctx); err != nil {
		return PilotAllocation{}, ErrCommitUnknown
	}
	return p, nil
}

func readPilotAllocation(ctx context.Context, db interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, ownerID, accountID string) (PilotAllocation, error) {
	var p PilotAllocation
	err := db.QueryRow(ctx, `SELECT owner_id::text,financial_account_id::text,provider_connection_id::text,
		capital_bucket_id::text,product_id,initial_cash_usd,maximum_order_usd,expires_at
		FROM execution_pilot_allocations WHERE owner_id=$1 AND financial_account_id=$2`, ownerID, accountID).
		Scan(&p.OwnerID, &p.AccountID, &p.ConnectionID, &p.CapitalBucketID, &p.ProductID, &p.InitialCashUSD,
			&p.Limits.MaximumOrderUSD, &p.Limits.ExpiresAt)
	return p, mapError(err)
}

// ReadPilotBalance is owner-scoped, credential-free and provider-free. A single
// statement gives one consistent settlement/hold snapshot; new claims repeat
// all checks under current account locks instead of trusting this read.
func (s *PostgresStore) ReadPilotBalance(ctx context.Context, ownerID, accountID string) (PilotBalance, error) {
	if !validUUID(ownerID) || !validUUID(accountID) {
		return PilotBalance{}, ErrInvalid
	}
	var p PilotBalance
	err := s.db.QueryRow(ctx, `SELECT p.owner_id::text,p.financial_account_id::text,p.provider_connection_id::text,
		p.capital_bucket_id::text,p.product_id,p.initial_cash_usd,p.maximum_order_usd,p.expires_at,p.registered_at,
		b.cash_usd::text,b.base_quantity::text,b.settled_order_count,
		EXISTS(SELECT 1 FROM execution_account_holds WHERE financial_account_id=p.financial_account_id)
		OR EXISTS(SELECT 1 FROM execution_capital_reservations WHERE financial_account_id=p.financial_account_id AND released_at IS NULL)
		FROM execution_pilot_allocations p CROSS JOIN LATERAL execution_pilot_balance(p.financial_account_id) b
		WHERE p.owner_id=$1 AND p.financial_account_id=$2`, ownerID, accountID).
		Scan(&p.OwnerID, &p.AccountID, &p.ConnectionID, &p.CapitalBucketID, &p.ProductID, &p.InitialCashUSD,
			&p.Limits.MaximumOrderUSD, &p.Limits.ExpiresAt, &p.RegisteredAt, &p.CashUSD, &p.BaseQuantity,
			&p.SettledOrderCount, &p.Pending)
	return p, mapError(err)
}
