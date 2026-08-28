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
	Begin(context.Context) (pgx.Tx, error)
}
type PostgresStore struct {
	db       DB
	registry Registry
}

func NewPostgresStore(db DB, r Registry) *PostgresStore { return &PostgresStore{db, r} }

func (s *PostgresStore) WithLock(ctx context.Context, id string, fn func() error) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, id); err != nil {
		return err
	}
	if err = fn(); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

const columns = `id::text,provider_name,display_name,status,(status<>'disabled'),COALESCE(credential_metadata->>'hint',''),created_at,updated_at,last_verified_at,credential_generation`

func (s *PostgresStore) scan(row pgx.Row) (Connection, error) {
	var c Connection
	err := row.Scan(&c.ID, &c.Provider, &c.DisplayName, &c.Status, &c.Enabled, &c.CredentialHint, &c.CreatedAt, &c.UpdatedAt, &c.LastVerifiedAt, &c.CredentialGeneration)
	if p, ok := s.registry.Get(c.Provider); ok {
		c.ProviderLabel = p.Label
	}
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrNotFound
	}
	return c, err
}
func (s *PostgresStore) List(ctx context.Context, user string) ([]Connection, error) {
	rows, err := s.db.Query(ctx, `SELECT `+columns+`,
		(SELECT count(*)::integer FROM automation_mandates m
		 WHERE m.user_id=p.user_id AND m.ai_provider_connection_id=p.id
		   AND m.status IN ('READY','PAUSED')),
		(SELECT count(*)::integer
		 FROM strategy_instances i
		 JOIN automation_mandate_versions v
		   ON v.mandate_id=i.automation_mandate_id AND v.version_number=i.mandate_version
		 WHERE i.user_id=p.user_id AND i.status IN ('ACTIVE','PAUSED')
		   AND v.snapshot->>'ai_provider_connection_id'=p.id::text),
		(SELECT count(DISTINCT dependency.mandate_id)::integer
		 FROM (
			SELECT m.id AS mandate_id FROM automation_mandates m
			WHERE m.user_id=p.user_id AND m.ai_provider_connection_id=p.id
			UNION
			SELECT v.mandate_id FROM automation_mandate_versions v
			JOIN automation_mandates m ON m.id=v.mandate_id
			WHERE m.user_id=p.user_id AND v.snapshot->>'ai_provider_connection_id'=p.id::text
		 ) dependency),
		EXISTS(SELECT 1 FROM neural_engine_preferences n
		 WHERE n.user_id=p.user_id AND n.provider_connection_id=p.id)
	FROM provider_connections p
	WHERE p.user_id=$1 AND p.provider_category='ai'
	ORDER BY p.created_at`, user)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Connection{}
	for rows.Next() {
		var c Connection
		e := rows.Scan(
			&c.ID, &c.Provider, &c.DisplayName, &c.Status, &c.Enabled,
			&c.CredentialHint, &c.CreatedAt, &c.UpdatedAt, &c.LastVerifiedAt,
			&c.CredentialGeneration, &c.ProtectedMandateCount,
			&c.ActiveStrategyCount, &c.RetainedAutomationCount,
			&c.DefaultModelSelected,
		)
		if e != nil {
			return nil, e
		}
		if provider, ok := s.registry.Get(c.Provider); ok {
			c.ProviderLabel = provider.Label
		}
		c.RuntimeProtected = c.ProtectedMandateCount > 0 || c.ActiveStrategyCount > 0
		c.RemovalProtected = c.DefaultModelSelected || c.RetainedAutomationCount > 0
		out = append(out, c)
	}
	return out, rows.Err()
}
func (s *PostgresStore) Create(ctx context.Context, user, provider, name, hint string) (Connection, error) {
	return s.scan(s.db.QueryRow(ctx, `INSERT INTO provider_connections(user_id,provider_category,provider_name,display_name,status,credential_metadata) VALUES($1,'ai',$2,$3,'pending',jsonb_build_object('hint',$4::text)) RETURNING `+columns, user, provider, name, hint))
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
func (s *PostgresStore) SetVerification(ctx context.Context, user, id, status string, verified bool, generation int64) (Connection, error) {
	return s.scan(s.db.QueryRow(ctx, `UPDATE provider_connections SET status=$3,last_verified_at=CASE WHEN $4 THEN now() ELSE last_verified_at END,updated_at=now() WHERE id=$1 AND user_id=$2 AND provider_category='ai' AND credential_generation=$5 RETURNING `+columns, id, user, status, verified, generation))
}
func (s *PostgresStore) CommitStagedCredential(ctx context.Context, user, id, token, hint, expectedStatus, nextStatus string, generation int64, verified bool) (Connection, error) {
	return s.scan(s.db.QueryRow(ctx, `UPDATE provider_connections SET encrypted_credential_payload=pending_encrypted_credential_payload,pending_encrypted_credential_payload=NULL,pending_credential_token=NULL,credential_storage='encrypted_database',credential_reference=NULL,credential_metadata=jsonb_build_object('hint',$4::text),status=$6,last_verified_at=CASE WHEN $8 THEN now() ELSE NULL END,credential_generation=credential_generation+1,updated_at=now() WHERE id=$1 AND user_id=$2 AND provider_category='ai' AND pending_credential_token=$3 AND status=$5 AND credential_generation=$7 RETURNING `+columns, id, user, token, hint, expectedStatus, nextStatus, generation, verified))
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
	e := s.db.QueryRow(ctx, `SELECT EXISTS(
		SELECT 1
		FROM provider_connections p
		WHERE p.id=$1 AND p.user_id=$2 AND p.provider_category='ai' AND (
			EXISTS(
				SELECT 1 FROM neural_engine_preferences n
				WHERE n.user_id=p.user_id AND n.provider_connection_id=p.id
			)
			OR EXISTS(
				SELECT 1 FROM automation_mandates m
				WHERE m.user_id=p.user_id AND m.ai_provider_connection_id=p.id
			)
			OR EXISTS(
				SELECT 1
				FROM automation_mandate_versions v
				JOIN automation_mandates m ON m.id=v.mandate_id
				WHERE m.user_id=p.user_id AND v.snapshot->>'ai_provider_connection_id'=p.id::text
			)
		)
	)`, id, user).Scan(&found)
	return found, e
}

func (s *PostgresStore) ConnectionInUse(ctx context.Context, user, id string) (bool, error) {
	var found bool
	e := s.db.QueryRow(ctx, `SELECT EXISTS(
		SELECT 1
		FROM provider_connections p
		WHERE p.id=$1 AND p.user_id=$2 AND p.provider_category='ai' AND (
			EXISTS(
				SELECT 1 FROM automation_mandates m
				WHERE m.user_id=p.user_id AND m.ai_provider_connection_id=p.id
				AND m.status IN ('READY','PAUSED')
			)
			OR EXISTS(
				SELECT 1
				FROM strategy_instances i
				JOIN automation_mandates m ON m.id=i.automation_mandate_id AND m.user_id=i.user_id
				JOIN automation_mandate_versions v
					ON v.mandate_id=i.automation_mandate_id AND v.version_number=i.mandate_version
				WHERE i.user_id=p.user_id AND i.status IN ('ACTIVE','PAUSED')
				AND v.snapshot->>'ai_provider_connection_id'=p.id::text
			)
		)
	)`, id, user).Scan(&found)
	return found, e
}
