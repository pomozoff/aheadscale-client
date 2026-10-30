//go:build linux && !android

package cmd

import (
	"context"
	"github.com/netbirdio/netbird/client/proto"
	"google.golang.org/grpc"
	"testing"
)

type nativeRouteDaemon struct {
	proto.DaemonServiceClient
	enabled bool
}

func (d *nativeRouteDaemon) GetFeatures(context.Context, *proto.GetFeaturesRequest, ...grpc.CallOption) (*proto.GetFeaturesResponse, error) {
	return &proto.GetFeaturesResponse{NativeNetworkmanager: d.enabled}, nil
}

type nativeRouteManager struct{ up, down int }

func (m *nativeRouteManager) Connect(context.Context) error { m.up++; return nil }
func (m *nativeRouteManager) SetConnected(_ context.Context, on bool) error {
	if on {
		panic("must wait for connection")
	}
	m.down++
	return nil
}
func TestDesktopCLIUsesSystemVPNOnlyWhenInstalled(t *testing.T) {
	d := &nativeRouteDaemon{}
	m := &nativeRouteManager{}
	factory := func() (desktopVPNManager, error) { return m, nil }
	for _, native := range []bool{false, true} {
		d.enabled = native
		for _, on := range []bool{true, false} {
			handled, err := routeDesktopVPN(context.Background(), d, factory, on)
			if err != nil || handled != native {
				t.Fatalf("route=%v %v", handled, err)
			}
		}
	}
	if m.up != 1 || m.down != 1 {
		t.Fatalf("wrong route %+v", m)
	}
}
