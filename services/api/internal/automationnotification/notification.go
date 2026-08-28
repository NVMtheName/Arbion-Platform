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
	EvaluationCompleted          Kind = "EVALUATION_COMPLETED"
	LifecycleRequired            Kind = "LIFECYCLE_REQUIRED"
	FirstFailure                 Kind = "FIRST_FAILURE"
	ReconciliationReviewRequired Kind = "RECONCILIATION_REVIEW_REQUIRED"
)

type Event struct {
	Recipient          string
	MandateID          string
	FinancialAccountID string
	ReconciliationID   string
	ExecutionMode      string
	Kind               Kind
	SafeErrorCode      string
	ScheduledFor       time.Time
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
	var subject, summary, heading, link string
	switch event.Kind {
	case EvaluationCompleted:
		subject = fmt.Sprintf("Arbion %s scheduled evaluation completed", event.ExecutionMode)
		summary = "A guarded non-live scheduled evaluation completed. Review its durable decision and risk evidence in Arbion."
		heading = "Scheduled evaluation complete"
	case LifecycleRequired:
		subject = "Arbion PAPER simulation needs lifecycle review"
		summary = "A simulated option is open and the PAPER strategy is waiting for you to record its lifecycle outcome."
		heading = "Lifecycle review needed"
	case FirstFailure:
		if !safeErrorCode.MatchString(event.SafeErrorCode) {
			return errors.New("failed automation notification requires a safe code")
		}
		subject = fmt.Sprintf("Arbion %s schedule needs attention", event.ExecutionMode)
		summary = fmt.Sprintf("A guarded non-live scheduled evaluation failed with safe status code %s. Repeated failures are visible in Arbion.", event.SafeErrorCode)
		heading = "Schedule needs attention"
	case ReconciliationReviewRequired:
		if event.ExecutionMode != "SHADOW" || event.FinancialAccountID == "" || event.ReconciliationID == "" {
			return errors.New("drift-review notification is incomplete")
		}
		subject = "Arbion portfolio change needs review"
		summary = "Arbion found a new change in tradable portfolio inventory. Review the immutable comparison before any new autonomous action can proceed."
		heading = "Portfolio change needs review"
		link = s.baseURL + "/accounts/" + url.PathEscape(event.FinancialAccountID)
	default:
		return errors.New("automation notification kind is unsupported")
	}
	if link == "" {
		link = s.baseURL + "/automations/" + url.PathEscape(event.MandateID)
	}
	text := fmt.Sprintf("%s\n\nScheduled for: %s\nReview: %s\n\nThis email is informational only. It contains no approval or execution action. No broker order was sent, and Arbion has no live execution capability.\n", summary, event.ScheduledFor.UTC().Format(time.RFC3339), link)
	html, err := mailer.RenderBrandedHTML(mailer.BrandedEmailContent{
		Preheader:   summary,
		LogoURL:     s.baseURL + "/brand/arbion-wordmark.svg",
		Heading:     heading,
		Intro:       summary,
		ActionLabel: "Review in Arbion",
		ActionURL:   link,
		Detail:      fmt.Sprintf("Scheduled for %s. This email is informational only. It contains no approval or execution action. No broker order was sent, and Arbion has no live execution capability.", event.ScheduledFor.UTC().Format(time.RFC3339)),
	})
	if err != nil {
		return err
	}
	return s.sender.Send(ctx, mailer.Message{To: event.Recipient, Subject: subject, Text: text, HTML: html})
}
