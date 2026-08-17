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
func (s *PostgresStore) Create(ctx context.Context, email, normalized, hash, name string) (User, error) {
	u, err := scanUser(s.db.QueryRow(ctx, `INSERT INTO users(email,normalized_email,password_hash,display_name) VALUES($1,$2,$3,NULLIF($4,'')) RETURNING `+userColumns, email, normalized, hash, name))
	var pe *pgconn.PgError
	if errors.As(err, &pe) && pe.Code == "23505" {
		return User{}, ErrConflict
	}
	return u, err
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
