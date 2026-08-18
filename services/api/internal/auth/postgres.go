package auth

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type DB interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}
type PostgresStore struct{ db DB }

func NewPostgresStore(db DB) *PostgresStore { return &PostgresStore{db: db} }

const userColumns = `id::text,email,normalized_email,password_hash,COALESCE(display_name,''),status,email_verified_at,last_login_at,created_at,role,COALESCE((SELECT entitlement_key FROM user_entitlements e WHERE e.user_id=users.id AND e.status='active' AND (e.expires_at IS NULL OR e.expires_at>now()) ORDER BY CASE entitlement_key WHEN 'founder' THEN 5 WHEN 'premium' THEN 4 WHEN 'pro' THEN 3 WHEN 'internal_comped' THEN 2 ELSE 1 END DESC LIMIT 1),'free'),COALESCE((SELECT billing_required FROM user_entitlements e WHERE e.user_id=users.id AND e.status='active' AND (e.expires_at IS NULL OR e.expires_at>now()) ORDER BY CASE entitlement_key WHEN 'founder' THEN 5 WHEN 'premium' THEN 4 WHEN 'pro' THEN 3 WHEN 'internal_comped' THEN 2 ELSE 1 END DESC LIMIT 1),false)`

func scanUser(row pgx.Row) (User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Email, &u.NormalizedEmail, &u.PasswordHash, &u.DisplayName, &u.Status, &u.EmailVerifiedAt, &u.LastLoginAt, &u.CreatedAt, &u.Role, &u.Entitlement, &u.BillingRequired)
	return u, err
}
func (s *PostgresStore) Create(ctx context.Context, email, normalized, hash, name, status string) (User, error) {
	u, err := scanUser(s.db.QueryRow(ctx, `INSERT INTO users(email,normalized_email,password_hash,display_name,status) VALUES($1,$2,$3,NULLIF($4,''),$5) RETURNING `+userColumns, email, normalized, hash, name, status))
	var pe *pgconn.PgError
	if errors.As(err, &pe) && pe.Code == "23505" {
		return User{}, ErrConflict
	}
	return u, err
}

func (s *PostgresStore) ReplaceEmailToken(ctx context.Context, userID string, purpose EmailTokenPurpose, tokenHash []byte, expiresAt, now time.Time) error {
	_, err := s.db.Exec(ctx, `WITH replaced AS (
  UPDATE auth_email_tokens SET consumed_at=$5 WHERE user_id=$1 AND purpose=$2 AND consumed_at IS NULL
)
INSERT INTO auth_email_tokens(user_id,purpose,token_hash,expires_at,created_at) VALUES($1,$2,$3,$4,$5)`, userID, purpose, tokenHash, expiresAt, now)
	return err
}

func (s *PostgresStore) ActiveEmailTokenUser(ctx context.Context, purpose EmailTokenPurpose, tokenHash []byte, now time.Time) (string, error) {
	var userID string
	err := s.db.QueryRow(ctx, `SELECT t.user_id::text FROM auth_email_tokens t JOIN users u ON u.id=t.user_id WHERE t.purpose=$1 AND t.token_hash=$2 AND t.consumed_at IS NULL AND t.expires_at>$3 AND u.status='active'`, purpose, tokenHash, now).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrInvalidEmailToken
	}
	return userID, err
}

func (s *PostgresStore) ConsumeVerificationToken(ctx context.Context, tokenHash []byte, now time.Time) (string, error) {
	var userID string
	err := s.db.QueryRow(ctx, `WITH consumed AS (
  UPDATE auth_email_tokens SET consumed_at=$2
  WHERE purpose='verify_email' AND token_hash=$1 AND consumed_at IS NULL AND expires_at>$2
  RETURNING user_id
)
UPDATE users SET email_verified_at=COALESCE(email_verified_at,$2),status=CASE WHEN status='pending_verification' THEN 'active' ELSE status END,updated_at=$2
FROM consumed WHERE users.id=consumed.user_id RETURNING users.id::text`, tokenHash, now).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrInvalidEmailToken
	}
	return userID, err
}

func (s *PostgresStore) ConsumePasswordResetToken(ctx context.Context, tokenHash []byte, passwordHash string, now time.Time) (string, error) {
	var userID string
	err := s.db.QueryRow(ctx, `WITH consumed AS (
  UPDATE auth_email_tokens SET consumed_at=$3
  WHERE purpose='reset_password' AND token_hash=$1 AND consumed_at IS NULL AND expires_at>$3
  RETURNING user_id
)
UPDATE users SET password_hash=$2,updated_at=$3 FROM consumed WHERE users.id=consumed.user_id AND users.status='active' RETURNING users.id::text`, tokenHash, passwordHash, now).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrInvalidEmailToken
	}
	return userID, err
}

func (s *PostgresStore) MFAStatus(ctx context.Context, userID string) (MFAStatus, error) {
	var status MFAStatus
	err := s.db.QueryRow(ctx, `SELECT enabled_at IS NOT NULL,
  (SELECT count(*)::int FROM auth_mfa_recovery_codes r WHERE r.user_id=f.user_id AND r.used_at IS NULL)
FROM auth_totp_factors f WHERE f.user_id=$1`, userID).Scan(&status.Enabled, &status.RecoveryCodesRemaining)
	if errors.Is(err, pgx.ErrNoRows) {
		return MFAStatus{}, nil
	}
	return status, err
}

func (s *PostgresStore) SetPendingTOTP(ctx context.Context, userID string, ciphertext []byte, expiresAt, now time.Time) error {
	tag, err := s.db.Exec(ctx, `INSERT INTO auth_totp_factors(user_id,secret_ciphertext,pending_expires_at,created_at,updated_at)
VALUES($1,$2,$3,$4,$4)
ON CONFLICT (user_id) DO UPDATE SET secret_ciphertext=EXCLUDED.secret_ciphertext,pending_expires_at=EXCLUDED.pending_expires_at,last_used_step=-1,updated_at=EXCLUDED.updated_at
WHERE auth_totp_factors.enabled_at IS NULL`, userID, ciphertext, expiresAt, now)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrMFAAlreadyEnabled
	}
	return nil
}

func (s *PostgresStore) TOTPFactor(ctx context.Context, userID string) (TOTPFactor, error) {
	var factor TOTPFactor
	err := s.db.QueryRow(ctx, `SELECT secret_ciphertext,pending_expires_at,enabled_at FROM auth_totp_factors WHERE user_id=$1`, userID).Scan(&factor.SecretCiphertext, &factor.PendingExpiresAt, &factor.EnabledAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return TOTPFactor{}, ErrMFANotEnabled
	}
	return factor, err
}

func (s *PostgresStore) ActivateTOTP(ctx context.Context, userID string, recoveryHashes [][]byte, initialStep int64, now time.Time) error {
	var inserted int
	err := s.db.QueryRow(ctx, `WITH activated AS (
  UPDATE auth_totp_factors SET enabled_at=$2,pending_expires_at=NULL,last_used_step=$4,updated_at=$2
  WHERE user_id=$1 AND enabled_at IS NULL AND pending_expires_at>$2 RETURNING user_id
), removed AS (
  DELETE FROM auth_mfa_recovery_codes WHERE user_id IN (SELECT user_id FROM activated)
), inserted AS (
  INSERT INTO auth_mfa_recovery_codes(user_id,code_hash,created_at)
  SELECT activated.user_id,code_hash,$2 FROM activated CROSS JOIN unnest($3::bytea[]) AS code_hash
  RETURNING 1
)
SELECT count(*)::int FROM inserted`, userID, now, recoveryHashes, initialStep).Scan(&inserted)
	if err != nil {
		return err
	}
	if inserted != len(recoveryHashes) || inserted == 0 {
		return ErrMFAEnrollmentExpired
	}
	return nil
}

func (s *PostgresStore) AdvanceTOTPStep(ctx context.Context, userID string, step int64, now time.Time) (bool, error) {
	tag, err := s.db.Exec(ctx, `UPDATE auth_totp_factors SET last_used_step=$2,updated_at=$3 WHERE user_id=$1 AND enabled_at IS NOT NULL AND last_used_step<$2`, userID, step, now)
	return tag.RowsAffected() == 1, err
}

func (s *PostgresStore) ConsumeRecoveryCode(ctx context.Context, userID string, codeHash []byte, now time.Time) (bool, error) {
	tag, err := s.db.Exec(ctx, `UPDATE auth_mfa_recovery_codes SET used_at=$3 WHERE user_id=$1 AND code_hash=$2 AND used_at IS NULL`, userID, codeHash, now)
	return tag.RowsAffected() == 1, err
}

func (s *PostgresStore) ReplaceRecoveryCodes(ctx context.Context, userID string, recoveryHashes [][]byte, now time.Time) error {
	var inserted int
	err := s.db.QueryRow(ctx, `WITH target AS (
  SELECT user_id FROM auth_totp_factors WHERE user_id=$1 AND enabled_at IS NOT NULL
), removed AS (
  DELETE FROM auth_mfa_recovery_codes WHERE user_id IN (SELECT user_id FROM target)
), inserted AS (
  INSERT INTO auth_mfa_recovery_codes(user_id,code_hash,created_at)
  SELECT target.user_id,code_hash,$2 FROM target CROSS JOIN unnest($3::bytea[]) AS code_hash
  RETURNING 1
)
SELECT count(*)::int FROM inserted`, userID, now, recoveryHashes).Scan(&inserted)
	if err != nil {
		return err
	}
	if inserted != len(recoveryHashes) || inserted == 0 {
		return ErrMFANotEnabled
	}
	return nil
}

func (s *PostgresStore) DisableTOTP(ctx context.Context, userID string) (bool, error) {
	tag, err := s.db.Exec(ctx, `DELETE FROM auth_totp_factors WHERE user_id=$1 AND enabled_at IS NOT NULL`, userID)
	return tag.RowsAffected() == 1, err
}

func (s *PostgresStore) ByNormalizedEmail(ctx context.Context, email string) (User, error) {
	return scanUser(s.db.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE normalized_email=$1`, email))
}
func (s *PostgresStore) ByID(ctx context.Context, id string) (User, error) {
	return scanUser(s.db.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE id=$1`, id))
}
func (s *PostgresStore) RecordLogin(ctx context.Context, id string, at time.Time) error {
	_, err := s.db.Exec(ctx, `UPDATE users SET last_login_at=$2,updated_at=$2 WHERE id=$1`, id, at)
	return err
}
func (s *PostgresStore) UpdatePassword(ctx context.Context, id, currentHash, nextHash string, at time.Time) (bool, error) {
	tag, err := s.db.Exec(ctx, `UPDATE users SET password_hash=$3,updated_at=$4 WHERE id=$1 AND password_hash=$2`, id, currentHash, nextHash, at)
	return tag.RowsAffected() == 1, err
}
func (s *PostgresStore) Record(ctx context.Context, userID *string, action string, metadata map[string]any) error {
	raw, _ := json.Marshal(metadata)
	_, err := s.db.Exec(ctx, `INSERT INTO audit_events(user_id,actor_type,actor_id,action,target_type,target_id,metadata) VALUES($1,'user',$2,$3,'authentication',$2,$4)`, userID, userID, action, raw)
	return err
}
