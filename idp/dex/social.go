package dex

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/mail"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/dexidp/dex/connector"
	"github.com/dexidp/dex/server"
	"golang.org/x/oauth2"
)

// These connectors use Dex's existing state, PKCE, sessions and account identity
// handling. Provider credentials belong in private StaticConnectors configuration.
func init() {
	for _, provider := range []string{"vkid", "mailru", "ok", "yandex"} {
		server.ConnectorsConfig["aheadscale-"+provider] = func() server.ConnectorConfig {
			return &socialConfig{provider: provider}
		}
	}
}

type socialConfig struct {
	ClientID     string `json:"clientID"`
	ClientSecret string `json:"clientSecret"`
	RedirectURI  string `json:"redirectURI"`
	provider     string
}

func (c *socialConfig) Open(_ string, _ *slog.Logger) (connector.Connector, error) {
	resolve := func(value string) string {
		if strings.HasPrefix(value, "$") {
			return os.Getenv(strings.TrimPrefix(value, "$"))
		}
		return value
	}
	id, secret := resolve(c.ClientID), resolve(c.ClientSecret)
	redirect, err := url.Parse(c.RedirectURI)
	if err != nil || redirect.Scheme != "https" || redirect.Host == "" || redirect.User != nil || redirect.RawQuery != "" || redirect.Fragment != "" || redirect.Path != "/oauth2/callback" {
		return nil, errors.New("social login requires the public HTTPS /oauth2/callback URI")
	}
	if id == "" || (c.provider == "yandex" && secret == "") || (strings.HasPrefix(c.ClientSecret, "$") && secret == "") {
		return nil, errors.New("social login credentials are missing")
	}
	result := &socialConnector{provider: c.provider, clientID: id, clientSecret: secret, redirectURI: c.RedirectURI,
		authorizationURL: "https://id.vk.ru/authorize", tokenURL: "https://id.vk.ru/oauth2/auth", profileURL: "https://id.vk.ru/oauth2/user_info",
		client: &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }},
	}
	if c.provider == "yandex" {
		result.authorizationURL = "https://oauth.yandex.ru/authorize"
		result.tokenURL = "https://oauth.yandex.ru/token"
		result.profileURL = "https://login.yandex.ru/info?format=json"
	}
	return result, nil
}

type socialConnector struct {
	provider, clientID, clientSecret, redirectURI string
	authorizationURL, tokenURL, profileURL        string
	client                                        *http.Client
}

type socialPending struct {
	State    string `json:"state"`
	Verifier string `json:"verifier"`
}
type socialSession struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	DeviceID     string `json:"device_id"`
	State        string `json:"state"`
}
type socialToken struct {
	AccessToken  string          `json:"access_token"`
	RefreshToken string          `json:"refresh_token"`
	State        string          `json:"state"`
	UserID       json.RawMessage `json:"user_id"`
	Error        json.RawMessage `json:"error"`
}

func (c *socialConnector) LoginURL(_ connector.Scopes, callbackURL, state string) (string, []byte, error) {
	if callbackURL != c.redirectURI || state == "" {
		return "", nil, errors.New("invalid social login callback")
	}
	verifier := oauth2.GenerateVerifier()
	endpoint, err := url.Parse(c.authorizationURL)
	if err != nil {
		return "", nil, err
	}
	query := endpoint.Query()
	query.Set("response_type", "code")
	query.Set("client_id", c.clientID)
	query.Set("redirect_uri", callbackURL)
	query.Set("state", state)
	query.Set("code_challenge", oauth2.S256ChallengeFromVerifier(verifier))
	query.Set("code_challenge_method", "S256")
	if c.provider == "yandex" {
		query.Set("scope", "login:email")
	} else {
		query.Set("scope", "email")
		query.Set("provider", map[string]string{"vkid": "vkid", "mailru": "mail_ru", "ok": "ok_ru"}[c.provider])
		query.Set("prompt", "login")
	}
	endpoint.RawQuery = query.Encode()
	pending, err := json.Marshal(socialPending{State: state, Verifier: verifier})
	return endpoint.String(), pending, err
}

func (c *socialConnector) HandleCallback(_ connector.Scopes, data []byte, r *http.Request) (connector.Identity, error) {
	var pending socialPending
	query := r.URL.Query()
	if json.Unmarshal(data, &pending) != nil || pending.State == "" || pending.Verifier == "" || subtle.ConstantTimeCompare([]byte(query.Get("state")), []byte(pending.State)) != 1 {
		return connector.Identity{}, errors.New("social login state mismatch")
	}
	if query.Get("error") != "" || query.Get("code") == "" {
		return connector.Identity{}, errors.New("social login was not completed")
	}
	session := socialSession{DeviceID: query.Get("device_id"), State: pending.State}
	if c.provider != "yandex" && (session.DeviceID == "" || len(session.DeviceID) > 1024) {
		return connector.Identity{}, errors.New("VK ID device identifier missing")
	}
	values := url.Values{"grant_type": {"authorization_code"}, "code": {query.Get("code")}, "code_verifier": {pending.Verifier}}
	token, err := c.exchange(r.Context(), values, session)
	if err != nil {
		return connector.Identity{}, err
	}
	session.AccessToken, session.RefreshToken = token.AccessToken, token.RefreshToken
	identity, err := c.profile(r.Context(), session)
	if err != nil {
		return connector.Identity{}, err
	}
	if len(token.UserID) > 0 && socialID(token.UserID) != identity.UserID {
		return connector.Identity{}, errors.New("social login identity mismatch")
	}
	identity.ConnectorData, err = json.Marshal(session)
	return identity, err
}

// Refresh revalidates the provider identity; changing the email cannot move a
// session to a different person or grant access to another Aheadscale account.
func (c *socialConnector) Refresh(ctx context.Context, _ connector.Scopes, identity connector.Identity) (connector.Identity, error) {
	var session socialSession
	if json.Unmarshal(identity.ConnectorData, &session) != nil || session.AccessToken == "" {
		return connector.Identity{}, errors.New("social login session missing; sign in again")
	}
	if session.RefreshToken != "" {
		token, err := c.exchange(ctx, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {session.RefreshToken}}, session)
		if err != nil {
			return connector.Identity{}, err
		}
		session.AccessToken = token.AccessToken
		if token.RefreshToken != "" {
			session.RefreshToken = token.RefreshToken
		}
	}
	renewed, err := c.profile(ctx, session)
	if err != nil {
		return connector.Identity{}, err
	}
	if renewed.UserID != identity.UserID {
		return connector.Identity{}, errors.New("social login identity changed; sign in again")
	}
	renewed.ConnectorData, err = json.Marshal(session)
	return renewed, err
}

func (c *socialConnector) exchange(ctx context.Context, values url.Values, session socialSession) (socialToken, error) {
	values.Set("client_id", c.clientID)
	values.Set("redirect_uri", c.redirectURI)
	if c.clientSecret != "" {
		values.Set("client_secret", c.clientSecret)
	}
	if c.provider != "yandex" {
		values.Set("device_id", session.DeviceID)
		values.Set("state", session.State)
	}
	var token socialToken
	if err := c.request(ctx, http.MethodPost, c.tokenURL, values, "", &token); err != nil {
		return token, err
	}
	if len(token.Error) > 0 || token.AccessToken == "" {
		return token, errors.New("social login token exchange rejected; sign in again")
	}
	if c.provider != "yandex" && subtle.ConstantTimeCompare([]byte(token.State), []byte(session.State)) != 1 {
		return token, errors.New("VK ID token state mismatch")
	}
	return token, nil
}

func (c *socialConnector) profile(ctx context.Context, session socialSession) (connector.Identity, error) {
	var profile struct {
		ID       json.RawMessage `json:"id"`
		ClientID string          `json:"client_id"`
		Email    string          `json:"default_email"`
		Name     string          `json:"real_name"`
		Login    string          `json:"login"`
		User     struct {
			ID        json.RawMessage `json:"user_id"`
			Email     string          `json:"email"`
			FirstName string          `json:"first_name"`
			LastName  string          `json:"last_name"`
		} `json:"user"`
		Error json.RawMessage `json:"error"`
	}
	var err error
	if c.provider == "yandex" {
		err = c.request(ctx, http.MethodGet, c.profileURL, nil, "OAuth "+session.AccessToken, &profile)
		if profile.ClientID != c.clientID {
			return connector.Identity{}, errors.New("Yandex ID application mismatch")
		}
	} else {
		err = c.request(ctx, http.MethodPost, c.profileURL, url.Values{"client_id": {c.clientID}, "access_token": {session.AccessToken}}, "", &profile)
		profile.ID, profile.Email = profile.User.ID, profile.User.Email
		profile.Name = strings.TrimSpace(profile.User.FirstName + " " + profile.User.LastName)
	}
	if err != nil {
		return connector.Identity{}, err
	}
	email := strings.ToLower(strings.TrimSpace(profile.Email))
	address, parseErr := mail.ParseAddress(email)
	at := strings.LastIndex(email, "@")
	if len(profile.Error) > 0 || socialID(profile.ID) == "" || parseErr != nil || address.Address != email || len(email) > 254 || at < 0 || !strings.Contains(email[at+1:], ".") {
		return connector.Identity{}, errors.New("social provider did not return a valid identity and email; allow email access or use email sign in")
	}
	name := strings.TrimSpace(profile.Name)
	if name == "" {
		name = email
	}
	return connector.Identity{UserID: socialID(profile.ID), Username: name, PreferredUsername: profile.Login, Email: email, EmailVerified: false}, nil
}

func socialID(raw json.RawMessage) string {
	var value string
	if json.Unmarshal(raw, &value) == nil {
		if len(value) > 256 {
			return ""
		}
		return value
	}
	var number json.Number
	if json.Unmarshal(raw, &number) == nil {
		if integer, err := number.Int64(); err == nil && integer > 0 {
			return number.String()
		}
	}
	return ""
}

func (c *socialConnector) request(ctx context.Context, method, endpoint string, values url.Values, authorization string, result any) error {
	var body io.Reader
	if values != nil {
		body = strings.NewReader(values.Encode())
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return errors.New("invalid social provider request")
	}
	if values != nil {
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	request.Header.Set("Accept", "application/json")
	response, err := c.client.Do(request)
	if err != nil {
		return errors.New("social provider unavailable; try again")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("social provider returned HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 65537))
	if err != nil || len(data) > 65536 {
		return errors.New("invalid social provider response")
	}
	if json.Unmarshal(data, result) != nil {
		return errors.New("invalid social provider response")
	}
	return nil
}

var _ connector.CallbackConnector = (*socialConnector)(nil)
var _ connector.RefreshConnector = (*socialConnector)(nil)
