//go:build windows && windowsvpnprototype

package iface

import (
	"github.com/netbirdio/netbird/client/iface/device"
	"testing"
)

func TestWindowsVPNPrototypeSelectsNativeChannel(t *testing.T) {
	iface, err := NewWGIFace(WGIFaceOpts{MTU: 1280})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := iface.tun.(*device.VpnTunDevice); !ok {
		t.Fatalf("prototype selected %T; a second Wintun must never be created", iface.tun)
	}
}

func TestWindowsVPNGUIDBeforeChannelReturnsError(t *testing.T) {
	iface, err := NewWGIFace(WGIFaceOpts{MTU: 1280})
	if err != nil {
		t.Fatal(err)
	}
	guid, err := iface.GetInterfaceGUIDString()
	if err == nil || guid != "" {
		t.Fatalf("uninitialized native channel returned GUID %q, error %v", guid, err)
	}
}
