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
	var canonical string
	err := db.QueryRow(ctx, `SELECT CASE
		WHEN pg_input_is_valid($1::text,'uuid') THEN $1::text::uuid::text
		ELSE $1::text END`, key).Scan(&canonical)
	return canonical, err
}
