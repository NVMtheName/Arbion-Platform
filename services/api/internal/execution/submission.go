package execution

import (
	"context"
	"errors"
	"regexp"
	"time"

	"github.com/arbion/platform/services/api/internal/financial"
)

var ErrSubmissionRejected = errors.New("provider reported submission rejection; evidence retained, never resend")

// SubmissionRejectedError contains only a validated machine code, never a
// provider message/body. It is a reported rejection, not proof of settlement.
type SubmissionRejectedError struct{ Code string }

func (*SubmissionRejectedError) Error() string { return "provider reported submission rejection" }

var rejectionCodePattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,127}$`)

func validRejectionCode(code string) bool { return rejectionCodePattern.MatchString(code) }

// SubmissionLookup cannot submit, refresh a preview or manufacture a terminal.
// An absent/unprovable result is UNKNOWN, never permission to resend.
type SubmissionLookup interface {
	LookupSubmission(context.Context, *financial.Credentials, ConfirmedSubmission, Attempt) (SubmissionAcknowledgement, error)
}

// ValidateSubmission checks immutable terms and the pinned, currently fresh
// preflight. It is necessary adapter validation, not owner/risk authorization.
func ValidateSubmission(s ConfirmedSubmission, now time.Time) error {
	if !validUUID(s.Order.ID) || s.Order.CreatedAt.IsZero() || s.Order.CreatedAt.After(now) || s.PreviewID != s.Preflight.PreviewID {
		return ErrNotAuthorized
	}
	return validateProviderPreflight(s.Preflight, s.Order, s.PortfolioID, now)
}

// ValidateRecoverySubmission validates archived evidence at its original time.
// It deliberately cannot authorize a send with an expired approval or preview.
func ValidateRecoverySubmission(s ConfirmedSubmission, a Attempt, now time.Time) error {
	if ValidateSubmission(s, s.Preflight.CompletedAt) != nil || s.Preflight.CompletedAt.After(now) ||
		a.OrderID != s.Order.ID || a.ClientOrderID != s.Order.Request.ClientOrderID || a.RequestDigest != s.Order.RequestDigest ||
		!validUUID(a.AuthorizationID) || a.CredentialGeneration <= 0 || a.ClaimedAt.Before(s.Preflight.CompletedAt) || a.ClaimedAt.After(now) ||
		!a.ExpiresAt.After(a.ClaimedAt) || a.ExpiresAt.After(a.ClaimedAt.Add(time.Minute)) || (a.ProviderOrderID != "" && !validUUID(a.ProviderOrderID)) {
		return ErrNotAuthorized
	}
	return nil
}
