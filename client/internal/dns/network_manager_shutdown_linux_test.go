//go:build linux && !android

package dns

import (
	"testing"

	"github.com/godbus/dbus/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNetworkManagerShutdownOnlyOwnExternalConnection(t *testing.T) {
	device := dbus.ObjectPath("/org/freedesktop/NetworkManager/Devices/7")
	other := dbus.ObjectPath("/org/freedesktop/NetworkManager/Devices/8")
	tests := []struct {
		name    string
		kind    string
		flags   uint32
		devices []dbus.ObjectPath
		want    bool
	}{
		{"external WireGuard device", "wireguard", 220, []dbus.ObjectPath{device}, true},
		{"administrator managed profile", "wireguard", 92, []dbus.ObjectPath{device}, false},
		{"another interface", "wireguard", 220, []dbus.ObjectPath{other}, false},
		{"shared connection", "wireguard", 220, []dbus.ObjectPath{device, other}, false},
		{"physical connection", "802-3-ethernet", 220, []dbus.ObjectPath{device}, false},
		{"missing device", "wireguard", 220, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			properties := map[string]dbus.Variant{
				"Type":       dbus.MakeVariant(tt.kind),
				"StateFlags": dbus.MakeVariant(tt.flags),
				"Devices":    dbus.MakeVariant(tt.devices),
			}
			assert.Equal(t, tt.want, isOwnExternalWireGuardConnection(properties, device), "only the client's external single-device connection may be disconnected")
		})
	}
	assert.False(t, isOwnExternalWireGuardConnection(nil, device), "missing properties must not authorize a disconnect")
}

func TestNetworkManagerShutdownWithoutSystemBus(t *testing.T) {
	t.Setenv("DBUS_SYSTEM_BUS_ADDRESS", "unix:path="+t.TempDir()+"/missing-bus")
	require.NoError(t, PrepareInterfaceRemoval("as0"), "Linux without a system bus must still disconnect normally")
	release, err := PrepareNetworkManagerInterface("as0")
	require.NoError(t, err)
	if release != nil {
		require.NoError(t, release(), "Linux without NetworkManager has no policy to clean up")
	}
}
