//go:build linux && !android

package linuxvpn

import (
	"bufio"
	"context"
	"fmt"
	"github.com/godbus/dbus/v5"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
)

type fixtureNM struct {
	missing  bool
	creates  int
	mu       sync.Mutex
	settings connectionSettings
	active   bool
	state    uint32
	calls    []string
}

func (f *fixtureNM) GetConnectionByUuid(id string) (dbus.ObjectPath, *dbus.Error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.missing {
		return "", dbus.NewError(nmDest+".Settings.InvalidConnection", []interface{}{"not found"})
	}
	return "/settings/aheadscale", nil
}
func (f *fixtureNM) GetSettings() (connectionSettings, *dbus.Error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.settings, nil
}
func (f *fixtureNM) ActivateConnection(profile, device, specific dbus.ObjectPath) (dbus.ObjectPath, *dbus.Error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if profile != "/settings/aheadscale" || device != "/" || specific != "/" {
		return "", dbus.MakeFailedError(fmt.Errorf("wrong activation"))
	}
	f.active = true
	f.calls = append(f.calls, "activate")
	return "/active/aheadscale", nil
}
func (f *fixtureNM) DeactivateConnection(active dbus.ObjectPath) *dbus.Error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if active != "/active/aheadscale" {
		return dbus.MakeFailedError(fmt.Errorf("wrong deactivation"))
	}
	f.active = false
	f.calls = append(f.calls, "deactivate")
	return nil
}
func (f *fixtureNM) GetAll(iface string) (map[string]dbus.Variant, *dbus.Error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if iface == nmDest {
		paths := []dbus.ObjectPath{}
		if f.active {
			paths = append(paths, "/active/aheadscale")
		}
		return map[string]dbus.Variant{"ActiveConnections": dbus.MakeVariant(paths)}, nil
	}
	state := f.state
	if state == 0 {
		state = 2
	}
	return map[string]dbus.Variant{"Uuid": f.settings["connection"]["uuid"], "State": dbus.MakeVariant(state)}, nil
}
func TestManagerCommandsUseOwnedNMProfile(t *testing.T) {
	cmd := exec.Command("dbus-daemon", "--session", "--nofork", "--print-address=1", "--address=unix:path="+filepath.Join(t.TempDir(), "bus"))
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	scanner := bufio.NewScanner(out)
	if !scanner.Scan() {
		t.Fatal("no bus address")
	}
	address := scanner.Text()
	t.Setenv("DBUS_SYSTEM_BUS_ADDRESS", address)
	conn, err := dbus.Connect(address)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	id := "aabbccdd-1111-4222-8333-001122334455"
	f := &fixtureNM{missing: true, settings: profileSettings(id, "https://aheadscale.ru", "https://aheadscale.ru")}
	for _, export := range []struct {
		path  dbus.ObjectPath
		iface string
	}{{nmPath, nmDest}, {settingsPath, nmDest + ".Settings"}, {"/settings/aheadscale", nmDest + ".Settings.Connection"}, {nmPath, "org.freedesktop.DBus.Properties"}, {"/active/aheadscale", "org.freedesktop.DBus.Properties"}} {
		if err = conn.Export(f, export.path, export.iface); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = conn.RequestName(nmDest, dbus.NameFlagDoNotQueue); err != nil {
		t.Fatal(err)
	}
	m := &Manager{receipt: Receipt{UUID: id, ManagementURL: "https://aheadscale.ru", AdminURL: "https://aheadscale.ru"}}
	if err = m.EnsureProfile(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Upgrade a pre-timeout profile without changing its UUID or user label.
	f.mu.Lock()
	f.settings["vpn"]["timeout"] = dbus.MakeVariant(uint32(0))
	f.settings["connection"]["id"] = dbus.MakeVariant("My Aheadscale")
	f.mu.Unlock()
	if err = m.EnsureProfile(context.Background()); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	gotTimeout := f.settings["vpn"]["timeout"].Value()
	gotName := f.settings["connection"]["id"].Value()
	f.mu.Unlock()
	if gotTimeout != uint32(720) || gotName != "My Aheadscale" {
		t.Fatalf("legacy profile was not safely upgraded: timeout=%v name=%v", gotTimeout, gotName)
	}
	// A delayed plugin Begin must not turn a user-cancelled NM session ON.
	f.mu.Lock()
	f.active = true
	f.state = 3
	f.mu.Unlock()
	controller, createErr := NewController(filepath.Join(t.TempDir(), "intent"), "boot", false)
	if createErr != nil {
		t.Fatal(createErr)
	}
	controller.track = m.trackCurrentSession
	if _, lateErr := controller.Handle(0, Message{Version: 2, Operation: "Begin"}); lateErr == nil || controller.Intent().Connected {
		t.Fatal("late Begin revived a deactivating connection")
	}
	if _, reattachErr := m.reattachCurrentSession(context.Background(), 1, controller.Intent(), false); reattachErr != nil {
		t.Fatalf("startup cannot reattach for final OFF classification: %v", reattachErr)
	}
	f.mu.Lock()
	f.active = false
	f.state = 0
	f.mu.Unlock()
	if err = m.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	snapshot, err := m.Snapshot(context.Background())
	if err != nil || !snapshot.VPNActive {
		t.Fatalf("no active snapshot: %+v %v", snapshot, err)
	}
	if err = m.SetConnected(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if err = m.SetConnected(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if err = m.activateAutomatically(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	if _, err = m.trackCurrentSession(context.Background(), 3, Intent{Connected: false, Generation: 2}); err == nil {
		t.Fatal("stale automatic path overrode OFF")
	}
	lease, err := m.trackCurrentSession(context.Background(), 4, Intent{Connected: true, Generation: 1})
	if err != nil || !lease.automatic {
		t.Fatalf("automatic activation lost trusted provenance: %+v %v", lease, err)
	}
	if err = m.SetConnected(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	f.settings["vpn"]["service-type"] = dbus.MakeVariant("foreign")
	f.mu.Unlock()
	if err = m.SetConnected(context.Background(), true); err == nil {
		t.Fatal("activated foreign profile")
	}
	if err = m.RemoveProfile(context.Background()); err == nil {
		t.Fatal("removed foreign profile")
	}
	f.mu.Lock()
	f.settings = profileSettings(id, "https://aheadscale.ru", "https://aheadscale.ru")
	f.mu.Unlock()
	if err = m.RemoveProfile(context.Background()); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.missing {
		t.Fatal("profile not deleted")
	}
	if f.creates != 1 {
		t.Fatalf("profile recreated %d times", f.creates)
	}
	if len(f.calls) != 4 || f.calls[0] != "activate" || f.calls[1] != "deactivate" || f.calls[2] != "activate" || f.calls[3] != "deactivate" {
		t.Fatalf("wrong commands: %v", f.calls)
	}
}

func (f *fixtureNM) AddConnection(settings connectionSettings) (dbus.ObjectPath, *dbus.Error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.creates++
	f.settings = settings
	f.missing = false
	return "/settings/aheadscale", nil
}

func (f *fixtureNM) Delete() *dbus.Error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.missing = true
	return nil
}

func (f *fixtureNM) Update(settings connectionSettings) *dbus.Error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.settings = settings
	return nil
}
