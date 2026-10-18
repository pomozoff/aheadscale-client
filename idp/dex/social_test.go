package dex

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/dexidp/dex/connector"
	"github.com/dexidp/dex/server"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

func TestAheadscaleSocialConnectors(t *testing.T) {
	for _, provider := range []string{"vkid", "mailru", "ok", "yandex"} {
		t.Run(provider, func(t *testing.T) {
			factory := server.ConnectorsConfig["aheadscale-"+provider]
			require.NotNil(t, factory, "provider must be registered in Dex")
			cfg := factory()
			require.NoError(t, json.Unmarshal([]byte(`{"clientID":"123","clientSecret":"synthetic-secret","redirectURI":"https://aheadscale.example/oauth2/callback"}`), cfg))
			opened, err := cfg.Open(provider, slog.New(slog.NewTextHandler(io.Discard, nil)))
			require.NoError(t, err)
			c := opened.(*socialConnector)
			login, pending, err := c.LoginURL(connector.Scopes{OfflineAccess: true}, "https://aheadscale.example/oauth2/callback", "original-state")
			require.NoError(t, err)
			authorize, err := url.Parse(login)
			require.NoError(t, err)
			require.Equal(t, "original-state", authorize.Query().Get("state"))
			require.Equal(t, "S256", authorize.Query().Get("code_challenge_method"))
			require.NotEmpty(t, authorize.Query().Get("code_challenge"))
			calls := 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				require.NoError(t, r.ParseForm())
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/token" {
					require.Equal(t, "123", r.PostForm.Get("client_id"))
					require.Empty(t, r.URL.RawQuery, "credentials must not be in URLs")
					if r.PostForm.Get("grant_type") == "authorization_code" {
						require.Equal(t, "S256", authorize.Query().Get("code_challenge_method"))
						require.Equal(t, authorize.Query().Get("code_challenge"), oauth2.S256ChallengeFromVerifier(r.PostForm.Get("code_verifier")))
					} else {
						require.Equal(t, "refresh_token", r.PostForm.Get("grant_type"))
						require.Equal(t, "synthetic-refresh", r.PostForm.Get("refresh_token"))
					}
					if provider != "yandex" {
						require.Equal(t, "synthetic-device", r.PostForm.Get("device_id"))
					}
					_, _ = io.WriteString(w, `{"access_token":"synthetic-access","refresh_token":"synthetic-refresh","state":"original-state","user_id":42}`)
				} else if provider == "yandex" {
					require.Equal(t, "OAuth synthetic-access", r.Header.Get("Authorization"))
					_, _ = io.WriteString(w, `{"id":"42","client_id":"123","default_email":"person@example.test","real_name":"Person"}`)
				} else {
					require.Equal(t, "synthetic-access", r.PostForm.Get("access_token"))
					_, _ = io.WriteString(w, `{"user":{"user_id":"42","email":"person@example.test","first_name":"Person"}}`)
				}
			}))
			defer upstream.Close()
			c.tokenURL = upstream.URL + "/token"
			c.profileURL = upstream.URL + "/profile"
			request := httptest.NewRequest("GET", "https://aheadscale.example/oauth2/callback?code=synthetic-code&state=wrong&device_id=synthetic-device", nil)
			_, err = c.HandleCallback(connector.Scopes{}, pending, request)
			require.Error(t, err)
			require.Zero(t, calls)
			request = httptest.NewRequest("GET", "https://aheadscale.example/oauth2/callback?code=synthetic-code&state=original-state&device_id=synthetic-device", nil)
			identity, err := c.HandleCallback(connector.Scopes{OfflineAccess: true}, pending, request)
			require.NoError(t, err)
			require.Equal(t, "42", identity.UserID)
			require.Equal(t, "person@example.test", identity.Email)
			require.False(t, identity.EmailVerified, "missing verification claims must not be invented")
			require.NotEmpty(t, identity.ConnectorData, "offline clients must retain a renewable session")
			renewed, err := c.Refresh(context.Background(), connector.Scopes{}, identity)
			require.NoError(t, err)
			require.Equal(t, identity.UserID, renewed.UserID)
			if provider != "yandex" {
				require.Equal(t, map[string]string{"vkid": "vkid", "mailru": "mail_ru", "ok": "ok_ru"}[provider], authorize.Query().Get("provider"))
			}
			require.NotEqual(t, EncodeDexUserID("42", "local"), EncodeDexUserID(identity.UserID, provider))
		})
	}
}

func TestAheadscaleSocialRejectsUntrustedResponses(t *testing.T) {
	for _, scenario := range []struct{ name, provider, token, profile string }{
		{"token state mismatch", "vkid", `{"access_token":"synthetic","state":"wrong"}`, `{}`},
		{"missing email", "vkid", `{"access_token":"synthetic","state":"expected"}`, `{"user":{"user_id":"42"}}`},
		{"provider error", "vkid", `{"access_token":"synthetic","state":"expected"}`, `{"error":"denied"}`},
		{"token profile mismatch", "vkid", `{"access_token":"synthetic","state":"expected","user_id":43}`, `{"user":{"user_id":42,"email":"person@example.test"}}`},
		{"wrong Yandex app", "yandex", `{"access_token":"synthetic"}`, `{"id":"42","client_id":"other-app","default_email":"person@example.test"}`},
		{"malformed email", "yandex", `{"access_token":"synthetic"}`, `{"id":"42","client_id":"123","default_email":"person@example"}`},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/token" {
					_, _ = io.WriteString(w, scenario.token)
				} else {
					_, _ = io.WriteString(w, scenario.profile)
				}
			}))
			defer upstream.Close()
			c := &socialConnector{provider: scenario.provider, clientID: "123", redirectURI: "https://aheadscale.example/oauth2/callback", tokenURL: upstream.URL + "/token", profileURL: upstream.URL + "/profile", client: upstream.Client()}
			pending, _ := json.Marshal(socialPending{State: "expected", Verifier: oauth2.GenerateVerifier()})
			request := httptest.NewRequest("GET", "https://aheadscale.example/oauth2/callback?state=expected&code=synthetic&device_id=device", nil)
			_, err := c.HandleCallback(connector.Scopes{}, pending, request)
			require.Error(t, err)
		})
	}
	t.Run("refresh cannot change user", func(t *testing.T) {
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(w, `{"id":"other-person","client_id":"123","default_email":"same@example.test"}`)
		}))
		defer upstream.Close()
		c := &socialConnector{provider: "yandex", clientID: "123", profileURL: upstream.URL, client: upstream.Client()}
		session, _ := json.Marshal(socialSession{AccessToken: "synthetic"})
		_, err := c.Refresh(context.Background(), connector.Scopes{}, connector.Identity{UserID: "original-person", Email: "same@example.test", ConnectorData: session})
		require.Error(t, err)
	})
	t.Run("credentials and callback required", func(t *testing.T) {
		for _, cfg := range []*socialConfig{{provider: "yandex", ClientID: "id", RedirectURI: "https://aheadscale.example/oauth2/callback"}, {provider: "vkid", ClientID: "id", RedirectURI: "http://aheadscale.example/oauth2/callback"}, {provider: "vkid", RedirectURI: "https://aheadscale.example/oauth2/callback"}} {
			_, err := cfg.Open("test", slog.Default())
			require.Error(t, err)
		}
	})
}
