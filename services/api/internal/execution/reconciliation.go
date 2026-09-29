package execution

import (
	"errors"
	"math/big"
	"regexp"
	"strings"
	"time"
)

var (
	ErrUnreconciled          = errors.New("terminal totals do not yet match complete saved fills; account held")
	ErrReconciliationBlocked = errors.New("conflicting broker evidence; account blocked for review")
	tradeIDPattern           = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,128}$`)
	amountPattern            = regexp.MustCompile(`^(0|[1-9][0-9]{0,35})(\.[0-9]{1,36})?$`)
)

// BrokerIdentity must be resolved by the private provider adapter. Neither a
// model nor a browser may assert broker evidence through this boundary.
type BrokerIdentity struct {
	OwnerID         string
	OrderID         string
	AccountID       string
	ConnectionID    string
	ClientOrderID   string
	ProviderOrderID string
	ProductID       string
	Side            string
}

type Fill struct {
	BrokerIdentity
	TradeID          string
	BaseQuantity     string
	PriceUSD         string
	GrossUSD         string
	FeeUSD           string
	TradedAt         time.Time
	ObservedAt       time.Time
	ProviderEvidence *FillProviderEvidence `json:",omitempty"`
}

// FillProviderEvidence preserves the source units and provider identities of
// this narrow Coinbase adapter without changing historical display evidence.
type FillProviderEvidence struct {
	EntryID                       string
	SequenceAt                    time.Time
	Size                          string
	SizeInQuote                   bool
	FeeCurrency, FeeCurrencyBasis string
}

// TerminalReport is a normalized final broker order plus a completed fill
// pagination result. A terminal order status alone is insufficient. This is
// order reconciliation, not proof that account cash/positions have reconciled.
type TerminalReport struct {
	BrokerIdentity
	Status        string
	CompleteFills bool
	FillCount     int64
	BaseQuantity  string
	GrossUSD      string
	FeeUSD        string
	CompletedAt   time.Time
	ObservedAt    time.Time
	// Empty means a provider-supplied completion time (legacy exact reports).
	// OBSERVED_TERMINAL_STATUS means CompletedAt is the first verified final
	// observation, not an invented exchange cancellation/completion timestamp.
	CompletionTimeBasis string `json:",omitempty"`
}

type Totals struct {
	FillCount    int64
	BaseQuantity string
	GrossUSD     string
	FeeUSD       string
}

type Reconciliation struct {
	Totals
	TerminalStatus string
	AccountHeld    bool
	AccountBlocked bool
}

func amount(s string) (*big.Rat, bool) {
	if !amountPattern.MatchString(s) {
		return nil, false
	}
	n, ok := new(big.Rat).SetString(s)
	return n, ok
}

func canonical(n *big.Rat) string {
	s := strings.TrimRight(strings.TrimRight(n.FloatString(36), "0"), ".")
	if s == "" {
		return "0"
	}
	return s
}

func identityMatches(o Order, a Attempt, b BrokerIdentity) bool {
	r := o.Request
	return b.OwnerID == r.OwnerID && b.OrderID == o.ID && b.AccountID == r.AccountID && b.ConnectionID == r.ConnectionID &&
		b.ClientOrderID == r.ClientOrderID && b.ProviderOrderID != "" && b.ProviderOrderID == a.ProviderOrderID &&
		b.ProductID == r.ProductID && b.Side == r.Side
}

func normalizeFill(o Order, a Attempt, f Fill, now time.Time) (Fill, error) {
	if !identityMatches(o, a, f.BrokerIdentity) || !tradeIDPattern.MatchString(f.TradeID) || f.TradedAt.Before(a.ClaimedAt) ||
		f.ObservedAt.Before(f.TradedAt) || f.ObservedAt.After(now) {
		return Fill{}, ErrInvalid
	}
	q, qOK := decimal(f.BaseQuantity, true)
	p, pOK := decimal(f.PriceUSD, true)
	g, gOK := amount(f.GrossUSD)
	fee, fOK := decimal(f.FeeUSD, false)
	if !qOK || !pOK || !gOK || !fOK || g.Sign() <= 0 || new(big.Rat).Mul(q, p).Cmp(g) != 0 {
		return Fill{}, ErrInvalid
	}
	limit, _ := decimal(o.Request.LimitPrice, true)
	if (o.Request.Side == "BUY" && p.Cmp(limit) > 0) || (o.Request.Side == "SELL" && p.Cmp(limit) < 0) {
		return Fill{}, ErrInvalid
	}
	f.BaseQuantity = canonical(q)
	f.PriceUSD = canonical(p)
	f.GrossUSD = canonical(g)
	f.FeeUSD = canonical(fee)
	f.TradedAt = f.TradedAt.UTC()
	f.ObservedAt = f.ObservedAt.UTC()
	if e := f.ProviderEvidence; e != nil {
		if !tradeIDPattern.MatchString(e.EntryID) || e.SequenceAt.IsZero() || e.SequenceAt.After(f.ObservedAt) || e.FeeCurrency != "USD" || e.FeeCurrencyBasis != "COINBASE_ADVANCED_QUOTE_ASSET_1_91" {
			return Fill{}, ErrInvalid
		}
		size, ok := amount(e.Size)
		want := q
		if e.SizeInQuote {
			want = g
		}
		if !ok || size.Cmp(want) != 0 {
			return Fill{}, ErrInvalid
		}
		copy := *e
		copy.Size, copy.SequenceAt = canonical(size), e.SequenceAt.UTC()
		f.ProviderEvidence = &copy
	}
	return f, nil
}

func sameFill(a, b Fill) bool {
	// Polling time is delivery metadata, not a second economic fill identity.
	return a.BrokerIdentity == b.BrokerIdentity && a.TradeID == b.TradeID && a.BaseQuantity == b.BaseQuantity && a.PriceUSD == b.PriceUSD &&
		a.GrossUSD == b.GrossUSD && a.FeeUSD == b.FeeUSD && a.TradedAt.Equal(b.TradedAt) && sameFillProviderEvidence(a.ProviderEvidence, b.ProviderEvidence)
}

func sameFillProviderEvidence(a, b *FillProviderEvidence) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.EntryID == b.EntryID && a.SequenceAt.Equal(b.SequenceAt) && a.Size == b.Size && a.SizeInQuote == b.SizeInQuote && a.FeeCurrency == b.FeeCurrency && a.FeeCurrencyBasis == b.FeeCurrencyBasis
}

func withinBounds(r Request, t Totals) bool {
	q, qOK := amount(t.BaseQuantity)
	g, gOK := amount(t.GrossUSD)
	fee, fOK := amount(t.FeeUSD)
	maxQ, mOK := decimal(r.BaseSize, true)
	feeLimit, lOK := decimal(r.FeeAllowanceUSD, false)
	debit, dOK := decimal(r.MaximumDebitUSD, r.Side == "BUY")
	if !qOK || !gOK || !fOK || !mOK || !lOK || !dOK || t.FillCount < 0 || q.Cmp(maxQ) > 0 || fee.Cmp(feeLimit) > 0 {
		return false
	}
	if t.FillCount == 0 {
		return q.Sign() == 0 && g.Sign() == 0 && fee.Sign() == 0
	}
	if q.Sign() <= 0 || g.Sign() <= 0 {
		return false
	}
	if r.Side == "BUY" {
		return new(big.Rat).Add(g, fee).Cmp(debit) <= 0
	}
	return r.Side == "SELL" && g.Cmp(fee) >= 0
}

func normalizeTerminal(o Order, a Attempt, t TerminalReport, now time.Time) (TerminalReport, error) {
	if !identityMatches(o, a, t.BrokerIdentity) || t.CompletedAt.Before(a.ClaimedAt) || t.ObservedAt.Before(t.CompletedAt) || t.ObservedAt.After(now) {
		return TerminalReport{}, ErrInvalid
	}
	if t.CompletionTimeBasis != "" && (t.CompletionTimeBasis != "OBSERVED_TERMINAL_STATUS" || !t.CompletedAt.Equal(t.ObservedAt)) {
		return TerminalReport{}, ErrInvalid
	}
	switch t.Status {
	case "FILLED", "CANCELLED", "REJECTED", "EXPIRED":
	default:
		return TerminalReport{}, ErrInvalid
	}
	q, qOK := amount(t.BaseQuantity)
	g, gOK := amount(t.GrossUSD)
	fee, fOK := amount(t.FeeUSD)
	if !qOK || !gOK || !fOK || t.FillCount < 0 {
		return TerminalReport{}, ErrInvalid
	}
	t.BaseQuantity = canonical(q)
	t.GrossUSD = canonical(g)
	t.FeeUSD = canonical(fee)
	t.CompletedAt = t.CompletedAt.UTC()
	t.ObservedAt = t.ObservedAt.UTC()
	if !t.CompleteFills {
		return TerminalReport{}, ErrUnreconciled
	}
	if !withinBounds(o.Request, Totals{t.FillCount, t.BaseQuantity, t.GrossUSD, t.FeeUSD}) {
		return TerminalReport{}, ErrInvalid
	}
	maxQ, _ := decimal(o.Request.BaseSize, true)
	if (t.Status == "FILLED" && q.Cmp(maxQ) != 0) || (t.Status == "REJECTED" && t.FillCount != 0) {
		return TerminalReport{}, ErrUnreconciled
	}
	return t, nil
}

func sameTerminal(a, b TerminalReport) bool {
	return a.BrokerIdentity == b.BrokerIdentity && a.Status == b.Status && a.CompleteFills == b.CompleteFills && a.FillCount == b.FillCount &&
		a.BaseQuantity == b.BaseQuantity && a.GrossUSD == b.GrossUSD && a.FeeUSD == b.FeeUSD && a.CompletionTimeBasis == b.CompletionTimeBasis &&
		// Poll completion order is not an economic fact. Identical observations
		// may arrive out of order; preserve the first saved receipt unchanged.
		(a.CompletedAt.Equal(b.CompletedAt) || a.CompletionTimeBasis == "OBSERVED_TERMINAL_STATUS")
}
