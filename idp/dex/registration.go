package dex

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/dexidp/dex/storage"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// EnsureRegistrationUser resumes only the identity reserved by this registration.
// Matching an email is never sufficient to replace somebody else's password.
func (p *Provider) EnsureRegistrationUser(ctx context.Context, email, password, rawID string) (string, error) {
	if _, err := uuid.Parse(rawID); err != nil {
		return "", errors.New("invalid registration identity")
	}
	check := func(existing storage.Password) (string, error) {
		if existing.UserID != rawID || bcrypt.CompareHashAndPassword(existing.Hash, []byte(password)) != nil {
			return "", errors.New("account already exists; sign in")
		}
		return EncodeDexUserID(rawID, LocalConnectorID), nil
	}
	existing, err := p.storage.GetPassword(ctx, email)
	if err == nil {
		return check(existing)
	}
	if !errors.Is(err, storage.ErrNotFound) {
		return "", fmt.Errorf("read registration identity: %w", err)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	err = p.storage.CreatePassword(ctx, storage.Password{Email: email, Username: email, UserID: rawID, Hash: hash})
	if err != nil {
		// A retry may race the successful insert from the previous process.
		if existing, readErr := p.storage.GetPassword(ctx, email); readErr == nil {
			return check(existing)
		}
		return "", errors.New("could not save registration identity")
	}
	return EncodeDexUserID(rawID, LocalConnectorID), nil
}

// RegistrationContinuation accepts only a live request managed by this Dex instance.
func (p *Provider) RegistrationContinuation(ctx context.Context, value string) bool {
	u, err := url.Parse(value)
	if err != nil || u.IsAbs() || u.Host != "" || u.Fragment != "" || u.Path != "/oauth2/auth/local/login" {
		return false
	}
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil || len(q) != 1 || len(q["state"]) != 1 {
		return false
	}
	req, err := p.storage.GetAuthRequest(ctx, q.Get("state"))
	return err == nil && req.Expiry.After(time.Now()) && req.ConnectorID == LocalConnectorID && !req.LoggedIn
}
