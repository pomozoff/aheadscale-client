//go:build linux && !android

package linuxvpn

import (
	"github.com/godbus/dbus/v5"
	"testing"
)

func TestOwnedProfileIsStable(t *testing.T) {
	uuid := "aabbccdd-1111-4222-8333-001122334455"
	settings := profileSettings(uuid, "https://aheadscale.ru", "https://aheadscale.ru")
	if err := validateProfile(settings, uuid); err != nil {
		t.Fatal(err)
	}
	if settings["connection"]["autoconnect"].Value() != false {
		t.Fatal("NM can bypass controller autoconnect policy")
	}
	if settings["vpn"]["service-type"].Value() != ServiceType {
		t.Fatal("wrong service type")
	}
	if settings["connection"]["uuid"].Value() != uuid {
		t.Fatal("replaced existing UUID")
	}
	for _, group := range settings {
		if _, ok := group["private-key"]; ok {
			t.Fatal("copied a private key")
		}
	}
	settings["connection"]["uuid"] = dbus.MakeVariant("foreign-uuid")
	if err := validateProfile(settings, uuid); err == nil {
		t.Fatal("accepted foreign profile")
	}
	settings = profileSettings(uuid, "https://aheadscale.ru", "https://aheadscale.ru")
	settings["vpn"]["service-type"] = dbus.MakeVariant("org.freedesktop.NetworkManager.netbird")
	if err := validateProfile(settings, uuid); err == nil {
		t.Fatal("accepted original NetBird profile")
	}
}
