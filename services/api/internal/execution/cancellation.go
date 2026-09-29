package execution

import (
	"context"
	"errors"
	"time"

	"github.com/arbion/platform/services/api/internal/financial"
)

var (
	ErrCancellationUnknown     = errors.New("cancellation outcome unknown; reconcile, never resend")
	ErrCancellationNotAccepted = errors.New("cancellation not accepted; reconcile original order")
)

// CancellationAcknowledgement describes only a cancellation request result.
// Accepted is not a terminal status, evidence of no fills, or capital release.
type CancellationAcknowledgement struct {
	ProviderOrderID string
	Accepted        bool
}

// CancellationSender remains separate from runtime read/preview interfaces.
// Implementations must use only the original broker identity, honor the bounded
// context synchronously, and disable redirects, retries and replacement orders.
type CancellationSender interface {
	CancelOnce(context.Context, *financial.Credentials, ConfirmedSubmission, Attempt) (CancellationAcknowledgement, error)
}

// CancellationAttempt is immutable operation history, never order completion.
// A missing receipt is UNKNOWN even if the process died before network I/O.
type CancellationAttempt struct {
	OrderID, ProviderOrderID string
	CredentialGeneration     int64
	ClaimedAt, ExpiresAt     time.Time
	Outcome                  string
	ReceivedAt               *time.Time
}

func cancellationOutcomeError(outcome string) error {
	switch outcome {
	case "ACCEPTED":
		return nil
	case "NOT_ACCEPTED":
		return ErrCancellationNotAccepted
	default:
		return ErrCancellationUnknown
	}
}
