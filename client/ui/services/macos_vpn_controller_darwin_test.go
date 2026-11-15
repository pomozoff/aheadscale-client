//go:build darwin && macosvpnprototype

package services

import (
 "context"
 "testing"
 "github.com/netbirdio/netbird/client/internal/macosvpn"
)
func TestMacOSFactoryUsesSystemProfile(t *testing.T) {
 if newPlatformVPNController(nil)==nil {t.Fatal("native macOS silently fell back to daemon Up")}
}
func TestMacOSStatusRequiresBothLayers(t *testing.T) {
 st:=Status{Status:StatusConnected}
 got:=reconcileMacOSStatus(st,macosvpn.ControlSnapshot{Version:1,SystemState:"disconnected",ProfileExists:true},nil)
 if got.Status==StatusConnected {t.Fatal("GUI still reports Connected after system OFF")}
 _=context.Background()
}
