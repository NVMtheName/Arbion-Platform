package execution

import (
	"context"
	"encoding/json"
	"math/big"
	"strings"
	"time"

	"github.com/arbion/platform/services/api/internal/risk"
	"github.com/jackc/pgx/v5"
)

// VerifiedPreflight is private normalized evidence, never a browser/model
// command. Available amounts exclude broker holds; buying power/total inventory
// cannot substitute for them. The provider verifier must bind the actual key
// used for all reads to CredentialGeneration, exact account and request.
type VerifiedPreflight struct {
	EvidenceID, RequestDigest, AccountID, ConnectionID, ReconciliationID string
	CredentialGeneration                                                 int64
	ObservedAt, ExpiresAt                                                time.Time
	CashUSD, AvailableCashUSD, TotalBase, AvailableBase                  string
}

// PreflightVerifier is the remaining provider-adapter boundary. Implementations
// must verify saved exact LIMIT_IOC product/precision/preview and current quote
// evidence, account isolation and actual View+Trade/no-Transfer key permissions,
// plus complete fresh available balances/inventory. This method is DB-only on
// the passed transaction; provider reads happen BEFORE claiming, not under locks.
// SavedPreflightVerifier validates private persisted evidence. Nil fails closed; a cached trade
// capability, proposal preview or successful risk decision is not a substitute.
type PreflightVerifier interface {
	VerifyDispatchPreflight(context.Context, pgx.Tx, Order, int64) (VerifiedPreflight, error)
}

// OwnerAuthority covers the initial explicit one-order pilot only. It cannot
// authorize AI/autonomous mandate execution or promote a proposal-only review.
type OwnerAuthority struct{ preflight PreflightVerifier }

func NewOwnerAuthority(verifier PreflightVerifier) *OwnerAuthority {
	return &OwnerAuthority{preflight: verifier}
}

type checkedOwnerAuthority struct {
	order         Order
	authorization Authorization
	approvalID    string
	preflight     VerifiedPreflight
	decision      risk.RiskEvaluation
	checkedAt     time.Time
}

func (a *OwnerAuthority) AuthorizeDispatch(ctx context.Context, tx pgx.Tx, o Order, _ time.Time) (Authorization, error) {
	checked, err := a.check(ctx, tx, o, false)
	if err != nil {
		return Authorization{}, err
	}
	o = checked.order
	authorization := checked.authorization
	preflightJSON, _ := json.Marshal(checked.preflight)
	riskJSON, _ := json.Marshal(checked.decision)
	_, err = tx.Exec(ctx, `INSERT INTO execution_authorizations(id,order_id,owner_id,approval_id,reconciliation_id,financial_account_id,credential_generation,request_digest,preflight,risk_evaluation,checked_at,expires_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, authorization.ID, o.ID, o.Request.OwnerID, checked.approvalID, checked.preflight.ReconciliationID, o.Request.AccountID, authorization.CredentialGeneration, o.RequestDigest, preflightJSON, riskJSON, checked.checkedAt, authorization.ExpiresAt)
	if err != nil {
		return Authorization{}, mapError(err)
	}
	return authorization, nil
}

// check re-evaluates current owner authority without saving a new authorization.
// A claimed order must retain its exact durable holds; its own reservation is
// the only execution reservation excluded from the competing-capital check.
func (a *OwnerAuthority) check(ctx context.Context, tx pgx.Tx, o Order, claimed bool) (checkedOwnerAuthority, error) {
	if a == nil || a.preflight == nil {
		return checkedOwnerAuthority{}, ErrNotAuthorized
	}
	stored, err := readOrder(ctx, tx, o.Request.OwnerID, "id", o.ID)
	if err != nil {
		return checkedOwnerAuthority{}, mapError(err)
	}
	if stored.RequestDigest != o.RequestDigest {
		return checkedOwnerAuthority{}, ErrNotAuthorized
	}
	o = stored
	var generation int64
	if err := tx.QueryRow(ctx, `SELECT lock_execution_claim_controls($1)`, o.ID).Scan(&generation); err != nil {
		return checkedOwnerAuthority{}, mapError(err)
	}
	if claimed {
		if err = checkClaimedOwnerHolds(ctx, tx, o); err != nil {
			return checkedOwnerAuthority{}, err
		}
	}
	var approvalID, digest string
	var approvedGeneration int64
	var expiry, enabled time.Time
	err = tx.QueryRow(ctx, `SELECT id::text,request_digest,credential_generation,expires_at,mfa_enabled_at FROM execution_owner_approvals WHERE order_id=$1 AND owner_id=$2 FOR SHARE`, o.ID, o.Request.OwnerID).Scan(&approvalID, &digest, &approvedGeneration, &expiry, &enabled)
	if err != nil {
		return checkedOwnerAuthority{}, ErrNotAuthorized
	}
	var revoked, payload bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM execution_approval_revocations WHERE approval_id=$1)`, approvalID).Scan(&revoked); err != nil {
		return checkedOwnerAuthority{}, err
	}
	var currentEnabled *time.Time
	if err = tx.QueryRow(ctx, `SELECT enabled_at FROM auth_totp_factors WHERE user_id=$1 FOR SHARE`, o.Request.OwnerID).Scan(&currentEnabled); err != nil {
		return checkedOwnerAuthority{}, ErrNotAuthorized
	}
	if err = tx.QueryRow(ctx, `SELECT encrypted_credential_payload IS NOT NULL AND credential_reference IS NULL FROM provider_connections WHERE id=$1`, o.Request.ConnectionID).Scan(&payload); err != nil {
		return checkedOwnerAuthority{}, err
	}
	if revoked || digest != o.RequestDigest || approvedGeneration != generation || !payload || currentEnabled == nil || !currentEnabled.Equal(enabled) {
		return checkedOwnerAuthority{}, ErrNotAuthorized
	}

	// Account row is locked; migration48 serializes new reconciliations on it.
	// Existing reconciliation amounts are NOT assumed to describe the current
	// credential. They provide only the separate enforced drift/coverage gate.
	var reconciliationID, provider string
	var rec risk.ReconciliationSnapshot
	err = tx.QueryRow(ctx, `SELECT id::text,financial_account_id::text,provider_name,comparison_status,balances_status,positions_status,autonomy_signal,autonomy_enforcement_active,blocks_new_actions,change_count,blocking_change_count,observed_at
		FROM portfolio_reconciliations WHERE user_id=$1 AND financial_account_id=$2 ORDER BY observed_at DESC,id DESC LIMIT 1`, o.Request.OwnerID, o.Request.AccountID).Scan(&reconciliationID, &rec.AccountID, &provider, &rec.ComparisonStatus, &rec.BalancesStatus, &rec.PositionsStatus, &rec.AutonomySignal, &rec.AutonomyEnforcementActive, &rec.BlocksNewActions, &rec.ChangeCount, &rec.BlockingChangeCount, &rec.ObservedAt)
	if err != nil || provider != "coinbase" {
		return checkedOwnerAuthority{}, ErrNotAuthorized
	}
	var bucket risk.CapitalBucket
	err = tx.QueryRow(ctx, `SELECT id::text,user_id::text,financial_account_id::text,name,allocation_type,allocation_value::text,currency,protected_amount::text,status,allocation_limit::text,is_reserve FROM capital_buckets WHERE id=$1`, o.Request.CapitalBucketID).Scan(&bucket.ID, &bucket.UserID, &bucket.AccountID, &bucket.Name, &bucket.AllocationType, &bucket.AllocationValue, &bucket.Currency, &bucket.ProtectedAmount, &bucket.Status, &bucket.AllocationLimit, &bucket.IsReserve)
	if err != nil {
		return checkedOwnerAuthority{}, err
	}
	proof, err := a.preflight.VerifyDispatchPreflight(ctx, tx, o, generation)
	if err != nil {
		return checkedOwnerAuthority{}, err
	}
	var now time.Time
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return checkedOwnerAuthority{}, err
	}
	if !expiry.After(now) || risk.CheckAutonomousReconciliation(&rec, o.Request.AccountID, now).Result != risk.Pass {
		return checkedOwnerAuthority{}, ErrNotAuthorized
	}
	if !validPreflight(proof, o, generation, reconciliationID, now) {
		return checkedOwnerAuthority{}, ErrNotAuthorized
	}
	// Provider reads and saved reconciliation have distinct observation times.
	// Both must be fresh and their complete funding facts must agree exactly;
	// never relabel a prior snapshot with the newer provider-read timestamp.
	if rec.ObservedAt.Before(o.CreatedAt) || rec.ObservedAt.After(now) || now.Sub(rec.ObservedAt) > 30*time.Second {
		return checkedOwnerAuthority{}, ErrNotAuthorized
	}
	if err = matchFundingReconciliation(ctx, tx, o, proof); err != nil {
		return checkedOwnerAuthority{}, err
	}
	var competing bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM capital_reservations WHERE financial_account_id=$1 AND expires_at>$2)
		OR EXISTS(SELECT 1 FROM execution_capital_reservations WHERE financial_account_id=$1 AND (NOT $3::boolean OR order_id<>$4))
		OR EXISTS(SELECT 1 FROM strategy_capital_reservations WHERE financial_account_id=$1 AND execution_mode<>'PAPER' AND released_at IS NULL)`, o.Request.AccountID, now, claimed, o.ID).Scan(&competing)
	if err != nil {
		return checkedOwnerAuthority{}, err
	}
	if competing {
		return checkedOwnerAuthority{}, ErrCapitalHeld
	}
	decision := evaluateOwnerFunding(o, proof, bucket, now)
	if decision.Decision != risk.Allow || !decision.ApprovalRequired || decision.PlatformExecutionAvailable {
		return checkedOwnerAuthority{}, ErrNotAuthorized
	}
	until := now.Add(30 * time.Second)
	for _, limit := range []time.Time{expiry, proof.ExpiresAt} {
		if limit.Before(until) {
			until = limit
		}
	}
	if !until.After(now) {
		return checkedOwnerAuthority{}, ErrNotAuthorized
	}
	return checkedOwnerAuthority{
		order:         o,
		authorization: Authorization{ID: decision.ID, RequestDigest: o.RequestDigest, CredentialGeneration: generation, ExpiresAt: until},
		approvalID:    approvalID,
		preflight:     proof,
		decision:      decision,
		checkedAt:     now,
	}, nil
}

func checkClaimedOwnerHolds(ctx context.Context, tx pgx.Tx, o Order) error {
	resourceType, asset, quantity := "ASSET", strings.TrimSuffix(o.Request.ProductID, "-USD"), o.Request.BaseSize
	if o.Request.Side == "BUY" {
		resourceType, asset, quantity = "CASH", "USD", o.Request.MaximumDebitUSD
	}
	var blocked, observed, held, reserved bool
	err := tx.QueryRow(ctx, `SELECT
		EXISTS(SELECT 1 FROM execution_reconciliation_blocks WHERE financial_account_id=$1),
		EXISTS(SELECT 1 FROM execution_broker_acknowledgements WHERE order_id=$2)
		  OR EXISTS(SELECT 1 FROM execution_fills WHERE order_id=$2)
		  OR EXISTS(SELECT 1 FROM execution_order_terminals WHERE order_id=$2),
		EXISTS(SELECT 1 FROM execution_account_holds WHERE financial_account_id=$1 AND order_id=$2 AND owner_id=$3),
		EXISTS(SELECT 1 FROM execution_capital_reservations r
		  JOIN execution_dispatch_attempts a ON a.order_id=r.order_id AND a.owner_id=r.owner_id AND a.financial_account_id=r.financial_account_id
		  WHERE r.financial_account_id=$1 AND r.order_id=$2 AND r.owner_id=$3 AND r.capital_bucket_id=$4
		    AND r.resource_type=$5 AND r.resource_asset=$6 AND r.quantity=$7::numeric AND r.reserved_at=a.claimed_at)`,
		o.Request.AccountID, o.ID, o.Request.OwnerID, o.Request.CapitalBucketID, resourceType, asset, quantity).Scan(&blocked, &observed, &held, &reserved)
	if err != nil {
		return err
	}
	if blocked {
		return ErrReconciliationBlocked
	}
	if observed {
		return ErrAlreadyAttempted
	}
	if !held || !reserved {
		return ErrNotAuthorized
	}
	return nil
}

// The isolated first-order pilot requires complete, exact saved cash and
// target inventory. A preflight cannot borrow an older MATCHED flag while
// presenting different funding facts or assets outside the selected pair.
func matchFundingReconciliation(ctx context.Context, tx pgx.Tx, o Order, p VerifiedPreflight) error {
	var cash, available *string
	var cashCurrency, availableCurrency *string
	var expected int
	err := tx.QueryRow(ctx, `SELECT cash_amount::text,cash_currency,available_cash_amount::text,available_cash_currency,observed_position_count FROM portfolio_reconciliations WHERE id=$1 AND user_id=$2 AND financial_account_id=$3`, p.ReconciliationID, o.Request.OwnerID, o.Request.AccountID).Scan(&cash, &cashCurrency, &available, &availableCurrency, &expected)
	if err != nil {
		return err
	}
	if cash == nil || available == nil || cashCurrency == nil || availableCurrency == nil || *cashCurrency != "USD" || *availableCurrency != "USD" || !sameAmount(*cash, p.CashUSD) || !sameAmount(*available, p.AvailableCashUSD) {
		return ErrNotAuthorized
	}
	rows, err := tx.Query(ctx, `SELECT symbol,instrument_type,direction,quantity::text,available_quantity::text FROM portfolio_reconciliation_positions WHERE reconciliation_id=$1 AND user_id=$2 AND financial_account_id=$3`, p.ReconciliationID, o.Request.OwnerID, o.Request.AccountID)
	if err != nil {
		return err
	}
	defer rows.Close()
	count, matched := 0, false
	total, free := "0", "0"
	for rows.Next() {
		count++
		if count > 1000 {
			return ErrNotAuthorized
		}
		var symbol, kind, direction, quantity string
		var available *string
		if err = rows.Scan(&symbol, &kind, &direction, &quantity, &available); err != nil {
			return err
		}
		if symbol != strings.TrimSuffix(o.Request.ProductID, "-USD") || kind != "CRYPTO" || direction != "long" || matched || available == nil {
			return ErrNotAuthorized
		}
		matched = true
		total = quantity
		free = *available
	}
	if err = rows.Err(); err != nil {
		return err
	}
	if count != expected || !sameAmount(total, p.TotalBase) || !sameAmount(free, p.AvailableBase) {
		return ErrNotAuthorized
	}
	return nil
}

func sameAmount(stored, proof string) bool {
	a, ok := amount(stored)
	b, valid := decimal(proof, false)
	return ok && valid && a.Cmp(b) == 0
}

func validPreflight(p VerifiedPreflight, o Order, generation int64, reconciliationID string, now time.Time) bool {
	if !validUUID(p.EvidenceID) || p.RequestDigest != o.RequestDigest || p.AccountID != o.Request.AccountID || p.ConnectionID != o.Request.ConnectionID || p.ReconciliationID != reconciliationID || p.CredentialGeneration != generation {
		return false
	}
	if p.ObservedAt.IsZero() || p.ObservedAt.Before(o.CreatedAt) || p.ObservedAt.After(now) || now.Sub(p.ObservedAt) > 30*time.Second || !p.ExpiresAt.After(now) || p.ExpiresAt.After(p.ObservedAt.Add(30*time.Second)) {
		return false
	}
	cash, cok := decimal(p.CashUSD, false)
	available, aok := decimal(p.AvailableCashUSD, false)
	total, tok := decimal(p.TotalBase, false)
	base, bok := decimal(p.AvailableBase, false)
	return cok && aok && tok && bok && available.Cmp(cash) <= 0 && base.Cmp(total) <= 0
}

func evaluateOwnerFunding(o Order, p VerifiedPreflight, bucket risk.CapitalBucket, now time.Time) risk.RiskEvaluation {
	symbol := strings.TrimSuffix(o.Request.ProductID, "-USD")
	qty, _ := decimal(o.Request.BaseSize, true)
	price, _ := decimal(o.Request.LimitPrice, true)
	notional := canonical(new(big.Rat).Mul(qty, price))
	actionType := risk.ActionSell
	if o.Request.Side == "BUY" {
		actionType = risk.ActionBuy
		notional = o.Request.MaximumDebitUSD
	}
	context := risk.EvaluationContext{UserID: o.Request.OwnerID, AccountOwned: true, FinancialEntitled: true, ConnectionUsable: true, Bucket: &bucket, Now: now, MaxStaleness: 30 * time.Second,
		Account:      &risk.AccountRiskSnapshot{AccountID: o.Request.AccountID, Currency: "USD", Timestamp: p.ObservedAt, Cash: p.CashUSD, AvailableCash: p.AvailableCashUSD, BuyingPower: p.AvailableCashUSD, CurrentExposure: "0", Options: risk.CapabilityUnsupported, Margin: risk.CapabilityUnsupported, Positions: []risk.Position{{Instrument: symbol, AvailableQuantity: p.AvailableBase, Exposure: "0"}}},
		Reservations: &risk.CapitalReservationSnapshot{Timestamp: now, AccountReservedCash: "0", BucketReservedCash: "0", TargetInstrument: symbol, TargetReservedQuantity: "0"}}
	return risk.NewEngine().Evaluate(context, risk.ProposedAction{ID: o.ID, CorrelationID: o.Request.ClientOrderID, FinancialAccountID: o.Request.AccountID, Source: risk.SourceUI, ActionType: actionType, Instrument: symbol, Side: o.Request.Side, Quantity: o.Request.BaseSize, Notional: notional, CreatedAt: o.CreatedAt})
}
