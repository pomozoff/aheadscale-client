//go:build linux && !android

package linuxvpn

import (
	"context"
	"errors"
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
	automatic  map[sessionKey]uint64
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
	path, _, err := m.profileSnapshot(ctx, conn, owner)
	return path, err
}
func (m *Manager) profileSnapshot(ctx context.Context, conn *dbus.Conn, owner string) (dbus.ObjectPath, connectionSettings, error) {
	var path dbus.ObjectPath
	if err := conn.Object(owner, settingsPath).CallWithContext(ctx, nmDest+".Settings.GetConnectionByUuid", 0, m.receipt.UUID).Store(&path); err != nil {
		return "", nil, fmt.Errorf("постоянный профиль Aheadscale недоступен: %w", err)
	}
	var settings connectionSettings
	if err := conn.Object(owner, path).CallWithContext(ctx, nmDest+".Settings.Connection.GetSettings", 0).Store(&settings); err != nil {
		return "", nil, err
	}
	if err := validateProfile(settings, m.receipt.UUID); err != nil {
		return "", nil, err
	}
	data, ok := settings["vpn"]["data"].Value().(map[string]string)
	if !ok || data["management-url"] != m.receipt.ManagementURL || data["admin-url"] != m.receipt.AdminURL || data["interface-name"] != "Aheadscale" {
		return "", nil, fmt.Errorf("параметры собственного профиля Aheadscale изменены")
	}
	return path, settings, nil
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

// EnsureProfile creates only the UUID from a verified installation receipt.
// A foreign or edited profile is an error, never a reason to replace it.
func (m *Manager) EnsureProfile(ctx context.Context) error {
	conn, err := dbus.ConnectSystemBus(dbus.WithContext(ctx))
	if err != nil {
		return err
	}
	defer conn.Close()
	owner, err := getOwner(ctx, conn)
	if err != nil {
		return err
	}
	path, settings, err := m.profileSnapshot(ctx, conn, owner)
	if err == nil {
		// Upgrade only the legacy/default timeout. Preserve user settings,
		// including an explicitly chosen nonzero timeout and the display name.
		value, present := settings["vpn"]["timeout"]
		if !present || value.Value() == uint32(0) {
			settings["vpn"]["timeout"] = dbus.MakeVariant(vpnActivationTimeoutSeconds)
			return conn.Object(owner, path).CallWithContext(ctx, nmDest+".Settings.Connection.Update", 0, settings).Err
		}
		return nil
	}
	var notFound dbus.Error
	if !errors.As(err, &notFound) || notFound.Name != nmDest+".Settings.InvalidConnection" {
		return err
	}
	if err = conn.Object(owner, settingsPath).CallWithContext(ctx, nmDest+".Settings.AddConnection", 0, profileSettings(m.receipt.UUID, m.receipt.ManagementURL, m.receipt.AdminURL)).Store(&path); err != nil {
		return err
	}
	_, err = m.profile(ctx, conn, owner)
	return err
}

// Automatic activation never waits for a deactivating connection while the
// controller gate is held. Its source generation follows the returned path.
func (m *Manager) activateAutomatically(ctx context.Context, generation uint64) error {
	conn, err := dbus.ConnectSystemBus(dbus.WithContext(ctx))
	if err != nil {
		return err
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
	active, _, err := m.active(ctx, conn, owner)
	if err != nil || active != "" {
		return err
	}
	m.trackingMu.Lock()
	defer m.trackingMu.Unlock()
	var path dbus.ObjectPath
	if err = conn.Object(owner, nmPath).CallWithContext(ctx, nmDest+".ActivateConnection", 0, profile, dbus.ObjectPath("/"), dbus.ObjectPath("/")).Store(&path); err != nil {
		return err
	}
	if m.automatic == nil {
		m.automatic = make(map[sessionKey]uint64)
	}
	m.automatic[sessionKey{owner, path}] = generation
	return nil
}
func (m *Manager) failConnection(ctx context.Context) error {
	conn, err := dbus.ConnectSystemBus(dbus.WithContext(ctx))
	if err != nil {
		return err
	}
	defer conn.Close()
	// This is a service failure, not a user DeactivateConnection operation.
	return conn.Object(ServiceType, "/org/freedesktop/NetworkManager/VPN/Plugin").CallWithContext(ctx, "org.freedesktop.NetworkManager.VPN.Plugin.SetFailure", dbus.FlagNoReplyExpected, "Aheadscale engine unavailable").Err
}

// NewDesktopManager reads only the root-owned non-secret descriptor; the daemon
// registration directory remains private and is never made world-readable.
func NewDesktopManager() (*Manager, error) {
	r, err := loadReceipt(PublicReceiptPath)
	if err != nil {
		return nil, err
	}
	return &Manager{receipt: r}, nil
}

// Connect waits for the actual NM activation, using only our plugin's banner
// for a useful failure message. No daemon Up or privileged broker is used.
func (m *Manager) Connect(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	conn, err := dbus.ConnectSystemBus(dbus.WithContext(ctx))
	if err != nil {
		return err
	}
	defer conn.Close()
	const pluginInterface = "org.freedesktop.NetworkManager.VPN.Plugin"
	pluginPath := dbus.ObjectPath("/org/freedesktop/NetworkManager/VPN/Plugin")
	if err = conn.AddMatchSignal(dbus.WithMatchSender(ServiceType), dbus.WithMatchObjectPath(pluginPath), dbus.WithMatchInterface(pluginInterface), dbus.WithMatchMember("LoginBanner")); err != nil {
		return err
	}
	signals := make(chan *dbus.Signal, 16)
	conn.Signal(signals)
	defer conn.RemoveSignal(signals)
	if err = m.SetConnected(ctx, true); err != nil {
		return err
	}
	banner := ""
	timer := time.NewTicker(100 * time.Millisecond)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case sig := <-signals:
			if sig == nil || len(sig.Body) != 1 {
				continue
			}
			var owner string
			if conn.BusObject().CallWithContext(ctx, "org.freedesktop.DBus.GetNameOwner", 0, ServiceType).Store(&owner) == nil && sig.Sender == owner {
				if value, ok := sig.Body[0].(string); ok {
					banner = value
				}
			}
		case <-timer.C:
			observation, err := m.Snapshot(ctx)
			if err != nil {
				return err
			}
			if observation.VPNActive {
				return nil
			}
			if !observation.VPNConnecting {
				if banner != "" {
					return fmt.Errorf("подключение Aheadscale завершилось: %s", banner)
				}
				return fmt.Errorf("системный VPN Aheadscale не подключён; проверьте вход и журнал службы")
			}
		}
	}
}

func (m *Manager) ManagementURL() string { return m.receipt.ManagementURL }

// RemoveProfile never selects by display name. Missing owned UUID is harmless;
// an edited service, endpoint, or UUID is an explicit refusal before Delete.
func (m *Manager) RemoveProfile(ctx context.Context) error {
	conn, err := dbus.ConnectSystemBus(dbus.WithContext(ctx))
	if err != nil {
		return err
	}
	defer conn.Close()
	owner, err := getOwner(ctx, conn)
	if err != nil {
		return err
	}
	profile, err := m.profile(ctx, conn, owner)
	if err != nil {
		var missing dbus.Error
		if errors.As(err, &missing) && missing.Name == nmDest+".Settings.InvalidConnection" {
			return nil
		}
		return err
	}
	return conn.Object(owner, profile).CallWithContext(ctx, nmDest+".Settings.Connection.Delete", 0).Err
}
