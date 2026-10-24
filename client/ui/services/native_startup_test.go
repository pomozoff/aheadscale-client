//go:build !android && !ios && !freebsd && !js

package services

import (
	"context"
	"fmt"
	"google.golang.org/grpc"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/netbirdio/netbird/client/internal/windowsvpn"
	"github.com/netbirdio/netbird/client/proto"
)

func TestNativeStartupHonorsAutoConnectSetting(t *testing.T) {
	for _, disabled := range []bool{false, true} {
		calls := 0
		c := &Connection{conn: stubConn{client: &startupProfileDaemon{disabled: disabled}}, nativeVPN: windowsvpn.NewSessionController(func(_ context.Context, up bool) error {
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

// Настоящий сервер отвергает GetConfig без имени профиля. Stub должен
// сохранять этот контракт, иначе проверка автозапуска даёт ложный PASS.
type startupProfileDaemon struct {
	proto.DaemonServiceClient
	disabled bool
}

func (d *startupProfileDaemon) GetActiveProfile(context.Context, *proto.GetActiveProfileRequest, ...grpc.CallOption) (*proto.GetActiveProfileResponse, error) {
	return &proto.GetActiveProfileResponse{Id: "default", Username: "aheadtest"}, nil
}
func (d *startupProfileDaemon) GetConfig(_ context.Context, req *proto.GetConfigRequest, _ ...grpc.CallOption) (*proto.GetConfigResponse, error) {
	if req.ProfileName != "default" || req.Username != "aheadtest" {
		return nil, fmt.Errorf("resolve profile: profile handle is empty or belongs to another user")
	}
	return &proto.GetConfigResponse{DisableAutoConnect: d.disabled}, nil
}
