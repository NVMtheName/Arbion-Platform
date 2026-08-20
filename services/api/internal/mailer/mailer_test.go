package mailer

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestDisabledSenderFailsClosed(t *testing.T) {
	if err := (DisabledSender{}).Send(context.Background(), Message{}); !errors.Is(err, ErrDisabled) {
		t.Fatalf("disabled sender returned %v", err)
	}
}

func TestSMTPSenderRejectsHeaderInjectionBeforeDial(t *testing.T) {
	for name, sender := range map[string]*SMTPSender{
		"recipient": NewSMTPSender(SMTPConfig{Host: "127.0.0.1", Port: 1}),
		"sender":    NewSMTPSender(SMTPConfig{Host: "127.0.0.1", Port: 1, FromName: "Arbion\r\nBcc: attacker@example.com"}),
	} {
		message := Message{To: "person@example.com"}
		if name == "recipient" {
			message.To += "\r\nBcc: attacker@example.com"
		}
		if err := sender.Send(context.Background(), message); err == nil || err.Error() != "email headers contain a newline" {
			t.Fatalf("unsafe %s header returned %v", name, err)
		}
	}
}

func TestFormatMessageDataBuildsMultipartAlternative(t *testing.T) {
	data, err := formatMessageData("Arbion <no-reply@arbion.ai>", Message{
		To:      "person@example.com",
		Subject: "Verify your Arbion email",
		Text:    "Open the secure link.\n",
		HTML:    "<html><body><strong>Open the secure link.</strong></body></html>",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"Content-Type: multipart/alternative", "Content-Type: text/plain; charset=UTF-8", "Content-Type: text/html; charset=UTF-8", "Open the secure link.", "<strong>Open the secure link.</strong>"} {
		if !strings.Contains(data, required) {
			t.Fatalf("multipart email is missing %q", required)
		}
	}
}

func TestRenderBrandedHTMLUsesArbionIdentityAndEscapesContent(t *testing.T) {
	html, err := RenderBrandedHTML(BrandedEmailContent{
		Preheader:   "Verify your Arbion email",
		LogoURL:     "https://www.arbion.ai/brand/arbion-wordmark.svg",
		Heading:     "Verify <unsafe>",
		Intro:       "Activate your invited account.",
		ActionLabel: "Verify email",
		ActionURL:   "https://www.arbion.ai/verify-email#token=abc&next=<unsafe>",
		Detail:      "This link can be used once.",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"#07110e", "#19c9d6", "arbion-wordmark.svg", "Verify email", "Secure, disciplined financial decisions"} {
		if !strings.Contains(html, required) {
			t.Fatalf("branded email is missing %q", required)
		}
	}
	if strings.Contains(html, "<unsafe>") {
		t.Fatal("branded email rendered unescaped dynamic content")
	}
}
