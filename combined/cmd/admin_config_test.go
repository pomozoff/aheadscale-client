package cmd

import (
	"context"
	"os"
	"testing"

	"github.com/netbirdio/netbird/idp/dex"
	"github.com/stretchr/testify/require"

	nbconfig "github.com/netbirdio/netbird/management/internals/server/config"
)

func TestApplyAdminDefaultsCopiesServerStoreWithoutExposedAddress(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Server.ExposedAddress = ""
	cfg.Server.DataDir = "/srv/netbird"
	cfg.Server.Store = StoreConfig{
		Engine: "postgres",
		DSN:    "postgres://user:pass@example.com/netbird",
	}

	cfg.ApplyAdminDefaults()

	require.Equal(t, "/srv/netbird", cfg.Management.DataDir)
	require.Equal(t, "postgres", cfg.Management.Store.Engine)
	require.Equal(t, cfg.Server.Store.DSN, cfg.Management.Store.DSN)
}

func TestOpenAdminEventStoreMissingEncryptionKeyReturnsNilInterface(t *testing.T) {
	eventStore, err := openAdminEventStore(context.Background(), &CombinedConfig{}, &nbconfig.Config{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "encryption key")
	require.Nil(t, eventStore)
}

func TestApplyServerStoreEnv(t *testing.T) {
	t.Setenv("NB_STORE_ENGINE_POSTGRES_DSN", "")
	t.Setenv("NB_STORE_ENGINE_MYSQL_DSN", "")
	t.Setenv("NB_STORE_ENGINE_SQLITE_FILE", "")

	applyServerStoreEnv(StoreConfig{Engine: "postgres", DSN: "postgres-dsn", File: "store.db"})
	require.Equal(t, "postgres-dsn", os.Getenv("NB_STORE_ENGINE_POSTGRES_DSN"))
	require.Equal(t, "store.db", os.Getenv("NB_STORE_ENGINE_SQLITE_FILE"))

	applyServerStoreEnv(StoreConfig{Engine: "mysql", DSN: "mysql-dsn"})
	require.Equal(t, "mysql-dsn", os.Getenv("NB_STORE_ENGINE_MYSQL_DSN"))
}

func TestAheadscaleStaticConnectors(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Management.Auth.Issuer = "https://aheadscale.example/oauth2"
	cfg.Management.Auth.Connectors = []dex.Connector{{Type: "aheadscale-vkid", ID: "vkid", Name: "VK ID", Config: map[string]interface{}{"clientID": "123"}}}
	embedded, err := cfg.buildEmbeddedIdPConfig(cfg.Management)
	require.NoError(t, err)
	require.Equal(t, cfg.Management.Auth.Connectors, embedded.StaticConnectors)
	cfg.Management.Auth.Connectors = nil
	embedded, err = cfg.buildEmbeddedIdPConfig(cfg.Management)
	require.NoError(t, err)
	require.Empty(t, embedded.StaticConnectors)
}
