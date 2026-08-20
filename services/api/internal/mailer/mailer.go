package mailer

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"strings"
	"time"
)

var ErrDisabled = errors.New("email delivery is disabled")

type Message struct {
	To      string
	Subject string
	Text    string
	HTML    string
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
	data, err := formatMessageData(from, message)
	if err != nil {
		_ = w.Close()
		return err
	}
	if _, err = io.Copy(w, bufio.NewReader(strings.NewReader(data))); err != nil {
		_ = w.Close()
		return err
	}
	if err = w.Close(); err != nil {
		return err
	}
	return client.Quit()
}

func formatMessageData(from string, message Message) (string, error) {
	baseHeaders := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\n", from, message.To, message.Subject)
	if message.HTML == "" {
		return baseHeaders + "Content-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n" + normalizeCRLF(message.Text), nil
	}

	var body bytes.Buffer
	alternative := multipart.NewWriter(&body)
	writePart := func(contentType, content string) error {
		header := make(textproto.MIMEHeader)
		header.Set("Content-Type", contentType+"; charset=UTF-8")
		header.Set("Content-Transfer-Encoding", "8bit")
		part, err := alternative.CreatePart(header)
		if err != nil {
			return err
		}
		_, err = io.WriteString(part, normalizeCRLF(content))
		return err
	}
	if err := writePart("text/plain", message.Text); err != nil {
		return "", err
	}
	if err := writePart("text/html", message.HTML); err != nil {
		return "", err
	}
	if err := alternative.Close(); err != nil {
		return "", err
	}
	contentType := mime.FormatMediaType("multipart/alternative", map[string]string{"boundary": alternative.Boundary()})
	return baseHeaders + "Content-Type: " + contentType + "\r\n\r\n" + body.String(), nil
}

func normalizeCRLF(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	return strings.ReplaceAll(value, "\n", "\r\n")
}
