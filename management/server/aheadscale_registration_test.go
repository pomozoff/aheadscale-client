package server

import (
	"context"
	"encoding/json"
	"fmt"
	nbpeer "github.com/netbirdio/netbird/management/server/peer"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/netbirdio/netbird/idp/dex"
	"github.com/netbirdio/netbird/management/server/idp"
	"github.com/netbirdio/netbird/management/server/registration"
	"github.com/netbirdio/netbird/shared/auth"
	"github.com/stretchr/testify/require"
)

func TestAheadscaleRegistrationIsolation(t *testing.T) {
	t.Setenv("AS_SELF_REGISTRATION", "true")
	manager, _, err := createManagerWithEmbeddedIdPMode(t, "netbird.selfhosted")
	require.NoError(t, err)
	require.False(t, manager.singleAccountMode, "public signup must never join the self-hosted owner account")
	embedded := manager.idpManager.(*idp.EmbeddedIdPManager)
	handler := registration.NewHandler("https://aheadscale.ru/oauth2", embedded, func(ctx context.Context, user *idp.UserData) error {
		_, err := manager.GetAccountIDByUserID(ctx, auth.UserAuth{UserId: user.ID, Email: user.Email, Name: user.Name})
		return err
	}, http.NotFoundHandler())
	var accountIDs []string
	for _, email := range []string{"alice@example.com", "bob@example.com"} {
		body, err := json.Marshal(map[string]string{"email": email, "password": "correct-horse-123"})
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "https://aheadscale.ru/oauth2/signup", strings.NewReader(string(body)))
		req.Header.Set("Origin", "https://aheadscale.ru")
		req.Header.Set("Content-Type", "application/json")
		result := httptest.NewRecorder()
		handler.ServeHTTP(result, req)
		require.Equal(t, http.StatusCreated, result.Code, result.Body.String())
		require.NotContains(t, result.Body.String(), "correct-horse")

	}
	for _, scenario := range []struct {
		body, origin, kind string
		code               int
	}{
		{`{"email":"alice@example.com","password":"correct-horse-123"}`, "https://aheadscale.ru", "application/json", 409},
		{`{"email":"wrong","password":"short"}`, "https://aheadscale.ru", "application/json", 400},
		{`{"email":"other@example.com","password":"correct-horse-123"}`, "https://evil.example", "application/json", 403},
		{`{"email":"other@example.com","password":"correct-horse-123"}`, "https://aheadscale.ru", "text/plain", 415},
	} {
		request := httptest.NewRequest(http.MethodPost, "https://aheadscale.ru/oauth2/signup", strings.NewReader(scenario.body))
		request.Header.Set("Origin", scenario.origin)
		request.Header.Set("Content-Type", scenario.kind)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		require.Equal(t, scenario.code, response.Code, response.Body.String())
	}

	accounts := manager.Store.GetAllAccounts(context.Background())
	require.Len(t, accounts, 2, "equal email domains must remain two independent networks")
	for _, account := range accounts {
		require.Len(t, account.Users, 1)
		accountIDs = append(accountIDs, account.Id)
		for userID, user := range account.Users {
			require.Equal(t, "owner", string(user.Role))
			actual, _, err := manager.GetAccountIDFromUserAuth(context.Background(), auth.UserAuth{UserId: userID})
			require.NoError(t, err)
			require.Equal(t, account.Id, actual, "subsequent OAuth login must keep the personal account")
		}
	}
	require.NotEqual(t, accountIDs[0], accountIDs[1])
}

func TestAheadscaleDeviceLimit(t *testing.T) {
	t.Setenv("AS_SELF_REGISTRATION", "true")
	manager, _, err := createManagerWithEmbeddedIdPMode(t, "")
	require.NoError(t, err)
	user, err := manager.idpManager.(*idp.EmbeddedIdPManager).CreateUserWithPassword(context.Background(), "quota@example.com", "correct-horse-123", "quota")
	require.NoError(t, err)
	account, err := manager.GetOrCreateAccountByUser(context.Background(), auth.UserAuth{UserId: user.ID, Email: user.Email, Name: user.Name})
	require.NoError(t, err)
	add := func(index int) error {
		key, err := wgtypes.GeneratePrivateKey()
		if err != nil {
			return err
		}
		_, _, _, _, err = manager.AddPeer(context.Background(), account.Id, "", user.ID, &nbpeer.Peer{Key: key.PublicKey().String(), Meta: nbpeer.PeerSystemMeta{Hostname: fmt.Sprintf("device-%d", index)}}, false)
		return err
	}
	for i := 0; i < 4; i++ {
		require.NoError(t, add(i))
	}
	results := make(chan error, 2)
	var workers sync.WaitGroup
	for i := 4; i < 6; i++ {
		workers.Add(1)
		go func(index int) { defer workers.Done(); results <- add(index) }(i)
	}
	workers.Wait()
	close(results)
	successful := 0
	for err := range results {
		if err == nil {
			successful++
		}
	}
	require.Equal(t, 1, successful, "only one concurrent registration may take the fifth slot")
	peers, err := manager.Store.GetAccountPeers(context.Background(), "", account.Id, "", "")
	require.NoError(t, err)
	require.Len(t, peers, 5, "the limit counts registered devices, including offline devices")
}

func TestAheadscaleSocialAccountIsolation(t *testing.T) {
	t.Setenv("AS_SELF_REGISTRATION", "true")
	manager, _, err := createManagerWithEmbeddedIdPMode(t, "")
	require.NoError(t, err)
	accounts := make(map[string]bool)
	for _, provider := range []string{"local", "vkid", "mailru", "ok", "yandex"} {
		claims := auth.UserAuth{UserId: dex.EncodeDexUserID("same-provider-id", provider), Email: "same@example.test", Name: provider}
		account, err := manager.GetOrCreateAccountByUser(context.Background(), claims)
		require.NoError(t, err)
		require.False(t, accounts[account.Id], "matching email must not link different provider identities")
		accounts[account.Id] = true
		again, err := manager.GetOrCreateAccountByUser(context.Background(), claims)
		require.NoError(t, err)
		require.Equal(t, account.Id, again.Id)
	}
	require.Len(t, accounts, 5)
}
