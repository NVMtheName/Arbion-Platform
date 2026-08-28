package automationnotification

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/mailer"
)

type senderFake struct{ message mailer.Message }

func (f *senderFake) Send(_ context.Context, message mailer.Message) error {
	f.message = message
	return nil
}

func TestEmailSenderProducesInformationalNonLiveMessage(t *testing.T) {
	for _, kind := range []Kind{EvaluationCompleted, LifecycleRequired, FirstFailure} {
		t.Run(string(kind), func(t *testing.T) {
			out := &senderFake{}
			sender := NewEmailSender(out, "https://www.arbion.ai/")
			event := Event{Recipient: "owner@example.com", MandateID: "mandate-1", ExecutionMode: "PAPER", Kind: kind, ScheduledFor: time.Date(2026, 8, 18, 15, 0, 0, 0, time.UTC), SafeErrorCode: "PROVIDER"}
			if err := sender.Send(context.Background(), event); err != nil {
				t.Fatal(err)
			}
			if out.message.To != "owner@example.com" || !strings.Contains(out.message.Text, "https://www.arbion.ai/automations/mandate-1") || !strings.Contains(out.message.Text, "informational only") || !strings.Contains(out.message.Text, "No broker order was sent") || !strings.Contains(out.message.HTML, "https://www.arbion.ai/brand/arbion-wordmark.svg") || !strings.Contains(out.message.HTML, "Review in Arbion") {
				t.Fatalf("unsafe or incomplete message: %#v", out.message)
			}
		})
	}
}

func TestEmailSenderRoutesCredentialFreeDriftReviewToTheAccount(t *testing.T) {
	out := &senderFake{}
	sender := NewEmailSender(out, "https://www.arbion.ai")
	event := Event{
		Recipient: "owner@example.com", MandateID: "mandate-1", FinancialAccountID: "account-1",
		ReconciliationID: "reconciliation-1", ExecutionMode: "SHADOW", Kind: ReconciliationReviewRequired,
		ScheduledFor: time.Date(2026, 8, 27, 15, 0, 0, 0, time.UTC),
	}
	if err := sender.Send(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	if out.message.Subject != "Arbion portfolio change needs review" || !strings.Contains(out.message.Text, "https://www.arbion.ai/accounts/account-1") || !strings.Contains(out.message.HTML, "Portfolio change needs review") {
		t.Fatalf("drift review was not routed safely: %#v", out.message)
	}
	for _, prohibited := range []string{"reconciliation-1", "API key", "quantity", "symbol"} {
		if strings.Contains(out.message.Text+out.message.HTML, prohibited) {
			t.Fatalf("drift email leaked %q: %#v", prohibited, out.message)
		}
	}
}

func TestEmailSenderHighlightsDurableShadowEvidenceStatusWithoutAddingAuthority(t *testing.T) {
	tests := []struct {
		status  string
		subject string
		heading string
	}{
		{status: "COLLECTING_EVIDENCE", subject: "Arbion Shadow evaluation completed", heading: "Shadow evidence is still collecting"},
		{status: "EVIDENCE_REVIEWABLE", subject: "Arbion Shadow evidence is ready for review", heading: "Shadow evidence is ready for review"},
	}
	for _, testCase := range tests {
		t.Run(testCase.status, func(t *testing.T) {
			out := &senderFake{}
			sender := NewEmailSender(out, "https://www.arbion.ai")
			event := Event{
				Recipient: "owner@example.com", MandateID: "mandate-1",
				ExecutionMode: "SHADOW", Kind: EvaluationCompleted,
				EvidenceGateStatus: testCase.status,
				ScheduledFor:       time.Date(2026, 9, 1, 1, 0, 0, 0, time.UTC),
			}
			if err := sender.Send(context.Background(), event); err != nil {
				t.Fatal(err)
			}
			if out.message.Subject != testCase.subject || !strings.Contains(out.message.HTML, testCase.heading) || !strings.Contains(out.message.Text, "informational only") || !strings.Contains(out.message.Text, "No broker order was sent") || !strings.Contains(out.message.Text, "https://www.arbion.ai/automations/mandate-1") {
				t.Fatalf("Shadow evidence email was unsafe or incomplete: %#v", out.message)
			}
		})
	}
}

func TestEmailSenderRejectsIncompleteOrUnsupportedEvents(t *testing.T) {
	sender := NewEmailSender(&senderFake{}, "https://www.arbion.ai")
	if err := sender.Send(context.Background(), Event{}); err == nil {
		t.Fatal("incomplete event was accepted")
	}
	if err := sender.Send(context.Background(), Event{Recipient: "owner@example.com", MandateID: "mandate-1", ExecutionMode: "LIVE", Kind: EvaluationCompleted, ScheduledFor: time.Now()}); err == nil {
		t.Fatal("live event was accepted")
	}
	if err := sender.Send(context.Background(), Event{Recipient: "owner@example.com", MandateID: "mandate-1", ExecutionMode: "PAPER", Kind: FirstFailure, SafeErrorCode: "raw provider detail", ScheduledFor: time.Now()}); err == nil {
		t.Fatal("unsafe error detail was accepted")
	}
	if err := sender.Send(context.Background(), Event{Recipient: "owner@example.com", MandateID: "mandate-1", ExecutionMode: "SHADOW", Kind: ReconciliationReviewRequired, ScheduledFor: time.Now()}); err == nil {
		t.Fatal("drift review without exact account evidence was accepted")
	}
	if err := sender.Send(context.Background(), Event{Recipient: "owner@example.com", MandateID: "mandate-1", ExecutionMode: "SHADOW", Kind: EvaluationCompleted, EvidenceGateStatus: "LIVE_READY", ScheduledFor: time.Now()}); err == nil {
		t.Fatal("unknown evidence status was accepted")
	}
	if err := sender.Send(context.Background(), Event{Recipient: "owner@example.com", MandateID: "mandate-1", ExecutionMode: "PAPER", Kind: EvaluationCompleted, EvidenceGateStatus: "EVIDENCE_REVIEWABLE", ScheduledFor: time.Now()}); err == nil {
		t.Fatal("Shadow evidence status crossed into PAPER mode")
	}
}
