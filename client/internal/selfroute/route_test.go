package selfroute

import (
	"context"
	"errors"
	"net/netip"
	"reflect"
	"testing"

	"github.com/netbirdio/netbird/client/internal/statemanager"
)

var testIP = netip.MustParseAddr("192.0.2.217")
var errInjected = errors.New("errInjected failure")

type fakeStore struct {
	state      statemanager.State
	events     *[]string
	persistErr error
	updateErr  error
	onPersist  func()
}

func (s *fakeStore) LoadState(statemanager.State) error             { return nil }
func (s *fakeStore) GetState(statemanager.State) statemanager.State { return s.state }
func (s *fakeStore) UpdateState(v statemanager.State) error {
	*s.events = append(*s.events, "journal")
	if s.updateErr != nil {
		return s.updateErr
	}
	s.state = v
	return nil
}
func (s *fakeStore) DeleteState(statemanager.State) error {
	s.state = nil
	*s.events = append(*s.events, "forget")
	return nil
}
func (s *fakeStore) PersistState(context.Context) error {
	if s.onPersist != nil {
		s.onPersist()
	}
	*s.events = append(*s.events, "persist")
	return s.persistErr
}

type fakeOps struct {
	route                         *routeInfo
	events                        *[]string
	moveErr, deleteErr, lookupErr error
	noChange                      bool
}

func (o *fakeOps) lookup(netip.Addr) (*routeInfo, error) {
	if o.route == nil && o.lookupErr == nil {
		return nil, errRouteNotFound
	}
	return o.route, o.lookupErr
}
func (o *fakeOps) interfaceIndex(name string, ip netip.Addr) (int, error) {
	if name == "lo0" {
		return 1, nil
	}
	return 12, nil
}
func (o *fakeOps) move(ip netip.Addr) error {
	*o.events = append(*o.events, "move")
	if o.moveErr != nil {
		return o.moveErr
	}
	if !o.noChange {
		o.route = &routeInfo{ip, ip, 1, true, true}
	}
	return nil
}
func (o *fakeOps) remove(ip netip.Addr) error {
	*o.events = append(*o.events, "remove")
	if o.deleteErr != nil {
		return o.deleteErr
	}
	o.route = nil
	return nil
}
func fixture() (*Manager, *fakeStore, *fakeOps, *[]string) {
	events := []string{}
	store := &fakeStore{events: &events}
	ops := &fakeOps{route: &routeInfo{testIP, testIP, 12, true, true}, events: &events}
	return &Manager{store: store, ops: ops}, store, ops, &events
}
func TestSetPersistsBeforeChangingRoute(t *testing.T) {
	m, s, o, ev := fixture()
	if err := m.Set(testIP, "utun120"); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(*ev, []string{"journal", "persist", "move"}) {
		t.Fatal(*ev)
	}
	if s.state == nil || o.route.index != 1 {
		t.Fatal("route or journal missing")
	}
	if err := m.Set(testIP, "utun120"); err != nil {
		t.Fatal(err)
	}
	if len(*ev) != 3 {
		t.Fatalf("repeat Set changed route: %v", *ev)
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	if o.route != nil || s.state != nil {
		t.Fatal("cleanup left state")
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
}
func TestSetStopsWhenJournalCannotBeSaved(t *testing.T) {
	for _, kind := range []string{"update", "persist"} {
		t.Run(kind, func(t *testing.T) {
			m, s, o, ev := fixture()
			if kind == "update" {
				s.updateErr = errInjected
			} else {
				s.persistErr = errInjected
			}
			if err := m.Set(testIP, "utun120"); !errors.Is(err, errInjected) {
				t.Fatal(err)
			}
			if o.route.index != 12 {
				t.Fatal("changed route before durable journal")
			}
			for _, e := range *ev {
				if e == "move" {
					t.Fatal("move called")
				}
			}
		})
	}
}
func TestSetRejectsForeignRoute(t *testing.T) {
	for _, kind := range []string{"missing", "interface", "destination", "gateway", "not-host", "not-local"} {
		t.Run(kind, func(t *testing.T) {
			m, s, o, _ := fixture()
			switch kind {
			case "missing":
				o.route = nil
			case "interface":
				o.route.index = 7
			case "destination":
				o.route.destination = netip.MustParseAddr("192.0.2.218")
			case "gateway":
				o.route.gateway = netip.MustParseAddr("192.0.2.1")
			case "not-host":
				o.route.host = false
			case "not-local":
				o.route.local = false
			}
			if err := m.Set(testIP, "utun120"); err == nil {
				t.Fatal("accepted foreign route")
			}
			if s.state != nil {
				t.Fatal("recorded foreign route")
			}
		})
	}
}
func TestCleanupLeavesForeignChangesAlone(t *testing.T) {
	for _, kind := range []string{"absent", "interface", "gateway", "destination", "not-host", "not-local"} {
		t.Run(kind, func(t *testing.T) {
			m, s, o, ev := fixture()
			if err := m.Set(testIP, "utun120"); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "absent":
				o.route = nil
			case "interface":
				o.route.index = 8
			case "gateway":
				o.route.gateway = netip.MustParseAddr("192.0.2.1")
			case "destination":
				o.route.destination = netip.MustParseAddr("192.0.2.218")
			case "not-host":
				o.route.host = false
			case "not-local":
				o.route.local = false
			}
			if err := m.Close(); err != nil {
				t.Fatal(err)
			}
			for _, e := range *ev {
				if e == "remove" {
					t.Fatal("removed foreign route")
				}
			}
			if s.state != nil {
				t.Fatal("stale intent left")
			}
		})
	}
}
func TestCleanupFailureRetainsRecoveryState(t *testing.T) {
	m, s, o, _ := fixture()
	if err := m.Set(testIP, "utun120"); err != nil {
		t.Fatal(err)
	}
	o.deleteErr = errInjected
	if err := m.Close(); !errors.Is(err, errInjected) {
		t.Fatal(err)
	}
	if s.state == nil {
		t.Fatal("lost recovery state")
	}
	o.deleteErr = nil
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	if s.state != nil || o.route != nil {
		t.Fatal("retry did not clean")
	}
}
func TestFailedMoveAndFalseSuccessAreDetected(t *testing.T) {
	for _, kind := range []string{"error", "unchanged"} {
		t.Run(kind, func(t *testing.T) {
			m, _, o, _ := fixture()
			if kind == "error" {
				o.moveErr = errInjected
			} else {
				o.noChange = true
			}
			if err := m.Set(testIP, "utun120"); err == nil {
				t.Fatal("accepted failed move")
			}
			if err := m.Close(); err != nil {
				t.Fatal(err)
			}
			if o.route == nil || o.route.index != 12 {
				t.Fatal("removed original kernel route")
			}
		})
	}
}
func TestNewSessionCleansPreviousJournalBeforeReplacingIt(t *testing.T) {
	m, s, o, _ := fixture()
	s.state = &ShutdownState{IP: testIP, LoopbackIndex: 1}
	o.route = &routeInfo{testIP, testIP, 1, true, true}
	// Recovery removes the old route. Assignment must then be retried by the caller;
	// it must never silently adopt a residual loopback route as a new native route.
	if err := m.Set(testIP, "utun120"); err == nil {
		t.Fatal("adopted residual route")
	}
	if o.route != nil || s.state != nil {
		t.Fatal("residual not cleaned")
	}
}
func TestInvalidAddressNeverChangesRoutes(t *testing.T) {
	for _, ip := range []netip.Addr{{}, netip.IPv4Unspecified(), netip.MustParseAddr("::1"), netip.MustParseAddr("224.0.0.1"), netip.MustParseAddr("127.0.0.1")} {
		m, _, _, ev := fixture()
		if err := m.Set(ip, "utun120"); err == nil {
			t.Fatal(ip)
		}
		if len(*ev) != 0 {
			t.Fatal(*ev)
		}
	}
}

func TestMissingJournalRefusesChange(t *testing.T) {
	m := New(nil)
	if err := m.Set(testIP, "utun120"); err == nil {
		t.Fatal("accepted route change without journal")
	}
}

func TestRouteChangedDuringJournalWriteIsNotOverwritten(t *testing.T) {
	m, s, o, ev := fixture()
	s.onPersist = func() { o.route.index = 7 }
	if err := m.Set(testIP, "utun120"); err == nil {
		t.Fatal("overwrote route changed during journal write")
	}
	for _, event := range *ev {
		if event == "move" {
			t.Fatal("changed foreign route")
		}
	}
}
