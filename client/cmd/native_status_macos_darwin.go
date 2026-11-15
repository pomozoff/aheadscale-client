//go:build darwin && macosvpnprototype

package cmd

import (
	"context"
	"github.com/netbirdio/netbird/client/internal/macosvpn"
	"github.com/netbirdio/netbird/client/proto"
	"os"
)

func platformVPNStatus(ctx context.Context, response *proto.StatusResponse) error {
	if os.Geteuid() == 0 {
		return nil
	}
	system, err := macosvpn.RunControl(ctx, "status")
	if err != nil {
		return err
	}
	if response.GetStatus() == "Connected" && (!response.GetFullStatus().GetManagementState().GetConnected() || !response.GetFullStatus().GetSignalState().GetConnected() || response.GetFullStatus().GetLocalPeerState().GetIP() == "") {
		response.Status = "Connecting"
	}
	response.Status = macosvpn.VisibleStatus(system, response.GetStatus())
	return nil
}
