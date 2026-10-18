// Package registration provides Aheadscale signup within the existing browser OAuth flow.
package registration

import (
	"context"
	"encoding/json"
	log "github.com/sirupsen/logrus"
	"io"
	"mime"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
	"time"

	"github.com/netbirdio/netbird/management/server/idp"
	"golang.org/x/time/rate"
)

type identityStore interface {
	CreateUserWithPassword(context.Context, string, string, string) (*idp.UserData, error)
	DeleteUser(context.Context, string) error
}

// NewHandler adds signup to Dex without replacing its OAuth/PKCE or session handlers.
func NewHandler(issuer string, identities identityStore, provision func(context.Context, *idp.UserData) error, next http.Handler) http.Handler {
	parsed, _ := url.Parse(issuer)
	origin := ""
	if parsed != nil && parsed.Host != "" {
		origin = parsed.Scheme + "://" + parsed.Host
	}
	limit := rate.NewLimiter(rate.Every(5*time.Second), 10)
	pending := make(chan struct{}, 2)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth2/signup" {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", "POST")
			http.Error(w, "Method not allowed", 405)
			return
		}
		if origin == "" || r.Header.Get("Origin") != origin {
			http.Error(w, "Forbidden", 403)
			return
		}
		kind, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || kind != "application/json" {
			http.Error(w, "Expected JSON", 415)
			return
		}
		if !limit.Allow() {
			w.Header().Set("Retry-After", "30")
			http.Error(w, "Please try again shortly", 429)
			return
		}
		select {
		case pending <- struct{}{}:
			defer func() { <-pending }()
		default:
			http.Error(w, "Please try again shortly", 429)
			return
		}
		var input struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&input) != nil || decoder.Decode(new(any)) != io.EOF {
			http.Error(w, "Invalid registration details", 400)
			return
		}
		input.Email = strings.ToLower(strings.TrimSpace(input.Email))
		address, err := mail.ParseAddress(input.Email)
		if err != nil || address.Address != input.Email || len(input.Email) > 254 || !strings.Contains(input.Email, ".") || len(input.Password) < 8 || len(input.Password) > 72 {
			http.Error(w, "Enter a valid email and a password of 8–72 bytes", 400)
			return
		}
		user, err := identities.CreateUserWithPassword(r.Context(), input.Email, input.Password, strings.SplitN(input.Email, "@", 2)[0])
		if err != nil {
			http.Error(w, "Could not create account. If you already registered, sign in.", 409)
			return
		}
		if err := provision(r.Context(), user); err != nil {
			log.WithContext(r.Context()).WithError(err).Error("Aheadscale network provisioning failed")
			if rollback := identities.DeleteUser(r.Context(), user.ID); rollback != nil {
				log.WithContext(r.Context()).WithError(rollback).Error("Aheadscale signup rollback failed")
			}
			http.Error(w, "Could not create network. Please try again.", 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"created":true}`))
	})
}
