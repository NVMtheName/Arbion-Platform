package execution

import (
	"context"
	"math/big"
	"time"
)

// OwnerOrder is an informational snapshot, never execution authority. Scope
// bindings stay private for the authenticated workflow's independent checks.
type OwnerOrder struct {
	ID                 string                `json:"id"`
	RequestDigest      string                `json:"request_digest"`
	ProductID          string                `json:"product_id"`
	Side               string                `json:"side"`
	BaseSize           string                `json:"base_size"`
	LimitPrice         string                `json:"limit_price"`
	FeeAllowanceUSD    string                `json:"fee_allowance_usd"`
	MaximumDebitUSD    string                `json:"maximum_debit_usd"`
	CreatedAt          time.Time             `json:"created_at"`
	AccountID          string                `json:"-"`
	ConnectionID       string                `json:"-"`
	CapitalBucketID    string                `json:"-"`
	Attempted          bool                  `json:"-"`
	State              string                `json:"state"`
	Summary            string                `json:"summary"`
	ApprovalStatus     string                `json:"approval_status"`
	ApprovalExpiresAt  *time.Time            `json:"approval_expires_at,omitempty"`
	CancellationStatus string                `json:"cancellation_status"`
	AccountHeld        bool                  `json:"account_held"`
	CapitalHeld        bool                  `json:"capital_held"`
	AccountBlocked     bool                  `json:"account_blocked"`
	FillCount          int64                 `json:"fill_count"`
	BaseFilled         string                `json:"base_filled"`
	GrossUSD           string                `json:"gross_usd"`
	FeeUSD             string                `json:"fee_usd"`
	TerminalStatus     string                `json:"terminal_status,omitempty"`
	Accounting         *OwnerOrderAccounting `json:"accounting,omitempty"`
}

type OwnerOrderAccounting struct {
	OpeningCashUSD string    `json:"opening_cash_usd"`
	OpeningBase    string    `json:"opening_base"`
	ClosingCashUSD string    `json:"closing_cash_usd"`
	ClosingBase    string    `json:"closing_base"`
	RecordedAt     time.Time `json:"recorded_at"`
}

type ownerOrderFacts struct {
	now                                time.Time
	termsValid                         bool
	approval, revoked, acknowledged    bool
	rejected, capital, cancellation    bool
	cancellationReceipt                string
	capitalReleased, noSend            *time.Time
	terminalMatches, settlementMatches bool
}

// ReadOwnerOrder deliberately uses one SELECT: approvals, durable attempts,
// broker facts, exact own holds, and closure receipts share one MVCC snapshot.
// It loads no credentials, raw payloads or broker identifiers. Original client
// correlation is scanned privately only to verify the saved request digest.
func (s *PostgresStore) ReadOwnerOrder(ctx context.Context, ownerID, orderID string) (OwnerOrder, error) {
	if !validUUID(ownerID) || !validUUID(orderID) {
		return OwnerOrder{}, ErrInvalid
	}
	var o OwnerOrder
	var f ownerOrderFacts
	var accounting OwnerOrderAccounting
	var recorded *time.Time
	var clientID string
	err := s.db.QueryRow(ctx, `SELECT o.id::text,o.request_digest,o.request->>'ProductID',o.request->>'Side',
	 o.request->>'BaseSize',o.request->>'LimitPrice',o.request->>'FeeAllowanceUSD',o.request->>'MaximumDebitUSD',o.created_at,
	 o.financial_account_id::text,o.provider_connection_id::text,o.capital_bucket_id::text,o.client_order_id::text,statement_timestamp(),
	 p.id IS NOT NULL,p.expires_at,v.approval_id IS NOT NULL,a.order_id IS NOT NULL,b.order_id IS NOT NULL,r.order_id IS NOT NULL,
	 h.order_id IS NOT NULL,c.order_id IS NOT NULL,c.released_at,
	 EXISTS(SELECT 1 FROM execution_reconciliation_blocks q WHERE q.financial_account_id=o.financial_account_id),
	 fills.n,fills.q::text,fills.g::text,fills.fee::text,COALESCE(t.status,''),
	 t.order_id IS NULL OR (t.fill_count=fills.n AND t.base_quantity=fills.q AND t.gross_usd=fills.g AND t.fee_usd=fills.fee),
	 n.recorded_at,k.order_id IS NOT NULL,COALESCE(kr.outcome,''),
	 COALESCE(z.opening_cash_usd::text,''),COALESCE(z.opening_base::text,''),COALESCE(z.closing_cash_usd::text,''),COALESCE(z.closing_base::text,''),z.recorded_at,
	 z.order_id IS NULL OR COALESCE(t.order_id IS NOT NULL AND z.terminal_status=t.status AND z.fill_count=fills.n AND z.base_quantity=fills.q AND z.gross_usd=fills.g AND z.fee_usd=fills.fee AND c.released_at=z.recorded_at,false)
	 FROM execution_orders o
	 LEFT JOIN execution_owner_approvals p ON p.order_id=o.id
	 LEFT JOIN execution_approval_revocations v ON v.approval_id=p.id
	 LEFT JOIN execution_dispatch_attempts a ON a.order_id=o.id
	 LEFT JOIN execution_broker_acknowledgements b ON b.order_id=o.id
	 LEFT JOIN execution_submission_rejections r ON r.order_id=o.id
	 LEFT JOIN execution_account_holds h ON h.order_id=o.id AND h.financial_account_id=o.financial_account_id
	 LEFT JOIN execution_capital_reservations c ON c.order_id=o.id
	 LEFT JOIN execution_order_terminals t ON t.order_id=o.id
	 LEFT JOIN execution_account_settlements z ON z.order_id=o.id
	 LEFT JOIN execution_no_send_resolutions n ON n.order_id=o.id
	 LEFT JOIN execution_cancellation_attempts k ON k.order_id=o.id
	 LEFT JOIN execution_cancellation_receipts kr ON kr.order_id=o.id
	 CROSS JOIN LATERAL (SELECT count(*) n,COALESCE(sum(base_quantity),0) q,COALESCE(sum(gross_usd),0) g,COALESCE(sum(fee_usd),0) fee FROM execution_fills WHERE order_id=o.id) fills
	 WHERE o.id=$1 AND o.owner_id=$2`, orderID, ownerID).Scan(
		&o.ID, &o.RequestDigest, &o.ProductID, &o.Side, &o.BaseSize, &o.LimitPrice, &o.FeeAllowanceUSD, &o.MaximumDebitUSD, &o.CreatedAt,
		&o.AccountID, &o.ConnectionID, &o.CapitalBucketID, &clientID, &f.now, &f.approval, &o.ApprovalExpiresAt, &f.revoked, &o.Attempted, &f.acknowledged, &f.rejected,
		&o.AccountHeld, &f.capital, &f.capitalReleased, &o.AccountBlocked, &o.FillCount, &o.BaseFilled, &o.GrossUSD, &o.FeeUSD, &o.TerminalStatus, &f.terminalMatches,
		&f.noSend, &f.cancellation, &f.cancellationReceipt, &accounting.OpeningCashUSD, &accounting.OpeningBase, &accounting.ClosingCashUSD, &accounting.ClosingBase, &recorded, &f.settlementMatches)
	if err != nil {
		return OwnerOrder{}, mapError(err)
	}
	if recorded != nil {
		accounting.RecordedAt = *recorded
		o.Accounting = &accounting
	}
	digest, err := requestDigest(Request{OwnerID: ownerID, AccountID: o.AccountID, ConnectionID: o.ConnectionID, CapitalBucketID: o.CapitalBucketID, ClientOrderID: clientID, ProductID: o.ProductID, Side: o.Side, BaseSize: o.BaseSize, LimitPrice: o.LimitPrice, FeeAllowanceUSD: o.FeeAllowanceUSD, MaximumDebitUSD: o.MaximumDebitUSD})
	f.termsValid = err == nil && digest == o.RequestDigest
	return projectOwnerOrder(o, f), nil
}

func projectOwnerOrder(o OwnerOrder, f ownerOrderFacts) OwnerOrder {
	o.State, o.ApprovalStatus, o.CancellationStatus = "UNAVAILABLE", "NONE", "NONE"
	o.CapitalHeld = f.capital && f.capitalReleased == nil
	valid := f.termsValid && f.approval == (o.ApprovalExpiresAt != nil) && (!f.revoked || f.approval)
	if !f.termsValid {
		// Do not echo malformed public fields or present an unverified digest.
		o.RequestDigest, o.ProductID, o.Side = "", "", ""
		o.BaseSize, o.LimitPrice, o.FeeAllowanceUSD, o.MaximumDebitUSD = "", "", "", ""
	}
	if f.approval && o.ApprovalExpiresAt != nil {
		o.ApprovalStatus = "RECORDED"
		if f.revoked {
			o.ApprovalStatus = "REVOKED"
		} else if !o.ApprovalExpiresAt.After(f.now) {
			o.ApprovalStatus = "EXPIRED"
		}
	}
	if f.cancellation {
		o.CancellationStatus = "UNKNOWN"
	}
	if f.cancellationReceipt != "" {
		switch f.cancellationReceipt {
		case "UNKNOWN", "ACCEPTED", "NOT_ACCEPTED":
			o.CancellationStatus = f.cancellationReceipt
		default:
			valid = false
		}
		valid = valid && f.cancellation
	}
	for _, field := range []*string{&o.BaseFilled, &o.GrossUSD, &o.FeeUSD} {
		n, ok := amount(*field)
		if !ok {
			valid = false
			*field = "" // Unavailable is not an invented exact zero.
		} else {
			*field = canonical(n)
		}
	}
	terms := Request{Side: o.Side, BaseSize: o.BaseSize, FeeAllowanceUSD: o.FeeAllowanceUSD, MaximumDebitUSD: o.MaximumDebitUSD}
	valid = valid && (o.Side == "BUY" || o.Side == "SELL") && o.FillCount <= 1000 && withinBounds(terms, Totals{o.FillCount, o.BaseFilled, o.GrossUSD, o.FeeUSD})
	terminal := o.TerminalStatus != ""
	if terminal {
		switch o.TerminalStatus {
		case "FILLED", "CANCELLED", "REJECTED", "EXPIRED":
		default:
			valid = false
			o.TerminalStatus = ""
		}
		valid = valid && f.terminalMatches
		if valid && o.TerminalStatus == "FILLED" {
			valid = sameAmount(o.BaseFilled, o.BaseSize)
		}
		if o.TerminalStatus == "REJECTED" {
			valid = valid && o.FillCount == 0
		}
	}
	settled, notSent := o.Accounting != nil, f.noSend != nil
	valid = valid && (f.capitalReleased == nil || f.capital) && !(f.acknowledged && f.rejected)
	if !o.Attempted {
		valid = valid && !f.capital && !o.AccountHeld && !f.acknowledged && !f.rejected && o.FillCount == 0 && !terminal && !settled && !notSent && !f.cancellation
	} else {
		valid = valid && f.approval && f.capital
	}
	valid = valid && (!f.cancellation || (o.Attempted && f.acknowledged)) && (o.FillCount == 0 || f.acknowledged)
	if notSent {
		valid = valid && o.Attempted && !f.acknowledged && !f.rejected && o.FillCount == 0 && !terminal && !settled && !f.cancellation && !o.AccountHeld && f.capitalReleased != nil && f.capitalReleased.Equal(*f.noSend)
	} else if settled {
		valid = valid && o.Attempted && f.acknowledged && terminal && !o.AccountHeld && f.capitalReleased != nil && f.capitalReleased.Equal(o.Accounting.RecordedAt) && f.settlementMatches && validOwnerAccounting(o)
	} else if o.Attempted {
		valid = valid && o.CapitalHeld && (o.AccountHeld != terminal) && (!terminal || f.acknowledged)
	}
	if f.rejected {
		valid = valid && !terminal && o.FillCount == 0 && !f.cancellation
	}
	if valid {
		switch {
		case notSent:
			o.State = "NOT_SENT"
		case settled:
			o.State = "SETTLED"
		case terminal:
			o.State = "AWAITING_ACCOUNT_SETTLEMENT"
		case f.rejected:
			o.State = "REJECTED_HELD"
		case o.FillCount > 0:
			o.State = "PARTIALLY_FILLED"
		case f.acknowledged:
			o.State = "BROKER_ACKNOWLEDGED"
		case o.Attempted:
			o.State = "SUBMISSION_UNKNOWN"
		case o.ApprovalStatus == "RECORDED":
			o.State = "APPROVED"
		default:
			o.State = "PREPARED"
		}
	} else {
		o.Accounting = nil // An inconsistent receipt must not present settled balances.
	}
	summaries := map[string]string{
		"PREPARED":                    "Order prepared. It has not been submitted.",
		"APPROVED":                    "Approval is recorded. Sending still requires current checks.",
		"SUBMISSION_UNKNOWN":          "Submission outcome is unresolved. Do not submit again.",
		"REJECTED_HELD":               "The broker rejected the submission. Capital remains held for review.",
		"BROKER_ACKNOWLEDGED":         "The broker acknowledged the order. Fills are not yet verified.",
		"PARTIALLY_FILLED":            "Verified fills are recorded. The order is not yet final.",
		"AWAITING_ACCOUNT_SETTLEMENT": "Final order history is verified. Cash and position settlement is still required.",
		"SETTLED":                     "Final order history and exact cash and position accounting are recorded.",
		"NOT_SENT":                    "The sender was never entered. Original reservations were released; this attempt cannot be reused.",
		"UNAVAILABLE":                 "Order records are inconsistent. Review is required before further actions.",
	}
	o.Summary = summaries[o.State]
	if o.CancellationStatus != "NONE" {
		o.Summary += " A cancellation result is not proof of final order status."
	}
	if o.AccountBlocked {
		o.Summary += " The account remains blocked for reconciliation review."
	}
	return o
}

func validOwnerAccounting(o OwnerOrder) bool {
	a := o.Accounting
	if a == nil || a.RecordedAt.IsZero() {
		return false
	}
	values := []*string{&a.OpeningCashUSD, &a.OpeningBase, &a.ClosingCashUSD, &a.ClosingBase}
	nums := make([]*big.Rat, len(values))
	for i, p := range values {
		n, ok := decimal(*p, false)
		if !ok {
			return false
		}
		nums[i], *p = n, canonical(n)
	}
	q, _ := amount(o.BaseFilled)
	g, _ := amount(o.GrossUSD)
	fee, _ := amount(o.FeeUSD)
	cash, base := new(big.Rat).Set(nums[0]), new(big.Rat).Set(nums[1])
	if o.Side == "BUY" {
		cash.Sub(cash, g).Sub(cash, fee)
		base.Add(base, q)
	} else if o.Side == "SELL" {
		cash.Add(cash, g).Sub(cash, fee)
		base.Sub(base, q)
	} else {
		return false
	}
	return cash.Cmp(nums[2]) == 0 && base.Cmp(nums[3]) == 0
}
