package execution

import (
	"context"
	"time"

	"github.com/arbion/platform/services/api/internal/authorization"
)

type ExecutionStepUp interface {
	VerifyExecutionStepUp(context.Context, string, string) (string, time.Time, error)
}

type OwnerApproval struct {
	ID, OrderID, RequestDigest string
	CredentialGeneration       int64
	ApprovedAt, ExpiresAt      time.Time
}

// ApproveOrder creates an exact one-order confirmation, not an autonomous
// mandate. No HTTP/scheduler caller is wired. MFA verification consumes a fresh
// TOTP before the transaction; the database binds its current factor receipt.
// Codes are never persisted. A commit error is recovered by ReadOwnerApproval,
// never by automatically consuming another code or extending approval expiry.
func (s *PostgresStore) ApproveOrder(ctx context.Context, p authorization.Principal, orderID, expectedDigest, code string, stepUp ExecutionStepUp) (OwnerApproval, error) {
	if p.Entitlement != authorization.EntitlementFounder || !validUUID(p.UserID) || !validUUID(orderID) || stepUp == nil {
		return OwnerApproval{}, ErrNotAuthorized
	}
	var saved string
	var autonomous bool
	var pilotExpiry time.Time
	if err := s.db.QueryRow(ctx, `SELECT request_digest,check_execution_pilot_limits(id),request ? 'MandateApprovalID' FROM execution_orders WHERE id=$1 AND owner_id=$2`, orderID, p.UserID).Scan(&saved, &pilotExpiry, &autonomous); err != nil {
		return OwnerApproval{}, mapError(err)
	}
	if autonomous {
		return OwnerApproval{}, ErrNotAuthorized
	}
	if saved != expectedDigest {
		return OwnerApproval{}, ErrConflict
	}
	method, verified, err := stepUp.VerifyExecutionStepUp(ctx, p.UserID, code)
	if err != nil {
		return OwnerApproval{}, err
	}
	if method != "totp" || verified.IsZero() {
		return OwnerApproval{}, ErrNotAuthorized
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return OwnerApproval{}, err
	}
	defer rollback(tx)
	var generation int64
	if err = tx.QueryRow(ctx, `SELECT lock_execution_claim_controls($1)`, orderID).Scan(&generation); err != nil {
		return OwnerApproval{}, mapError(err)
	}
	var now time.Time
	var enabled *time.Time
	if err = tx.QueryRow(ctx, `SELECT enabled_at FROM auth_totp_factors WHERE user_id=$1 FOR UPDATE`, p.UserID).Scan(&enabled); err != nil {
		return OwnerApproval{}, ErrNotAuthorized
	}
	if pilotExpiry, err = checkPilotLimits(ctx, tx, orderID); err != nil {
		return OwnerApproval{}, err
	}
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return OwnerApproval{}, err
	}
	if enabled == nil || verified.After(now) || now.Sub(verified) > 10*time.Second {
		return OwnerApproval{}, ErrNotAuthorized
	}
	until := now.Add(5 * time.Minute)
	if pilotExpiry.Before(until) {
		until = pilotExpiry
	}
	var a OwnerApproval
	err = tx.QueryRow(ctx, `INSERT INTO execution_owner_approvals(order_id,owner_id,request_digest,credential_generation,mfa_method,mfa_verified_at,mfa_enabled_at,approved_at,expires_at)
		VALUES($1,$2,$3,$4,'totp',$5,$6,$7,$8) RETURNING id::text,order_id::text,request_digest,credential_generation,approved_at,expires_at`, orderID, p.UserID, saved, generation, verified, *enabled, now, until).Scan(&a.ID, &a.OrderID, &a.RequestDigest, &a.CredentialGeneration, &a.ApprovedAt, &a.ExpiresAt)
	if err != nil {
		return OwnerApproval{}, mapError(err)
	}
	if err = tx.Commit(ctx); err != nil {
		return OwnerApproval{}, ErrCommitUnknown
	}
	return a, nil
}

// ReadOwnerApproval is recovery-only and does not imply unrevoked authority.
func (s *PostgresStore) ReadOwnerApproval(ctx context.Context, ownerID, orderID string) (OwnerApproval, error) {
	if !validUUID(ownerID) || !validUUID(orderID) {
		return OwnerApproval{}, ErrInvalid
	}
	var a OwnerApproval
	err := s.db.QueryRow(ctx, `SELECT id::text,order_id::text,request_digest,credential_generation,approved_at,expires_at FROM execution_owner_approvals WHERE owner_id=$1 AND order_id=$2`, ownerID, orderID).Scan(&a.ID, &a.OrderID, &a.RequestDigest, &a.CredentialGeneration, &a.ApprovedAt, &a.ExpiresAt)
	return a, mapError(err)
}

// Revocation is append-only and idempotent. It grants no broker cancellation or
// resend authority. A later sender must synchronize revocation again at send.
func (s *PostgresStore) RevokeOwnerApproval(ctx context.Context, ownerID, orderID string) error {
	if !validUUID(ownerID) || !validUUID(orderID) {
		return ErrInvalid
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(tx)
	var id string
	if err = tx.QueryRow(ctx, `SELECT id::text FROM execution_owner_approvals WHERE owner_id=$1 AND order_id=$2 FOR UPDATE`, ownerID, orderID).Scan(&id); err != nil {
		return mapError(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO execution_approval_revocations(approval_id) VALUES($1) ON CONFLICT DO NOTHING`, id); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return ErrCommitUnknown
	}
	return nil
}
