package registration

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"os"
	"strconv"
	"strings"
	"time"
)

type SMTPMailer struct {
	Host, Port, Mode, Username, Password, From string
	tlsConfig                                  *tls.Config
}

// ConfigFromEnvironment does not accept passwords in environment variables.
func ConfigFromEnvironment(issuer string) (Config, *SMTPMailer, error) {
	c := Config{Issuer: issuer}
	for _, v := range []struct {
		name string
		dst  *time.Duration
	}{{"AS_REGISTRATION_LINK_TTL", &c.LinkTTL}, {"AS_REGISTRATION_RESEND_AFTER", &c.ResendAfter}} {
		if value := os.Getenv(v.name); value != "" {
			d, err := time.ParseDuration(value)
			if err != nil || d <= 0 || d > 24*time.Hour {
				return c, nil, errors.New("invalid registration duration")
			}
			*v.dst = d
		}
	}
	m := &SMTPMailer{Host: os.Getenv("AS_SMTP_HOST"), Port: os.Getenv("AS_SMTP_PORT"), Mode: os.Getenv("AS_SMTP_TLS"), Username: os.Getenv("AS_SMTP_USERNAME"), From: os.Getenv("AS_SMTP_FROM")}
	if m.Port == "" {
		m.Port = "587"
	}
	if m.Mode == "" {
		m.Mode = "starttls"
	}
	if m.Host == "" || m.Username == "" || os.Getenv("AS_SMTP_PASSWORD_FILE") == "" {
		return c, nil, ErrDelivery
	}
	port, err := strconv.Atoi(m.Port)
	if err != nil || port < 1 || port > 65535 || (m.Mode != "starttls" && m.Mode != "implicit") {
		return c, nil, errors.New("invalid SMTP configuration")
	}
	if _, err = mail.ParseAddress(m.From); err != nil || strings.ContainsAny(m.From, "\r\n") {
		return c, nil, errors.New("invalid SMTP sender")
	}
	info, err := os.Stat(os.Getenv("AS_SMTP_PASSWORD_FILE"))
	if err != nil || !info.Mode().IsRegular() || info.Size() > 65536 || info.Mode().Perm()&0077 != 0 {
		return c, nil, errors.New("SMTP secret file must be a private regular file")
	}
	secret, err := os.ReadFile(os.Getenv("AS_SMTP_PASSWORD_FILE"))
	if err != nil {
		return c, nil, ErrDelivery
	}
	m.Password = strings.TrimSpace(string(secret))
	if m.Password == "" {
		return c, nil, ErrDelivery
	}
	return c, m, nil
}

func (m *SMTPMailer) Send(ctx context.Context, to, link string) error {
	// Do not return network/provider messages: they may echo credentials or recipients.
	if err := m.send(ctx, to, link); err != nil {
		return ErrDelivery
	}
	return nil
}
func (m *SMTPMailer) send(ctx context.Context, to, link string) error {
	from, err := mail.ParseAddress(m.From)
	if err != nil || !validEmail(to) || strings.ContainsAny(link, "\r\n") || strings.ContainsAny(m.From, "\r\n") {
		return ErrInvalid
	}
	if m.Mode != "starttls" && m.Mode != "implicit" {
		return ErrDelivery
	}
	conn, err := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "tcp", net.JoinHostPort(m.Host, m.Port))
	if err != nil {
		return err
	}
	defer conn.Close()
	deadline := time.Now().Add(10 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	_ = conn.SetDeadline(deadline)
	tc := &tls.Config{ServerName: m.Host, MinVersion: tls.VersionTLS12}
	if m.tlsConfig != nil {
		tc = m.tlsConfig.Clone()
		tc.ServerName = m.Host
		tc.MinVersion = tls.VersionTLS12
	}
	if m.Mode == "implicit" {
		secure := tls.Client(conn, tc)
		if err = secure.HandshakeContext(ctx); err != nil {
			return err
		}
		conn = secure
	}
	client, err := smtp.NewClient(conn, m.Host)
	if err != nil {
		return err
	}
	defer client.Close()
	if m.Mode == "starttls" {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return ErrDelivery
		}
		if err = client.StartTLS(tc); err != nil {
			return err
		}
	}
	if err = client.Auth(smtp.PlainAuth("", m.Username, m.Password, m.Host)); err != nil {
		return err
	}
	if err = client.Mail(from.Address); err != nil {
		return err
	}
	if err = client.Rcpt(to); err != nil {
		return err
	}
	body, err := client.Data()
	if err != nil {
		return err
	}
	message := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nDate: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\nПодтвердите email для регистрации в Aheadscale:\r\n\r\n%s\r\n\r\nСсылка действует ограниченное время. Если вы не начинали регистрацию, не переходите по ней.\r\n", from.String(), to, mime.QEncoding.Encode("UTF-8", "Подтвердите email в Aheadscale"), time.Now().Format(time.RFC1123Z), link)
	if _, err = io.WriteString(body, message); err != nil {
		_ = body.Close()
		return err
	}
	if err = body.Close(); err != nil {
		return err
	}
	return client.Quit()
}
