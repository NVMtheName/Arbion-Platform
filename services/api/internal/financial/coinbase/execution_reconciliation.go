package coinbase

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/arbion/platform/services/api/internal/execution"
	"github.com/arbion/platform/services/api/internal/financial"
)

var executionEvidenceAmountPattern = regexp.MustCompile(`^(0|[1-9][0-9]{0,35})(\.[0-9]{1,36})?$`)
var executionFillIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,128}$`)
var executionFillCountPattern = regexp.MustCompile(`^(0|[1-9][0-9]{0,3})$`)

var _ execution.BrokerObservationProvider = (*ExecutionAdapter)(nil)

type executionEvidenceAmount string

func (d *executionEvidenceAmount) UnmarshalJSON(b []byte) error {
	if len(b) > 76 {
		return errors.New("invalid amount")
	}
	s := string(b)
	if len(b) > 0 && b[0] == '"' {
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
	}
	if !executionEvidenceAmountPattern.MatchString(s) {
		return errors.New("invalid amount")
	}
	*d = executionEvidenceAmount(s)
	return nil
}

type executionFillPage struct {
	Fills              *[]executionWireFill `json:"fills"`
	Cursor             *string              `json:"cursor"`
	ProofTokenRequired *bool                `json:"proof_token_required"`
}

type executionWireFill struct {
	EntryID            *string           `json:"entry_id"`
	TradeID            *string           `json:"trade_id"`
	OrderID            *string           `json:"order_id"`
	RetailPortfolioID  *string           `json:"retail_portfolio_id"`
	ProductID          *string           `json:"product_id"`
	ProductType        *string           `json:"product_type"`
	Side               *string           `json:"side"`
	TradeType          *string           `json:"trade_type"`
	TradeTime          *time.Time        `json:"trade_time"`
	SequenceTimestamp  *time.Time        `json:"sequence_timestamp"`
	Price              *executionDecimal `json:"price"`
	Size               *executionDecimal `json:"size"`
	Commission         *executionDecimal `json:"commission"`
	SizeInQuote        *bool             `json:"size_in_quote"`
	FutureLegs         json.RawMessage   `json:"future_legs"`
	CommissionDetail   json.RawMessage   `json:"commission_detail_total"`
	CommissionCurrency *string           `json:"commission_currency"`
	FeeCurrency        *string           `json:"fee_currency"`
}

type executionOrderSnapshot struct {
	status                 string
	totals                 execution.Totals
	pendingCancel, settled bool
	createdAt, lastFillAt  time.Time
}

// CollectExecutionObservation performs bounded GETs only. Two unchanged order
// snapshots bracket a complete fill traversal; any ambiguity returns no usable
// observation. It cannot cancel, submit, settle account balances, or free funds.
func (a *ExecutionAdapter) CollectExecutionObservation(ctx context.Context, credentials *financial.Credentials, s execution.ConfirmedSubmission, attempt execution.Attempt) (execution.BrokerObservation, error) {
	fail := func(err error) (execution.BrokerObservation, error) { return execution.BrokerObservation{}, err }
	c, closeTransport, err := a.isolatedClient()
	if err != nil {
		return fail(err)
	}
	defer closeTransport()
	started := c.now().UTC()
	if execution.ValidateRecoverySubmission(s, attempt, started) != nil || !executionUUID(attempt.ProviderOrderID) {
		return fail(invalidExecutionResponse())
	}
	key, err := executionSubmissionKey(credentials, s.PortfolioID)
	if err != nil {
		return fail(err)
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := executionCheckPermissions(ctx, c, key, false); err != nil {
		return fail(err)
	}
	before, err := c.executionSnapshot(ctx, key, s, attempt)
	if err != nil {
		return fail(err)
	}
	r := s.Order.Request
	identity := execution.BrokerIdentity{OwnerID: r.OwnerID, OrderID: s.Order.ID, AccountID: r.AccountID, ConnectionID: r.ConnectionID,
		ClientOrderID: r.ClientOrderID, ProviderOrderID: attempt.ProviderOrderID, ProductID: r.ProductID, Side: r.Side}
	fills, err := c.executionCompleteFills(ctx, key, s, attempt, identity)
	if err != nil {
		return fail(err)
	}
	after, err := c.executionSnapshot(ctx, key, s, attempt)
	if err != nil {
		return fail(err)
	}
	observed := c.now().UTC()
	if before != after || ctx.Err() != nil {
		return fail(invalidExecutionResponse())
	}
	for i := range fills {
		fills[i].ObservedAt = observed
	}
	if after.totals.FillCount > 0 {
		if len(fills) == 0 || !fills[len(fills)-1].TradedAt.Equal(after.lastFillAt) {
			return fail(invalidExecutionResponse())
		}
	}
	obs := execution.BrokerObservation{BrokerIdentity: identity, Status: after.status, CompleteFills: true, Totals: after.totals, Fills: fills, StartedAt: started, ObservedAt: observed}
	if err := execution.ValidateBrokerObservation(s, attempt, obs, observed); err != nil {
		return fail(err)
	}
	return obs, nil
}

func (c *Client) executionSnapshot(ctx context.Context, key *financial.Credentials, s execution.ConfirmedSubmission, a execution.Attempt) (executionOrderSnapshot, error) {
	fail := func() (executionOrderSnapshot, error) { return executionOrderSnapshot{}, invalidExecutionResponse() }
	var detail executionOrderDetail
	if err := c.submissionRequest(ctx, key, http.MethodGet, executionHistoryPath+a.ProviderOrderID, nil, &detail); err != nil {
		return executionOrderSnapshot{}, err
	}
	now := c.now().UTC()
	if _, ok := exactExecutionDetail(detail, s, a, a.ProviderOrderID, now); !ok {
		return fail()
	}
	o := detail.Order
	if o.Status == nil || o.NumberOfFills == nil || !executionFillCountPattern.MatchString(*o.NumberOfFills) ||
		o.FilledSize == nil || o.FilledValue == nil || o.TotalFees == nil || !requiredExecutionBools(o.PendingCancel, o.Settled) {
		return fail()
	}
	count, err := strconv.ParseInt(*o.NumberOfFills, 10, 64)
	if err != nil || count > 100*maxPages {
		return fail()
	}
	status := *o.Status
	switch status {
	case "PENDING", "OPEN", "QUEUED", "CANCEL_QUEUED", "EDIT_QUEUED":
	case "FILLED", "CANCELLED", "EXPIRED", "FAILED":
		if *o.PendingCancel || !*o.Settled {
			return fail()
		}
		if status == "FAILED" {
			if count != 0 {
				return fail()
			}
			status = "REJECTED"
		}
	default:
		return fail()
	}
	var last time.Time
	if count > 0 {
		if o.LastFillTime == nil {
			return fail()
		}
		last, err = time.Parse(time.RFC3339Nano, *o.LastFillTime)
		if err != nil || last.Before(a.ClaimedAt) || last.After(now) {
			return fail()
		}
	} else if o.LastFillTime != nil && *o.LastFillTime != "" {
		return fail()
	}
	canonicalAmount := func(v executionEvidenceAmount) string {
		n, _ := new(big.Rat).SetString(string(v))
		return exactExecutionAmount(n, 36)
	}
	return executionOrderSnapshot{status: status, totals: execution.Totals{FillCount: count, BaseQuantity: canonicalAmount(*o.FilledSize), GrossUSD: canonicalAmount(*o.FilledValue), FeeUSD: canonicalAmount(*o.TotalFees)}, pendingCancel: *o.PendingCancel, settled: *o.Settled, createdAt: o.CreatedTime.UTC(), lastFillAt: last.UTC()}, nil
}

func (c *Client) executionCompleteFills(ctx context.Context, key *financial.Credentials, s execution.ConfirmedSubmission, a execution.Attempt, identity execution.BrokerIdentity) ([]execution.Fill, error) {
	fills := make([]execution.Fill, 0)
	cursor := ""
	seenCursor, seenEntry, seenTrade := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for page := 0; page < maxPages; page++ {
		q := url.Values{"order_ids": {a.ProviderOrderID}, "limit": {"100"}}
		if cursor != "" {
			q.Set("cursor", cursor)
		}
		var p executionFillPage
		if err := c.submissionRequest(ctx, key, http.MethodGet, executionHistoryPath+"fills?"+q.Encode(), nil, &p); err != nil {
			return nil, err
		}
		if p.Fills == nil || len(*p.Fills) > 100 || p.Cursor == nil || len(*p.Cursor) > 1024 || !safeCursor(*p.Cursor) || strings.TrimSpace(*p.Cursor) != *p.Cursor || (p.ProofTokenRequired != nil && *p.ProofTokenRequired) {
			return nil, invalidExecutionResponse()
		}
		for _, raw := range *p.Fills {
			f, err := normalizeExecutionWireFill(raw, s, a, identity, c.now().UTC())
			if err != nil {
				return nil, err
			}
			if seenTrade[f.TradeID] || seenEntry[f.ProviderEvidence.EntryID] {
				return nil, invalidExecutionResponse()
			}
			seenTrade[f.TradeID], seenEntry[f.ProviderEvidence.EntryID] = true, true
			fills = append(fills, f)
		}
		if *p.Cursor == "" {
			sort.Slice(fills, func(i, j int) bool {
				if fills[i].TradedAt.Equal(fills[j].TradedAt) {
					return fills[i].TradeID < fills[j].TradeID
				}
				return fills[i].TradedAt.Before(fills[j].TradedAt)
			})
			return fills, nil
		}
		if len(*p.Fills) == 0 || seenCursor[*p.Cursor] {
			return nil, invalidExecutionResponse()
		}
		cursor = *p.Cursor
		seenCursor[cursor] = true
	}
	return nil, invalidExecutionResponse()
}

func normalizeExecutionWireFill(f executionWireFill, s execution.ConfirmedSubmission, a execution.Attempt, id execution.BrokerIdentity, now time.Time) (execution.Fill, error) {
	fail := func() (execution.Fill, error) { return execution.Fill{}, invalidExecutionResponse() }
	if f.EntryID == nil || !executionFillIDPattern.MatchString(*f.EntryID) || f.TradeID == nil || !executionFillIDPattern.MatchString(*f.TradeID) ||
		f.OrderID == nil || *f.OrderID != a.ProviderOrderID || f.RetailPortfolioID == nil || *f.RetailPortfolioID != s.PortfolioID ||
		f.ProductID == nil || *f.ProductID != id.ProductID || f.Side == nil || *f.Side != id.Side || f.TradeType == nil || *f.TradeType != "FILL" ||
		(f.ProductType != nil && *f.ProductType != "SPOT") || f.SizeInQuote == nil || !executionPositive(f.Price) || !executionPositive(f.Size) || f.Commission == nil ||
		f.TradeTime == nil || f.TradeTime.Before(a.ClaimedAt) || f.TradeTime.After(now) || f.SequenceTimestamp == nil || f.SequenceTimestamp.IsZero() || f.SequenceTimestamp.After(now) ||
		(f.CommissionCurrency != nil && *f.CommissionCurrency != "USD") || (f.FeeCurrency != nil && *f.FeeCurrency != "USD") {
		return fail()
	}
	if len(f.FutureLegs) > 0 && !bytes.Equal(bytes.TrimSpace(f.FutureLegs), []byte("null")) && !bytes.Equal(bytes.TrimSpace(f.FutureLegs), []byte("[]")) {
		return fail()
	}
	fee, _ := new(big.Rat).SetString(string(*f.Commission))
	if !emptyExecutionField(f.CommissionDetail) {
		var d map[string]executionDecimal
		if json.Unmarshal(f.CommissionDetail, &d) != nil {
			return fail()
		}
		total, ok := d["total_commission"]
		if !ok || compareDecimal(financial.Decimal(total), financial.Decimal(*f.Commission)) != 0 {
			return fail()
		}
	}
	price, _ := new(big.Rat).SetString(string(*f.Price))
	size, _ := new(big.Rat).SetString(string(*f.Size))
	base := new(big.Rat).Set(size)
	if *f.SizeInQuote {
		base.Quo(base, price)
	}
	baseText := exactExecutionAmount(base, 18)
	if baseText == "" || !previewDecimalPattern.MatchString(baseText) {
		return fail()
	}
	grossText := exactExecutionAmount(new(big.Rat).Mul(price, base), 36)
	if grossText == "" {
		return fail()
	}
	return execution.Fill{BrokerIdentity: id, TradeID: *f.TradeID, BaseQuantity: baseText, PriceUSD: exactExecutionAmount(price, 18), GrossUSD: grossText, FeeUSD: exactExecutionAmount(fee, 18), TradedAt: f.TradeTime.UTC(), ObservedAt: now,
		ProviderEvidence: &execution.FillProviderEvidence{EntryID: *f.EntryID, SequenceAt: f.SequenceTimestamp.UTC(), Size: string(*f.Size), SizeInQuote: *f.SizeInQuote, FeeCurrency: "USD", FeeCurrencyBasis: "COINBASE_ADVANCED_QUOTE_ASSET_1_91"}}, nil
}

// A decimal is accepted only when the finite representation is exact. No
// rounding is allowed to manufacture base quantity from quote-denominated size.
func exactExecutionAmount(n *big.Rat, scale int) string {
	if n == nil {
		return ""
	}
	text := n.FloatString(scale)
	roundTrip, ok := new(big.Rat).SetString(text)
	if !ok || roundTrip.Cmp(n) != 0 {
		return ""
	}
	text = strings.TrimRight(strings.TrimRight(text, "0"), ".")
	if text == "" {
		return "0"
	}
	return text
}
