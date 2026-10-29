package registration

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/mail"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/netbirdio/netbird/management/server/idp"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrInvalid     = errors.New("invalid registration details")
	ErrToken       = errors.New("invalid or expired verification link")
	ErrRateLimited = errors.New("please wait before requesting another email")
	ErrDelivery    = errors.New("email delivery is temporarily unavailable")
)

type Config struct {
	RedirectURIs         []string
	Issuer               string
	LinkTTL, ResendAfter time.Duration
	ValidateContinuation func(context.Context, string) bool
}
type Mailer interface {
	Send(context.Context, string, string) error
}
type Identity interface {
	EnsureRegistrationUser(context.Context, string, string, string) (*idp.UserData, error)
}
type Completion struct {
	Email        string `json:"email"`
	Continuation string `json:"continuation,omitempty"`
}

// PendingRegistration contains no password or usable email token.
type PendingRegistration struct {
	Email                            string `gorm:"primaryKey;size:254"`
	TokenHash                        string `gorm:"size:64;index"`
	BrowserHash, Continuation, RawID string
	ExpiresAt, NextSend, LeaseUntil  time.Time
	State, ClaimID                   string
	VerifiedAt                       *time.Time
}
type RegistrationSendLimit struct {
	Key       string `gorm:"primaryKey;size:160"`
	Count     int
	ExpiresAt time.Time `gorm:"index"`
}

type Service struct {
	db         *gorm.DB
	config     Config
	mailer     Mailer
	identities Identity
	provision  func(context.Context, *idp.UserData) error
	now        func() time.Time
}

func NewService(db *gorm.DB, c Config, m Mailer, i Identity, p func(context.Context, *idp.UserData) error) (*Service, error) {
	u, err := url.Parse(c.Issuer)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || strings.TrimRight(u.Path, "/") != "/oauth2" {
		return nil, ErrInvalid
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "127.0.0.1" || u.Hostname() == "localhost" || u.Hostname() == "::1")) {
		return nil, ErrInvalid
	}
	if c.LinkTTL == 0 {
		c.LinkTTL = 30 * time.Minute
	}
	for _, value := range c.RedirectURIs {
		u, err := url.Parse(value)
		if err != nil || u.Host == "" || u.User != nil || strings.ContainsAny(u.Host, " \t\r\n;,'\"") {
			return nil, ErrInvalid
		}
		if u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1")) {
			return nil, ErrInvalid
		}
	}
	if c.ResendAfter == 0 {
		c.ResendAfter = time.Minute
	}
	if db == nil || m == nil || i == nil || p == nil || c.LinkTTL <= 0 || c.ResendAfter <= 0 {
		return nil, ErrInvalid
	}
	if err = db.AutoMigrate(&PendingRegistration{}, &RegistrationSendLimit{}); err != nil {
		return nil, fmt.Errorf("registration schema: %w", err)
	}
	return &Service{db: db, config: c, mailer: m, identities: i, provision: p, now: time.Now}, nil
}
func digest(value string) string { h := sha256.Sum256([]byte(value)); return hex.EncodeToString(h[:]) }
func randomSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
func validEmail(value string) bool {
	a, err := mail.ParseAddress(value)
	return err == nil && a.Address == value && len(value) <= 254 && strings.Contains(strings.SplitN(value, "@", 2)[1], ".")
}
func validContinuation(value string) bool {
	u, err := url.Parse(value)
	if err != nil || u.IsAbs() || u.Host != "" || u.Fragment != "" || u.Path != "/oauth2/auth/local/login" {
		return false
	}
	q, err := url.ParseQuery(u.RawQuery)
	return err == nil && len(q) == 1 && len(q["state"]) == 1 && len(q.Get("state")) > 0 && len(q.Get("state")) <= 256
}

func (s *Service) Start(ctx context.Context, email, binding, continuation, source string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	if !validEmail(email) || binding == "" {
		return ErrInvalid
	}
	if continuation != "" && (!validContinuation(continuation) || s.config.ValidateContinuation == nil || !s.config.ValidateContinuation(ctx, continuation)) {
		return ErrInvalid
	}
	token, err := randomSecret()
	if err != nil {
		return err
	}
	now := s.now().UTC()
	row := PendingRegistration{Email: email, TokenHash: digest(token), BrowserHash: digest(binding), Continuation: continuation, RawID: uuid.NewString(), ExpiresAt: now.Add(s.config.LinkTTL), NextSend: now.Add(s.config.ResendAfter), State: "pending"}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// A fixed-window key makes increment/limit atomic across processes and restarts.
		for _, limit := range []struct {
			scope, value string
			max          int
		}{{"global", "all", 300}, {"source", source, 60}, {"email", email, 5}} {
			key := fmt.Sprintf("%s:%s:%d", limit.scope, digest(limit.value), now.Unix()/3600)
			if e := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&RegistrationSendLimit{Key: key, ExpiresAt: now.Truncate(time.Hour).Add(time.Hour)}).Error; e != nil {
				return e
			}
			r := tx.Model(&RegistrationSendLimit{}).Where("key = ? AND count < ?", key, limit.max).UpdateColumn("count", gorm.Expr("count + 1"))
			if r.Error != nil {
				return r.Error
			}
			if r.RowsAffected != 1 {
				return ErrRateLimited
			}
		}
		var old PendingRegistration
		e := tx.Where("email = ?", email).Take(&old).Error
		if e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		if e == nil {
			if now.Before(old.NextSend) || now.Before(old.LeaseUntil) {
				return ErrRateLimited
			}
			if old.State != "done" {
				row.RawID = old.RawID
			}
			row.VerifiedAt = old.VerifiedAt
			r := tx.Model(&PendingRegistration{}).Where("email = ? AND token_hash = ? AND next_send <= ? AND lease_until <= ?", email, old.TokenHash, now, now).Updates(map[string]any{"token_hash": row.TokenHash, "browser_hash": row.BrowserHash, "continuation": row.Continuation, "raw_id": row.RawID, "expires_at": row.ExpiresAt, "next_send": row.NextSend, "state": "pending", "claim_id": "", "lease_until": time.Time{}})
			if r.Error != nil {
				return r.Error
			}
			if r.RowsAffected != 1 {
				return ErrRateLimited
			}
		} else if e = tx.Create(&row).Error; e != nil {
			return e
		}
		if e = tx.Where("expires_at < ? AND state <> ? AND lease_until <= ?", now.Add(-24*time.Hour), "done", now).Delete(&PendingRegistration{}).Error; e != nil {
			return e
		}
		return tx.Where("expires_at < ?", now).Delete(&RegistrationSendLimit{}).Error
	})
	if err != nil {
		return err
	}
	if err = s.mailer.Send(ctx, email, strings.TrimRight(s.config.Issuer, "/")+"/verify-email#"+token); err != nil {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = s.db.WithContext(cleanup).Model(&PendingRegistration{}).Where("email = ? AND token_hash = ? AND state = ?", email, row.TokenHash, "pending").Update("state", "delivery_failed").Error
		return ErrDelivery
	}
	return nil
}

func (s *Service) Complete(ctx context.Context, token, password, binding string) (Completion, error) {
	var result Completion
	b, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(b) != 32 || len(password) < 8 || len(password) > 72 {
		return result, ErrToken
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	now := s.now().UTC()
	hash := digest(token)
	claim := uuid.NewString()
	r := s.db.WithContext(ctx).Model(&PendingRegistration{}).Where("token_hash = ? AND expires_at > ? AND (state = ? OR (state = ? AND lease_until <= ?))", hash, now, "pending", "completing", now).Updates(map[string]any{"state": "completing", "claim_id": claim, "lease_until": now.Add(time.Minute)})
	if r.Error != nil {
		return result, r.Error
	}
	if r.RowsAffected != 1 {
		return result, ErrToken
	}
	completed := false
	defer func() {
		if !completed {
			cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			_ = s.db.WithContext(cleanup).Model(&PendingRegistration{}).Where("token_hash = ? AND claim_id = ?", hash, claim).Updates(map[string]any{"state": "pending", "claim_id": "", "lease_until": time.Time{}}).Error
		}
	}()
	var row PendingRegistration
	if err = s.db.WithContext(ctx).Where("token_hash = ? AND claim_id = ?", hash, claim).Take(&row).Error; err != nil {
		return result, err
	}
	user, err := s.identities.EnsureRegistrationUser(ctx, row.Email, password, row.RawID)
	if err != nil {
		return result, ErrToken
	}
	if err = s.provision(ctx, user); err != nil {
		return result, errors.New("could not create network; retry this link shortly")
	}
	r = s.db.WithContext(ctx).Model(&PendingRegistration{}).Where("token_hash = ? AND claim_id = ?", hash, claim).Updates(map[string]any{"state": "done", "verified_at": now, "claim_id": "", "lease_until": time.Time{}})
	if r.Error != nil {
		return result, r.Error
	}
	if r.RowsAffected != 1 {
		return result, ErrToken
	}
	completed = true
	result.Email = row.Email
	if binding != "" && digest(binding) == row.BrowserHash && row.Continuation != "" && s.config.ValidateContinuation != nil && s.config.ValidateContinuation(ctx, row.Continuation) {
		result.Continuation = row.Continuation
	}
	return result, nil
}
