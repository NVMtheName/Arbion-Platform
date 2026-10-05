package execution

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/arbion/platform/services/api/internal/credential"
	"github.com/arbion/platform/services/api/internal/financial"
	"github.com/arbion/platform/services/api/internal/risk"
	"github.com/jackc/pgx/v5"
)

// The version must be read atomically with the encrypted material, not from a
// separate query or a cached plaintext value. Legacy vault reads cannot satisfy
// this boundary, even if independent before/after control queries agree.
type FinancialCredentialReader interface {
	RetrieveFinancialVersion(context.Context, credential.Locator) ([]byte, int64, error)
}

// CapturePreflight is an unwired server-only read/preview operation. No caller
// can supply credentials, permission flags or normalized provider evidence.
// Vault/provider work occurs outside transactions. The monotonically increasing
// generation is checked before retrieval and again under the save locks.
func (s *PostgresStore) CapturePreflight(ctx context.Context, ownerID, orderID string, vault FinancialCredentialReader, provider ExecutionPreflightProvider) (string, error) {
	if !validUUID(ownerID) || !validUUID(orderID) || vault == nil || provider == nil {
		return "", ErrNotAuthorized
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer rollback(tx)
	o, err := readOrder(ctx, tx, ownerID, "id", orderID)
	if err != nil {
		return "", err
	}
	var generation int64
	if err = tx.QueryRow(ctx, `SELECT lock_execution_claim_controls($1)`, orderID).Scan(&generation); err != nil {
		return "", mapError(err)
	}
	if _, err = checkPilotLimits(ctx, tx, orderID); err != nil {
		return "", err
	}
	portfolio, err := executionPortfolio(ctx, tx, o)
	if err != nil {
		return "", err
	}
	if err = tx.Commit(ctx); err != nil {
		return "", ErrCommitUnknown
	}
	raw, materialGeneration, err := vault.RetrieveFinancialVersion(ctx, credential.Locator{ConnectionID: o.Request.ConnectionID, UserID: ownerID, Class: credential.Financial})
	defer clear(raw)
	if err != nil || materialGeneration != generation {
		return "", ErrNotAuthorized
	}
	var cr financial.Credentials
	if len(raw) > 16384 || json.Unmarshal(raw, &cr) != nil || cr.PortfolioID != portfolio {
		return "", ErrNotAuthorized
	}
	p, err := provider.CollectExecutionPreflight(ctx, &cr, o, portfolio)
	cr = financial.Credentials{}
	if err != nil {
		return "", err
	}
	return s.savePreflight(ctx, o, generation, portfolio, p)
}

func executionPortfolio(ctx context.Context, tx pgx.Tx, o Order) (string, error) {
	var account string
	var usable bool
	err := tx.QueryRow(ctx, `SELECT a.provider_account_id,c.encrypted_credential_payload IS NOT NULL AND c.credential_reference IS NULL
		FROM financial_accounts a JOIN provider_connections c ON c.id=a.provider_connection_id
		WHERE a.id=$1 AND a.user_id=$2 AND c.id=$3 AND c.provider_name='coinbase' AND c.provider_category='financial'`, o.Request.AccountID, o.Request.OwnerID, o.Request.ConnectionID).Scan(&account, &usable)
	portfolio := strings.TrimPrefix(account, "portfolio:")
	if err != nil || !usable || account != "portfolio:"+portfolio || !validUUID(portfolio) {
		return "", ErrNotAuthorized
	}
	return portfolio, nil
}

func latestPreflightReconciliation(ctx context.Context, tx pgx.Tx, o Order, p ProviderPreflight, now time.Time) (VerifiedPreflight, error) {
	var id, provider string
	var rec risk.ReconciliationSnapshot
	err := tx.QueryRow(ctx, `SELECT id::text,financial_account_id::text,provider_name,comparison_status,balances_status,positions_status,autonomy_signal,autonomy_enforcement_active,blocks_new_actions,change_count,blocking_change_count,observed_at
	 FROM portfolio_reconciliations WHERE user_id=$1 AND financial_account_id=$2 ORDER BY observed_at DESC,id DESC LIMIT 1`, o.Request.OwnerID, o.Request.AccountID).Scan(&id, &rec.AccountID, &provider, &rec.ComparisonStatus, &rec.BalancesStatus, &rec.PositionsStatus, &rec.AutonomySignal, &rec.AutonomyEnforcementActive, &rec.BlocksNewActions, &rec.ChangeCount, &rec.BlockingChangeCount, &rec.ObservedAt)
	if err != nil || provider != "coinbase" || rec.ObservedAt.Before(o.CreatedAt) || rec.ObservedAt.After(now) || now.Sub(rec.ObservedAt) > 30*time.Second || risk.CheckAutonomousReconciliation(&rec, o.Request.AccountID, now).Result != risk.Pass {
		return VerifiedPreflight{}, ErrNotAuthorized
	}
	until := p.StartedAt.Add(30 * time.Second)
	for _, end := range []time.Time{rec.ObservedAt.Add(30 * time.Second), p.QuoteObservedAt.Add(10 * time.Second)} {
		if end.Before(until) {
			until = end
		}
	}
	proof := VerifiedPreflight{RequestDigest: o.RequestDigest, AccountID: o.Request.AccountID, ConnectionID: o.Request.ConnectionID, ReconciliationID: id, ObservedAt: p.StartedAt, ExpiresAt: until, CashUSD: p.CashUSD, AvailableCashUSD: p.AvailableCashUSD, TotalBase: p.TotalBase, AvailableBase: p.AvailableBase}
	if !until.After(now) {
		return VerifiedPreflight{}, ErrNotAuthorized
	}
	if err = matchFundingReconciliation(ctx, tx, o, proof); err != nil {
		return VerifiedPreflight{}, err
	}
	return proof, nil
}

func (s *PostgresStore) savePreflight(ctx context.Context, o Order, generation int64, portfolio string, p ProviderPreflight) (string, error) {
	// PostgreSQL timestamptz has microsecond precision; persist and compare the
	// same explicit precision in JSON and relational timestamps.
	p.StartedAt = p.StartedAt.UTC().Truncate(time.Microsecond)
	p.CompletedAt = p.CompletedAt.UTC().Truncate(time.Microsecond)
	p.QuoteObservedAt = p.QuoteObservedAt.UTC().Truncate(time.Microsecond)
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer rollback(tx)
	var current int64
	if err = tx.QueryRow(ctx, `SELECT lock_execution_claim_controls($1)`, o.ID).Scan(&current); err != nil {
		return "", mapError(err)
	}
	if _, err = checkPilotLimits(ctx, tx, o.ID); err != nil {
		return "", err
	}
	bound, err := executionPortfolio(ctx, tx, o)
	if err != nil || bound != portfolio || current != generation {
		return "", ErrNotAuthorized
	}
	var now time.Time
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return "", err
	}
	if err = validateProviderPreflight(p, o, portfolio, now); err != nil {
		return "", err
	}
	proof, err := latestPreflightReconciliation(ctx, tx, o, p, now)
	if err != nil {
		return "", err
	}
	body, _ := json.Marshal(p)
	var id string
	err = tx.QueryRow(ctx, `INSERT INTO execution_provider_preflights(order_id,owner_id,financial_account_id,provider_connection_id,credential_generation,request_digest,portfolio_id,reconciliation_id,observed_at,expires_at,evidence)
	 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING id::text`, o.ID, o.Request.OwnerID, o.Request.AccountID, o.Request.ConnectionID, generation, o.RequestDigest, portfolio, proof.ReconciliationID, p.StartedAt, proof.ExpiresAt, body).Scan(&id)
	if err != nil {
		return "", mapError(err)
	}
	if err = tx.Commit(ctx); err != nil {
		return "", ErrCommitUnknown
	}
	return id, nil
}

// SavedPreflightVerifier only reads immutable server-collected evidence on the
// claim transaction. It has no HTTP client/vault and cannot refresh a provider.
// The exact evidence ID is pinned explicitly; it never chooses a newer preview.
type SavedPreflightVerifier struct{ evidenceID string }

func NewSavedPreflightVerifier(id string) *SavedPreflightVerifier {
	return &SavedPreflightVerifier{evidenceID: id}
}

func (v *SavedPreflightVerifier) VerifyDispatchPreflight(ctx context.Context, tx pgx.Tx, o Order, generation int64) (VerifiedPreflight, error) {
	if v == nil || !validUUID(v.evidenceID) {
		return VerifiedPreflight{}, ErrNotAuthorized
	}
	var body []byte
	var savedGeneration int64
	var portfolio, digest, reconciliation string
	var observed, expires, now time.Time
	err := tx.QueryRow(ctx, `SELECT evidence,credential_generation,portfolio_id::text,request_digest,reconciliation_id::text,observed_at,expires_at,clock_timestamp() FROM execution_provider_preflights WHERE id=$1 AND order_id=$2 AND owner_id=$3 AND financial_account_id=$4 AND provider_connection_id=$5`, v.evidenceID, o.ID, o.Request.OwnerID, o.Request.AccountID, o.Request.ConnectionID).Scan(&body, &savedGeneration, &portfolio, &digest, &reconciliation, &observed, &expires, &now)
	if err != nil || savedGeneration != generation || digest != o.RequestDigest || !expires.After(now) {
		return VerifiedPreflight{}, ErrNotAuthorized
	}
	bound, err := executionPortfolio(ctx, tx, o)
	if err != nil || bound != portfolio {
		return VerifiedPreflight{}, ErrNotAuthorized
	}
	var p ProviderPreflight
	if !decodeProviderPreflight(body, &p) || !p.StartedAt.Equal(observed) {
		return VerifiedPreflight{}, ErrNotAuthorized
	}
	if err = validateProviderPreflight(p, o, portfolio, now); err != nil {
		return VerifiedPreflight{}, err
	}
	proof, err := latestPreflightReconciliation(ctx, tx, o, p, now)
	if err != nil {
		return VerifiedPreflight{}, err
	}
	if proof.ReconciliationID != reconciliation || !proof.ExpiresAt.Equal(expires) {
		return VerifiedPreflight{}, ErrNotAuthorized
	}
	proof.EvidenceID = v.evidenceID
	proof.CredentialGeneration = generation
	return proof, nil
}

func decodeProviderPreflight(body []byte, p *ProviderPreflight) bool {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if decoder.Decode(p) != nil {
		return false
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(body, &fields) != nil {
		return false
	}
	// JSONB has unique exact keys. Require the complete canonical field set and
	// types, including every false-valued safety flag; omitted/null is not false.
	canonicalBody, _ := json.Marshal(p)
	var canonicalFields map[string]json.RawMessage
	if json.Unmarshal(canonicalBody, &canonicalFields) != nil || len(fields) != len(canonicalFields) {
		return false
	}
	for name, expected := range canonicalFields {
		actual, ok := fields[name]
		if !ok || bytes.Equal(bytes.TrimSpace(actual), []byte("null")) || len(actual) == 0 || (expected[0] == '"') != (actual[0] == '"') {
			return false
		}
	}
	return true
}
