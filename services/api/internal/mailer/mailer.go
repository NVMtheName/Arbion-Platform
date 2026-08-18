package mailer

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"time"
)

var ErrDisabled = errors.New("email delivery is disabled")

type Message struct {
	To      string
	Subject string
	Text    string
}

type Sender interface {
	Send(context.Context, Message) error
}

type DisabledSender struct{}

func (DisabledSender) Send(context.Context, Message) error { return ErrDisabled }

type SMTPConfig struct {
	Host        string
	Port        int
	Username    string
	Password    string
	FromAddress string
	FromName    string
	Timeout     time.Duration
}

type SMTPSender struct{ cfg SMTPConfig }

func NewSMTPSender(cfg SMTPConfig) *SMTPSender {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 10 * time.Second
	}
	return &SMTPSender{cfg: cfg}
}

func (s *SMTPSender) Send(ctx context.Context, message Message) error {
	if strings.ContainsAny(message.To+message.Subject+s.cfg.FromAddress+s.cfg.FromName, "\r\n") {
		return errors.New("email headers contain a newline")
	}
	dialer := net.Dialer{Timeout: s.cfg.Timeout}
	connection, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(s.cfg.Host, fmt.Sprint(s.cfg.Port)))
	if err != nil {
		return err
	}
	defer connection.Close()
	deadline := time.Now().Add(s.cfg.Timeout)
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
		deadline = contextDeadline
	}
	_ = connection.SetDeadline(deadline)
	client, err := smtp.NewClient(connection, s.cfg.Host)
	if err != nil {
		return err
	}
	defer client.Close()
	if supported, _ := client.Extension("STARTTLS"); !supported {
		return errors.New("smtp server does not support STARTTLS")
	}
	if err = client.StartTLS(&tls.Config{ServerName: s.cfg.Host, MinVersion: tls.VersionTLS12}); err != nil {
		return err
	}
	if err = client.Auth(smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)); err != nil {
		return err
	}
	if err = client.Mail(s.cfg.FromAddress); err != nil {
		return err
	}
	if err = client.Rcpt(message.To); err != nil {
		return err
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	from := s.cfg.FromAddress
	if s.cfg.FromName != "" {
		from = (&mail.Address{Name: s.cfg.FromName, Address: s.cfg.FromAddress}).String()
	}
	body := strings.ReplaceAll(message.Text, "\n", "\r\n")
	data := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n%s", from, message.To, message.Subject, body)
	if _, err = io.Copy(w, bufio.NewReader(strings.NewReader(data))); err != nil {
		_ = w.Close()
		return err
	}
	if err = w.Close(); err != nil {
		return err
	}
	return client.Quit()
}
