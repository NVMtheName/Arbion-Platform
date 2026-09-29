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

func (a *OwnerAuthority) AuthorizeDispatch(ctx context.Context, tx pgx.Tx, o Order, _ time.Time) (Authorization, error) {
	if a == nil || a.preflight == nil {
		return Authorization{}, ErrNotAuthorized
	}
	stored, err := readOrder(ctx, tx, o.Request.OwnerID, "id", o.ID)
	if err != nil {
		return Authorization{}, mapError(err)
	}
	if stored.RequestDigest != o.RequestDigest {
		return Authorization{}, ErrNotAuthorized
	}
	o = stored
	var generation int64
	if err := tx.QueryRow(ctx, `SELECT lock_execution_claim_controls($1)`, o.ID).Scan(&generation); err != nil {
		return Authorization{}, mapError(err)
	}
	var approvalID, digest string
	var approvedGeneration int64
	var expiry, enabled time.Time
	err = tx.QueryRow(ctx, `SELECT id::text,request_digest,credential_generation,expires_at,mfa_enabled_at FROM execution_owner_approvals WHERE order_id=$1 AND owner_id=$2 FOR SHARE`, o.ID, o.Request.OwnerID).Scan(&approvalID, &digest, &approvedGeneration, &expiry, &enabled)
	if err != nil {
		return Authorization{}, ErrNotAuthorized
	}
	var revoked, payload bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM execution_approval_revocations WHERE approval_id=$1)`, approvalID).Scan(&revoked); err != nil {
		return Authorization{}, err
	}
	var currentEnabled *time.Time
	if err = tx.QueryRow(ctx, `SELECT enabled_at FROM auth_totp_factors WHERE user_id=$1 FOR SHARE`, o.Request.OwnerID).Scan(&currentEnabled); err != nil {
		return Authorization{}, ErrNotAuthorized
	}
	if err = tx.QueryRow(ctx, `SELECT encrypted_credential_payload IS NOT NULL AND credential_reference IS NULL FROM provider_connections WHERE id=$1`, o.Request.ConnectionID).Scan(&payload); err != nil {
		return Authorization{}, err
	}
	if revoked || digest != o.RequestDigest || approvedGeneration != generation || !payload || currentEnabled == nil || !currentEnabled.Equal(enabled) {
		return Authorization{}, ErrNotAuthorized
	}

	// Account row is locked; migration48 serializes new reconciliations on it.
	// Existing reconciliation amounts are NOT assumed to describe the current
	// credential. They provide only the separate enforced drift/coverage gate.
	var reconciliationID, provider string
	var rec risk.ReconciliationSnapshot
	err = tx.QueryRow(ctx, `SELECT id::text,financial_account_id::text,provider_name,comparison_status,balances_status,positions_status,autonomy_signal,autonomy_enforcement_active,blocks_new_actions,change_count,blocking_change_count,observed_at
		FROM portfolio_reconciliations WHERE user_id=$1 AND financial_account_id=$2 ORDER BY observed_at DESC,id DESC LIMIT 1`, o.Request.OwnerID, o.Request.AccountID).Scan(&reconciliationID, &rec.AccountID, &provider, &rec.ComparisonStatus, &rec.BalancesStatus, &rec.PositionsStatus, &rec.AutonomySignal, &rec.AutonomyEnforcementActive, &rec.BlocksNewActions, &rec.ChangeCount, &rec.BlockingChangeCount, &rec.ObservedAt)
	if err != nil || provider != "coinbase" {
		return Authorization{}, ErrNotAuthorized
	}
	var bucket risk.CapitalBucket
	err = tx.QueryRow(ctx, `SELECT id::text,user_id::text,financial_account_id::text,name,allocation_type,allocation_value::text,currency,protected_amount::text,status,allocation_limit::text,is_reserve FROM capital_buckets WHERE id=$1`, o.Request.CapitalBucketID).Scan(&bucket.ID, &bucket.UserID, &bucket.AccountID, &bucket.Name, &bucket.AllocationType, &bucket.AllocationValue, &bucket.Currency, &bucket.ProtectedAmount, &bucket.Status, &bucket.AllocationLimit, &bucket.IsReserve)
	if err != nil {
		return Authorization{}, err
	}
	proof, err := a.preflight.VerifyDispatchPreflight(ctx, tx, o, generation)
	if err != nil {
		return Authorization{}, err
	}
	var now time.Time
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return Authorization{}, err
	}
	if !expiry.After(now) || risk.CheckAutonomousReconciliation(&rec, o.Request.AccountID, now).Result != risk.Pass {
		return Authorization{}, ErrNotAuthorized
	}
	if !validPreflight(proof, o, generation, reconciliationID, now) {
		return Authorization{}, ErrNotAuthorized
	}
	// Provider reads and saved reconciliation have distinct observation times.
	// Both must be fresh and their complete funding facts must agree exactly;
	// never relabel a prior snapshot with the newer provider-read timestamp.
	if rec.ObservedAt.Before(o.CreatedAt) || rec.ObservedAt.After(now) || now.Sub(rec.ObservedAt) > 30*time.Second {
		return Authorization{}, ErrNotAuthorized
	}
	if err = matchFundingReconciliation(ctx, tx, o, proof); err != nil {
		return Authorization{}, err
	}
	var competing bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM capital_reservations WHERE financial_account_id=$1 AND expires_at>$2)
		OR EXISTS(SELECT 1 FROM execution_capital_reservations WHERE financial_account_id=$1)
		OR EXISTS(SELECT 1 FROM strategy_capital_reservations WHERE financial_account_id=$1 AND execution_mode<>'PAPER' AND released_at IS NULL)`, o.Request.AccountID, now).Scan(&competing)
	if err != nil {
		return Authorization{}, err
	}
	if competing {
		return Authorization{}, ErrCapitalHeld
	}
	decision := evaluateOwnerFunding(o, proof, bucket, now)
	if decision.Decision != risk.Allow || !decision.ApprovalRequired || decision.PlatformExecutionAvailable {
		return Authorization{}, ErrNotAuthorized
	}
	until := now.Add(30 * time.Second)
	for _, limit := range []time.Time{expiry, proof.ExpiresAt} {
		if limit.Before(until) {
			until = limit
		}
	}
	if !until.After(now) {
		return Authorization{}, ErrNotAuthorized
	}
	preflightJSON, _ := json.Marshal(proof)
	riskJSON, _ := json.Marshal(decision)
	_, err = tx.Exec(ctx, `INSERT INTO execution_authorizations(id,order_id,owner_id,approval_id,reconciliation_id,financial_account_id,credential_generation,request_digest,preflight,risk_evaluation,checked_at,expires_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, decision.ID, o.ID, o.Request.OwnerID, approvalID, reconciliationID, o.Request.AccountID, generation, o.RequestDigest, preflightJSON, riskJSON, now, until)
	if err != nil {
		return Authorization{}, mapError(err)
	}
	return Authorization{ID: decision.ID, RequestDigest: o.RequestDigest, CredentialGeneration: generation, ExpiresAt: until}, nil
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
