// Package automationnotification delivers informational automation updates.
// Messages cannot mutate a mandate, strategy state, or broker account.
package automationnotification

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/arbion/platform/services/api/internal/mailer"
)

type Kind string

var safeErrorCode = regexp.MustCompile(`^[A-Z][A-Z_]{0,63}$`)

const (
	EvaluationCompleted Kind = "EVALUATION_COMPLETED"
	LifecycleRequired   Kind = "LIFECYCLE_REQUIRED"
	FirstFailure        Kind = "FIRST_FAILURE"
)

type Event struct {
	Recipient     string
	MandateID     string
	ExecutionMode string
	Kind          Kind
	SafeErrorCode string
	ScheduledFor  time.Time
}

type Sender interface {
	Send(context.Context, Event) error
}

type EmailSender struct {
	sender  mailer.Sender
	baseURL string
}

func NewEmailSender(sender mailer.Sender, baseURL string) *EmailSender {
	return &EmailSender{sender: sender, baseURL: strings.TrimRight(baseURL, "/")}
}

func (s *EmailSender) Send(ctx context.Context, event Event) error {
	if s == nil || s.sender == nil || event.Recipient == "" || event.MandateID == "" || event.ScheduledFor.IsZero() || (event.ExecutionMode != "PAPER" && event.ExecutionMode != "SHADOW") {
		return errors.New("automation notification is incomplete")
	}
	var subject, summary string
	switch event.Kind {
	case EvaluationCompleted:
		subject = fmt.Sprintf("Arbion %s scheduled evaluation completed", event.ExecutionMode)
		summary = "A guarded non-live scheduled evaluation completed. Review its durable decision and risk evidence in Arbion."
	case LifecycleRequired:
		subject = "Arbion PAPER simulation needs lifecycle review"
		summary = "A simulated option is open and the PAPER strategy is waiting for you to record its lifecycle outcome."
	case FirstFailure:
		if !safeErrorCode.MatchString(event.SafeErrorCode) {
			return errors.New("failed automation notification requires a safe code")
		}
		subject = fmt.Sprintf("Arbion %s schedule needs attention", event.ExecutionMode)
		summary = fmt.Sprintf("A guarded non-live scheduled evaluation failed with safe status code %s. Repeated failures are visible in Arbion.", event.SafeErrorCode)
	default:
		return errors.New("automation notification kind is unsupported")
	}
	link := s.baseURL + "/automations/" + url.PathEscape(event.MandateID)
	text := fmt.Sprintf("%s\n\nScheduled for: %s\nReview: %s\n\nThis email is informational only. It contains no approval or execution action. No broker order was sent, and Arbion has no live execution capability.\n", summary, event.ScheduledFor.UTC().Format(time.RFC3339), link)
	return s.sender.Send(ctx, mailer.Message{To: event.Recipient, Subject: subject, Text: text})
}
