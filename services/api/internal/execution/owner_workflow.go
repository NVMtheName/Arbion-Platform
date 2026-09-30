package execution

import (
	"context"
	"crypto/sha256"
	"fmt"

	"github.com/arbion/platform/services/api/internal/authorization"
)

// OwnerScope is selected by trusted server composition, never HTTP/model input.
// Production startup leaves this disconnected. Supplying a scope is not pilot activation
// or a substitute for the existing transactionally current financial controls.
type OwnerScope struct {
	OwnerID, AccountID, ConnectionID, CapitalBucketID, ProductID string
}

// Dependencies stay server-only. Keep read/preview and write adapters separate;
// production composition requires a reviewed fixed Coinbase HTTPS transport.
type OwnerWorkflowDependencies struct {
	StepUp       ExecutionStepUp
	Vault        FinancialCredentialReader
	Preflight    ExecutionPreflightProvider
	Sender       ConfirmedOrderSender
	Lookup       SubmissionLookup
	Observation  BrokerObservationProvider
	Cancellation CancellationSender
	Settlement   AccountSettlementProvider
}

type OwnerWorkflow struct {
	store *PostgresStore
	scope OwnerScope
	deps  OwnerWorkflowDependencies
}

// OwnerPresentation is server-derived display context, not execution authority.
// ScopeID is an opaque fingerprint; it contains no credentials or broker IDs.
type OwnerPresentation struct {
	ProductID, AccountLabel, ScopeID string
}

func (w *OwnerWorkflow) Presentation(ctx context.Context, p authorization.Principal) (OwnerPresentation, error) {
	if err := w.checkOwner(ctx, p); err != nil {
		return OwnerPresentation{}, err
	}
	s := w.scope
	digest := sha256.Sum256([]byte("arbion-owner-scope-v1\x00" + s.OwnerID + "\x00" + s.AccountID + "\x00" + s.ConnectionID + "\x00" + s.CapitalBucketID + "\x00" + s.ProductID))
	return OwnerPresentation{ProductID: s.ProductID, AccountLabel: "Dedicated Coinbase portfolio", ScopeID: fmt.Sprintf("%x", digest)}, nil
}

func NewOwnerWorkflow(store *PostgresStore, scope OwnerScope, deps OwnerWorkflowDependencies) (*OwnerWorkflow, error) {
	if store == nil || store.db == nil || !validUUID(scope.OwnerID) || !validUUID(scope.AccountID) ||
		!validUUID(scope.ConnectionID) || !validUUID(scope.CapitalBucketID) || !productPattern.MatchString(scope.ProductID) || scope.ProductID == "USD-USD" ||
		deps.StepUp == nil || deps.Vault == nil || deps.Preflight == nil || deps.Sender == nil || deps.Lookup == nil || deps.Observation == nil || deps.Cancellation == nil || deps.Settlement == nil {
		return nil, ErrNotAuthorized
	}
	return &OwnerWorkflow{store: store, scope: scope, deps: deps}, nil
}

type OwnerPrepareCommand struct {
	RequestKey      string `json:"request_key"`
	Side            string `json:"side"`
	BaseSize        string `json:"base_size"`
	LimitPrice      string `json:"limit_price"`
	FeeAllowanceUSD string `json:"fee_allowance_usd"`
	MaximumDebitUSD string `json:"maximum_debit_usd"`
}

type OwnerApproveCommand struct {
	ExpectedDigest string `json:"expected_digest"`
	MFACode        string `json:"mfa_code"`
}

type OwnerSendCommand struct {
	EvidenceID string `json:"evidence_id"`
}

type OwnerPreflight struct {
	EvidenceID string     `json:"evidence_id"`
	Order      OwnerOrder `json:"order"`
}

// Fresh entry access is not dispatch authority or a logout fence. Core command
// methods retain their own current-access/approval/risk locks immediately before
// their effects. Saved history cannot be accessed by a forged/stale principal.
func (w *OwnerWorkflow) checkOwner(ctx context.Context, p authorization.Principal) error {
	if w == nil || w.store == nil || p.UserID != w.scope.OwnerID || p.Entitlement != authorization.EntitlementFounder {
		return ErrNotAuthorized
	}
	var allowed bool
	err := w.store.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users u JOIN user_entitlements e ON e.user_id=u.id
	 WHERE u.id=$1 AND u.status='active' AND e.entitlement_key='founder' AND e.status='active'
	 AND e.starts_at<=clock_timestamp() AND (e.expires_at IS NULL OR e.expires_at>clock_timestamp()))`, p.UserID).Scan(&allowed)
	if err != nil || !allowed {
		return ErrNotAuthorized
	}
	return nil
}

// The caller's idempotency key is not accepted as broker correlation. Deriving
// one domain-separated private UUID keeps Prepare replay stable after a lost
// response, without minting a new broker identity for the same owner/key.
func ownerClientID(ownerID, key string) string {
	h := sha256.Sum256([]byte("arbion-owner-execution-request-v1\x00" + ownerID + "\x00" + key))
	h[6] = (h[6] & 0x0f) | 0x80
	h[8] = (h[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", h[:4], h[4:6], h[6:8], h[8:10], h[10:16])
}

func (w *OwnerWorkflow) Prepare(ctx context.Context, p authorization.Principal, c OwnerPrepareCommand) (OwnerOrder, error) {
	if err := w.checkOwner(ctx, p); err != nil {
		return OwnerOrder{}, err
	}
	if !validUUID(c.RequestKey) {
		return OwnerOrder{}, ErrInvalid
	}
	r := Request{OwnerID: w.scope.OwnerID, AccountID: w.scope.AccountID, ConnectionID: w.scope.ConnectionID,
		CapitalBucketID: w.scope.CapitalBucketID, ProductID: w.scope.ProductID, ClientOrderID: ownerClientID(p.UserID, c.RequestKey),
		Side: c.Side, BaseSize: c.BaseSize, LimitPrice: c.LimitPrice, FeeAllowanceUSD: c.FeeAllowanceUSD, MaximumDebitUSD: c.MaximumDebitUSD}
	o, err := w.store.Prepare(ctx, r)
	if err != nil {
		return OwnerOrder{}, err
	}
	return w.Get(ctx, p, o.ID)
}

func (w *OwnerWorkflow) Get(ctx context.Context, p authorization.Principal, id string) (OwnerOrder, error) {
	if err := w.checkOwner(ctx, p); err != nil {
		return OwnerOrder{}, err
	}
	o, err := w.store.ReadOwnerOrder(ctx, p.UserID, id)
	if err != nil {
		return OwnerOrder{}, err
	}
	if o.AccountID != w.scope.AccountID || o.ConnectionID != w.scope.ConnectionID || o.CapitalBucketID != w.scope.CapitalBucketID || o.ProductID != w.scope.ProductID {
		return OwnerOrder{}, ErrNotFound
	}
	return o, nil
}

func (w *OwnerWorkflow) commandOrder(ctx context.Context, p authorization.Principal, id string, unattempted bool) (OwnerOrder, error) {
	o, err := w.Get(ctx, p, id)
	if err != nil {
		return OwnerOrder{}, err
	}
	if o.State == "UNAVAILABLE" {
		return OwnerOrder{}, ErrUnreconciled
	}
	if unattempted && o.Attempted {
		return OwnerOrder{}, ErrAlreadyAttempted
	}
	return o, nil
}

func (w *OwnerWorkflow) Approve(ctx context.Context, p authorization.Principal, id string, c OwnerApproveCommand) (OwnerOrder, error) {
	if _, err := w.commandOrder(ctx, p, id, true); err != nil {
		return OwnerOrder{}, err
	}
	_, err := w.store.ApproveOrder(ctx, p, id, c.ExpectedDigest, c.MFACode, w.deps.StepUp)
	c.MFACode = ""
	return w.after(ctx, p, id, err)
}

func (w *OwnerWorkflow) Revoke(ctx context.Context, p authorization.Principal, id string) (OwnerOrder, error) {
	if _, err := w.commandOrder(ctx, p, id, false); err != nil {
		return OwnerOrder{}, err
	}
	return w.after(ctx, p, id, w.store.RevokeOwnerApproval(ctx, p.UserID, id))
}

func (w *OwnerWorkflow) Capture(ctx context.Context, p authorization.Principal, id string) (OwnerPreflight, error) {
	if _, err := w.commandOrder(ctx, p, id, true); err != nil {
		return OwnerPreflight{}, err
	}
	evidence, err := w.store.CapturePreflight(ctx, p.UserID, id, w.deps.Vault, w.deps.Preflight)
	if err != nil {
		return OwnerPreflight{}, err
	}
	o, err := w.Get(ctx, p, id)
	if err != nil {
		return OwnerPreflight{}, err
	}
	return OwnerPreflight{EvidenceID: evidence, Order: o}, nil
}

func (w *OwnerWorkflow) Send(ctx context.Context, p authorization.Principal, id string, c OwnerSendCommand) (OwnerOrder, error) {
	if _, err := w.commandOrder(ctx, p, id, true); err != nil {
		return OwnerOrder{}, err
	}
	_, err := w.store.SendConfirmed(ctx, p.UserID, id, c.EvidenceID, w.deps.Vault, w.deps.Sender)
	return w.after(ctx, p, id, err)
}

func (w *OwnerWorkflow) Recover(ctx context.Context, p authorization.Principal, id string) (OwnerOrder, error) {
	if _, err := w.commandOrder(ctx, p, id, false); err != nil {
		return OwnerOrder{}, err
	}
	_, err := w.store.RecoverSubmission(ctx, p.UserID, id, w.deps.Vault, w.deps.Lookup)
	return w.after(ctx, p, id, err)
}

func (w *OwnerWorkflow) Reconcile(ctx context.Context, p authorization.Principal, id string) (OwnerOrder, error) {
	if _, err := w.commandOrder(ctx, p, id, false); err != nil {
		return OwnerOrder{}, err
	}
	_, err := w.store.ReconcileBrokerOrder(ctx, p.UserID, id, w.deps.Vault, w.deps.Observation)
	return w.after(ctx, p, id, err)
}

func (w *OwnerWorkflow) Cancel(ctx context.Context, p authorization.Principal, id string) (OwnerOrder, error) {
	if _, err := w.commandOrder(ctx, p, id, false); err != nil {
		return OwnerOrder{}, err
	}
	_, err := w.store.CancelBrokerOrder(ctx, p.UserID, id, w.deps.Vault, w.deps.Cancellation)
	return w.after(ctx, p, id, err)
}

func (w *OwnerWorkflow) Settle(ctx context.Context, p authorization.Principal, id string) (OwnerOrder, error) {
	if _, err := w.commandOrder(ctx, p, id, false); err != nil {
		return OwnerOrder{}, err
	}
	_, err := w.store.SettleBrokerAccount(ctx, p.UserID, id, w.deps.Vault, w.deps.Settlement)
	return w.after(ctx, p, id, err)
}

// Never retry a command, fetch a new preview, or convert an ambiguous write to
// success. A failed response directs the owner to GET saved state explicitly.
func (w *OwnerWorkflow) after(ctx context.Context, p authorization.Principal, id string, err error) (OwnerOrder, error) {
	if err != nil {
		return OwnerOrder{}, err
	}
	return w.Get(ctx, p, id)
}
