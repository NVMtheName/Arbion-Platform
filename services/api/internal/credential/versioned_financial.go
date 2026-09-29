package credential

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

var errFinancialVersionUnavailable = errors.New("financial credential version unavailable")

type financialVersionStore interface {
	GetFinancialVersion(context.Context, Locator) ([]byte, int64, error)
}

// RetrieveFinancialVersion returns the generation from the same storage
// snapshot as the decrypted credential. A caller must compare it with current
// control-plane state; a snapshot-backed store may legitimately return an older
// version. Unversioned stores are not safe substitutes and fail closed.
func (v *EncryptedVault) RetrieveFinancialVersion(ctx context.Context, l Locator) ([]byte, int64, error) {
	if v == nil || l.Class != Financial {
		return nil, 0, errFinancialVersionUnavailable
	}
	store, ok := v.store.(financialVersionStore)
	if !ok {
		return nil, 0, errFinancialVersionUnavailable
	}
	ciphertext, generation, err := store.GetFinancialVersion(ctx, l)
	if err != nil {
		return nil, 0, err
	}
	if generation <= 0 {
		return nil, 0, errFinancialVersionUnavailable
	}
	// Use the same AEAD and locator-bound associated data as Retrieve. Neither
	// a different owner/connection nor an AI credential can cross this boundary.
	n := v.aead.NonceSize()
	if len(ciphertext) < n {
		return nil, 0, errors.New("credential ciphertext is invalid")
	}
	plaintext, err := v.aead.Open(nil, ciphertext[:n], ciphertext[n:], l.aad())
	if err != nil {
		return nil, 0, errors.New("credential authentication failed")
	}
	return plaintext, generation, nil
}

// GetFinancialVersion selects material and generation atomically, including
// when this store uses an older transaction snapshot. It never separately reads
// the version or substitutes a managed credential reference.
func (s *PostgresStore) GetFinancialVersion(ctx context.Context, l Locator) ([]byte, int64, error) {
	if s == nil || l.Class != Financial {
		return nil, 0, errFinancialVersionUnavailable
	}
	var ciphertext []byte
	var generation int64
	err := s.db.QueryRow(ctx, `SELECT encrypted_credential_payload,credential_generation FROM provider_connections
		WHERE id=$1 AND user_id=$2 AND provider_category=$3 AND encrypted_credential_payload IS NOT NULL AND credential_reference IS NULL`, l.ConnectionID, l.UserID, l.Class).Scan(&ciphertext, &generation)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, 0, ErrNotFound
	}
	if err != nil {
		return nil, 0, err
	}
	if generation <= 0 {
		return nil, 0, errFinancialVersionUnavailable
	}
	return ciphertext, generation, nil
}
