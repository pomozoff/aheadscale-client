package registration

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"html"
	"io"
	"mime"
	"mime/multipart"
	"net/mail"
	"os"
	"path/filepath"
	"net"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"
	"time"
)

func captureSMTPVerification(t *testing.T, m *SMTPMailer, link string) string {
	t.Helper()
	// httptest provides a real test certificate; the SMTP protocol below uses it.
	certServer := httptest.NewTLSServer(nil)
	cert := certServer.TLS.Certificates[0]
	root := certServer.Certificate()
	certServer.Close()
	listener, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	messages := make(chan string, 1)
	failures := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			failures <- err
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
		r := textproto.NewReader(bufio.NewReader(conn))
		w := bufio.NewWriter(conn)
		reply := func(value string) { _, _ = w.WriteString(value + "\r\n"); _ = w.Flush() }
		reply("220 test SMTP")
		for {
			line, e := r.ReadLine()
			if e != nil {
				failures <- e
				return
			}
			switch {
			case strings.HasPrefix(line, "EHLO"):
				reply("250-test\r\n250 AUTH PLAIN")
			case strings.HasPrefix(line, "AUTH PLAIN "):
				reply("235 OK")
			case line == "MAIL FROM:<noreply@aheadscale.ru>" || line == "RCPT TO:<owner@example.test>":
				reply("250 OK")
			case line == "DATA":
				reply("354 Ready")
				body, e := r.ReadDotBytes()
				if e != nil {
					failures <- e
					return
				}
				messages <- string(body)
				reply("250 Accepted")
			case line == "QUIT":
				reply("221 Bye")
				return
			default:
				failures <- errors.New("unexpected SMTP command")
				return
			}
		}
	}()
	roots := x509.NewCertPool()
	roots.AddCert(root)
	_, port, _ := net.SplitHostPort(listener.Addr().String())
	m.Host, m.Port, m.Mode = "127.0.0.1", port, "implicit"
	m.tlsConfig = &tls.Config{RootCAs: roots}
	if err = m.Send(context.Background(), "owner@example.test", link); err != nil {
		t.Fatal(err)
	}
	select {
	case body := <-messages:
		return body
	case err := <-failures:
		t.Fatal(err)
	case <-time.After(5 * time.Second):
		t.Fatal("SMTP message not received")
	}
	return ""
}

func verificationParts(t *testing.T, raw string) map[string]string {
	t.Helper()
	message, err := mail.ReadMessage(strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	kind, params, err := mime.ParseMediaType(message.Header.Get("Content-Type"))
	if err != nil || kind != "multipart/alternative" {
		t.Fatalf("expected text and HTML alternatives, got %q", kind)
	}
	subject, err := new(mime.WordDecoder).DecodeHeader(message.Header.Get("Subject"))
	if err != nil || subject != "Подтвердите email — Aheadscale" {
		t.Fatal("incorrect UTF-8 subject")
	}
	parts := make(map[string]string)
	reader := multipart.NewReader(message.Body, params["boundary"])
	order := []string{}
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		kind, params, err := mime.ParseMediaType(part.Header.Get("Content-Type"))
		if err != nil || !strings.EqualFold(params["charset"], "UTF-8") {
			t.Fatal("missing UTF-8 MIME part")
		}
		body, err := io.ReadAll(part)
		if err != nil {
			t.Fatal(err)
		}
		parts[kind] = string(body)
		order = append(order, kind)
	}
	if strings.Join(order, ",") != "text/plain,text/html" {
		t.Fatal("incorrect alternative order")
	}
	return parts
}

func TestSMTPDeliveryUsesTLSAndContainsOnlyVerificationLink(t *testing.T) {
	link := "https://aheadscale.ru/oauth2/verify-email?source=signup&lang=ru#one-time-link"
	m := &SMTPMailer{Username: "ci", Password: "smtp-only-secret", From: "Aheadscale <noreply@aheadscale.ru>"}
	raw := captureSMTPVerification(t, m, link)
	parts := verificationParts(t, raw)
	for kind, body := range parts {
		if !strings.Contains(html.UnescapeString(body), link) || !strings.Contains(body, "30 минут") || strings.Contains(body, "smtp-only-secret") {
			t.Fatalf("incorrect content in %s", kind)
		}
		if strings.Contains(body, "you@example.com") || strings.Contains(body, "#preview") {
			t.Fatal("preview values leaked into registration mail")
		}
	}
	body := parts["text/html"]
	if !strings.Contains(body, "Подтвердить email") || !strings.Contains(body, "aheadscale-logo-v1.png") || !strings.Contains(body, "source=signup&amp;lang=ru") {
		t.Fatal("missing branded HTML or escaped verification link")
	}
	if strings.Count(html.UnescapeString(body), link) != 3 {
		t.Fatal("button, fallback href and fallback text must use the same link")
	}
}

func TestSMTPRegistrationLifetimeFromEnvironment(t *testing.T) {
	secret := filepath.Join(t.TempDir(), "smtp-password")
	if err := os.WriteFile(secret, []byte("smtp-only-secret"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AS_SMTP_HOST", "127.0.0.1")
	t.Setenv("AS_SMTP_USERNAME", "ci")
	t.Setenv("AS_SMTP_FROM", "Aheadscale <noreply@aheadscale.ru>")
	t.Setenv("AS_SMTP_PASSWORD_FILE", secret)
	t.Setenv("AS_REGISTRATION_LINK_TTL", "90m")
	config, m, err := ConfigFromEnvironment("https://aheadscale.ru/oauth2")
	if err != nil || config.LinkTTL != 90*time.Minute {
		t.Fatal("incorrect registration configuration")
	}
	parts := verificationParts(t, captureSMTPVerification(t, m, "https://aheadscale.ru/oauth2/verify-email#one-time-link"))
	for kind, body := range parts {
		if !strings.Contains(body, "90 минут") {
			t.Fatalf("configured lifetime missing in %s", kind)
		}
	}
}

func TestSMTPNeverAuthenticatesWithoutRequiredSTARTTLS(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	commands := make(chan string, 1)
	go func() {
		c, e := l.Accept()
		if e != nil {
			return
		}
		defer c.Close()
		_ = c.SetDeadline(time.Now().Add(3 * time.Second))
		_, _ = c.Write([]byte("220 plaintext test\r\n"))
		r := bufio.NewReader(c)
		_, _ = r.ReadString('\n')
		_, _ = c.Write([]byte("250 plaintext\r\n"))
		line, _ := r.ReadString('\n')
		commands <- line
	}()
	_, port, _ := net.SplitHostPort(l.Addr().String())
	m := &SMTPMailer{Host: "127.0.0.1", Port: port, Mode: "starttls", Username: "ci", Password: "must-not-leak", From: "Aheadscale <noreply@aheadscale.ru>"}
	if err = m.Send(context.Background(), "owner@example.test", "https://aheadscale.ru/oauth2/verify-email#token"); !errors.Is(err, ErrDelivery) {
		t.Fatalf("plaintext SMTP not rejected: %v", err)
	}
	if line := <-commands; strings.Contains(line, "AUTH") || strings.Contains(line, "must-not-leak") {
		t.Fatal("credentials sent without TLS")
	}
}
