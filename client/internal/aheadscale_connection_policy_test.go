package internal

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/netbirdio/netbird/client/internal/lazyconn"
	"github.com/netbirdio/netbird/client/internal/profilemanager"
)

func TestAheadscaleConnectionsStartWithoutWaitingForRemoteTraffic(t *testing.T) {
	previous := profilemanager.LockedServerOrigin
	t.Cleanup(func() { profilemanager.LockedServerOrigin = previous })
	profilemanager.LockedServerOrigin = "https://aheadscale.ru"
	t.Setenv(lazyconn.EnvLazyConn, "")
	require.Equal(t, lazyForceOff, resolveLazyForce(lazyconn.StateUnset))
	require.Equal(t, lazyForceOn, resolveLazyForce(lazyconn.StateOn), "explicit MDM policy wins")
	t.Setenv(lazyconn.EnvLazyConn, "on")
	require.Equal(t, lazyForceOn, resolveLazyForce(lazyconn.StateOff), "explicit environment wins")
}

func TestUnlockedConnectionPolicyStillDefersToManagement(t *testing.T) {
	previous := profilemanager.LockedServerOrigin
	t.Cleanup(func() { profilemanager.LockedServerOrigin = previous })
	profilemanager.LockedServerOrigin = ""
	t.Setenv(lazyconn.EnvLazyConn, "")
	require.Equal(t, lazyForceNone, resolveLazyForce(lazyconn.StateUnset))
}
