package strategy

import (
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/arbion/platform/services/api/internal/financial"
)

// QuoteRejectionEvidence is an allowlisted pre-model observation, not a quote,
// permission grant, or diagnosis of the provider's entitlement policy.
type QuoteRejectionEvidence struct {
	SchemaVersion      int        `json:"schema_version"`
	Provider           string     `json:"provider"`
	FinancialAccountID string     `json:"financial_account_id"`
	RequestedSymbol    string     `json:"requested_symbol"`
	QuoteType          string     `json:"quote_type"`
	Realtime           *bool      `json:"realtime"`
	ProviderObservedAt *time.Time `json:"provider_observed_at"`
	EvaluatedAt        time.Time  `json:"evaluated_at"`
	RejectionCode      string     `json:"rejection_code"`
	BeforeModel        bool       `json:"before_model"`
}

type quoteRejectionError struct {
	cause    error
	evidence QuoteRejectionEvidence
}

func (e *quoteRejectionError) Error() string { return e.cause.Error() }
func (e *quoteRejectionError) Unwrap() error { return e.cause }

var rejectedQuoteSymbol = regexp.MustCompile(`^[A-Z0-9][A-Z0-9./$^-]{0,31}$`)

func rejectSchwabQuote(cause error, accountID, symbol string, quote financial.Quote, now time.Time) error {
	quoteType := quote.QuoteType
	switch quoteType {
	case "NBBO", "NFL", "UNAVAILABLE", "UNRECOGNIZED":
	case "":
		quoteType = "UNAVAILABLE"
	default:
		quoteType = "UNRECOGNIZED"
	}
	evidence := QuoteRejectionEvidence{
		SchemaVersion: 1, Provider: "schwab", FinancialAccountID: accountID,
		RequestedSymbol: strings.ToUpper(symbol), QuoteType: quoteType,
		EvaluatedAt: now.UTC(), RejectionCode: classifyScheduleError(cause), BeforeModel: true,
	}
	if quote.Realtime != nil {
		value := *quote.Realtime
		evidence.Realtime = &value
	}
	// Preserve even future provider timestamps: a bad time can be the reason
	// for rejection. Zero or unrepresentable dates remain explicitly absent.
	if validEvidenceTime(quote.ProviderTimestamp) {
		observed := quote.ProviderTimestamp.UTC()
		evidence.ProviderObservedAt = &observed
	}
	return &quoteRejectionError{cause: cause, evidence: evidence}
}

func validEvidenceTime(value time.Time) bool {
	return !value.IsZero() && value.Year() >= 1 && value.Year() <= 9999
}

func (e *QuoteRejectionEvidence) valid(run ScheduledRun, completion ScheduleCompletion) bool {
	if e == nil || e.SchemaVersion != 1 || e.Provider != "schwab" ||
		e.FinancialAccountID == "" || e.FinancialAccountID != run.FinancialAccountID ||
		!rejectedQuoteSymbol.MatchString(e.RequestedSymbol) || !e.BeforeModel ||
		run.CurrentState != AIMonitoring || (run.ExecutionMode != Paper && run.ExecutionMode != Shadow) ||
		completion.Status != "FAILED" || e.RejectionCode != completion.ErrorCode ||
		completion.AIDecision != "" || completion.ExecutionStatus != "" || completion.DuplicateRecovered ||
		!validEvidenceTime(e.EvaluatedAt) || e.EvaluatedAt.Before(run.StartedAt) ||
		e.EvaluatedAt.After(completion.CompletedAt) ||
		(e.ProviderObservedAt != nil && !validEvidenceTime(*e.ProviderObservedAt)) {
		return false
	}
	switch e.QuoteType {
	case "NBBO", "NFL", "UNAVAILABLE", "UNRECOGNIZED":
	default:
		return false
	}
	switch e.RejectionCode {
	case "MARKET_DATA_INVALID", "MARKET_DATA_STALE":
		return true
	case "MARKET_DATA_DELAYED":
		return e.Realtime != nil && !*e.Realtime
	case "MARKET_DATA_REALTIME_UNCONFIRMED":
		return e.Realtime == nil
	default:
		return false
	}
}

func scheduledQuoteRejection(err error, run ScheduledRun, completion ScheduleCompletion) *QuoteRejectionEvidence {
	var rejected *quoteRejectionError
	if errors.As(err, &rejected) && rejected.evidence.valid(run, completion) {
		return &rejected.evidence
	}
	// Missing diagnostic metadata never substitutes for the original failure
	// or prevents its immutable scheduler result from being recorded.
	return nil
}
