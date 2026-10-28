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
	signals := make(chan *dbus.Signal, 32)
	conn.Signal(signals)
	// Subscribe to the well-known sender: the bus follows its changing owner.
	// The consumer additionally checks the current unique owner and device path.
	setupCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	matches := [][]dbus.MatchOption{
		{dbus.WithMatchSender(networkManagerDest), dbus.WithMatchInterface(networkManagerDbusDeviceInterface), dbus.WithMatchMember("StateChanged")},
		{dbus.WithMatchSender("org.freedesktop.DBus"), dbus.WithMatchInterface("org.freedesktop.DBus"), dbus.WithMatchMember("NameOwnerChanged"), dbus.WithMatchArg(0, networkManagerDest)},
		{dbus.WithMatchSender(networkManagerDest), dbus.WithMatchObjectPath(networkManagerDbusObjectNode), dbus.WithMatchInterface(networkManagerDest), dbus.WithMatchMember("DeviceAdded")},
		{dbus.WithMatchSender(networkManagerDest), dbus.WithMatchObjectPath(networkManagerDbusObjectNode), dbus.WithMatchInterface(networkManagerDest), dbus.WithMatchMember("DeviceRemoved")},
	}
	for _, match := range matches {
		if err := conn.AddMatchSignalContext(setupCtx, match...); err != nil {
			return nil, fmt.Errorf("subscribe to NetworkManager disconnect: %w", err)
		}
	}
	owner, device, disconnected, err := networkManagerDisconnectBinding(setupCtx, conn, ifaceName)
	if err != nil {
		return nil, err
	}
	if owner == "" || device == "" {
		return nil, nil
	}
	event := make(chan struct{}, 1)
	if disconnected {
		event <- struct{}{}
	}
	watching = true
	go func() {
		defer closeNetworkManagerBus(conn)
		defer conn.RemoveSignal(signals)
		// A restarted NM can publish its name before discovering our interface.
		// Rebind on device events, with a bounded retry for that discovery window.
		retry := time.NewTicker(time.Second)
		defer retry.Stop()
		rebind := func() bool {
			bindCtx, stop := context.WithTimeout(ctx, 2*time.Second)
			defer stop()
			nextOwner, nextDevice, off, bindErr := networkManagerDisconnectBinding(bindCtx, conn, ifaceName)
			if bindErr != nil {
				owner, device = "", ""
				return false
			}
			owner, device = nextOwner, nextDevice
			return off
		}
		notify := func() {
			select {
			case event <- struct{}{}:
			default:
			}
		}
		for {
			select {
			case <-ctx.Done():
				return
			case <-retry.C:
				if device == "" && rebind() && ctx.Err() == nil {
					notify()
					return
				}
			case sig, ok := <-signals:
				if !ok || ctx.Err() != nil {
					return
				}
				if sig == nil {
					continue
				}
				ownerChanged := sig.Sender == "org.freedesktop.DBus" && sig.Name == "org.freedesktop.DBus.NameOwnerChanged" && len(sig.Body) == 3 && sig.Body[0] == networkManagerDest
				deviceChanged := sig.Sender == owner && sig.Path == networkManagerDbusObjectNode && (sig.Name == networkManagerDest+".DeviceAdded" || sig.Name == networkManagerDest+".DeviceRemoved")
				if ownerChanged || deviceChanged {
					if rebind() && ctx.Err() == nil {
						notify()
						return
					}
					continue
				}
				if isNetworkManagerUserDisconnect(sig, owner, device) {
					notify()
					return
				}
			}
		}
	}()
	return event, nil
}

// Read through the unique owner so properties can never come from a replacement
// service at a recycled device path. Recheck ownership before trusting the result.
func networkManagerDisconnectBinding(ctx context.Context, conn *dbus.Conn, ifaceName string) (string, dbus.ObjectPath, bool, error) {
	var owner string
	if err := conn.BusObject().CallWithContext(ctx, "org.freedesktop.DBus.GetNameOwner", 0, networkManagerDest).Store(&owner); err != nil {
		if isMissingNetworkManagerConnection(err) {
			return "", "", false, nil
		}
		return "", "", false, err
	}
	var device dbus.ObjectPath
	if err := conn.Object(owner, networkManagerDbusObjectNode).CallWithContext(ctx, networkManagerDbusGetDeviceByIPIfaceMethod, 0, ifaceName).Store(&device); err != nil {
		if isMissingNetworkManagerConnection(err) {
			return owner, "", false, nil
		}
		return "", "", false, err
	}
	var properties map[string]dbus.Variant
	if err := conn.Object(owner, device).CallWithContext(ctx, "org.freedesktop.DBus.Properties.GetAll", 0, networkManagerDbusDeviceInterface).Store(&properties); err != nil {
		return "", "", false, err
	}
	var currentOwner string
	if err := conn.BusObject().CallWithContext(ctx, "org.freedesktop.DBus.GetNameOwner", 0, networkManagerDest).Store(&currentOwner); err != nil {
		return "", "", false, err
	}
	if currentOwner != owner {
		return "", "", false, nil
	}
	if properties["Interface"].Value() != ifaceName || properties["DeviceType"].Value() != networkManagerWireGuardDevice {
		return owner, "", false, nil
	}
	if managed, _ := properties["Managed"].Value().(bool); !managed {
		return owner, "", false, nil
	}
	var reason struct{ State, Reason uint32 }
	if err := dbus.Store([]interface{}{properties["StateReason"].Value()}, &reason); err != nil {
		return "", "", false, fmt.Errorf("read NetworkManager disconnect state: %w", err)
	}
	return owner, device, isNetworkManagerUserDisconnectedState(reason.State, reason.Reason), nil
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
