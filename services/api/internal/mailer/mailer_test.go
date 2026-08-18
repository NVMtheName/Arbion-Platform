package mailer

import (
	"context"
	"errors"
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
