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

// CancelBrokerOrder is deliberately runtime-unwired. A new durable claim is
// required before this invocation may cancel the original known broker order.
// Restart never reclaims or resends, including a crash before the first request.
// Stops/old approval expiry cannot inhibit this risk-reducing operation, but
// current owner/account/connection access and credential generation still apply.
// No cancellation response releases holds or changes the fill/terminal ledger.
func (s *PostgresStore) CancelBrokerOrder(ctx context.Context, ownerID, orderID string, vault FinancialCredentialReader, sender CancellationSender) (CancellationAttempt, error) {
	if !validUUID(ownerID) || !validUUID(orderID) || vault == nil || sender == nil {
		return CancellationAttempt{}, ErrNotAuthorized
	}
	sub, original, generation, err := s.loadRecoveryContext(ctx, ownerID, orderID)
	if err != nil {
		return CancellationAttempt{}, err
	}
	if original.ProviderOrderID == "" {
		return CancellationAttempt{}, ErrSubmissionUnknown
	}
	if saved, err := s.ReadCancellation(ctx, ownerID, orderID); err == nil {
		return saved, cancellationOutcomeError(saved.Outcome)
	} else if !errors.Is(err, ErrNotFound) {
		return CancellationAttempt{}, err
	}
	raw, materialGeneration, err := vault.RetrieveFinancialVersion(ctx, credential.Locator{ConnectionID: sub.Order.Request.ConnectionID, UserID: ownerID, Class: credential.Financial})
	defer clear(raw)
	if err != nil || materialGeneration != generation || len(raw) > 16384 {
		return CancellationAttempt{}, ErrNotAuthorized
	}
	var cr financial.Credentials
	if json.Unmarshal(raw, &cr) != nil || cr.PortfolioID != sub.PortfolioID || cr.APIKeyName == "" || cr.APIPrivateKey == "" {
		return CancellationAttempt{}, ErrNotAuthorized
	}
	defer func() { cr = financial.Credentials{} }()
	a, fresh, err := s.claimCancellation(ctx, sub, original, materialGeneration)
	if err != nil {
		return a, err // A lost commit response never admits the network callback.
	}
	if !fresh {
		return a, cancellationOutcomeError(a.Outcome)
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return a, err
	}
	defer rollback(tx)
	deadline, err := validateCancellationAdmission(ctx, tx, sub, a)
	if err != nil {
		return a, err
	}
	sendCtx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	if sendCtx.Err() != nil {
		return a, ErrNotAuthorized
	}
	// Synchronous under local revocation locks; never detach a provider request.
	// Losing the DB session can still release locks during network I/O. This is
	// not broker fencing, guaranteed cancellation or guaranteed cancel-on-stop.
	ack, sendErr := sender.CancelOnce(sendCtx, &cr, sub, original)
	outcome := "UNKNOWN"
	if sendErr == nil && sendCtx.Err() == nil && ack.ProviderOrderID == original.ProviderOrderID {
		outcome = "NOT_ACCEPTED"
		if ack.Accepted {
			outcome = "ACCEPTED"
		}
	}
	// Save only sanitized evidence of the already-attempted operation. Expiry
	// after admission must not erase its historical result or manufacture finality.
	receiptCtx, finish := context.WithTimeout(context.Background(), 5*time.Second)
	defer finish()
	_, err = tx.Exec(receiptCtx, `INSERT INTO execution_cancellation_receipts(order_id,owner_id,provider_order_id,outcome) VALUES($1,$2,$3,$4)`, orderID, ownerID, original.ProviderOrderID, outcome)
	if err != nil || tx.Commit(receiptCtx) != nil {
		return a, ErrCancellationUnknown
	}
	a, err = s.ReadCancellation(receiptCtx, ownerID, orderID)
	if err != nil {
		return a, ErrCancellationUnknown
	}
	return a, cancellationOutcomeError(a.Outcome)
}

func (s *PostgresStore) claimCancellation(ctx context.Context, sub ConfirmedSubmission, original Attempt, materialGeneration int64) (CancellationAttempt, bool, error) {
	a := CancellationAttempt{OrderID: sub.Order.ID, ProviderOrderID: original.ProviderOrderID, CredentialGeneration: materialGeneration, Outcome: "UNKNOWN"}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return a, false, err
	}
	defer rollback(tx)
	var current int64
	if err = tx.QueryRow(ctx, `SELECT lock_execution_cancellation_controls($1,$2)`, a.OrderID, sub.PortfolioID).Scan(&current); err != nil || current != materialGeneration {
		return a, false, ErrNotAuthorized
	}
	if saved, err := readCancellation(ctx, tx, sub.Order.Request.OwnerID, a.OrderID); err == nil {
		return saved, false, nil
	} else if !errors.Is(err, ErrNotFound) {
		return a, false, err
	}
	r := sub.Order.Request
	err = tx.QueryRow(ctx, `INSERT INTO execution_cancellation_attempts(order_id,owner_id,financial_account_id,provider_connection_id,provider_order_id,portfolio_id,credential_generation)
	 VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING claimed_at,expires_at`, a.OrderID, r.OwnerID, r.AccountID, r.ConnectionID, a.ProviderOrderID, sub.PortfolioID, materialGeneration).Scan(&a.ClaimedAt, &a.ExpiresAt)
	if err != nil {
		return a, false, mapError(err)
	}
	if err = tx.Commit(ctx); err != nil {
		return a, false, ErrCommitUnknown
	}
	return a, true, nil
}

func validateCancellationAdmission(ctx context.Context, tx pgx.Tx, sub ConfirmedSubmission, a CancellationAttempt) (time.Time, error) {
	var current int64
	if err := tx.QueryRow(ctx, `SELECT lock_execution_cancellation_controls($1,$2)`, a.OrderID, sub.PortfolioID).Scan(&current); err != nil || current != a.CredentialGeneration {
		return time.Time{}, ErrNotAuthorized
	}
	var saved CancellationAttempt
	err := tx.QueryRow(ctx, `SELECT order_id::text,provider_order_id::text,credential_generation,claimed_at,expires_at FROM execution_cancellation_attempts WHERE order_id=$1 AND owner_id=$2 FOR UPDATE`, a.OrderID, sub.Order.Request.OwnerID).
		Scan(&saved.OrderID, &saved.ProviderOrderID, &saved.CredentialGeneration, &saved.ClaimedAt, &saved.ExpiresAt)
	if err != nil || saved.OrderID != a.OrderID || saved.ProviderOrderID != a.ProviderOrderID || saved.CredentialGeneration != a.CredentialGeneration || !saved.ClaimedAt.Equal(a.ClaimedAt) || !saved.ExpiresAt.Equal(a.ExpiresAt) {
		return time.Time{}, ErrNotAuthorized
	}
	var received bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM execution_cancellation_receipts WHERE order_id=$1)`, a.OrderID).Scan(&received); err != nil || received {
		return time.Time{}, ErrNotAuthorized
	}
	// Charge the full database round trip against the remaining deadline, as at
	// initial submission. No slow query may renew an expired access window.
	clockStart := time.Now()
	var now time.Time
	var entitlementEnd, connectionEnd *time.Time
	err = tx.QueryRow(ctx, `SELECT lock_execution_cancellation_controls(o.id,$2),e.expires_at,c.authorization_expires_at,clock_timestamp()
	 FROM execution_orders o JOIN user_entitlements e ON e.user_id=o.owner_id AND e.entitlement_key='founder'
	 JOIN provider_connections c ON c.id=o.provider_connection_id WHERE o.id=$1`, a.OrderID, sub.PortfolioID).
		Scan(&current, &entitlementEnd, &connectionEnd, &now)
	if err != nil || current != a.CredentialGeneration {
		return time.Time{}, ErrNotAuthorized
	}
	budget := 5 * time.Second
	for _, end := range []*time.Time{&a.ExpiresAt, entitlementEnd, connectionEnd} {
		if end != nil && end.Sub(now) < budget {
			budget = end.Sub(now)
		}
	}
	deadline := clockStart.Add(budget)
	if ctx.Err() != nil || !deadline.After(time.Now()) {
		return time.Time{}, ErrNotAuthorized
	}
	return deadline, nil
}

func (s *PostgresStore) ReadCancellation(ctx context.Context, ownerID, orderID string) (CancellationAttempt, error) {
	if !validUUID(ownerID) || !validUUID(orderID) {
		return CancellationAttempt{}, ErrInvalid
	}
	return readCancellation(ctx, s.db, ownerID, orderID)
}

func readCancellation(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, ownerID, orderID string) (CancellationAttempt, error) {
	var a CancellationAttempt
	err := q.QueryRow(ctx, `SELECT a.order_id::text,a.provider_order_id::text,a.credential_generation,a.claimed_at,a.expires_at,COALESCE(r.outcome,'UNKNOWN'),r.received_at
	 FROM execution_cancellation_attempts a LEFT JOIN execution_cancellation_receipts r ON r.order_id=a.order_id WHERE a.order_id=$1 AND a.owner_id=$2`, orderID, ownerID).
		Scan(&a.OrderID, &a.ProviderOrderID, &a.CredentialGeneration, &a.ClaimedAt, &a.ExpiresAt, &a.Outcome, &a.ReceivedAt)
	return a, mapError(err)
}
