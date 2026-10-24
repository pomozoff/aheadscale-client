//go:build !android && !js

package iface

import "testing"

type platformOwnedCloseTun struct {
	WGTunDevice
	closed bool
}

func (t *platformOwnedCloseTun) Close() error              { t.closed = true; return nil }
func (*platformOwnedCloseTun) PlatformOwnsInterface() bool { return true }
func (*platformOwnedCloseTun) DeviceName() string {
	panic("backend must not inspect or destroy the platform-owned interface")
}

type platformCloseProxy struct{ wgProxyFactory }

func (platformCloseProxy) Free() error { return nil }

func TestWGIfaceCloseLeavesPlatformInterfaceToItsOwner(t *testing.T) {
	tun := &platformOwnedCloseTun{}
	iface := &WGIface{tun: tun, wgProxyFactory: platformCloseProxy{}}
	if err := iface.Close(); err != nil {
		t.Fatal(err)
	}
	if !tun.closed {
		t.Fatal("backend transport was not closed")
	}
}
