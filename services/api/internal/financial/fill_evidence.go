package financial

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"
)

var ErrInvalidFillEvidence = errors.New("fill evidence is unavailable or inconsistent")

var (
	evidenceID      = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,200}$`)
	evidenceHash    = regexp.MustCompile(`^[a-f0-9]{64}$`)
	evidenceAsset   = regexp.MustCompile(`^[A-Z0-9]{1,20}$`)
	evidenceDecimal = regexp.MustCompile(`^(0|[1-9][0-9]{0,39})(\.[0-9]{1,32})?$`)
)

// FillEvidence contains no raw provider identifiers. The account-bound
// correlation hashes are server-only, not credentials or execution authority.
// Never serialize this private projection to the browser or an AI provider.
type FillEvidence struct {
	AccountReference string    `json:"-"`
	EntryReference   string    `json:"-"`
	TradeReference   string    `json:"-"`
	OrderReference   string    `json:"-"`
	Fill             TradeFill `json:"-"`
	SequenceTime     time.Time `json:"-"`
	// The fill response supplies a commission amount but no currency field.
	// Do not promote the display's quote-currency convention to exact evidence.
	CommissionCurrencyStatus string `json:"-"`
}

type FillEvidencePage struct {
	Provider         string         `json:"-"`
	Feed             string         `json:"-"`
	AccountReference string         `json:"-"`
	Fills            []FillEvidence `json:"-"`
	HasMore          bool           `json:"-"`
	ObservedAt       time.Time      `json:"-"`
}

type FillEvidenceWindow struct{ From, To time.Time }

// PaginationExhausted is a cursor observation, NOT proof of complete account
// history or snapshot isolation. A bounded scan can be capped or inconsistent.
type FillEvidenceScan struct {
	Page                FillEvidencePage   `json:"-"`
	Window              FillEvidenceWindow `json:"-"`
	Pages               int                `json:"-"`
	PaginationExhausted bool               `json:"-"`
}

type FillEvidenceProvider interface {
	ReadFillEvidence(context.Context, *Credentials, string, FillEvidenceWindow) (FillEvidenceScan, error)
}

// CorrelationReference preserves equality inside one provider account without
// retaining the raw identifier. Hashing is not encryption or authorization.
func CorrelationReference(provider, account, kind, id string) (string, error) {
	if (provider != "coinbase" && provider != "schwab") || !evidenceID.MatchString(account) || !evidenceID.MatchString(id) || (kind != "account" && kind != "entry" && kind != "trade" && kind != "order") {
		return "", ErrInvalidFillEvidence
	}
	data, _ := json.Marshal([]string{"arbion-fill-evidence-v1", provider, account, kind, id})
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}

// NormalizeFillEvidence validates and canonicalizes decimals without rounding,
// deriving base quantity, net cash, settlement, or transaction cause.
func NormalizeFillEvidence(e FillEvidence, observed time.Time) (FillEvidence, error) {
	f := e.Fill
	if !evidenceHash.MatchString(e.AccountReference) || !evidenceHash.MatchString(e.EntryReference) || !evidenceHash.MatchString(e.TradeReference) || !evidenceHash.MatchString(e.OrderReference) || !evidenceAsset.MatchString(f.BaseAsset) || !evidenceAsset.MatchString(f.QuoteCurrency) || f.ProductID != f.BaseAsset+"-"+f.QuoteCurrency || (f.Side != "BUY" && f.Side != "SELL") || (f.SizeUnit != f.BaseAsset && f.SizeUnit != f.QuoteCurrency) || f.Commission.Currency != "" || e.CommissionCurrencyStatus != "UNAVAILABLE" || f.TradeTime.IsZero() || e.SequenceTime.IsZero() || observed.IsZero() || f.TradeTime.After(observed) || e.SequenceTime.After(observed) {
		return FillEvidence{}, ErrInvalidFillEvidence
	}
	if _, err := f.TradeTime.MarshalJSON(); err != nil {
		return FillEvidence{}, ErrInvalidFillEvidence
	}
	if _, err := e.SequenceTime.MarshalJSON(); err != nil {
		return FillEvidence{}, ErrInvalidFillEvidence
	}
	if f.Liquidity != "MAKER" && f.Liquidity != "TAKER" && f.Liquidity != "UNKNOWN" {
		return FillEvidence{}, ErrInvalidFillEvidence
	}
	values := []*Decimal{&f.Price, &f.Size, &f.Commission.Amount}
	for i, p := range values {
		s := string(*p)
		if !evidenceDecimal.MatchString(s) {
			return FillEvidence{}, ErrInvalidFillEvidence
		}
		if strings.Contains(s, ".") {
			s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
		}
		if i < 2 && s == "0" {
			return FillEvidence{}, ErrInvalidFillEvidence
		}
		*p = Decimal(s)
	}
	f.TradeTime = f.TradeTime.UTC()
	e.Fill, e.SequenceTime = f, e.SequenceTime.UTC()
	return e, nil
}

func FillEvidenceDigest(e FillEvidence, observed time.Time) (string, error) {
	n, err := NormalizeFillEvidence(e, observed)
	if err != nil {
		return "", err
	}
	f := n.Fill
	data, _ := json.Marshal([]string{"arbion-normalized-fill-v1", n.AccountReference, n.EntryReference, n.TradeReference, n.OrderReference, f.ProductID, f.BaseAsset, f.QuoteCurrency, f.Side, string(f.Price), string(f.Size), f.SizeUnit, string(f.Commission.Amount), f.Commission.Currency, n.CommissionCurrencyStatus, f.Liquidity, f.TradeTime.Format(time.RFC3339Nano), n.SequenceTime.Format(time.RFC3339Nano)})
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}
