//go:build linux && !android

package profilemanager

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAheadscaleLinuxTunnelIdentityUpgrade(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	legacy := "as0"
	original, err := UpdateOrCreateConfig(ConfigInput{ConfigPath: path, InterfaceName: &legacy})
	require.NoError(t, err)
	original.IFaceBlackList = []string{"as0", "docker", "lo"}
	require.NoError(t, WriteOutConfig(path, original))
	upgraded, err := ReadConfig(path)
	require.NoError(t, err)
	require.Equal(t, "Aheadscale", upgraded.WgIface, "Linux network menus must identify the actual Aheadscale tunnel")
	require.Contains(t, upgraded.IFaceBlackList, "Aheadscale", "ICE must not select the overlay as an underlay interface")
	require.Contains(t, upgraded.IFaceBlackList, "docker", "existing exclusions must be retained")
	require.True(t, original.PrivateKey == upgraded.PrivateKey, "an interface-name upgrade must preserve the device key")
	require.True(t, original.SSHKey == upgraded.SSHKey, "an interface-name upgrade must preserve the SSH key")
	require.Equal(t, original.ManagementURL, upgraded.ManagementURL, "the account's server must be retained")
	first, err := os.ReadFile(path)
	require.NoError(t, err)
	_, err = ReadConfig(path)
	require.NoError(t, err)
	second, err := os.ReadFile(path)
	require.NoError(t, err)
	require.True(t, bytes.Equal(first, second), "the migration must be idempotent")
}

func TestAheadscaleLinuxCustomTunnelNamePreserved(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	custom := "lab0"
	_, err := UpdateOrCreateConfig(ConfigInput{ConfigPath: path, InterfaceName: &custom})
	require.NoError(t, err)
	config, err := ReadConfig(path)
	require.NoError(t, err)
	require.Equal(t, custom, config.WgIface, "custom interface names must survive the upgrade")
}
