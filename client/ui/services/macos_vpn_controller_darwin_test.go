//go:build darwin && macosvpnprototype

package services

import (
	"context"
	"errors"
	"github.com/netbirdio/netbird/client/internal/macosvpn"
	"testing"
	"time"
)

func TestMacOSFactoryUsesSystemProfile(t *testing.T) {
	if newPlatformVPNController(nil) == nil {
		t.Fatal("native macOS silently fell back to daemon Up")
	}
}
func TestMacOSStatusRequiresBothLayers(t *testing.T) {
	st := Status{Status: StatusConnected}
	got := reconcileMacOSStatus(st, macosvpn.ControlSnapshot{Version: 1, SystemState: "disconnected", ProfileExists: true}, nil)
	if got.Status == StatusConnected {
		t.Fatal("GUI still reports Connected after system OFF")
	}
	_ = context.Background()
}

// The real macOS controller must cancel an older GUI operation, including its
// engine-readiness wait. A late result may not show an error after user OFF.
func TestMacOSDisconnectCancelsPendingGUIConnect(t *testing.T) {
	entered := make(chan context.Context, 1)
	release := make(chan struct{})
	controller := &macosVPNController{control: func(ctx context.Context, action string) (macosvpn.ControlSnapshot, error) {
		if action == "connect" {
			entered <- ctx
			<-release
			return macosvpn.ControlSnapshot{Version: 1, SystemState: "connected", ProfileExists: true}, nil
		}
		return macosvpn.ControlSnapshot{Version: 1, SystemState: "disconnected", ProfileExists: true}, nil
	}}
	done := make(chan error, 1)
	go func() { done <- controller.SetConnected(context.Background(), true) }()
	old := <-entered
	if err := controller.SetConnected(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	select {
	case <-old.Done():
	case <-time.After(time.Second):
		t.Error("Disconnect did not cancel older GUI Connect")
	}
	if old.Err() == nil {
		return
	}
	close(release)
	// The old operation must reject its late system response before probing RPC.
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("old GUI Connect did not finish")
	}
}
