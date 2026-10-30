//go:build linux && !android

package main

import (
	"context"
	"fmt"
	"testing"

	"github.com/netbirdio/netbird/client/proto"
	"google.golang.org/grpc"
)

// The daemon rejects an empty profile handle even when profiles are disabled.
type defaultProfileDaemon struct{ proto.DaemonServiceClient }

func (defaultProfileDaemon) GetConfig(_ context.Context, request *proto.GetConfigRequest, _ ...grpc.CallOption) (*proto.GetConfigResponse, error) {
	if request.ProfileName != "default" || request.Username != "" {
		return nil, fmt.Errorf("resolve profile: profile handle is empty or foreign")
	}
	return &proto.GetConfigResponse{InterfaceName: "Aheadscale", ManagementUrl: "https://aheadscale.ru:443"}, nil
}

func TestControllerReadsExistingDefaultProfile(t *testing.T) {
	config, err := daemonConfig(context.Background(), defaultProfileDaemon{})
	if err != nil {
		t.Fatal(err)
	}
	if config.InterfaceName != "Aheadscale" || config.ManagementUrl != "https://aheadscale.ru:443" {
		t.Fatal("controller did not read the existing default registration")
	}
}
