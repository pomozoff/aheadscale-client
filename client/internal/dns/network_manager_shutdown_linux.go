//go:build linux && !android

package dns

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/godbus/dbus/v5"
	log "github.com/sirupsen/logrus"
)

const (
	networkManagerActiveInterface              = "org.freedesktop.NetworkManager.Connection.Active"
	networkManagerExternalConnection    uint32 = 128
	networkManagerConnectionDeactivated uint32 = 4
)

// PrepareInterfaceRemoval notifies NetworkManager before an external interface disappears.
func PrepareInterfaceRemoval(ifaceName string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	conn, err := dbus.ConnectSystemBus(dbus.WithContext(ctx))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("connect NetworkManager bus: %w", err)
	}
	defer closeNetworkManagerBus(conn)

	manager := conn.Object(networkManagerDest, networkManagerDbusObjectNode)
	var device dbus.ObjectPath
	if err := manager.CallWithContext(ctx, networkManagerDbusGetDeviceByIPIfaceMethod, 0, ifaceName).Store(&device); err != nil {
		if isMissingNetworkManagerConnection(err) {
			return nil
		}
		return fmt.Errorf("find NetworkManager interface: %w", err)
	}
	properties, err := networkManagerShutdownProperties(ctx, conn, device, networkManagerDbusDeviceInterface)
	if err != nil {
		return err
	}
	if properties["Interface"].Value() != ifaceName {
		return fmt.Errorf("network manager interface identity mismatch")
	}
	active, ok := properties["ActiveConnection"].Value().(dbus.ObjectPath)
	if !ok || !active.IsValid() {
		return fmt.Errorf("invalid NetworkManager active connection")
	}
	if active == "/" {
		return nil
	}
	properties, err = networkManagerShutdownProperties(ctx, conn, active, networkManagerActiveInterface)
	if err != nil {
		return err
	}
	if !isOwnExternalWireGuardConnection(properties, device) {
		return nil
	}

	// GNOME treats removal without USER_DISCONNECTED as a failed VPN connection.
	if err := manager.CallWithContext(ctx, networkManagerDest+".DeactivateConnection", 0, active).Store(); err != nil {
		if isMissingNetworkManagerConnection(err) {
			return nil
		}
		return fmt.Errorf("deactivate NetworkManager connection: %w", err)
	}
	return waitForNetworkManagerDeactivation(ctx, conn, active)
}

func isOwnExternalWireGuardConnection(properties map[string]dbus.Variant, device dbus.ObjectPath) bool {
	kind, _ := properties["Type"].Value().(string)
	flags, _ := properties["StateFlags"].Value().(uint32)
	devices, _ := properties["Devices"].Value().([]dbus.ObjectPath)
	return kind == "wireguard" && flags&networkManagerExternalConnection != 0 &&
		len(devices) == 1 && devices[0] == device
}

func networkManagerShutdownProperties(ctx context.Context, conn *dbus.Conn, path dbus.ObjectPath, iface string) (map[string]dbus.Variant, error) {
	var properties map[string]dbus.Variant
	err := conn.Object(networkManagerDest, path).CallWithContext(ctx,
		"org.freedesktop.DBus.Properties.GetAll", 0, iface).Store(&properties)
	if err != nil {
		return nil, fmt.Errorf("read NetworkManager shutdown properties: %w", err)
	}
	return properties, nil
}

func waitForNetworkManagerDeactivation(ctx context.Context, conn *dbus.Conn, active dbus.ObjectPath) error {
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		properties, err := networkManagerShutdownProperties(ctx, conn, active, networkManagerActiveInterface)
		if err != nil {
			if isMissingNetworkManagerConnection(err) {
				return nil
			}
			return err
		}
		state, ok := properties["State"].Value().(uint32)
		if !ok {
			return fmt.Errorf("invalid NetworkManager connection state")
		}
		if state == networkManagerConnectionDeactivated {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait for NetworkManager deactivation: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

func isMissingNetworkManagerConnection(err error) bool {
	var busErr dbus.Error
	if !errors.As(err, &busErr) {
		return false
	}
	switch busErr.Name {
	case "org.freedesktop.DBus.Error.ServiceUnknown",
		"org.freedesktop.DBus.Error.NameHasNoOwner",
		"org.freedesktop.DBus.Error.UnknownObject",
		"org.freedesktop.DBus.Error.UnknownMethod",
		"org.freedesktop.NetworkManager.UnknownDevice",
		"org.freedesktop.NetworkManager.ConnectionNotActive":
		return true
	default:
		return false
	}
}

func closeNetworkManagerBus(conn *dbus.Conn) {
	if err := conn.Close(); err != nil {
		log.Debugf("close NetworkManager bus: %v", err)
	}
}
