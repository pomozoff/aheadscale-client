//go:build !darwin || !macosvpnprototype

package cmd

import (
	"context"
	"github.com/netbirdio/netbird/client/proto"
)

func platformVPNStatus(_ context.Context, _ *proto.StatusResponse) error { return nil }
