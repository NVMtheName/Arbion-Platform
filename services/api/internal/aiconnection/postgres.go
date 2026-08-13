package aiconnection

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type DB interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}
type PostgresStore struct {
	db       DB
	registry Registry
}

func NewPostgresStore(db DB, r Registry) *PostgresStore { return &PostgresStore{db, r} }

const columns = `id::text,provider_name,display_name,status,(status<>'disabled'),COALESCE(credential_metadata->>'hint',''),created_at,updated_at,last_verified_at`

func (s *PostgresStore) scan(row pgx.Row) (Connection, error) {
	var c Connection
	err := row.Scan(&c.ID, &c.Provider, &c.DisplayName, &c.Status, &c.Enabled, &c.CredentialHint, &c.CreatedAt, &c.UpdatedAt, &c.LastVerifiedAt)
	if p, ok := s.registry.Get(c.Provider); ok {
		c.ProviderLabel = p.Label
	}
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrNotFound
	}
	return c, err
}
func (s *PostgresStore) List(ctx context.Context, user string) ([]Connection, error) {
	rows, err := s.db.Query(ctx, `SELECT `+columns+` FROM provider_connections WHERE user_id=$1 AND provider_category='ai' ORDER BY created_at`, user)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Connection{}
	for rows.Next() {
		c, e := s.scan(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
func (s *PostgresStore) Create(ctx context.Context, user, provider, name, hint string) (Connection, error) {
	return s.scan(s.db.QueryRow(ctx, `INSERT INTO provider_connections(user_id,provider_category,provider_name,display_name,status,credential_metadata) VALUES($1,'ai',$2,$3,'pending',jsonb_build_object('hint',$4)) RETURNING `+columns, user, provider, name, hint))
}
func (s *PostgresStore) Get(ctx context.Context, user, id string) (Connection, error) {
	return s.scan(s.db.QueryRow(ctx, `SELECT `+columns+` FROM provider_connections WHERE id=$1 AND user_id=$2 AND provider_category='ai'`, id, user))
}
func (s *PostgresStore) Rename(ctx context.Context, user, id, name string) (Connection, error) {
	return s.scan(s.db.QueryRow(ctx, `UPDATE provider_connections SET display_name=$3,updated_at=now() WHERE id=$1 AND user_id=$2 AND provider_category='ai' RETURNING `+columns, id, user, name))
}
func (s *PostgresStore) SetStatus(ctx context.Context, user, id, status string) (Connection, error) {
	return s.scan(s.db.QueryRow(ctx, `UPDATE provider_connections SET status=$3,updated_at=now() WHERE id=$1 AND user_id=$2 AND provider_category='ai' RETURNING `+columns, id, user, status))
}
func (s *PostgresStore) SetCredentialPending(ctx context.Context, user, id, hint string) (Connection, error) {
	return s.scan(s.db.QueryRow(ctx, `UPDATE provider_connections SET status='pending',credential_metadata=jsonb_build_object('hint',$3),updated_at=now(),last_verified_at=NULL WHERE id=$1 AND user_id=$2 AND provider_category='ai' RETURNING `+columns, id, user, hint))
}
func (s *PostgresStore) SetVerification(ctx context.Context, user, id, status string, verified bool) (Connection, error) {
	return s.scan(s.db.QueryRow(ctx, `UPDATE provider_connections SET status=$3,last_verified_at=CASE WHEN $4 THEN now() ELSE last_verified_at END,updated_at=now() WHERE id=$1 AND user_id=$2 AND provider_category='ai' RETURNING `+columns, id, user, status, verified))
}
func (s *PostgresStore) GetPreference(ctx context.Context, user string) (*Preference, error) {
	var p Preference
	err := s.db.QueryRow(ctx, `SELECT provider_connection_id::text,model_id,updated_at FROM neural_engine_preferences WHERE user_id=$1`, user).Scan(&p.ConnectionID, &p.ModelID, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &p, err
}
func (s *PostgresStore) SetPreference(ctx context.Context, user, id, model string) (Preference, error) {
	var p Preference
	err := s.db.QueryRow(ctx, `INSERT INTO neural_engine_preferences(user_id,provider_connection_id,model_id) VALUES($1,$2,$3) ON CONFLICT(user_id) DO UPDATE SET provider_connection_id=excluded.provider_connection_id,model_id=excluded.model_id,updated_at=now() RETURNING provider_connection_id::text,model_id,updated_at`, user, id, model).Scan(&p.ConnectionID, &p.ModelID, &p.UpdatedAt)
	return p, err
}
func (s *PostgresStore) Delete(ctx context.Context, user, id string) error {
	tag, e := s.db.Exec(ctx, `DELETE FROM provider_connections WHERE id=$1 AND user_id=$2 AND provider_category='ai'`, id, user)
	if e != nil {
		return e
	}
	if tag.RowsAffected() != 1 {
		return ErrNotFound
	}
	return nil
}
func (s *PostgresStore) HasDependencies(ctx context.Context, user, id string) (bool, error) {
	var found bool
	e := s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM automation_configs a JOIN provider_connections p ON p.id=a.ai_provider_connection_id WHERE p.id=$1 AND p.user_id=$2 AND p.provider_category='ai')`, id, user).Scan(&found)
	return found, e
}
