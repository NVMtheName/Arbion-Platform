package execution

import (
	"context"
	"math/big"
	"time"

	"github.com/jackc/pgx/v5"
)

// OwnerPilotLimits are server-selected terms for individually confirmed orders,
// not an autonomous mandate or mutable global policy. BUY caps maximum debit
// including fees. SELL caps submitted limit-price notional plus fees, not
// proceeds from price improvement. No default amount or lifetime grants money.
type OwnerPilotLimits struct {
	MaximumOrderUSD string
	ExpiresAt       time.Time
}

func validPilotLimits(l *OwnerPilotLimits) bool {
	if l == nil {
		return false
	}
	_, valid := decimal(l.MaximumOrderUSD, true)
	_, offset := l.ExpiresAt.Zone()
	return valid && !l.ExpiresAt.IsZero() && l.ExpiresAt.Year() >= 1 && l.ExpiresAt.Year() <= 9999 &&
		offset == 0 && l.ExpiresAt.Nanosecond()%1000 == 0
}

func withinPilotCap(r Request) bool {
	if !validPilotLimits(r.PilotLimits) {
		return false
	}
	cap, _ := decimal(r.PilotLimits.MaximumOrderUSD, true)
	bound, ok := decimal(r.MaximumDebitUSD, true)
	if r.Side == "SELL" {
		quantity, qOK := decimal(r.BaseSize, true)
		price, pOK := decimal(r.LimitPrice, true)
		fee, fOK := decimal(r.FeeAllowanceUSD, false)
		if !qOK || !pOK || !fOK {
			return false
		}
		bound, ok = new(big.Rat).Add(new(big.Rat).Mul(quantity, price), fee), true
	}
	return ok && (r.Side == "BUY" || r.Side == "SELL") && bound.Cmp(cap) <= 0
}

func samePilotLimits(a *OwnerPilotLimits, b OwnerPilotLimits) bool {
	return validPilotLimits(a) && a.MaximumOrderUSD == b.MaximumOrderUSD && a.ExpiresAt.Equal(b.ExpiresAt)
}

// Read DB wall time after lock waits. Historical terms remain readable even
// without limits, but cannot authorize new approval, preflight or submission.
func checkPilotLimits(ctx context.Context, tx pgx.Tx, orderID string) (time.Time, error) {
	var expiry time.Time
	err := tx.QueryRow(ctx, `SELECT check_execution_pilot_limits($1)`, orderID).Scan(&expiry)
	return expiry, mapError(err)
}
