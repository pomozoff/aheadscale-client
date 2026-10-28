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
	"github.com/netbirdio/netbird/management/server/store"
	"github.com/netbirdio/netbird/shared/auth"
	"github.com/netbirdio/netbird/shared/management/status"
	"github.com/stretchr/testify/require"
)

func TestAheadscaleRegistrationIsolation(t *testing.T) {
	t.Setenv("AS_SELF_REGISTRATION", "true")
	manager, _, err := createManagerWithEmbeddedIdPMode(t, "netbird.selfhosted")
	require.NoError(t, err)
	configureRegistrationDeviceQuota(t, manager, 5)
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
	configureRegistrationDeviceQuota(t, manager, 5)
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
	configureRegistrationDeviceQuota(t, manager, 5)
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

// Guards against leaking an operator quota to another account or honoring the obsolete env.
func TestAheadscaleAccountDeviceQuotaIsolation(t *testing.T) {
	t.Setenv("AS_SELF_REGISTRATION", "true")
	manager, _, err := createManagerWithEmbeddedIdPMode(t, "")
	require.NoError(t, err)
	configureRegistrationDeviceQuota(t, manager, 5)
	ctx := context.Background()
	type owner struct{ userID, accountID string }
	var owners []owner
	for _, email := range []string{"operator@example.test", "ordinary@example.test"} {
		user, err := manager.idpManager.(*idp.EmbeddedIdPManager).CreateUserWithPassword(ctx, email, "correct-horse-123", email)
		require.NoError(t, err)
		account, err := manager.GetOrCreateAccountByUser(ctx, auth.UserAuth{UserId: user.ID, Email: user.Email, Name: user.Name})
		require.NoError(t, err)
		owners = append(owners, owner{user.ID, account.Id})
	}
	require.NoError(t, manager.Store.(*store.SqlStore).GetDB().Exec("UPDATE account_device_quotas SET max_devices=NULL WHERE account_id=?", owners[0].accountID).Error)
	t.Setenv("AS_UNLIMITED_DEVICE_ACCOUNT_IDS", owners[1].accountID)
	for index, owner := range owners {
		for i := 0; i < 7; i++ {
			key, err := wgtypes.GeneratePrivateKey()
			require.NoError(t, err)
			_, _, _, _, err = manager.AddPeer(ctx, owner.accountID, "", owner.userID, &nbpeer.Peer{Key: key.PublicKey().String(), Meta: nbpeer.PeerSystemMeta{Hostname: fmt.Sprintf("device-%d", i)}}, false)
			if index == 0 || i < 5 {
				require.NoError(t, err, "only the explicitly listed account may exceed five devices")
			} else {
				quota, ok := status.FromError(err)
				require.True(t, ok)
				require.Equal(t, status.PermissionDenied, quota.Type())
				require.Contains(t, err.Error(), "5 registered devices")
			}
		}
		peers, err := manager.Store.GetAccountPeers(ctx, store.LockingStrengthNone, owner.accountID, "", "")
		require.NoError(t, err)
		if index == 0 {
			require.Len(t, peers, 7)
		} else {
			require.Len(t, peers, 5)
		}
	}
	// Lowering the database quota blocks new enrollment without deleting existing devices.
	require.NoError(t, manager.Store.(*store.SqlStore).GetDB().Exec("UPDATE account_device_quotas SET max_devices=? WHERE account_id=?", 5, owners[0].accountID).Error)
	key, err := wgtypes.GeneratePrivateKey()
	require.NoError(t, err)
	_, _, _, _, err = manager.AddPeer(ctx, owners[0].accountID, "", owners[0].userID, &nbpeer.Peer{Key: key.PublicKey().String(), Meta: nbpeer.PeerSystemMeta{Hostname: "after-removal"}}, false)
	require.ErrorContains(t, err, "5 registered devices")
	peers, err := manager.Store.GetAccountPeers(ctx, store.LockingStrengthNone, owners[0].accountID, "", "")
	require.NoError(t, err)
	require.Len(t, peers, 7, "removing the exception must not delete existing devices")
}

// A database quota must replace both the fixed free limit and the legacy env exception.
func TestAheadscaleDatabaseDeviceLimit(t *testing.T) {
	t.Setenv("AS_SELF_REGISTRATION", "true")
	manager, _, err := createManagerWithEmbeddedIdPMode(t, "")
	require.NoError(t, err)
	ctx := context.Background()
	db := manager.Store.(*store.SqlStore).GetDB()
	require.NoError(t, db.Exec("CREATE TABLE IF NOT EXISTS device_quota_policies (id TEXT NOT NULL PRIMARY KEY, max_devices INTEGER CONSTRAINT device_quota_policy_nonnegative CHECK (max_devices IS NULL OR max_devices >= 0))").Error)
	require.NoError(t, db.Exec("CREATE TABLE IF NOT EXISTS account_device_quotas (account_id TEXT NOT NULL PRIMARY KEY, max_devices INTEGER CONSTRAINT account_device_quota_nonnegative CHECK (max_devices IS NULL OR max_devices >= 0))").Error)
	require.NoError(t, db.Exec("INSERT INTO device_quota_policies (id,max_devices) VALUES (?,?)", "registration", 2).Error)
	user, err := manager.idpManager.(*idp.EmbeddedIdPManager).CreateUserWithPassword(ctx, "database-quota@example.test", "correct-horse-123", "database quota")
	require.NoError(t, err)
	account, err := manager.GetOrCreateAccountByUser(ctx, auth.UserAuth{UserId: user.ID, Email: user.Email, Name: user.Name})
	require.NoError(t, err)
	require.NoError(t, db.Exec("INSERT INTO account_device_quotas (account_id,max_devices) VALUES (?,?) ON CONFLICT(account_id) DO UPDATE SET max_devices=excluded.max_devices", account.Id, 2).Error)
	t.Setenv("AS_UNLIMITED_DEVICE_ACCOUNT_IDS", account.Id)
	add := func(index int) error {
		key, err := wgtypes.GeneratePrivateKey()
		if err != nil {
			return err
		}
		_, _, _, _, err = manager.AddPeer(ctx, account.Id, "", user.ID, &nbpeer.Peer{Key: key.PublicKey().String(), Meta: nbpeer.PeerSystemMeta{Hostname: fmt.Sprintf("database-device-%d", index)}}, false)
		return err
	}
	require.NoError(t, add(0))
	require.NoError(t, add(1))
	require.ErrorContains(t, add(2), "2 registered devices")
	require.NoError(t, db.Exec("UPDATE account_device_quotas SET max_devices=? WHERE account_id=?", 4, account.Id).Error)
	require.NoError(t, add(2))
	require.NoError(t, add(3))
	require.ErrorContains(t, add(4), "4 registered devices")
	require.NoError(t, db.Exec("UPDATE account_device_quotas SET max_devices=NULL WHERE account_id=?", account.Id).Error)
	require.NoError(t, add(4))
	require.NoError(t, add(5))
	require.NoError(t, db.Exec("UPDATE account_device_quotas SET max_devices=0 WHERE account_id=?", account.Id).Error)
	require.ErrorContains(t, add(6), "0 registered devices")
	peers, err := manager.Store.GetAccountPeers(ctx, store.LockingStrengthNone, account.Id, "", "")
	require.NoError(t, err)
	require.Len(t, peers, 6)
	require.NoError(t, db.Exec("DELETE FROM account_device_quotas WHERE account_id=?", account.Id).Error)
	require.ErrorContains(t, add(6), "device quota")
	peers, err = manager.Store.GetAccountPeers(ctx, store.LockingStrengthNone, account.Id, "", "")
	require.NoError(t, err)
	require.Len(t, peers, 6)
}

func configureRegistrationDeviceQuota(t *testing.T, manager *DefaultAccountManager, maximum int64) {
	t.Helper()
	require.NoError(t, manager.Store.(*store.SqlStore).GetDB().Exec("INSERT INTO device_quota_policies (id,max_devices) VALUES (?,?) ON CONFLICT(id) DO UPDATE SET max_devices=excluded.max_devices", "registration", maximum).Error)
}
