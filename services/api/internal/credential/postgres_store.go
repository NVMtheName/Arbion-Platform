package credential

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type DB interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}
type PostgresStore struct{ db DB }

func NewPostgresStore(db DB) *PostgresStore { return &PostgresStore{db: db} }
func (s *PostgresStore) Put(ctx context.Context, l Locator, payload []byte, create bool) error {
	condition := "encrypted_credential_payload IS NOT NULL"
	if create {
		condition = "encrypted_credential_payload IS NULL"
	}
	tag, err := s.db.Exec(ctx, `UPDATE provider_connections SET encrypted_credential_payload=$1, credential_storage='encrypted_database', updated_at=now() WHERE id=$2 AND user_id=$3 AND provider_category=$4 AND `+condition, payload, l.ConnectionID, l.UserID, l.Class)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrNotFound
	}
	return nil
}
func (s *PostgresStore) Get(ctx context.Context, l Locator) ([]byte, error) {
	var p []byte
	err := s.db.QueryRow(ctx, `SELECT encrypted_credential_payload FROM provider_connections WHERE id=$1 AND user_id=$2 AND provider_category=$3 AND encrypted_credential_payload IS NOT NULL`, l.ConnectionID, l.UserID, l.Class).Scan(&p)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return p, err
}
func (s *PostgresStore) Delete(ctx context.Context, l Locator) error {
	tag, err := s.db.Exec(ctx, `UPDATE provider_connections SET encrypted_credential_payload=NULL, credential_reference=NULL, updated_at=now() WHERE id=$1 AND user_id=$2 AND provider_category=$3 AND (encrypted_credential_payload IS NOT NULL OR credential_reference IS NOT NULL)`, l.ConnectionID, l.UserID, l.Class)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrNotFound
	}
	return nil
}
