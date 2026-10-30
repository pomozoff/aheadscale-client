package registration

import (
	"errors"
	"net/mail"
	"strings"
	"testing"
	"time"
)

func TestVerificationTemplateEscapingAndLongLink(t *testing.T) {
	link := "https://aheadscale.ru/oauth2/verify-email?source=signup&lang=ru#" + strings.Repeat("a", 180)
	raw, err := buildVerificationMessage(&mail.Address{Name: "Aheadscale", Address: "noreply@aheadscale.ru"}, "a&b@example.test", link, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	parts := verificationParts(t, string(raw))
	if !strings.Contains(parts["text/plain"], link) || !strings.Contains(parts["text/plain"], "1 час") {
		t.Fatal("plain text lost the original link or lifetime")
	}
	if !strings.Contains(parts["text/html"], "a&amp;b@example.test") || strings.Contains(parts["text/html"], "<script") {
		t.Fatal("unsafe HTML content")
	}
	if !strings.Contains(string(raw), "=\r\n") {
		t.Fatal("long MIME lines were not wrapped")
	}
}

func TestVerificationLifetimeText(t *testing.T) {
	for _, item := range []struct {
		ttl  time.Duration
		want string
	}{
		{time.Minute, "1 минута"},
		{2 * time.Minute, "2 минуты"},
		{11 * time.Minute, "11 минут"},
		{21 * time.Minute, "21 минута"},
		{90 * time.Second, "90 секунд"},
		{1500 * time.Millisecond, "1.5 с"},
	} {
		if got := verificationLifetime(item.ttl); got != item.want {
			t.Errorf("%s: got %q, want %q", item.ttl, got, item.want)
		}
	}
}

func TestVerificationMessageRejectsUnsafeLinks(t *testing.T) {
	from := &mail.Address{Address: "noreply@aheadscale.ru"}
	for _, link := range []string{"javascript:alert(1)", "data:text/html,test", "//example.test/token", "https://aheadscale.ru/#token\r\nX-Injected: yes"} {
		if _, err := buildVerificationMessage(from, "owner@example.test", link, time.Minute); !errors.Is(err, ErrInvalid) {
			t.Fatal("unsafe verification link accepted")
		}
	}
}
