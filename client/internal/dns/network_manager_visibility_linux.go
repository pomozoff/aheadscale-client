//go:build linux && !android

package dns

import (
	"context"
	"fmt"
	"time"

	"github.com/godbus/dbus/v5"
)

// ConfigureNetworkManagerInterface waits for desktop network visibility before applying DNS.
func ConfigureNetworkManagerInterface(ifaceName string) error {
	return ConfigureNetworkManagerInterfaceForMode(ifaceName, false)
}
func ConfigureNetworkManagerInterfaceForMode(ifaceName string, native bool) error {
	if native {
		return nil
	}
	running, err := networkManagerIsRunning()
	if err != nil {
		return err
	}
	if !running {
		return nil
	}
	managerType, _, err := getOSDNSManagerType()
	if err != nil {
		return err
	}
	if managerType != networkManager {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := dbus.ConnectSystemBus(dbus.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("connect NetworkManager visibility bus: %w", err)
	}
	defer closeNetworkManagerBus(conn)
	return waitForNetworkManagerExternalConnection(ctx, conn, ifaceName)
}

func waitForNetworkManagerExternalConnection(ctx context.Context, conn *dbus.Conn, ifaceName string) error {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		ready, err := networkManagerExternalConnectionReady(ctx, conn, ifaceName)
		if err != nil {
			return err
		}
		if ready {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait for NetworkManager external connection: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

func networkManagerExternalConnectionReady(ctx context.Context, conn *dbus.Conn, ifaceName string) (bool, error) {
	manager := conn.Object(networkManagerDest, networkManagerDbusObjectNode)
	var device dbus.ObjectPath
	if err := manager.CallWithContext(ctx, networkManagerDbusGetDeviceByIPIfaceMethod, 0, ifaceName).Store(&device); err != nil {
		if isMissingNetworkManagerConnection(err) {
			return false, nil
		}
		return false, fmt.Errorf("find NetworkManager visible device: %w", err)
	}
	properties, err := networkManagerShutdownProperties(ctx, conn, device, networkManagerDbusDeviceInterface)
	if err != nil {
		return false, err
	}
	if properties["Interface"].Value() != ifaceName {
		return false, fmt.Errorf("NetworkManager interface identity mismatch")
	}
	active, ok := properties["ActiveConnection"].Value().(dbus.ObjectPath)
	if !ok || active == "/" {
		return false, nil
	}
	properties, err = networkManagerShutdownProperties(ctx, conn, active, networkManagerActiveInterface)
	if err != nil {
		return false, err
	}
	state, _ := properties["State"].Value().(uint32)
	if state != 2 {
		return false, nil
	}
	return true, nil
}
