// Protocol fixture compiled against Aheadscale, independently of the plugin.
package main

import (
	"context"
	"github.com/netbirdio/netbird/client/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"net"
	"os"
	"sync"
)

type daemon struct {
	proto.UnimplementedDaemonServiceServer
	mu        sync.Mutex
	connected bool
}

func (d *daemon) GetFeatures(context.Context, *proto.GetFeaturesRequest) (*proto.GetFeaturesResponse, error) {
	return &proto.GetFeaturesResponse{DisableProfiles: true, DisableUpdateSettings: true}, nil
}
func (d *daemon) GetConfig(_ context.Context, r *proto.GetConfigRequest) (*proto.GetConfigResponse, error) {
	if r.GetProfileName() != "default" {
		return nil, status.Error(codes.InvalidArgument, "default profile required")
	}
	return &proto.GetConfigResponse{ManagementUrl: "https://aheadscale.ru", AdminURL: "https://aheadscale.ru"}, nil
}
func (d *daemon) Login(_ context.Context, r *proto.LoginRequest) (*proto.LoginResponse, error) {
	if r.ProfileName != nil || r.Username != nil || !r.IsUnixDesktopClient || r.ManagementUrl != "https://aheadscale.ru" {
		return nil, status.Error(codes.InvalidArgument, "unexpected login fields")
	}
	return &proto.LoginResponse{NeedsSSOLogin: true, UserCode: "TEST-CODE", VerificationURI: "https://aheadscale.ru/oauth2/device"}, nil
}
func (d *daemon) WaitSSOLogin(_ context.Context, r *proto.WaitSSOLoginRequest) (*proto.WaitSSOLoginResponse, error) {
	if r.UserCode != "TEST-CODE" {
		return nil, status.Error(codes.InvalidArgument, "wrong user code")
	}
	return &proto.WaitSSOLoginResponse{Email: "test@example.invalid"}, nil
}
func (d *daemon) Up(_ context.Context, r *proto.UpRequest) (*proto.UpResponse, error) {
	if r.ProfileName != nil {
		return nil, status.Error(codes.InvalidArgument, "must reuse default")
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.connected = true
	return &proto.UpResponse{}, nil
}
func (d *daemon) Down(context.Context, *proto.DownRequest) (*proto.DownResponse, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.connected = false
	return &proto.DownResponse{}, nil
}
func (d *daemon) Status(context.Context, *proto.StatusRequest) (*proto.StatusResponse, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	s := "Idle"
	if d.connected {
		s = "Connected"
	}
	return &proto.StatusResponse{Status: s}, nil
}
func main() {
	l, err := net.Listen("unix", os.Args[1])
	if err != nil {
		panic(err)
	}
	defer os.Remove(os.Args[1])
	s := grpc.NewServer()
	proto.RegisterDaemonServiceServer(s, &daemon{})
	if err = s.Serve(l); err != nil {
		panic(err)
	}
}
