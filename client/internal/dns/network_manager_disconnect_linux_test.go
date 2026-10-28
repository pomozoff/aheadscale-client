//go:build linux && !android

package dns

import (
	"context"
	"testing"

	"github.com/godbus/dbus/v5"
	"github.com/stretchr/testify/require"
)

func TestNetworkManagerDisconnectFilters(t *testing.T) {
	device := dbus.ObjectPath("/org/freedesktop/NetworkManager/Devices/7")
	for _, tc := range []struct {
		name   string
		sender string
		path   dbus.ObjectPath
		member string
		body   []interface{}
		want   bool
	}{
		{"user disconnect", ":1.7", device, networkManagerDbusDeviceInterface + ".StateChanged", []interface{}{uint32(110), uint32(100), uint32(39)}, true},
		{"completed user disconnect", ":1.7", device, networkManagerDbusDeviceInterface + ".StateChanged", []interface{}{uint32(30), uint32(110), uint32(39)}, true},
		{"another sender", ":1.8", device, networkManagerDbusDeviceInterface + ".StateChanged", []interface{}{uint32(110), uint32(100), uint32(39)}, false},
		{"another interface", ":1.7", "/org/freedesktop/NetworkManager/Devices/8", networkManagerDbusDeviceInterface + ".StateChanged", []interface{}{uint32(110), uint32(100), uint32(39)}, false},
		{"network sleep", ":1.7", device, networkManagerDbusDeviceInterface + ".StateChanged", []interface{}{uint32(30), uint32(100), uint32(37)}, false},
		{"failure", ":1.7", device, networkManagerDbusDeviceInterface + ".StateChanged", []interface{}{uint32(120), uint32(100), uint32(4)}, false},
		{"activation", ":1.7", device, networkManagerDbusDeviceInterface + ".StateChanged", []interface{}{uint32(100), uint32(90), uint32(39)}, false},
		{"different member", ":1.7", device, "other.StateChanged", []interface{}{uint32(110), uint32(100), uint32(39)}, false},
		{"invalid body", ":1.7", device, networkManagerDbusDeviceInterface + ".StateChanged", []interface{}{"110", uint32(100), uint32(39)}, false},
		{"short body", ":1.7", device, networkManagerDbusDeviceInterface + ".StateChanged", []interface{}{uint32(110)}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sig := &dbus.Signal{Sender: tc.sender, Path: tc.path, Name: tc.member, Body: tc.body}
			require.Equal(t, tc.want, isNetworkManagerUserDisconnect(sig, ":1.7", device))
		})
	}
	require.False(t, isNetworkManagerUserDisconnect(nil, ":1.7", device))
}

func TestNetworkManagerDisconnectWithoutSystemBus(t *testing.T) {
	t.Setenv("DBUS_SYSTEM_BUS_ADDRESS", "unix:path="+t.TempDir()+"/missing-bus")
	event, err := WatchNetworkManagerDisconnect(context.Background(), "as0")
	require.NoError(t, err)
	require.Nil(t, event, "headless Linux must keep working without NetworkManager")
}
