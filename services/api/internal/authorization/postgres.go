package authorization

import (
	"context"
	"errors"
	"github.com/arbion/platform/services/api/internal/auth"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type DB interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}
type PostgresStore struct{ db DB }

func NewPostgresStore(db DB) *PostgresStore { return &PostgresStore{db: db} }

const effective = `SELECT u.id::text,u.email,COALESCE(u.display_name,''),u.role,COALESCE(e.entitlement_key,'free'),COALESCE(e.billing_required,false) FROM users u LEFT JOIN LATERAL (SELECT entitlement_key,billing_required FROM user_entitlements WHERE user_id=u.id AND status='active' AND starts_at<=now() AND (expires_at IS NULL OR expires_at>now()) ORDER BY CASE entitlement_key WHEN 'founder' THEN 5 WHEN 'premium' THEN 4 WHEN 'pro' THEN 3 WHEN 'internal_comped' THEN 2 ELSE 1 END DESC LIMIT 1)e ON true`

func scan(row pgx.Row) (User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Email, &u.DisplayName, &u.Role, &u.Entitlement, &u.BillingRequired)
	return u, err
}
func (s *PostgresStore) Get(c context.Context, id string) (User, error) {
	return scan(s.db.QueryRow(c, effective+` WHERE u.id=$1`, id))
}
func (s *PostgresStore) List(c context.Context) ([]User, error) {
	rows, e := s.db.Query(c, effective+` ORDER BY u.created_at,u.id`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []User{}
	for rows.Next() {
		u, e := scan(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, u)
	}
	return out, rows.Err()
}
func (s *PostgresStore) SetRole(c context.Context, id string, r Role) (Role, error) {
	var old Role
	e := s.db.QueryRow(c, `UPDATE users SET role=$2,updated_at=now() WHERE id=$1 RETURNING role`, id, r).Scan(&old)
	return old, e
}
func (s *PostgresStore) SetEntitlement(c context.Context, id string, e Entitlement, b bool, source string) (Entitlement, error) {
	var old Entitlement
	_ = s.db.QueryRow(c, `SELECT COALESCE((SELECT entitlement_key FROM user_entitlements WHERE user_id=$1 AND status='active' ORDER BY updated_at DESC LIMIT 1),'free')`, id).Scan(&old)
	_, err := s.db.Exec(c, `WITH revoked AS (
		UPDATE user_entitlements
		SET status='revoked',updated_at=now()
		WHERE user_id=$1 AND entitlement_key<>$2 AND status='active'
	)
	INSERT INTO user_entitlements(user_id,entitlement_key,source,status,billing_required)
	VALUES($1,$2,$3,'active',$4)
	ON CONFLICT(user_id,entitlement_key) DO UPDATE
	SET source=$3,status='active',billing_required=$4,expires_at=NULL,updated_at=now()`, id, e, source, b)
	return old, err
}
func (s *PostgresStore) BootstrapFounder(c context.Context, email string) (User, bool, error) {
	u, e := scan(s.db.QueryRow(c, effective+` WHERE u.normalized_email=$1`, auth.NormalizeEmail(email)))
	if e != nil {
		return User{}, false, e
	}
	changed := u.Role != RoleSuperadmin || u.Entitlement != EntitlementFounder || u.BillingRequired
	_, e = s.db.Exec(c, `WITH updated_user AS (
		UPDATE users
		SET role='superadmin',updated_at=now()
		WHERE id=$1
		RETURNING id
	)
	INSERT INTO user_entitlements(user_id,entitlement_key,source,status,billing_required)
	SELECT id,'founder','bootstrap','active',false FROM updated_user
	ON CONFLICT(user_id,entitlement_key) DO UPDATE
	SET source='bootstrap',status='active',billing_required=false,expires_at=NULL,updated_at=now()`, u.ID)
	if e != nil {
		return User{}, false, e
	}
	u, e = s.Get(c, u.ID)
	return u, changed, e
}

var _ = errors.Is
