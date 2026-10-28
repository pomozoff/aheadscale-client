//go:build linux && !android

package dns

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/godbus/dbus/v5"
)

const (
	networkManagerDeviceDeactivating uint32 = 110
	networkManagerDeviceDisconnected uint32 = 30
	networkManagerUserRequested      uint32 = 39
	networkManagerWireGuardDevice    uint32 = 29
)

// WatchNetworkManagerDisconnect reports a user-requested disconnect of this
// engine's interface. The channel stays silent on cancellation or bus failure;
// nil disables monitoring when NetworkManager does not own the interface.
func WatchNetworkManagerDisconnect(ctx context.Context, ifaceName string) (<-chan struct{}, error) {
	conn, err := dbus.ConnectSystemBus(dbus.WithContext(ctx))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("connect NetworkManager disconnect bus: %w", err)
	}
	watching := false
	defer func() {
		if !watching {
			closeNetworkManagerBus(conn)
		}
	}()
	setupCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	var owner string
	if err := conn.BusObject().CallWithContext(setupCtx, "org.freedesktop.DBus.GetNameOwner", 0, networkManagerDest).Store(&owner); err != nil {
		if isMissingNetworkManagerConnection(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("resolve NetworkManager owner: %w", err)
	}
	var device dbus.ObjectPath
	if err := conn.Object(networkManagerDest, networkManagerDbusObjectNode).CallWithContext(setupCtx, networkManagerDbusGetDeviceByIPIfaceMethod, 0, ifaceName).Store(&device); err != nil {
		if isMissingNetworkManagerConnection(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("find NetworkManager disconnect device: %w", err)
	}
	signals := make(chan *dbus.Signal, 16)
	conn.Signal(signals)
	if err := conn.AddMatchSignalContext(setupCtx, dbus.WithMatchSender(owner), dbus.WithMatchObjectPath(device), dbus.WithMatchInterface(networkManagerDbusDeviceInterface), dbus.WithMatchMember("StateChanged")); err != nil {
		return nil, fmt.Errorf("subscribe to NetworkManager disconnect: %w", err)
	}
	// Subscribe before reading state, covering a user click during setup. The
	// interface was created by this engine; never monitor another device type.
	properties, err := networkManagerShutdownProperties(setupCtx, conn, device, networkManagerDbusDeviceInterface)
	if err != nil {
		return nil, err
	}
	if properties["Interface"].Value() != ifaceName || properties["DeviceType"].Value() != networkManagerWireGuardDevice {
		return nil, nil
	}
	managed, _ := properties["Managed"].Value().(bool)
	if !managed {
		return nil, nil
	}
	event := make(chan struct{}, 1)
	var reason struct {
		State  uint32
		Reason uint32
	}
	if err := dbus.Store([]interface{}{properties["StateReason"].Value()}, &reason); err != nil {
		return nil, fmt.Errorf("read NetworkManager disconnect state: %w", err)
	}
	if isNetworkManagerUserDisconnectedState(reason.State, reason.Reason) {
		event <- struct{}{}
	}
	watching = true
	go func() {
		defer closeNetworkManagerBus(conn)
		defer conn.RemoveSignal(signals)
		for {
			select {
			case <-ctx.Done():
				return
			case sig, ok := <-signals:
				if !ok {
					return
				}
				if ctx.Err() != nil {
					return
				}
				if isNetworkManagerUserDisconnect(sig, owner, device) {
					select {
					case event <- struct{}{}:
					default:
					}
					return
				}
			}
		}
	}()
	return event, nil
}

func isNetworkManagerUserDisconnectedState(state, reason uint32) bool {
	return reason == networkManagerUserRequested && (state == networkManagerDeviceDeactivating || state == networkManagerDeviceDisconnected)
}

func isNetworkManagerUserDisconnect(sig *dbus.Signal, owner string, device dbus.ObjectPath) bool {
	if sig == nil || sig.Sender != owner || sig.Path != device || sig.Name != networkManagerDbusDeviceInterface+".StateChanged" || len(sig.Body) != 3 {
		return false
	}
	state, stateOK := sig.Body[0].(uint32)
	_, oldOK := sig.Body[1].(uint32)
	reason, reasonOK := sig.Body[2].(uint32)
	return stateOK && oldOK && reasonOK && isNetworkManagerUserDisconnectedState(state, reason)
}
