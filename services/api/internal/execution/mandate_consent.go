package execution

import (
	"context"
	"regexp"
	"time"

	"github.com/arbion/platform/services/api/internal/authorization"
)

type MandateConsent struct {
	ID, OwnerID, AccountID, ConnectionID, CapitalBucketID, MandateID, SnapshotDigest string
	MandateVersion                                                                   int
	CredentialGeneration                                                             int64
	ApprovedAt, ExpiresAt                                                            time.Time
}

var snapshotDigestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// ApproveMandate is an inert explicit owner-consent boundary, not a runtime
// registration. It consumes one fresh step-up only after checking the owner's
// expected immutable snapshot. An ambiguous commit must be recovered by reading
// the saved receipt, never by automatically retrying MFA or extending consent.
func (s *PostgresStore) ApproveMandate(ctx context.Context, p authorization.Principal, bucketID, mandateID string, version int, expectedSnapshotDigest, code string, stepUp ExecutionStepUp) (MandateConsent, error) {
	return s.approveMandate(ctx, p, bucketID, mandateID, version, expectedSnapshotDigest, code, stepUp, "")
}

// fixedID is supplied only by the initial commissioning boundary. The public
// generic consent API retains its existing independent receipt semantics.
func (s *PostgresStore) approveMandate(ctx context.Context, p authorization.Principal, bucketID, mandateID string, version int, expectedSnapshotDigest, code string, stepUp ExecutionStepUp, fixedID string) (MandateConsent, error) {
	if p.Entitlement != authorization.EntitlementFounder || !validUUID(p.UserID) || !validUUID(bucketID) || !validUUID(mandateID) || version < 1 || !snapshotDigestPattern.MatchString(expectedSnapshotDigest) || stepUp == nil {
		return MandateConsent{}, ErrNotAuthorized
	}
	var saved string
	err := s.db.QueryRow(ctx, `SELECT encode(sha256(convert_to(v.snapshot::text,'UTF8')),'hex')
	 FROM automation_mandates m JOIN automation_mandate_versions v ON v.mandate_id=m.id AND v.version_number=m.current_version
	 JOIN execution_pilot_allocations p ON p.capital_bucket_id=m.capital_bucket_id AND p.owner_id=m.user_id AND p.financial_account_id=m.financial_account_id
	 WHERE m.id=$1 AND m.user_id=$2 AND m.capital_bucket_id=$3 AND m.current_version=$4
	 AND m.status='READY' AND m.execution_mode='LIVE' AND m.automation_type='AI_AUTONOMOUS' AND m.autonomy_level='FULL_AUTONOMOUS'
	 AND execution_mandate_snapshot_matches(m,v.snapshot) AND p.expires_at>clock_timestamp()`, mandateID, p.UserID, bucketID, version).Scan(&saved)
	if err != nil {
		return MandateConsent{}, mapError(err)
	}
	if saved != expectedSnapshotDigest {
		return MandateConsent{}, ErrConflict
	}
	method, verified, err := stepUp.VerifyExecutionStepUp(ctx, p.UserID, code)
	code = ""
	if err != nil {
		return MandateConsent{}, err
	}
	if method != "totp" || verified.IsZero() {
		return MandateConsent{}, ErrNotAuthorized
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return MandateConsent{}, err
	}
	defer rollback(tx)
	var generation int64
	if err = tx.QueryRow(ctx, `SELECT lock_execution_consent_controls($1,$2)`, p.UserID, bucketID).Scan(&generation); err != nil {
		return MandateConsent{}, mapError(err)
	}
	mandate, digest, err := readCurrentMandate(ctx, tx, p.UserID, bucketID, mandateID, version)
	if err != nil {
		return MandateConsent{}, err
	}
	if digest != expectedSnapshotDigest {
		return MandateConsent{}, ErrConflict
	}
	var enabled *time.Time
	if err = tx.QueryRow(ctx, `SELECT enabled_at FROM auth_totp_factors WHERE user_id=$1 FOR UPDATE`, p.UserID).Scan(&enabled); err != nil {
		return MandateConsent{}, ErrNotAuthorized
	}
	var now, pilotExpiry time.Time
	var account, connection string
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp(),expires_at,financial_account_id::text,provider_connection_id::text FROM execution_pilot_allocations WHERE capital_bucket_id=$1 AND owner_id=$2`, bucketID, p.UserID).Scan(&now, &pilotExpiry, &account, &connection); err != nil {
		return MandateConsent{}, mapError(err)
	}
	if enabled == nil || verified.After(now) || now.Sub(verified) > 10*time.Second {
		return MandateConsent{}, ErrNotAuthorized
	}
	until := now.Add(24 * time.Hour)
	if pilotExpiry.Before(until) {
		until = pilotExpiry
	}
	if mandate.EffectiveUntil != nil && mandate.EffectiveUntil.Before(until) {
		until = *mandate.EffectiveUntil
	}
	if !until.After(now) {
		return MandateConsent{}, ErrNotAuthorized
	}
	var a MandateConsent
	err = tx.QueryRow(ctx, `INSERT INTO execution_mandate_approvals(owner_id,financial_account_id,provider_connection_id,capital_bucket_id,mandate_id,mandate_version,snapshot_digest,credential_generation,mfa_method,mfa_verified_at,mfa_enabled_at,approved_at,expires_at,id)
	 VALUES($1,$2,$3,$4,$5,$6,$7,$8,'totp',$9,$10,$11,$12,COALESCE(NULLIF($13,'')::uuid,gen_random_uuid()))
	 RETURNING id::text,owner_id::text,financial_account_id::text,provider_connection_id::text,capital_bucket_id::text,mandate_id::text,mandate_version,snapshot_digest,credential_generation,approved_at,expires_at`,
		p.UserID, account, connection, bucketID, mandateID, version, digest, generation, verified, *enabled, now, until, fixedID).Scan(&a.ID, &a.OwnerID, &a.AccountID, &a.ConnectionID, &a.CapitalBucketID, &a.MandateID, &a.MandateVersion, &a.SnapshotDigest, &a.CredentialGeneration, &a.ApprovedAt, &a.ExpiresAt)
	if err != nil {
		return MandateConsent{}, mapError(err)
	}
	if err = tx.Commit(ctx); err != nil {
		// Return the known receipt identity so callers can recover exactly once.
		return a, ErrCommitUnknown
	}
	return a, nil
}

// ReadMandateConsent is recovery-only: a historical receipt is not proof of
// current authority, continued MFA enrollment, current policy or non-revocation.
func (s *PostgresStore) ReadMandateConsent(ctx context.Context, ownerID, consentID string) (MandateConsent, error) {
	if !validUUID(ownerID) || !validUUID(consentID) {
		return MandateConsent{}, ErrInvalid
	}
	var a MandateConsent
	err := s.db.QueryRow(ctx, `SELECT id::text,owner_id::text,financial_account_id::text,provider_connection_id::text,capital_bucket_id::text,mandate_id::text,mandate_version,snapshot_digest,credential_generation,approved_at,expires_at
	 FROM execution_mandate_approvals WHERE owner_id=$1 AND id=$2`, ownerID, consentID).Scan(&a.ID, &a.OwnerID, &a.AccountID, &a.ConnectionID, &a.CapitalBucketID, &a.MandateID, &a.MandateVersion, &a.SnapshotDigest, &a.CredentialGeneration, &a.ApprovedAt, &a.ExpiresAt)
	return a, mapError(err)
}

// RevokeMandateConsent is append-only and idempotent. Locking the consent row
// FOR UPDATE serializes even the first revocation with claim/final-send checks.
func (s *PostgresStore) RevokeMandateConsent(ctx context.Context, ownerID, consentID string) error {
	if !validUUID(ownerID) || !validUUID(consentID) {
		return ErrInvalid
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(tx)
	var id string
	if err = tx.QueryRow(ctx, `SELECT id::text FROM execution_mandate_approvals WHERE owner_id=$1 AND id=$2 FOR UPDATE`, ownerID, consentID).Scan(&id); err != nil {
		return mapError(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO execution_mandate_revocations(approval_id) VALUES($1) ON CONFLICT DO NOTHING`, id); err != nil {
		return mapError(err)
	}
	if err = tx.Commit(ctx); err != nil {
		return ErrCommitUnknown
	}
	return nil
}
