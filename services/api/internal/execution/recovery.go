package execution

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/arbion/platform/services/api/internal/credential"
	"github.com/arbion/platform/services/api/internal/financial"
	"github.com/jackc/pgx/v5"
)

type SubmissionRejection struct {
	OrderID, Code string
	ReceivedAt    time.Time
}

func (s *PostgresStore) ReadSubmissionRejection(ctx context.Context, ownerID, orderID string) (SubmissionRejection, error) {
	if !validUUID(ownerID) || !validUUID(orderID) {
		return SubmissionRejection{}, ErrInvalid
	}
	var r SubmissionRejection
	err := s.db.QueryRow(ctx, `SELECT order_id::text,error_code,received_at FROM execution_submission_rejections WHERE order_id=$1 AND owner_id=$2`, orderID, ownerID).Scan(&r.OrderID, &r.Code, &r.ReceivedAt)
	return r, mapError(err)
}

// RecoverSubmission is deliberately unwired. This read-only provider boundary
// uses the original client identity and saved preview, never Claim/SubmitOnce.
// Stops and expired/revoked order approval do not inhibit safe recovery. Current
// owner/account/connection access and the actual credential version still apply.
// No fill, terminal, account update or capital release is inferred from an ack.
func (s *PostgresStore) RecoverSubmission(ctx context.Context, ownerID, orderID string, vault FinancialCredentialReader, lookup SubmissionLookup) (Attempt, error) {
	if !validUUID(ownerID) || !validUUID(orderID) || vault == nil || lookup == nil {
		return Attempt{}, ErrNotAuthorized
	}
	sub, a, generation, err := s.loadRecoveryContext(ctx, ownerID, orderID)
	if err != nil || a.ProviderOrderID != "" {
		return a, err
	}
	raw, materialGeneration, err := vault.RetrieveFinancialVersion(ctx, credential.Locator{ConnectionID: sub.Order.Request.ConnectionID, UserID: ownerID, Class: credential.Financial})
	defer clear(raw)
	if err != nil || materialGeneration != generation || len(raw) > 16384 {
		return a, ErrNotAuthorized
	}
	var cr financial.Credentials
	if json.Unmarshal(raw, &cr) != nil || cr.PortfolioID != sub.PortfolioID || cr.APIKeyName == "" || cr.APIPrivateKey == "" {
		return a, ErrNotAuthorized
	}
	defer func() { cr = financial.Credentials{} }()
	// Bounded GET-only work outside locks. A later access/key change invalidates
	// persistence, and a subsequent authorized read can recover the same result.
	lookupCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	ack, err := lookup.LookupSubmission(lookupCtx, &cr, sub, a)
	if err != nil || lookupCtx.Err() != nil || !validSubmissionAcknowledgement(ack, sub.Order) {
		return a, ErrSubmissionUnknown
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return a, err
	}
	defer rollback(tx)
	current, err := lockRecoveryAccess(ctx, tx, sub.Order, sub.PortfolioID)
	if err != nil || current != materialGeneration {
		return a, ErrNotAuthorized
	}
	var id string
	if err = tx.QueryRow(ctx, `SELECT order_id::text FROM execution_dispatch_attempts WHERE order_id=$1 AND owner_id=$2 FOR UPDATE`, orderID, ownerID).Scan(&id); err != nil {
		return a, mapError(err)
	}
	// A competing receipt writer may have made us wait past connection expiry.
	// Held row locks prevent revocation writes, but do not stop the wall clock.
	current, err = lockRecoveryAccess(ctx, tx, sub.Order, sub.PortfolioID)
	if err != nil || current != materialGeneration {
		return a, ErrNotAuthorized
	}
	if _, err = tx.Exec(ctx, `INSERT INTO execution_broker_acknowledgements(order_id,owner_id,provider_order_id) VALUES($1,$2,$3) ON CONFLICT(order_id) DO NOTHING`, orderID, ownerID, ack.ProviderOrderID); err != nil {
		return a, mapError(err)
	}
	var saved string
	if err = tx.QueryRow(ctx, `SELECT provider_order_id::text FROM execution_broker_acknowledgements WHERE order_id=$1 AND owner_id=$2`, orderID, ownerID).Scan(&saved); err != nil || saved != ack.ProviderOrderID {
		return a, ErrConflict
	}
	if err = tx.Commit(ctx); err != nil {
		return a, ErrSubmissionUnknown
	}
	a.ProviderOrderID = saved
	return a, nil
}

func (s *PostgresStore) loadRecoveryContext(ctx context.Context, ownerID, orderID string) (ConfirmedSubmission, Attempt, int64, error) {
	if _, err := s.ReadNoSendResolution(ctx, ownerID, orderID); err == nil {
		return ConfirmedSubmission{}, Attempt{}, 0, ErrSubmissionNotSent
	} else if !errors.Is(err, ErrNotFound) {
		return ConfirmedSubmission{}, Attempt{}, 0, err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return ConfirmedSubmission{}, Attempt{}, 0, err
	}
	defer rollback(tx)
	o, err := readOrder(ctx, tx, ownerID, "id", orderID)
	if err != nil {
		return ConfirmedSubmission{}, Attempt{}, 0, err
	}
	a := Attempt{OrderID: o.ID, ClientOrderID: o.Request.ClientOrderID, RequestDigest: o.RequestDigest}
	var body []byte
	var rejection string
	err = tx.QueryRow(ctx, `SELECT a.authorization_id::text,a.credential_generation,a.claimed_at,a.expires_at,COALESCE(b.provider_order_id::text,''),COALESCE(r.error_code,''),p.evidence
	 FROM execution_dispatch_attempts a JOIN execution_authorizations z ON z.id=a.authorization_id AND z.order_id=a.order_id AND z.owner_id=a.owner_id
	 JOIN execution_provider_preflights p ON p.id::text=z.preflight->>'EvidenceID' AND p.order_id=a.order_id AND p.owner_id=a.owner_id AND p.credential_generation=a.credential_generation AND p.request_digest=z.request_digest
	 LEFT JOIN execution_broker_acknowledgements b ON b.order_id=a.order_id
	 LEFT JOIN execution_submission_rejections r ON r.order_id=a.order_id
	 WHERE a.order_id=$1 AND a.owner_id=$2`, orderID, ownerID).Scan(&a.AuthorizationID, &a.CredentialGeneration, &a.ClaimedAt, &a.ExpiresAt, &a.ProviderOrderID, &rejection, &body)
	if err != nil {
		return ConfirmedSubmission{}, a, 0, mapError(err)
	}
	var p ProviderPreflight
	if !decodeProviderPreflight(body, &p) {
		return ConfirmedSubmission{}, a, 0, ErrNotAuthorized
	}
	sub := ConfirmedSubmission{Order: o, PortfolioID: p.PortfolioID, PreviewID: p.PreviewID, Preflight: p}
	generation, err := lockRecoveryAccess(ctx, tx, o, sub.PortfolioID)
	if err != nil {
		return sub, a, 0, err
	}
	var now time.Time
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil || ValidateRecoverySubmission(sub, a, now) != nil {
		return sub, a, 0, ErrNotAuthorized
	}
	if rejection != "" {
		return sub, a, generation, ErrSubmissionRejected
	}
	return sub, a, generation, nil
}

// Lock in the existing owner -> entitlement -> account -> connection order.
// No allocation, approval, reconciliation or stop locks: these are read access
// checks only. Require READ COMMITTED and check expiry after all lock waits.
func lockRecoveryAccess(ctx context.Context, tx pgx.Tx, o Order, portfolio string) (int64, error) {
	return lockEvidenceAccess(ctx, tx, o, portfolio, false)
}

func lockEvidenceAccess(ctx context.Context, tx pgx.Tx, o Order, portfolio string, writeAccount bool) (int64, error) {
	var isolation, ignored string
	if err := tx.QueryRow(ctx, `SHOW transaction_isolation`).Scan(&isolation); err != nil || isolation != "read committed" {
		return 0, ErrNotAuthorized
	}
	accountLock := `SELECT id::text FROM financial_accounts WHERE id=$1 AND user_id=$2 FOR SHARE`
	if writeAccount {
		accountLock = `SELECT id::text FROM financial_accounts WHERE id=$1 AND user_id=$2 FOR UPDATE`
	}
	queries := []struct {
		sql  string
		args []any
	}{
		{`SELECT id::text FROM users WHERE id=$1 FOR SHARE`, []any{o.Request.OwnerID}},
		{`SELECT id::text FROM user_entitlements WHERE user_id=$1 AND entitlement_key='founder' FOR SHARE`, []any{o.Request.OwnerID}},
		{accountLock, []any{o.Request.AccountID, o.Request.OwnerID}},
		{`SELECT id::text FROM provider_connections WHERE id=$1 AND user_id=$2 FOR SHARE`, []any{o.Request.ConnectionID, o.Request.OwnerID}},
	}
	for _, q := range queries {
		if err := tx.QueryRow(ctx, q.sql, q.args...).Scan(&ignored); err != nil {
			return 0, ErrNotAuthorized
		}
	}
	var gen int64
	err := tx.QueryRow(ctx, `SELECT c.credential_generation FROM users u
	 JOIN user_entitlements e ON e.user_id=u.id AND e.entitlement_key='founder'
	 JOIN financial_accounts a ON a.user_id=u.id AND a.id=$2
	 JOIN provider_connections c ON c.user_id=u.id AND c.id=$3 AND c.id=a.provider_connection_id
	 WHERE u.id=$1 AND u.status='active' AND e.status='active' AND e.starts_at<=clock_timestamp() AND (e.expires_at IS NULL OR e.expires_at>clock_timestamp())
	 AND a.status='active' AND a.provider_name='coinbase' AND a.base_currency='USD' AND a.provider_account_id=$4
	 AND c.status='active' AND c.provider_category='financial' AND c.provider_name='coinbase'
	 AND (c.authorization_expires_at IS NULL OR c.authorization_expires_at>clock_timestamp())
	 AND c.encrypted_credential_payload IS NOT NULL AND c.credential_reference IS NULL`, o.Request.OwnerID, o.Request.AccountID, o.Request.ConnectionID, "portfolio:"+portfolio).Scan(&gen)
	if err != nil || gen <= 0 {
		return 0, ErrNotAuthorized
	}
	return gen, nil
}
