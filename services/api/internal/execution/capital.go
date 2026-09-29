package execution

import (
	"context"
	"time"
)

// CapitalReservation is a durable accounting fence, not a broker balance or
// live approval. A final order alone does not release it; only exact account
// settlement can set ReleasedAt. Original reservation terms remain immutable.
type CapitalReservation struct {
	OrderID, AccountID, CapitalBucketID string
	ResourceType, Asset, Quantity       string
	ReservedAt                          time.Time
	ReleasedAt                          *time.Time
}

func (s *PostgresStore) ReadCapitalReservation(ctx context.Context, ownerID, orderID string) (CapitalReservation, error) {
	if !validUUID(ownerID) || !validUUID(orderID) {
		return CapitalReservation{}, ErrInvalid
	}
	var r CapitalReservation
	err := s.db.QueryRow(ctx, `SELECT order_id::text,financial_account_id::text,capital_bucket_id::text,
		resource_type,resource_asset,quantity::text,reserved_at,released_at FROM execution_capital_reservations
		WHERE owner_id=$1 AND order_id=$2`, ownerID, orderID).Scan(&r.OrderID, &r.AccountID, &r.CapitalBucketID,
		&r.ResourceType, &r.Asset, &r.Quantity, &r.ReservedAt, &r.ReleasedAt)
	if err != nil {
		return CapitalReservation{}, mapError(err)
	}
	value, valid := decimal(r.Quantity, true)
	if !valid {
		return CapitalReservation{}, ErrInvalid
	}
	r.Quantity = canonical(value)
	return r, nil
}
