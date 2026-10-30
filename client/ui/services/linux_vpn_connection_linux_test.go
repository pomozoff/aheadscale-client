//go:build linux && !android

package services

import (
	"context"
	"errors"
	"github.com/netbirdio/netbird/client/proto"
	"google.golang.org/grpc"
	"testing"
)

type gatedNativeVPN struct {
	enabled       bool
	err           error
	calls, logins int
}

func (v *gatedNativeVPN) Enabled(context.Context) (bool, error)    { return v.enabled, v.err }
func (v *gatedNativeVPN) SetConnected(context.Context, bool) error { v.calls++; return nil }
func (v *gatedNativeVPN) Login(context.Context, LoginParams) (LoginResult, error) {
	v.logins++
	return LoginResult{ProfileID: "default"}, nil
}

type routingDaemon struct {
	proto.DaemonServiceClient
	ups, downs int
}

func (d *routingDaemon) Up(context.Context, *proto.UpRequest, ...grpc.CallOption) (*proto.UpResponse, error) {
	d.ups++
	return &proto.UpResponse{}, nil
}
func (d *routingDaemon) Down(context.Context, *proto.DownRequest, ...grpc.CallOption) (*proto.DownResponse, error) {
	d.downs++
	return &proto.DownResponse{}, nil
}
func TestLinuxNativeCapabilityRoutesEveryRequest(t *testing.T) {
	d := &routingDaemon{}
	v := &gatedNativeVPN{}
	c := &Connection{conn: stubConn{client: d}, nativeVPN: v}
	if err := c.Up(context.Background(), UpParams{}); err != nil {
		t.Fatal(err)
	}
	if err := c.Down(context.Background()); err != nil {
		t.Fatal(err)
	}
	if d.ups != 1 || d.downs != 1 || v.calls != 0 {
		t.Fatalf("standalone sent to NM: %+v %+v", d, v)
	}
	v.enabled = true
	if err := c.Up(context.Background(), UpParams{}); err != nil {
		t.Fatal(err)
	}
	if err := c.Down(context.Background()); err != nil {
		t.Fatal(err)
	}
	if d.ups != 1 || d.downs != 1 || v.calls != 2 {
		t.Fatal("installed capability did not change routing")
	}
	v.err = errors.New("daemon unavailable")
	if err := c.Up(context.Background(), UpParams{}); !errors.Is(err, v.err) {
		t.Fatalf("silently fell back: %v", err)
	}
}
func TestLinuxNativeLoginUsesOneSystemActivation(t *testing.T) {
	v := &gatedNativeVPN{enabled: true}
	c := &Connection{nativeVPN: v}
	r, err := c.Login(context.Background(), LoginParams{})
	if err != nil || r.NeedsSSOLogin || r.ProfileID != "default" || v.calls != 0 || v.logins != 1 {
		t.Fatalf("extra daemon flow: %+v %v %+v", r, err, v)
	}
}
