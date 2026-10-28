//go:build linux && !android

package linuxvpn

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
)

const nmDest = "org.freedesktop.NetworkManager"
const nmPath = dbus.ObjectPath("/org/freedesktop/NetworkManager")
const settingsPath = dbus.ObjectPath("/org/freedesktop/NetworkManager/Settings")
const activeInterface = nmDest + ".Connection.Active"

type Manager struct {
	receipt    Receipt
	trackingMu sync.Mutex
	tracked    map[sessionKey]uint64
}

func NewInstalledManager() (*Manager, error) {
	r, err := loadReceipt(ReceiptPath)
	if err != nil {
		return nil, err
	}
	return &Manager{receipt: r}, nil
}
func getAll(ctx context.Context, conn *dbus.Conn, owner string, path dbus.ObjectPath, iface string) (map[string]dbus.Variant, error) {
	var properties map[string]dbus.Variant
	err := conn.Object(owner, path).CallWithContext(ctx, "org.freedesktop.DBus.Properties.GetAll", 0, iface).Store(&properties)
	return properties, err
}
func getOwner(ctx context.Context, conn *dbus.Conn) (string, error) {
	var owner string
	err := conn.BusObject().CallWithContext(ctx, "org.freedesktop.DBus.GetNameOwner", 0, nmDest).Store(&owner)
	return owner, err
}
func (m *Manager) profile(ctx context.Context, conn *dbus.Conn, owner string) (dbus.ObjectPath, error) {
	var path dbus.ObjectPath
	if err := conn.Object(owner, settingsPath).CallWithContext(ctx, nmDest+".Settings.GetConnectionByUuid", 0, m.receipt.UUID).Store(&path); err != nil {
		return "", fmt.Errorf("постоянный профиль Aheadscale недоступен: %w", err)
	}
	var settings connectionSettings
	if err := conn.Object(owner, path).CallWithContext(ctx, nmDest+".Settings.Connection.GetSettings", 0).Store(&settings); err != nil {
		return "", err
	}
	if err := validateProfile(settings, m.receipt.UUID); err != nil {
		return "", err
	}
	data, ok := settings["vpn"]["data"].Value().(map[string]string)
	if !ok || data["management-url"] != m.receipt.ManagementURL || data["admin-url"] != m.receipt.AdminURL {
		return "", fmt.Errorf("параметры собственного профиля Aheadscale изменены")
	}
	return path, nil
}
func (m *Manager) active(ctx context.Context, conn *dbus.Conn, owner string) (dbus.ObjectPath, uint32, error) {
	properties, err := getAll(ctx, conn, owner, nmPath, nmDest)
	if err != nil {
		return "", 0, err
	}
	paths, ok := properties["ActiveConnections"].Value().([]dbus.ObjectPath)
	if !ok {
		return "", 0, fmt.Errorf("некорректный ответ ActiveConnections")
	}
	for _, path := range paths {
		active, err := getAll(ctx, conn, owner, path, activeInterface)
		if err != nil {
			return "", 0, err
		}
		if active["Uuid"].Value() != m.receipt.UUID {
			continue
		}
		state, ok := active["State"].Value().(uint32)
		if !ok {
			return "", 0, fmt.Errorf("некорректное состояние Aheadscale")
		}
		return path, state, nil
	}
	return "", 0, nil
}

// SetConnected uses the caller's system-bus identity and NM's normal Polkit rules.
func (m *Manager) SetConnected(ctx context.Context, connected bool) error {
	conn, err := dbus.ConnectSystemBus(dbus.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("NetworkManager недоступен: %w", err)
	}
	defer conn.Close()
	owner, err := getOwner(ctx, conn)
	if err != nil {
		return err
	}
	profile, err := m.profile(ctx, conn, owner)
	if err != nil {
		return err
	}
	active, state, err := m.active(ctx, conn, owner)
	if err != nil {
		return err
	}
	if !connected {
		if active == "" {
			return nil
		}
		return conn.Object(owner, nmPath).CallWithContext(ctx, nmDest+".DeactivateConnection", 0, active).Err
	}
	if active != "" && (state == 1 || state == 2) {
		return nil
	}
	// A quick Connect after Cancel waits for NM to remove the old activation.
	for active != "" {
		timer := time.NewTimer(100 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
		active, _, err = m.active(ctx, conn, owner)
		if err != nil {
			return err
		}
	}
	var activated dbus.ObjectPath
	return conn.Object(owner, nmPath).CallWithContext(ctx, nmDest+".ActivateConnection", 0, profile, dbus.ObjectPath("/"), dbus.ObjectPath("/")).Store(&activated)
}
func (m *Manager) Snapshot(ctx context.Context) (Observation, error) {
	conn, err := dbus.ConnectSystemBus(dbus.WithContext(ctx))
	if err != nil {
		return Observation{}, err
	}
	defer conn.Close()
	owner, err := getOwner(ctx, conn)
	if err != nil {
		return Observation{}, err
	}
	if _, err = m.profile(ctx, conn, owner); err != nil {
		return Observation{}, err
	}
	_, state, err := m.active(ctx, conn, owner)
	if err != nil {
		return Observation{}, err
	}
	currentOwner, err := getOwner(ctx, conn)
	if err != nil || currentOwner != owner {
		return Observation{}, fmt.Errorf("NetworkManager сменил владельца")
	}
	return Observation{Owner: owner, VPNActive: state == 2, VPNConnecting: state == 1 || state == 3}, nil
}
