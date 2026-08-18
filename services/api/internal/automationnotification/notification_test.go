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
			if out.message.To != "owner@example.com" || !strings.Contains(out.message.Text, "https://www.arbion.ai/automations/mandate-1") || !strings.Contains(out.message.Text, "informational only") || !strings.Contains(out.message.Text, "No broker order was sent") {
				t.Fatalf("unsafe or incomplete message: %#v", out.message)
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
}
