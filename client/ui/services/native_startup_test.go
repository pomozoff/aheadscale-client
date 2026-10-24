//go:build !android && !ios && !freebsd && !js

package services

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/netbirdio/netbird/client/internal/windowsvpn"
	"github.com/netbirdio/netbird/client/proto"
)

func TestNativeStartupHonorsAutoConnectSetting(t *testing.T) {
	for _, disabled := range []bool{false, true} {
		calls := 0
		c := &Connection{conn: stubConn{client: &stubDaemon{stored: &proto.GetConfigResponse{DisableAutoConnect: disabled}}}, nativeVPN: windowsvpn.NewSessionController(func(_ context.Context, up bool) error {
			if up {
				calls++
			}
			return nil
		})}
		require.NoError(t, c.ResumeOnLaunch(context.Background()))
		if disabled {
			require.Zero(t, calls)
		} else {
			require.Equal(t, 1, calls)
		}
	}
}
func TestStandardStartupDoesNotTouchNativeVPN(t *testing.T) {
	require.NoError(t, (&Connection{}).ResumeOnLaunch(context.Background()))
}
