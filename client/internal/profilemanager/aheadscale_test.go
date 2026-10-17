package profilemanager

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/netbirdio/netbird/client/mdm"
)

func TestAheadscaleNewProfileDefaults(t *testing.T) {
	config, err := UpdateOrCreateConfig(ConfigInput{ConfigPath: filepath.Join(t.TempDir(), "config.json")})
	require.NoError(t, err)
	require.Equal(t, "https://aheadscale.ru:443", config.ManagementURL.String())
	require.Equal(t, "https://aheadscale.ru:443", config.AdminURL.String())
	require.False(t, config.DisableAutoConnect)
	require.Equal(t, 51830, config.WgPort)
	require.NotContains(t, []string{"wt0", "utun100"}, config.WgIface)
}

func TestAheadscaleBuildRejectsOtherChannelWithoutChangingProfile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	_, err := UpdateOrCreateConfig(ConfigInput{ConfigPath: path, ManagementURL: "https://private.example.test:10443"})
	require.NoError(t, err)
	before, err := os.ReadFile(path)
	require.NoError(t, err)
	previous := LockedServerOrigin
	LockedServerOrigin = DefaultManagementURL
	t.Cleanup(func() { LockedServerOrigin = previous })
	_, err = ReadConfig(path)
	require.ErrorContains(t, err, "another Aheadscale environment")
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, before, after, "rejecting a channel must not migrate keys or registration")
}

func TestAheadscaleLockedBuildAcceptsOwnOriginAndRejectsMDMRedirect(t *testing.T) {
	previous := LockedServerOrigin
	LockedServerOrigin = DefaultManagementURL
	t.Cleanup(func() { LockedServerOrigin = previous })
	config, err := CreateInMemoryConfig(ConfigInput{})
	require.NoError(t, err)
	config.ApplyMDMPolicy(mdm.NewPolicy(map[string]any{
		mdm.KeyManagementURL: "https://other.example.test:10443",
	}))
	require.Equal(t, DefaultManagementURL, config.ManagementURL.String())
}
