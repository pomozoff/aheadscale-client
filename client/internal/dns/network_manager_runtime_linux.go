//go:build linux && !android

package dns

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
)

const networkManagerRuntimeDirectory = "/run/NetworkManager/conf.d"

// PrepareNetworkManagerInterface coordinates ownership before creating the interface.
func PrepareNetworkManagerInterface(ifaceName string) (func() error, error) {
	return PrepareNetworkManagerInterfaceForMode(ifaceName, false)
}
func PrepareNetworkManagerInterfaceForMode(ifaceName string, native bool) (func() error, error) {
	running, err := networkManagerIsRunning()
	if err != nil {
		return nil, err
	}
	if !running {
		return func() error { return nil }, nil
	}
	manager, _, err := getOSDNSManagerType()
	if err != nil {
		return nil, fmt.Errorf("detect DNS owner before interface creation: %w", err)
	}
	release, _, err := networkManagerRuntimeRule(networkManagerRuntimeDirectory, ifaceName, native || manager != networkManager)
	if err != nil {
		return nil, err
	}
	// A stale rule may predate a crash before Reload completed.
	if err := reloadNetworkManagerRuntimeConfiguration(); err != nil {
		if release != nil {
			err = errors.Join(err, release())
		}
		return nil, err
	}
	if release == nil {
		return func() error { return nil }, nil
	}
	return func() error {
		if err := release(); err != nil {
			return err
		}
		return reloadNetworkManagerRuntimeConfiguration()
	}, nil
}

func networkManagerIsRunning() (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	conn, err := dbus.ConnectSystemBus(dbus.WithContext(ctx))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("connect NetworkManager bus: %w", err)
	}
	defer closeNetworkManagerBus(conn)
	var running bool
	err = conn.BusObject().CallWithContext(ctx, "org.freedesktop.DBus.NameHasOwner", 0, networkManagerDest).Store(&running)
	if err != nil {
		return false, fmt.Errorf("check NetworkManager availability: %w", err)
	}
	return running, nil
}

func reloadNetworkManagerRuntimeConfiguration() error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	conn, err := dbus.ConnectSystemBus(dbus.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("connect NetworkManager reload bus: %w", err)
	}
	defer closeNetworkManagerBus(conn)
	err = conn.Object(networkManagerDest, networkManagerDbusObjectNode).CallWithContext(ctx, networkManagerDest+".Reload", 0, uint32(1)).Store()
	if isMissingNetworkManagerConnection(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("reload NetworkManager interface policy: %w", err)
	}
	return nil
}

func networkManagerRuntimeRule(directory, ifaceName string, enabled bool) (func() error, bool, error) {
	if len(ifaceName) == 0 || len(ifaceName) > 15 || strings.IndexFunc(ifaceName, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("_.-+", r))
	}) >= 0 {
		return nil, false, fmt.Errorf("unsupported NetworkManager interface name")
	}
	path := filepath.Join(directory, "90-aheadscale-"+ifaceName+".conf")
	data := []byte(fmt.Sprintf("# Managed by Aheadscale (runtime).\n[device-aheadscale-%s]\nmatch-device=interface-name:%s\nmanaged=0\n", ifaceName, ifaceName))
	if !enabled {
		changed, err := removeOwnedNetworkManagerRule(path, data)
		return nil, changed, err
	}
	if err := os.MkdirAll(directory, 0755); err != nil {
		return nil, false, fmt.Errorf("create NetworkManager runtime directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	changed := err == nil
	if err != nil {
		if !errors.Is(err, os.ErrExist) {
			return nil, false, fmt.Errorf("create NetworkManager runtime policy: %w", err)
		}
		if err := checkOwnedNetworkManagerRule(path, data); err != nil {
			return nil, false, err
		}
	} else {
		_, writeErr := file.Write(data)
		closeErr := file.Close()
		if err := errors.Join(writeErr, closeErr); err != nil {
			return nil, false, errors.Join(err, os.Remove(path))
		}
	}
	return func() error {
		_, err := removeOwnedNetworkManagerRule(path, data)
		return err
	}, changed, nil
}

func checkOwnedNetworkManagerRule(path string, expected []byte) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("network manager runtime policy is not a regular file")
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if !bytes.Equal(contents, expected) {
		return fmt.Errorf("network manager runtime policy was changed externally")
	}
	return nil
}

func removeOwnedNetworkManagerRule(path string, expected []byte) (bool, error) {
	if err := checkOwnedNetworkManagerRule(path, expected); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	if err := os.Remove(path); err != nil {
		return false, fmt.Errorf("remove NetworkManager runtime policy: %w", err)
	}
	return true, nil
}
