//go:build linux && !android

package dns

import (
	"bufio"
	"context"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/stretchr/testify/require"
)

type rebindManager struct{ path dbus.ObjectPath }

func (m *rebindManager) GetDeviceByIpIface(name string) (dbus.ObjectPath, *dbus.Error) {
	if name != "Aheadscale" {
		return "", dbus.MakeFailedError(context.Canceled)
	}
	return m.path, nil
}

type rebindProperties struct{ queried chan struct{} }

func (p *rebindProperties) GetAll(_ string) (map[string]dbus.Variant, *dbus.Error) {
	select {
	case p.queried <- struct{}{}:
	default:
	}
	return map[string]dbus.Variant{"Interface": dbus.MakeVariant("Aheadscale"), "DeviceType": dbus.MakeVariant(uint32(29)), "Managed": dbus.MakeVariant(true), "StateReason": dbus.MakeVariant(struct{ State, Reason uint32 }{100, 0})}, nil
}
func startRebindBus(t *testing.T) string {
	t.Helper()
	cmd := exec.Command("dbus-daemon", "--session", "--nofork", "--print-address=1", "--address=unix:path="+filepath.Join(t.TempDir(), "bus"))
	out, err := cmd.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, cmd.Start())
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	scanner := bufio.NewScanner(out)
	require.True(t, scanner.Scan())
	return scanner.Text()
}
func exportRebindManager(t *testing.T, address string, path dbus.ObjectPath) (*dbus.Conn, <-chan struct{}) {
	t.Helper()
	c, err := dbus.Connect(address)
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })
	p := &rebindProperties{queried: make(chan struct{}, 16)}
	require.NoError(t, c.Export(&rebindManager{path: path}, networkManagerDbusObjectNode, networkManagerDest))
	require.NoError(t, c.Export(p, path, "org.freedesktop.DBus.Properties"))
	reply, err := c.RequestName(networkManagerDest, dbus.NameFlagDoNotQueue)
	require.NoError(t, err)
	require.Equal(t, dbus.RequestNameReplyPrimaryOwner, reply)
	return c, p.queried
}
func TestNetworkManagerDisconnectRebind(t *testing.T) {
	address := startRebindBus(t)
	t.Setenv("DBUS_SYSTEM_BUS_ADDRESS", address)
	a, pathA := dbus.ObjectPath("/org/freedesktop/NetworkManager/Devices/7"), dbus.ObjectPath("/org/freedesktop/NetworkManager/Devices/8")
	ownerA, _ := exportRebindManager(t, address, a)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	event, err := WatchNetworkManagerDisconnect(ctx, "Aheadscale")
	require.NoError(t, err)
	require.NotNil(t, event)
	_, err = ownerA.ReleaseName(networkManagerDest)
	require.NoError(t, err)
	ownerB, queried := exportRebindManager(t, address, pathA)
	select {
	case <-queried:
	case <-time.After(3 * time.Second):
		t.Fatal("watcher did not rebind to replacement NetworkManager")
	}
	// A valid-looking message from the old owner must not turn the tunnel off.
	require.NoError(t, ownerA.Emit(a, networkManagerDbusDeviceInterface+".StateChanged", uint32(110), uint32(100), uint32(39)))
	require.NoError(t, ownerB.Emit(a, networkManagerDbusDeviceInterface+".StateChanged", uint32(110), uint32(100), uint32(39)))
	select {
	case <-event:
		t.Fatal("accepted old owner or old device path")
	case <-time.After(50 * time.Millisecond):
	}
	require.NoError(t, ownerB.Emit(pathA, networkManagerDbusDeviceInterface+".StateChanged", uint32(110), uint32(100), uint32(39)))
	select {
	case <-event:
	case <-time.After(3 * time.Second):
		t.Fatal("replacement owner's user disconnect was lost")
	}
}
