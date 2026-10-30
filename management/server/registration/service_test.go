package registration

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/netbirdio/netbird/management/server/idp"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type testMail struct {
	links []string
	fail  bool
}

func (m *testMail) Send(_ context.Context, _ string, link string) error {
	if m.fail {
		return errors.New("test SMTP unavailable")
	}
	m.links = append(m.links, link)
	return nil
}

type testIdentity struct{ users map[string]string }

func (i *testIdentity) EnsureRegistrationUser(_ context.Context, email, password, rawID string) (*idp.UserData, error) {
	if old, ok := i.users[email]; ok && old != rawID {
		return nil, idp.ErrRegistrationAccountExists
	}
	i.users[email] = rawID
	return &idp.UserData{ID: rawID, Email: email}, nil
}
func newTestService(t *testing.T) (*Service, *testMail, *testIdentity, *time.Time) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "registration.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	mail := &testMail{}
	identities := &testIdentity{users: map[string]string{}}
	s, err := NewService(db, Config{Issuer: "https://aheadscale.ru/oauth2", LinkTTL: 30 * time.Minute, ResendAfter: time.Minute, ValidateContinuation: func(context.Context, string) bool { return true }}, mail, identities, func(context.Context, *idp.UserData) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	return s, mail, identities, &now
}
func tokenFromMail(t *testing.T, m *testMail) string {
	t.Helper()
	if len(m.links) == 0 {
		t.Fatal("no verification email")
	}
	_, token, ok := strings.Cut(m.links[len(m.links)-1], "#")
	if !ok {
		t.Fatal("missing fragment token")
	}
	return token
}
func TestEmailProofPrecedesIdentity(t *testing.T) {
	s, m, i, _ := newTestService(t)
	ctx := context.Background()
	if err := s.Start(ctx, "owner@example.test", "browser-a", "", "192.0.2.1"); err != nil {
		t.Fatal(err)
	}
	if len(i.users) != 0 {
		t.Fatal("identity created before email proof")
	}
	got, err := s.Complete(ctx, tokenFromMail(t, m), "correct-horse-123", "browser-a")
	if err != nil || got.Email != "owner@example.test" || len(i.users) != 1 {
		t.Fatalf("completion: %+v %v", got, err)
	}
	if _, err = s.Complete(ctx, tokenFromMail(t, m), "correct-horse-123", "browser-a"); err == nil {
		t.Fatal("replayed token accepted")
	}
}
func TestReplacementExpiryAndPersistentRateLimit(t *testing.T) {
	s, m, _, now := newTestService(t)
	ctx := context.Background()
	if err := s.Start(ctx, "owner@example.test", "browser-a", "", "192.0.2.1"); err != nil {
		t.Fatal(err)
	}
	old := tokenFromMail(t, m)
	// Another service instance represents a restart/second replica sharing the DB.
	other, err := NewService(s.db, s.config, m, s.identities, s.provision)
	if err != nil {
		t.Fatal(err)
	}
	other.now = s.now
	if err = other.Start(ctx, "owner@example.test", "browser-a", "", "192.0.2.1"); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("resend not throttled across restart: %v", err)
	}
	*now = now.Add(time.Minute)
	if err = s.Start(ctx, "owner@example.test", "browser-a", "", "192.0.2.1"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Complete(ctx, old, "correct-horse-123", "browser-a"); err == nil {
		t.Fatal("replaced link accepted")
	}
	*now = now.Add(31 * time.Minute)
	if _, err = s.Complete(ctx, tokenFromMail(t, m), "correct-horse-123", "browser-a"); err == nil {
		t.Fatal("expired link accepted")
	}
}
func TestCompletionRecoveryUsesReservedIdentity(t *testing.T) {
	s, m, i, now := newTestService(t)
	ctx := context.Background()
	attempts := 0
	s.provision = func(context.Context, *idp.UserData) error {
		attempts++
		if attempts == 1 {
			return errors.New("temporary network failure")
		}
		return nil
	}
	if err := s.Start(ctx, "owner@example.test", "browser-a", "", "192.0.2.1"); err != nil {
		t.Fatal(err)
	}
	token := tokenFromMail(t, m)
	if _, err := s.Complete(ctx, token, "correct-horse-123", "browser-a"); err == nil {
		t.Fatal("provision failure hidden")
	}
	reserved := i.users["owner@example.test"]
	*now = now.Add(2 * time.Minute)
	if _, err := s.Complete(ctx, token, "correct-horse-123", "browser-a"); err != nil {
		t.Fatal(err)
	}
	if reserved == "" || reserved != i.users["owner@example.test"] || attempts != 2 {
		t.Fatal("recovery changed identity")
	}
}
func TestSMTPFailureAndForeignAccountDoNotActivate(t *testing.T) {
	s, m, i, now := newTestService(t)
	ctx := context.Background()
	m.fail = true
	if err := s.Start(ctx, "owner@example.test", "browser-a", "", "192.0.2.1"); !errors.Is(err, ErrDelivery) {
		t.Fatalf("SMTP failure: %v", err)
	}
	if len(i.users) != 0 {
		t.Fatal("SMTP failure activated identity")
	}
	m.fail = false
	*now = now.Add(time.Minute)
	if err := s.Start(ctx, "owner@example.test", "browser-a", "", "192.0.2.1"); err != nil {
		t.Fatal(err)
	}
	i.users["owner@example.test"] = "existing-user"
	if _, err := s.Complete(ctx, tokenFromMail(t, m), "correct-horse-123", "browser-a"); err == nil {
		t.Fatal("existing account overwritten")
	}
	if i.users["owner@example.test"] != "existing-user" {
		t.Fatal("changed existing identity")
	}
}
func TestForeignBrowserCannotResumeInitiator(t *testing.T) {
	s, m, _, _ := newTestService(t)
	ctx := context.Background()
	if err := s.Start(ctx, "owner@example.test", "browser-a", "/oauth2/auth/local/login?state=original", "192.0.2.1"); err != nil {
		t.Fatal(err)
	}
	got, err := s.Complete(ctx, tokenFromMail(t, m), "correct-horse-123", "browser-b")
	if err != nil {
		t.Fatal(err)
	}
	if got.Continuation != "" {
		t.Fatal("another browser received initiating OAuth request")
	}
}
func TestConcurrentCompletionCreatesOneAccount(t *testing.T) {
	s, m, _, _ := newTestService(t)
	ctx := context.Background()
	if err := s.Start(ctx, "owner@example.test", "browser-a", "", "192.0.2.1"); err != nil {
		t.Fatal(err)
	}
	token := tokenFromMail(t, m)
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for j := 0; j < 2; j++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.Complete(ctx, token, "correct-horse-123", "browser-a")
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("successful completions: %d", success)
	}
}
