//go:build windows && windowsvpnprototype

package cmd

import (
	"context"
	"github.com/netbirdio/netbird/client/internal/ipcauth"
	"github.com/netbirdio/netbird/client/internal/windowsvpn"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func init() {
	rootCmd.PersistentFlags().StringVar(&windowsvpn.OwnerSID, "vpn-prototype-owner-sid", "", "Windows SID authorized for the isolated native VPN prototype")
}

func prototypeServerOptions() []grpc.ServerOption {
	allowed := func(ctx context.Context) bool {
		id, ok := ipcauth.CallerIdentity(ctx)
		return ok && (id.SID == windowsvpn.OwnerSID && windowsvpn.OwnerSID != "" || ipcauth.IsDaemonSelf(id))
	}
	return []grpc.ServerOption{
		grpc.UnaryInterceptor(func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
			if !allowed(ctx) {
				return nil, status.Error(codes.PermissionDenied, "Native VPN belongs to another Windows user")
			}
			return handler(ctx, req)
		}),
		grpc.StreamInterceptor(func(srv any, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
			if !allowed(stream.Context()) {
				return status.Error(codes.PermissionDenied, "Native VPN belongs to another Windows user")
			}
			return handler(srv, stream)
		}),
	}
}
