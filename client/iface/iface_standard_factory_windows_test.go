//go:build windows && !windowsvpnprototype

package iface

import (
	"github.com/netbirdio/netbird/client/iface/device"
	"testing"
)

func TestWindowsStandardKeepsWintunBackend(t *testing.T) {
	iface, err := NewWGIFace(WGIFaceOpts{MTU: 1280})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := iface.tun.(*device.TunDevice); !ok {
		t.Fatalf("standard build selected %T instead of the existing Wintun backend", iface.tun)
	}
}
