package store

import (
	"context"
	"testing"

	"github.com/netbirdio/netbird/management/server/types"
	"github.com/stretchr/testify/require"
)

func TestSqlStore_DeviceQuotaPersistence(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s, err := NewSqliteStore(ctx, dir, nil, false)
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close(ctx) })
	require.True(t, s.GetDB().Migrator().HasTable("account_device_quotas"))
	require.True(t, s.GetDB().Migrator().HasTable("device_quota_policies"))
	require.NoError(t, s.GetDB().Exec("INSERT INTO device_quota_policies (id,max_devices) VALUES (?,?)", "registration", 3).Error)
	account := &types.Account{Id: "quota-account", Settings: &types.Settings{}}
	require.NoError(t, s.SaveAccount(ctx, account))
	read := func(id string) *int64 {
		t.Helper()
		var row struct{ MaxDevices *int64 }
		result := s.GetDB().Raw("SELECT max_devices FROM account_device_quotas WHERE account_id=?", id).Scan(&row)
		require.NoError(t, result.Error)
		require.EqualValues(t, 1, result.RowsAffected)
		return row.MaxDevices
	}
	require.NotNil(t, read(account.Id))
	require.EqualValues(t, 3, *read(account.Id))
	require.NoError(t, s.GetDB().Exec("UPDATE account_device_quotas SET max_devices=NULL WHERE account_id=?", account.Id).Error)
	require.NoError(t, s.GetDB().Exec("UPDATE device_quota_policies SET max_devices=6 WHERE id=?", "registration").Error)
	require.NoError(t, s.SaveAccount(ctx, account))
	require.Nil(t, read(account.Id), "saving the account must preserve its operator quota")
	require.NoError(t, s.Close(ctx))
	s, err = NewSqliteStore(ctx, dir, nil, false)
	require.NoError(t, err)
	require.Nil(t, read(account.Id), "unlimited must survive reopening and migration")
	second := &types.Account{Id: "new-quota-account", Settings: &types.Settings{}}
	require.NoError(t, s.SaveAccount(ctx, second))
	require.EqualValues(t, 6, *read(second.Id), "new accounts inherit the current DB policy")
	require.Error(t, s.GetDB().Exec("UPDATE account_device_quotas SET max_devices=-1 WHERE account_id=?", second.Id).Error)
	require.EqualValues(t, 6, *read(second.Id))
	require.NoError(t, s.DeleteAccount(ctx, account))
	var count int64
	require.NoError(t, s.GetDB().Table("account_device_quotas").Where("account_id=?", account.Id).Count(&count).Error)
	require.Zero(t, count)
	require.EqualValues(t, 6, *read(second.Id))
}

// The operator migration creates the tables before the upgraded server starts.
func TestSqlStore_DeviceQuotaOperatorSchema(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s, err := NewSqliteStore(ctx, dir, nil, false)
	require.NoError(t, err)
	account := &types.Account{Id: "operator-schema-account", Settings: &types.Settings{}}
	require.NoError(t, s.SaveAccount(ctx, account))
	for _, statement := range []string{
		"DROP TABLE account_device_quotas",
		"DROP TABLE device_quota_policies",
		"CREATE TABLE device_quota_policies (id TEXT NOT NULL PRIMARY KEY, max_devices INTEGER, CONSTRAINT device_quota_policy_nonnegative CHECK (max_devices IS NULL OR max_devices >= 0))",
		"CREATE TABLE account_device_quotas (account_id TEXT NOT NULL PRIMARY KEY, max_devices INTEGER, CONSTRAINT account_device_quota_nonnegative CHECK (max_devices IS NULL OR max_devices >= 0))",
		"INSERT INTO device_quota_policies VALUES ('registration',7)",
		"INSERT INTO account_device_quotas VALUES ('operator-schema-account',NULL)",
	} {
		require.NoError(t, s.GetDB().Exec(statement).Error)
	}
	require.NoError(t, s.Close(ctx))
	s, err = NewSqliteStore(ctx, dir, nil, false)
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close(ctx) })
	quota, err := s.GetAccountDeviceQuota(ctx, LockingStrengthNone, account.Id)
	require.NoError(t, err)
	require.Nil(t, quota.MaxDevices)
	require.NoError(t, s.SaveAccount(ctx, &types.Account{Id: "after-operator-migration", Settings: &types.Settings{}}))
	quota, err = s.GetAccountDeviceQuota(ctx, LockingStrengthNone, "after-operator-migration")
	require.NoError(t, err)
	require.NotNil(t, quota.MaxDevices)
	require.EqualValues(t, 7, *quota.MaxDevices)
}
