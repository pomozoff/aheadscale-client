//go:build linux && !android

package linuxvpn

import (
	"github.com/godbus/dbus/v5"
	"testing"
"strings"
)

func TestNativeNMStopFiltersOwnerPathAndReason(t *testing.T) {
	path := dbus.ObjectPath("/active/aheadscale")
	signal := &dbus.Signal{Sender: ":1.2", Path: path, Name: activeInterface + ".StateChanged", Body: []interface{}{uint32(3), uint32(2)}}
	event, ok := parseNMStop(signal, ":1.2", path, 7)
	if !ok || !event.UserRequested || event.Generation != 7 {
		t.Fatal("lost explicit user stop")
	}
	for _, other := range []*dbus.Signal{
		{Sender: ":1.1", Path: path, Name: signal.Name, Body: signal.Body},
		{Sender: ":1.2", Path: "/active/foreign", Name: signal.Name, Body: signal.Body},
		{Sender: ":1.2", Path: path, Name: signal.Name, Body: []interface{}{uint32(3), uint32(0)}},
		{Sender: ":1.2", Path: path, Name: signal.Name, Body: []interface{}{uint32(2), uint32(2)}},
	} {
		if _, ok := parseNMStop(other, ":1.2", path, 7); ok {
			t.Fatal("accepted unrelated stop")
		}
	}
	signal.Body = []interface{}{uint32(4), uint32(1)}
	event, ok = parseNMStop(signal, ":1.2", path, 7)
	if !ok || event.UserRequested {
		t.Fatal("service failure became explicit OFF")
	}
}

func TestActivationFailureDoesNotExposeSSOBanner(t *testing.T) {
 for _, banner := range []string{
  "Aheadscale SSO login required. Open: https://aheadscale.ru/oauth2/auth?code=private-test-code",
  "unexpected https://example.invalid/?token=private-test-code",
 } {
  err:=activationFailure(banner)
  if err==nil { t.Fatal("failed activation reported success") }
  if strings.Contains(err.Error(),"https://") || strings.Contains(err.Error(),"private-test-code") { t.Fatal("internal authentication details exposed to UI") }
 }
}
