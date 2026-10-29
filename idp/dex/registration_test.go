package dex

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/dexidp/dex/storage"
	"github.com/dexidp/dex/storage/memory"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

func TestVerifiedRegistrationIdentityRecovery(t *testing.T) {
	p := &Provider{storage: memory.New(slog.New(slog.NewTextHandler(io.Discard, nil)))}
	ctx := context.Background()
	id := uuid.NewString()
	first, err := p.EnsureRegistrationUser(ctx, "owner@example.test", "correct-horse-123", id)
	if err != nil {
		t.Fatal(err)
	}
	again, err := p.EnsureRegistrationUser(ctx, "owner@example.test", "correct-horse-123", id)
	if err != nil || first != again {
		t.Fatalf("idempotent completion: %s %v", again, err)
	}
	if _, err = p.EnsureRegistrationUser(ctx, "owner@example.test", "attacker-password", uuid.NewString()); err == nil {
		t.Fatal("another registration replaced the existing account")
	}
	if _, err = p.EnsureRegistrationUser(ctx, "owner@example.test", "attacker-password", id); err == nil {
		t.Fatal("recovery replaced a saved password")
	}
	saved, err := p.storage.GetPassword(ctx, "owner@example.test")
	if err != nil {
		t.Fatal(err)
	}
	if err = bcrypt.CompareHashAndPassword(saved.Hash, []byte("correct-horse-123")); err != nil {
		t.Fatal("saved password changed")
	}
}

func TestVerifiedRegistrationContinuationRequiresLiveLocalRequest(t *testing.T) {
	p := &Provider{storage: memory.New(slog.New(slog.NewTextHandler(io.Discard, nil)))}
	ctx := context.Background()
	if err := p.storage.CreateAuthRequest(ctx, storage.AuthRequest{ID: "live", ConnectorID: LocalConnectorID, Expiry: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		value string
		want  bool
	}{
		{"/oauth2/auth/local/login?state=live", true},
		{"https://evil.example/oauth2/auth/local/login?state=live", false},
		{"/oauth2/auth/local/login?state=unknown", false},
		{"/oauth2/auth/local/login?state=live&return_url=https://evil.example", false},
		{"/oauth2/auth/local/login?state=live&state=other", false},
	} {
		if got := p.RegistrationContinuation(ctx, c.value); got != c.want {
			t.Errorf("continuation %q: %v", c.value, got)
		}
	}
	if err := p.storage.UpdateAuthRequest(ctx, "live", func(a storage.AuthRequest) (storage.AuthRequest, error) {
		a.Expiry = time.Now().Add(-time.Second)
		return a, nil
	}); err != nil {
		t.Fatal(err)
	}
	if p.RegistrationContinuation(ctx, "/oauth2/auth/local/login?state=live") {
		t.Fatal("expired OAuth request accepted")
	}
}

func TestVerifiedRegistrationRedirectsExcludeUnrelatedClients(t *testing.T) {
	p := &Provider{storage: memory.New(slog.New(slog.NewTextHandler(io.Discard, nil)))}
	ctx := context.Background()
	for _, c := range []storage.Client{
		{ID: "netbird-dashboard", RedirectURIs: []string{"https://aheadscale.ru/nb-auth"}},
		{ID: "netbird-cli", RedirectURIs: []string{"http://localhost:53000/", "/device/callback", "/oauth2/device/callback"}},
		{ID: "unrelated", RedirectURIs: []string{"https://evil.example/"}},
	} {
		if err := p.storage.CreateClient(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	got, err := p.RegistrationRedirectURIs(ctx)
	if err != nil || len(got) != 2 || got[0] != "https://aheadscale.ru/nb-auth" || got[1] != "http://localhost:53000/" {
		t.Fatalf("unexpected application redirect scope: %v %v", got, err)
	}
}
