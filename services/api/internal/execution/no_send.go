package execution

import (
	"context"
	"errors"
	"time"
)

var (
	ErrSubmissionNotSent       = errors.New("sender was never entered; original attempt closed, never resend")
	ErrNoSendResolutionUnknown = errors.New("no-send resolution not confirmed; read saved receipt, never resend")
)

// NoSendResolution is a local execution fact, not a broker rejection, fill or
// account settlement. Its immutable receipt closes only its original holds.
type NoSendResolution struct {
	OrderID, OwnerID, AccountID, CapitalBucketID, AuthorizationID, RequestDigest string
	CredentialGeneration                                                         int64
	ClaimedAt, RecordedAt                                                        time.Time
}

// recordNoSendResolution has exactly one production call site: the invocation
// that positively committed Claim and returned from its synchronous send helper
// without entering the adapter. Never expose it as an owner/worker unlock API.
// Absence of broker evidence is only a consistency check, NOT the source of proof.
func (s *PostgresStore) recordNoSendResolution(o Order, a Attempt) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return ErrNoSendResolutionUnknown
	}
	defer rollback(tx)
	r := o.Request
	// The trigger locks the account/fences/original attempt and checks every
	// binding, then atomically releases only this attempt's exact two holds.
	// This historical local fact does not renew revoked authority or clear stops.
	_, err = tx.Exec(ctx, `INSERT INTO execution_no_send_resolutions(order_id,owner_id,financial_account_id,capital_bucket_id,authorization_id,request_digest,credential_generation,claimed_at)
	 VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, o.ID, r.OwnerID, r.AccountID, r.CapitalBucketID, a.AuthorizationID, o.RequestDigest, a.CredentialGeneration, a.ClaimedAt)
	if err != nil {
		return ErrNoSendResolutionUnknown
	}
	if err = tx.Commit(ctx); err != nil {
		return ErrNoSendResolutionUnknown // Never retry an ambiguous receipt commit.
	}
	return nil
}

// ReadNoSendResolution is owner-scoped historical recovery. It never creates
// proof, releases a second reservation, retrieves credentials or calls a broker.
func (s *PostgresStore) ReadNoSendResolution(ctx context.Context, ownerID, orderID string) (NoSendResolution, error) {
	if !validUUID(ownerID) || !validUUID(orderID) {
		return NoSendResolution{}, ErrInvalid
	}
	var r NoSendResolution
	err := s.db.QueryRow(ctx, `SELECT order_id::text,owner_id::text,financial_account_id::text,capital_bucket_id::text,authorization_id::text,request_digest,credential_generation,claimed_at,recorded_at
	 FROM execution_no_send_resolutions WHERE order_id=$1 AND owner_id=$2`, orderID, ownerID).
		Scan(&r.OrderID, &r.OwnerID, &r.AccountID, &r.CapitalBucketID, &r.AuthorizationID, &r.RequestDigest, &r.CredentialGeneration, &r.ClaimedAt, &r.RecordedAt)
	return r, mapError(err)
}
