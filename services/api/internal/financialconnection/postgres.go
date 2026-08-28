package financialconnection

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/arbion/platform/services/api/internal/financial"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresStore struct{ db *pgxpool.Pool }

func NewPostgresStore(db *pgxpool.Pool) *PostgresStore { return &PostgresStore{db: db} }

const connectionColumns = `id::text,provider_name,display_name,status,token_expires_at,authorization_expires_at,last_verified_at,credential_storage,created_at,updated_at`
const joinedConnectionColumns = `p.id::text,p.provider_name,p.display_name,p.status,p.token_expires_at,p.authorization_expires_at,p.last_verified_at,p.credential_storage,p.created_at,p.updated_at`

func scanConnection(row pgx.Row) (Connection, error) {
	var c Connection
	e := row.Scan(&c.ID, &c.Provider, &c.DisplayName, &c.Status, &c.TokenExpiresAt, &c.AuthorizationExpiresAt, &c.LastSyncedAt, &c.CredentialStorage, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(e, pgx.ErrNoRows) {
		e = ErrNotFound
	}
	return c, e
}
func (s *PostgresStore) ListConnections(ctx context.Context, user string) ([]Connection, error) {
	rows, e := s.db.Query(ctx, `SELECT `+connectionColumns+`,
		(SELECT count(*)::integer
		 FROM automation_mandates m
		 JOIN financial_accounts a ON a.id=m.financial_account_id AND a.user_id=m.user_id
		 WHERE a.provider_connection_id=p.id AND m.user_id=p.user_id
		   AND m.status IN ('READY','PAUSED')),
		(SELECT count(*)::integer
		 FROM strategy_instances i
		 JOIN financial_accounts a ON a.id=i.financial_account_id AND a.user_id=i.user_id
		 WHERE a.provider_connection_id=p.id AND i.user_id=p.user_id
		   AND i.status IN ('ACTIVE','PAUSED'))
	FROM provider_connections p
	WHERE p.user_id=$1 AND p.provider_category='financial'
	ORDER BY p.created_at`, user)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Connection{}
	for rows.Next() {
		var c Connection
		e := rows.Scan(
			&c.ID, &c.Provider, &c.DisplayName, &c.Status, &c.TokenExpiresAt,
			&c.AuthorizationExpiresAt, &c.LastSyncedAt, &c.CredentialStorage,
			&c.CreatedAt, &c.UpdatedAt, &c.ProtectedMandateCount,
			&c.ActiveStrategyCount,
		)
		if e != nil {
			return nil, e
		}
		c.RuntimeProtected = c.ProtectedMandateCount > 0 || c.ActiveStrategyCount > 0
		out = append(out, c)
	}
	return out, rows.Err()
}
func (s *PostgresStore) UpsertConnection(ctx context.Context, user, provider, displayName string, expires, authorizationExpires *time.Time) (Connection, error) {
	return scanConnection(s.db.QueryRow(ctx, `INSERT INTO provider_connections(user_id,provider_category,provider_name,display_name,status,token_expires_at,authorization_expires_at) VALUES($1,'financial',$2,$3,'pending',$4,$5) ON CONFLICT(user_id,provider_category,provider_name,display_name) DO UPDATE SET status='pending',token_expires_at=excluded.token_expires_at,authorization_expires_at=excluded.authorization_expires_at,updated_at=now() RETURNING `+connectionColumns, user, provider, displayName, expires, authorizationExpires))
}
func (s *PostgresStore) UpsertConnectionForAccount(ctx context.Context, user, provider, displayName, providerAccountID string, expires, authorizationExpires *time.Time) (Connection, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Connection{}, err
	}
	defer tx.Rollback(ctx)
	lockKey := user + ":" + provider + ":" + providerAccountID
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, lockKey); err != nil {
		return Connection{}, err
	}
	connection, err := scanConnection(tx.QueryRow(ctx, `SELECT `+joinedConnectionColumns+` FROM provider_connections p JOIN financial_accounts a ON a.provider_connection_id=p.id WHERE p.user_id=$1 AND p.provider_category='financial' AND p.provider_name=$2 AND a.provider_account_id=$3 LIMIT 1 FOR UPDATE OF p`, user, provider, providerAccountID))
	if err == nil {
		connection, err = scanConnection(tx.QueryRow(ctx, `UPDATE provider_connections SET status='pending',token_expires_at=$2,authorization_expires_at=$3,updated_at=now() WHERE id=$1 RETURNING `+connectionColumns, connection.ID, expires, authorizationExpires))
	} else if errors.Is(err, ErrNotFound) {
		connection, err = scanConnection(tx.QueryRow(ctx, `INSERT INTO provider_connections(user_id,provider_category,provider_name,display_name,status,token_expires_at,authorization_expires_at) VALUES($1,'financial',$2,$3,'pending',$4,$5) ON CONFLICT(user_id,provider_category,provider_name,display_name) DO UPDATE SET status='pending',token_expires_at=excluded.token_expires_at,authorization_expires_at=excluded.authorization_expires_at,updated_at=now() RETURNING `+connectionColumns, user, provider, displayName, expires, authorizationExpires))
	}
	if err != nil {
		return Connection{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Connection{}, err
	}
	return connection, nil
}
func (s *PostgresStore) GetConnection(ctx context.Context, user, id string) (Connection, error) {
	return scanConnection(s.db.QueryRow(ctx, `SELECT `+connectionColumns+` FROM provider_connections WHERE id=$1 AND user_id=$2 AND provider_category='financial'`, id, user))
}
func (s *PostgresStore) SetStatus(ctx context.Context, user, id, status string, expires *time.Time) (Connection, error) {
	return scanConnection(s.db.QueryRow(ctx, `UPDATE provider_connections SET status=$3,token_expires_at=COALESCE($4,token_expires_at),last_verified_at=CASE WHEN $3='active' THEN now() ELSE last_verified_at END,updated_at=now() WHERE id=$1 AND user_id=$2 AND provider_category='financial' RETURNING `+connectionColumns, id, user, status, expires))
}
func (s *PostgresStore) ConnectionInUse(ctx context.Context, user, id string) (bool, error) {
	var inUse bool
	err := s.db.QueryRow(ctx, `SELECT EXISTS(
		SELECT 1
		FROM financial_accounts a
		JOIN provider_connections p ON p.id=a.provider_connection_id
		WHERE p.id=$2 AND p.user_id=$1 AND p.provider_category='financial' AND (
			EXISTS(SELECT 1 FROM automation_mandates m WHERE m.financial_account_id=a.id AND m.user_id=$1 AND m.status IN ('READY','PAUSED'))
			OR EXISTS(SELECT 1 FROM strategy_instances i WHERE i.financial_account_id=a.id AND i.user_id=$1 AND i.status IN ('ACTIVE','PAUSED'))
		)
	)`, user, id).Scan(&inUse)
	return inUse, err
}
func (s *PostgresStore) SyncAccounts(ctx context.Context, user, connection string, accounts []financial.FinancialAccount) error {
	tx, e := s.db.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	_, e = tx.Exec(ctx, `UPDATE financial_accounts SET status='unavailable',updated_at=now() WHERE user_id=$1 AND provider_connection_id=$2`, user, connection)
	if e != nil {
		return e
	}
	for _, a := range accounts {
		caps, _ := json.Marshal(a.Capabilities)
		_, e = tx.Exec(ctx, `INSERT INTO financial_accounts(user_id,provider_connection_id,provider_name,provider_account_id,display_name,masked_identifier,account_type,base_currency,status,capabilities) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'active',$9) ON CONFLICT(provider_connection_id,provider_account_id) DO UPDATE SET display_name=excluded.display_name,masked_identifier=excluded.masked_identifier,account_type=excluded.account_type,base_currency=excluded.base_currency,status='active',capabilities=excluded.capabilities,last_synced_at=now(),updated_at=now()`, user, connection, a.Provider, a.ProviderAccountID, a.DisplayName, a.MaskedIdentifier, a.AccountType, a.BaseCurrency, caps)
		if e != nil {
			return e
		}
	}
	_, e = tx.Exec(ctx, `UPDATE provider_connections SET last_verified_at=now(),updated_at=now() WHERE id=$1 AND user_id=$2`, connection, user)
	if e != nil {
		return e
	}
	return tx.Commit(ctx)
}

const accountColumns = `id::text,user_id::text,provider_connection_id::text,provider_name,provider_account_id,display_name,masked_identifier,account_type,base_currency,status,capabilities,discovered_at,last_synced_at`

func scanAccount(row pgx.Row) (financial.FinancialAccount, error) {
	var a financial.FinancialAccount
	var raw []byte
	e := row.Scan(&a.ID, &a.UserID, &a.ProviderConnectionID, &a.Provider, &a.ProviderAccountID, &a.DisplayName, &a.MaskedIdentifier, &a.AccountType, &a.BaseCurrency, &a.Status, &raw, &a.DiscoveredAt, &a.LastSyncedAt)
	if errors.Is(e, pgx.ErrNoRows) {
		e = ErrNotFound
	}
	if e == nil {
		e = json.Unmarshal(raw, &a.Capabilities)
	}
	return a, e
}
func (s *PostgresStore) ListAccounts(ctx context.Context, user string) ([]financial.FinancialAccount, error) {
	rows, e := s.db.Query(ctx, `SELECT `+accountColumns+` FROM financial_accounts WHERE user_id=$1 ORDER BY display_name`, user)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []financial.FinancialAccount{}
	for rows.Next() {
		a, e := scanAccount(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
func (s *PostgresStore) GetAccount(ctx context.Context, user, id string) (financial.FinancialAccount, error) {
	return scanAccount(s.db.QueryRow(ctx, `SELECT `+accountColumns+` FROM financial_accounts WHERE id=$1 AND user_id=$2`, id, user))
}
func (s *PostgresStore) Retire(ctx context.Context, user, id string) error {
	tx, e := s.db.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, `UPDATE financial_accounts SET status='closed',updated_at=now() WHERE provider_connection_id=$1 AND user_id=$2`, id, user); e != nil {
		return e
	}
	tag, e := tx.Exec(ctx, `UPDATE provider_connections SET status='revoked',updated_at=now() WHERE id=$1 AND user_id=$2 AND provider_category='financial'`, id, user)
	if e != nil || tag.RowsAffected() != 1 {
		if e == nil {
			e = ErrNotFound
		}
		return e
	}
	return tx.Commit(ctx)
}
func (s *PostgresStore) WithLock(ctx context.Context, id string, fn func() error) error {
	c, e := s.db.Acquire(ctx)
	if e != nil {
		return e
	}
	defer c.Release()
	if _, e = c.Exec(ctx, `SELECT pg_advisory_lock(hashtextextended($1,0))`, id); e != nil {
		return e
	}
	defer c.Exec(context.Background(), `SELECT pg_advisory_unlock(hashtextextended($1,0))`, id)
	return fn()
}
