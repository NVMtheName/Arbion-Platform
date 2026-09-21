package connectionguard

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// CanonicalLockKey uses the same UUID parser as the stored connection identity.
// Equivalent accepted spellings must contend on the same advisory lock. Other
// keys (including the financial reconciliation namespace) stay byte-for-byte
// unchanged. This normalizes lock identity only; it grants no row access.
func CanonicalLockKey(ctx context.Context, db interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, key string) (string, error) {
	var isUUID bool
	if err := db.QueryRow(ctx, `SELECT pg_input_is_valid($1::text,'uuid')`, key).Scan(&isUUID); err != nil {
		return "", err
	}
	if !isUUID {
		return key, nil
	}
	// Keep the cast in a separate statement: a custom plan may fold a constant
	// UUID cast in an unused CASE arm and reject a valid non-UUID lock key.
	var canonical string
	err := db.QueryRow(ctx, `SELECT $1::text::uuid::text`, key).Scan(&canonical)
	if err != nil {
		return "", err
	}
	return canonical, nil
}
