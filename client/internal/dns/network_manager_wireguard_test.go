//go:build (linux && !android) || freebsd

package dns

import (
	"testing"

	"github.com/godbus/dbus/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

func TestNetworkManagerDNSReapplyPreservesLiveWireGuardTransport(t *testing.T) {
	peers := []map[string]dbus.Variant{{"public-key": dbus.MakeVariant("existing-peer")}}
	settings := networkManagerConnSettings{
		"wireguard": {
			"listen-port": dbus.MakeVariant(uint32(35010)),
			"fwmark":      dbus.MakeVariant(uint32(0)),
			"peers":       dbus.MakeVariant(peers),
		},
		"ipv4": {"dns-search": dbus.MakeVariant([]string{"aheadscale.internal"})},
	}
	live := &wgtypes.Device{ListenPort: 51830, FirewallMark: 0x1bd00}

	settings.preserveWireGuardTransport(live)

	assert.Equal(t, uint32(51830), settings["wireguard"]["listen-port"].Value(), "DNS reapply must use the live UDP port instead of a stale NetworkManager snapshot")
	assert.Equal(t, uint32(0x1bd00), settings["wireguard"]["fwmark"].Value(), "DNS reapply must preserve the live routing mark")
	assert.Equal(t, peers, settings["wireguard"]["peers"].Value(), "transport preservation must not replace peer settings")
	assert.NotContains(t, settings["wireguard"], "private-key", "DNS settings must not copy the private key")
	assert.Equal(t, []string{"aheadscale.internal"}, settings["ipv4"]["dns-search"].Value())
}

func TestNetworkManagerDNSReapplyPreservesChosenDynamicWireGuardPort(t *testing.T) {
	settings := networkManagerConnSettings{"wireguard": {}}
	settings.preserveWireGuardTransport(&wgtypes.Device{ListenPort: 45211, FirewallMark: 0})
	require.Contains(t, settings["wireguard"], "listen-port")
	assert.Equal(t, uint32(45211), settings["wireguard"]["listen-port"].Value(), "the client's actual chosen port must not be replaced by a hardcoded default")
	assert.Equal(t, uint32(0), settings["wireguard"]["fwmark"].Value())
}
