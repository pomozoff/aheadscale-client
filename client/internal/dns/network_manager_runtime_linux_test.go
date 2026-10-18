//go:build linux && !android

package dns

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNetworkManagerRuntimeRuleLifecycle(t *testing.T) {
	dir := t.TempDir()
	release, changed, err := networkManagerRuntimeRule(dir, "as0", true)
	require.NoError(t, err)
	require.True(t, changed, "a fresh connection installs its own runtime policy")
	path := filepath.Join(dir, "90-aheadscale-as0.conf")
	_, err = os.Stat(path)
	require.NoError(t, err)
	require.NoError(t, release())
	_, err = os.Stat(path)
	require.ErrorIs(t, err, os.ErrNotExist)
	require.NoError(t, release(), "cleanup must be idempotent")
}

func TestNetworkManagerRuntimeRulePreservesAdministratorChanges(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "90-aheadscale-as0.conf")
	original := []byte("administrator policy\n")
	require.NoError(t, os.WriteFile(path, original, 0600))
	_, _, err := networkManagerRuntimeRule(dir, "as0", true)
	require.Error(t, err)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, original, data, "administrator policy must not be overwritten")
}

func TestNetworkManagerRuntimeRuleRemovesStalePolicyForNetworkManagerDNS(t *testing.T) {
	dir := t.TempDir()
	_, _, err := networkManagerRuntimeRule(dir, "as0", true)
	require.NoError(t, err)
	release, changed, err := networkManagerRuntimeRule(dir, "as0", false)
	require.NoError(t, err)
	assert.Nil(t, release, "NetworkManager DNS retains interface management")
	assert.True(t, changed, "a stale client policy must not override the new DNS owner")
	_, err = os.Stat(filepath.Join(dir, "90-aheadscale-as0.conf"))
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestNetworkManagerRuntimeRulePreservesEditedPolicyOnCleanup(t *testing.T) {
	dir := t.TempDir()
	release, _, err := networkManagerRuntimeRule(dir, "as0", true)
	require.NoError(t, err)
	path := filepath.Join(dir, "90-aheadscale-as0.conf")
	edited := []byte("administrator replacement\n")
	require.NoError(t, os.WriteFile(path, edited, 0600))
	require.Error(t, release())
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, edited, data, "cleanup must preserve a policy replaced by the administrator")
}

func TestNetworkManagerRuntimeRuleRejectsUnsafeInterfaceName(t *testing.T) {
	_, _, err := networkManagerRuntimeRule(t.TempDir(), "as0;eth0", true)
	require.Error(t, err, "the match must never expand to another interface")
}
