//go:build linux && !android

package linuxvpn

import (
	"bufio"
	"context"
	"fmt"
	"github.com/godbus/dbus/v5"
	"os/exec"
	"path/filepath"
	"reflect"
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
	wireIPv6 map[string]dbus.Variant
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
	// D-Bus can still marshal the reply after this method releases the lock.
	copy := make(connectionSettings, len(f.settings))
	for name, group := range f.settings {
		copy[name] = make(map[string]dbus.Variant, len(group))
		for key, value := range group {
			copy[name][key] = value
		}
	}
	return copy, nil
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
	id := "aabbccdd-1111-4222-8333-001122334455"
	f := &fixtureNM{missing: true, settings: profileSettings(id, "https://aheadscale.ru", "https://aheadscale.ru")}
	conn, err := dbus.Connect(address, dbus.WithIncomingInterceptor(func(message *dbus.Message) {
		if message.Headers[dbus.FieldMember].Value() != "Update" || len(message.Body) != 1 {
			return
		}
		settings, ok := message.Body[0].(map[string]map[string]dbus.Variant)
		if !ok {
			return
		}
		f.mu.Lock()
		f.wireIPv6 = settings["ipv6"]
		f.mu.Unlock()
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
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
	// NetworkManager returns legacy IPv6 tuple arrays even for disabled IPv6.
	// Observe the real wire message before godbus converts exported arguments.
	type ipv6Address struct {
		Address []byte
		Prefix uint32
		Gateway []byte
	}
	type ipv6Route struct {
		Destination []byte
		Prefix uint32
		Gateway []byte
		Metric uint32
	}
	addresses := []ipv6Address{{Address: []byte{0xfd, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}, Prefix: 64, Gateway: make([]byte, 16)}}
	routes := []ipv6Route{{Destination: make([]byte, 16), Prefix: 0, Gateway: []byte{0xfd, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2}, Metric: 7}}
	// Upgrade a pre-timeout profile without changing its UUID or user label.
	f.mu.Lock()
	f.settings["vpn"]["timeout"] = dbus.MakeVariant(uint32(0))
	f.settings["ipv6"]["addresses"] = dbus.MakeVariant(addresses)
	f.settings["ipv6"]["routes"] = dbus.MakeVariant(routes)
	f.settings["connection"]["id"] = dbus.MakeVariant("My Aheadscale")
	f.mu.Unlock()
	if err = m.EnsureProfile(context.Background()); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	gotTimeout := f.settings["vpn"]["timeout"].Value()
	gotName := f.settings["connection"]["id"].Value()
	wireAddresses, wireRoutes := f.wireIPv6["addresses"], f.wireIPv6["routes"]
	f.mu.Unlock()
	if wireAddresses.Signature().String() != "a(ayuay)" || wireRoutes.Signature().String() != "a(ayuayu)" {
		t.Fatalf("IPv6 wire types corrupted during timeout migration: addresses=%s routes=%s", wireAddresses.Signature(), wireRoutes.Signature())
	}
	var gotAddresses []ipv6Address
	var gotRoutes []ipv6Route
	if err := wireAddresses.Store(&gotAddresses); err != nil {
		t.Fatal(err)
	}
	if err := wireRoutes.Store(&gotRoutes); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotAddresses, addresses) || !reflect.DeepEqual(gotRoutes, routes) {
		t.Fatal("IPv6 values changed during timeout migration")
	}
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
