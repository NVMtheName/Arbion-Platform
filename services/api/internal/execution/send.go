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

var ErrSubmissionUnknown = errors.New("submission outcome unknown; reconcile, never resend")

// ConfirmedSubmission is private server data. The immutable order, preview and
// portfolio are selected by the control plane, never supplied by a browser or
// a model. This is a price-bounded spot LIMIT_IOC, not a market order.
type ConfirmedSubmission struct {
	Order                  Order
	PortfolioID, PreviewID string
	Preflight              ProviderPreflight
}

// SubmissionAcknowledgement is correlation only, never a fill or settlement.
type SubmissionAcknowledgement struct {
	ProviderOrderID, ClientOrderID, ProductID, Side string
}

// ConfirmedOrderSender has no runtime caller. An adapter must honor ctx
// synchronously, use ONLY these exact
// credentials/terms, disable redirects and all retries, and attempt at most one
// request. It must not spawn a background send or reload credentials. An error
// is indeterminate except a strictly parsed SubmissionRejectedError. Neither
// outcome releases capital or permits a retry.
type ConfirmedOrderSender interface {
	SubmitOnce(context.Context, *financial.Credentials, ConfirmedSubmission) (SubmissionAcknowledgement, error)
}

// SendConfirmed is deliberately unwired. Only the invocation that commits a new
// Claim can enter the send boundary. It accepts neither a recovered Attempt nor
// an injectable Authority. Any later invocation must fail at Claim, even after
// a crash before sending. There is no retry or refreshed evidence. Only this
// winning invocation can prove a pre-callback failure and close its own holds.
//
// Revocations committed before final lock acquisition deny the send. Once those
// locks are held, revocations wait for the synchronous bounded callback. This is
// local serialization, NOT broker-side fencing: losing the DB session can release
// locks while a network request is in flight. Runtime activation requires review
// of that unavoidable distributed boundary and the concrete transport.
func (s *PostgresStore) SendConfirmed(ctx context.Context, ownerID, orderID, evidenceID string, vault FinancialCredentialReader, sender ConfirmedOrderSender) (Attempt, error) {
	if !validUUID(ownerID) || !validUUID(orderID) || !validUUID(evidenceID) || vault == nil || sender == nil {
		return Attempt{}, ErrNotAuthorized
	}
	// Vault I/O is outside all authorization locks. Final validation binds the
	// atomically retrieved material generation, not a separate cached key value.
	o, portfolio, generation, err := s.loadSendContext(ctx, ownerID, orderID)
	if err != nil {
		return Attempt{}, err
	}
	raw, materialGeneration, err := vault.RetrieveFinancialVersion(ctx, credential.Locator{ConnectionID: o.Request.ConnectionID, UserID: ownerID, Class: credential.Financial})
	defer clear(raw)
	if err != nil || materialGeneration != generation || len(raw) > 16384 {
		return Attempt{}, ErrNotAuthorized
	}
	var cr financial.Credentials
	if json.Unmarshal(raw, &cr) != nil || cr.PortfolioID != portfolio || cr.APIKeyName == "" || cr.APIPrivateKey == "" {
		return Attempt{}, ErrNotAuthorized
	}
	defer func() { cr = financial.Credentials{} }()
	authority := NewOwnerAuthority(NewSavedPreflightVerifier(evidenceID))
	a, err := s.Claim(ctx, ownerID, orderID, authority)
	if err != nil {
		return Attempt{}, err // Commit ambiguity never reaches the sender.
	}
	boundary := sendBoundaryState{}
	result, err := s.sendClaimed(ctx, o, a, portfolio, materialGeneration, &cr, authority, sender, &boundary)
	if err == nil || boundary.entered {
		return result, err
	}
	// Only this fresh-claim invocation knows that the synchronous callback was
	// never entered. A restarted process cannot reconstruct this fact. Cleanup
	// must finish before a separate transaction can release the original holds.
	if !boundary.cleanupComplete || s.recordNoSendResolution(o, a) != nil {
		return result, errors.Join(err, ErrNoSendResolutionUnknown)
	}
	return result, errors.Join(err, ErrSubmissionNotSent)
}

// Private process-local evidence, never supplied by a caller or adapter.
// Panic/crash does not return to the only receipt writer in SendConfirmed.
type sendBoundaryState struct {
	entered, cleanupComplete bool
}

func (s *PostgresStore) sendClaimed(ctx context.Context, o Order, a Attempt, portfolio string, materialGeneration int64, cr *financial.Credentials, authority *OwnerAuthority, sender ConfirmedOrderSender, boundary *sendBoundaryState) (Attempt, error) {
	if a.CredentialGeneration != materialGeneration {
		boundary.cleanupComplete = true // No final transaction was opened.
		return a, ErrNotAuthorized
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		boundary.cleanupComplete = true // No transaction or callback was entered.
		return a, err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err := tx.Rollback(cleanup)
		boundary.cleanupComplete = err == nil || errors.Is(err, pgx.ErrTxClosed)
	}()
	orderID, ownerID := o.ID, o.Request.OwnerID
	var generation int64
	if err = tx.QueryRow(ctx, `SELECT lock_execution_claim_controls($1)`, orderID).Scan(&generation); err != nil {
		return a, mapError(err)
	}
	// Acknowledgement writers also lock this row; reconciliation writers use
	// the already-held account lock. Neither may supersede this send admission.
	var saved Attempt
	err = tx.QueryRow(ctx, `SELECT order_id::text,authorization_id::text,credential_generation,claimed_at,expires_at FROM execution_dispatch_attempts WHERE order_id=$1 AND owner_id=$2 FOR UPDATE`, orderID, ownerID).
		Scan(&saved.OrderID, &saved.AuthorizationID, &saved.CredentialGeneration, &saved.ClaimedAt, &saved.ExpiresAt)
	if err != nil {
		return a, mapError(err)
	}
	if saved.OrderID != a.OrderID || saved.AuthorizationID != a.AuthorizationID || saved.CredentialGeneration != generation || generation != materialGeneration || !saved.ClaimedAt.Equal(a.ClaimedAt) || !saved.ExpiresAt.Equal(a.ExpiresAt) {
		return a, ErrNotAuthorized
	}
	checked, err := authority.check(ctx, tx, o, true)
	if err != nil {
		return a, err
	}
	submission, deadline, err := validateSendAuthorization(ctx, tx, a, checked, portfolio)
	if err != nil {
		return a, err
	}
	sendCtx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	if sendCtx.Err() != nil {
		return a, ErrNotAuthorized
	}
	// Never detach this call into a goroutine: returning on a timeout while it
	// continues would release the revocation locks before the sender stops.
	boundary.entered = true // Every adapter outcome from here is potentially sent.
	ack, sendErr := sender.SubmitOnce(sendCtx, cr, submission)
	var rejected *SubmissionRejectedError
	if errors.As(sendErr, &rejected) && rejected != nil && validRejectionCode(rejected.Code) && sendCtx.Err() == nil && ack == (SubmissionAcknowledgement{}) {
		receiptCtx, cancelReceipt := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancelReceipt()
		if _, err = tx.Exec(receiptCtx, `INSERT INTO execution_submission_rejections(order_id,owner_id,error_code) VALUES($1,$2,$3)`, orderID, ownerID, rejected.Code); err != nil {
			return a, ErrSubmissionUnknown
		}
		if err = tx.Commit(receiptCtx); err != nil {
			return a, ErrSubmissionUnknown
		}
		return a, ErrSubmissionRejected
	}
	if sendErr != nil || sendCtx.Err() != nil || !validSubmissionAcknowledgement(ack, o) {
		return a, ErrSubmissionUnknown // Do not leak provider bodies or key data.
	}
	// No expired authority is reused here: this only saves evidence of the
	// already-attempted operation. A canceled caller cannot silently erase it.
	receiptCtx, cancelReceipt := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelReceipt()
	_, err = tx.Exec(receiptCtx, `INSERT INTO execution_broker_acknowledgements(order_id,owner_id,provider_order_id) VALUES($1,$2,$3)`, orderID, ownerID, ack.ProviderOrderID)
	if err != nil {
		return a, ErrSubmissionUnknown
	}
	if err = tx.Commit(receiptCtx); err != nil {
		return a, ErrSubmissionUnknown
	}
	a.ProviderOrderID = ack.ProviderOrderID
	return a, nil
}

func (s *PostgresStore) loadSendContext(ctx context.Context, ownerID, orderID string) (Order, string, int64, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Order{}, "", 0, err
	}
	defer rollback(tx)
	o, err := readOrder(ctx, tx, ownerID, "id", orderID)
	if err != nil {
		return Order{}, "", 0, err
	}
	var attempted bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM execution_dispatch_attempts WHERE order_id=$1)`, orderID).Scan(&attempted); err != nil {
		return Order{}, "", 0, err
	}
	if attempted {
		return Order{}, "", 0, ErrAlreadyAttempted
	}
	portfolio, err := executionPortfolio(ctx, tx, o)
	if err != nil {
		return Order{}, "", 0, err
	}
	var generation int64
	if err = tx.QueryRow(ctx, `SELECT credential_generation FROM provider_connections WHERE id=$1 AND user_id=$2`, o.Request.ConnectionID, ownerID).Scan(&generation); err != nil {
		return Order{}, "", 0, err
	}
	// Read-only snapshot, not an authorization. Rollback releases it before I/O.
	return o, portfolio, generation, nil
}

func validateSendAuthorization(ctx context.Context, tx pgx.Tx, a Attempt, checked checkedOwnerAuthority, portfolio string) (ConfirmedSubmission, time.Time, error) {
	o := checked.order
	var approval, digest, rec string
	var generation int64
	var expires, authorized, now, pilotExpiry time.Time
	var proofJSON, providerJSON []byte
	err := tx.QueryRow(ctx, `SELECT approval_id::text,request_digest,reconciliation_id::text,credential_generation,checked_at,expires_at,preflight FROM execution_authorizations WHERE id=$1 AND order_id=$2 AND owner_id=$3 AND financial_account_id=$4`, a.AuthorizationID, o.ID, o.Request.OwnerID, o.Request.AccountID).
		Scan(&approval, &digest, &rec, &generation, &authorized, &expires, &proofJSON)
	if err != nil {
		return ConfirmedSubmission{}, time.Time{}, ErrNotAuthorized
	}
	var proof VerifiedPreflight
	if json.Unmarshal(proofJSON, &proof) != nil || approval != checked.approvalID || digest != o.RequestDigest || rec != checked.preflight.ReconciliationID || generation != a.CredentialGeneration || authorized.After(a.ClaimedAt) || authorized.Before(o.CreatedAt) || !expires.Equal(a.ExpiresAt) || !sameSendPreflight(proof, checked.preflight) {
		return ConfirmedSubmission{}, time.Time{}, ErrNotAuthorized
	}
	err = tx.QueryRow(ctx, `SELECT evidence FROM execution_provider_preflights WHERE id=$1 AND order_id=$2 AND owner_id=$3`, proof.EvidenceID, o.ID, o.Request.OwnerID).Scan(&providerJSON)
	var p ProviderPreflight
	if err != nil || !decodeProviderPreflight(providerJSON, &p) || p.PortfolioID != portfolio || !validUUID(p.PreviewID) {
		return ConfirmedSubmission{}, time.Time{}, ErrNotAuthorized
	}
	// Repeat expiring control checks after all waits. Convert database remaining
	// validity to a monotonic local deadline, conservatively charging the entire
	// query round trip. Delay after this helper cannot extend the send window.
	clockStart := time.Now()
	var entitlementEnd, connectionEnd *time.Time
	err = tx.QueryRow(ctx, `SELECT lock_execution_claim_controls(o.id),e.expires_at,c.authorization_expires_at,check_execution_pilot_limits(o.id),clock_timestamp()
		FROM execution_orders o JOIN user_entitlements e ON e.user_id=o.owner_id AND e.entitlement_key='founder'
		JOIN provider_connections c ON c.id=o.provider_connection_id WHERE o.id=$1`, o.ID).Scan(&generation, &entitlementEnd, &connectionEnd, &pilotExpiry, &now)
	if err != nil {
		return ConfirmedSubmission{}, time.Time{}, mapError(err)
	}
	if generation != a.CredentialGeneration {
		return ConfirmedSubmission{}, time.Time{}, ErrNotAuthorized
	}
	budget := 5 * time.Second
	ends := []time.Time{expires, checked.authorization.ExpiresAt, pilotExpiry}
	for _, end := range []*time.Time{entitlementEnd, connectionEnd} {
		if end != nil {
			ends = append(ends, *end)
		}
	}
	for _, end := range ends {
		if left := end.Sub(now); left < budget {
			budget = left
		}
	}
	deadline := clockStart.Add(budget)
	if !deadline.After(time.Now()) || ctx.Err() != nil {
		return ConfirmedSubmission{}, time.Time{}, ErrNotAuthorized
	}
	return ConfirmedSubmission{Order: o, PortfolioID: portfolio, PreviewID: p.PreviewID, Preflight: p}, deadline, nil
}

func sameSendPreflight(a, b VerifiedPreflight) bool {
	return a.EvidenceID == b.EvidenceID && a.RequestDigest == b.RequestDigest && a.AccountID == b.AccountID && a.ConnectionID == b.ConnectionID && a.ReconciliationID == b.ReconciliationID && a.CredentialGeneration == b.CredentialGeneration && a.ObservedAt.Equal(b.ObservedAt) && a.ExpiresAt.Equal(b.ExpiresAt) && a.CashUSD == b.CashUSD && a.AvailableCashUSD == b.AvailableCashUSD && a.TotalBase == b.TotalBase && a.AvailableBase == b.AvailableBase
}

func validSubmissionAcknowledgement(a SubmissionAcknowledgement, o Order) bool {
	return validUUID(a.ProviderOrderID) && a.ClientOrderID == o.Request.ClientOrderID && a.ProductID == o.Request.ProductID && a.Side == o.Request.Side
}
