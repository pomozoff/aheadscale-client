// Package selfroute manages macOS delivery to the local overlay IPv4 address.
package selfroute

import (
	"context"
	"errors"
	"fmt"
	"net/netip"

	"github.com/netbirdio/netbird/client/internal/statemanager"
)

var errRouteNotFound = errors.New("local host route not found")

type routeInfo struct {
	destination, gateway netip.Addr
	index                int
	host, local          bool
}

type routeOps interface {
	lookup(netip.Addr) (*routeInfo, error)
	interfaceIndex(string, netip.Addr) (int, error)
	move(netip.Addr) error
	remove(netip.Addr) error
}

type stateStore interface {
	LoadState(statemanager.State) error
	GetState(statemanager.State) statemanager.State
	UpdateState(statemanager.State) error
	DeleteState(statemanager.State) error
	PersistState(context.Context) error
}

// ShutdownState is a write-ahead intent. Only a matching local route recorded by this client may
// be removed; an intent saved before the kernel change is harmless to replay.
type ShutdownState struct {
	IP            netip.Addr `json:"ip"`
	LoopbackIndex int        `json:"loopbackIndex"`
}

func (s *ShutdownState) Name() string { return "darwin_local_route" }

func (s *ShutdownState) Cleanup() error { return s.cleanup(newRouteOps()) }

func validIP(ip netip.Addr) bool {
	return ip.Is4() && ip.IsGlobalUnicast()
}

func (s *ShutdownState) owns(r *routeInfo) bool {
	return r != nil && r.destination == s.IP && r.gateway == s.IP &&
		r.index == s.LoopbackIndex && r.host && r.local
}

func (s *ShutdownState) cleanup(ops routeOps) error {
	if !validIP(s.IP) || s.LoopbackIndex <= 0 {
		return errors.New("invalid local route recovery state")
	}
	r, err := ops.lookup(s.IP)
	if errors.Is(err, errRouteNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect local route: %w", err)
	}
	if !s.owns(r) {
		return nil
	}
	if err := ops.remove(s.IP); err != nil {
		return fmt.Errorf("remove local route: %w", err)
	}
	r, err = ops.lookup(s.IP)
	if errors.Is(err, errRouteNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("verify local route removal: %w", err)
	}
	if s.owns(r) {
		return errors.New("local route still present after removal")
	}
	return nil
}

// Manager is serialized by its owning TunDevice. It must be closed before the
// UTUN disappears, and its state is also registered for daemon crash recovery.
type Manager struct {
	store  stateStore
	ops    routeOps
	state  *ShutdownState
	loaded bool
}

// New attaches route ownership to the existing private shutdown journal.
func New(sm *statemanager.Manager) *Manager {
	if sm == nil {
		return &Manager{ops: newRouteOps()}
	}
	return &Manager{store: sm, ops: newRouteOps()}
}

// Set moves only the kernel-created local /32 of the given UTUN to lo0.
// The journal is saved synchronously before any route mutation.
func (m *Manager) Set(ip netip.Addr, name string) error {
	if m.store == nil {
		return errors.New("local route requires a durable state manager")
	}
	if !validIP(ip) {
		return fmt.Errorf("invalid local overlay IPv4: %s", ip)
	}
	if !m.loaded {
		if err := m.store.LoadState(&ShutdownState{}); err != nil {
			return fmt.Errorf("load local route intent: %w", err)
		}
		if state, ok := m.store.GetState(&ShutdownState{}).(*ShutdownState); ok {
			m.state = state
		}
		m.loaded = true
	} else if m.state != nil && m.state.IP == ip {
		r, err := m.ops.lookup(ip)
		if err != nil {
			return err
		}
		if m.state.owns(r) {
			return nil
		}
	}
	if err := m.Close(); err != nil {
		return err
	}
	idx, err := m.ops.interfaceIndex(name, ip)
	if err != nil {
		return fmt.Errorf("inspect overlay interface: %w", err)
	}
	lo, err := m.ops.interfaceIndex("lo0", netip.Addr{})
	if err != nil {
		return fmt.Errorf("inspect loopback interface: %w", err)
	}
	r, err := m.ops.lookup(ip)
	if err != nil {
		return fmt.Errorf("inspect original local route: %w", err)
	}
	if r == nil || r.destination != ip || r.gateway != ip || r.index != idx ||
		!r.host || !r.local || idx == lo {
		return fmt.Errorf("own local host route for %s on %s not found", ip, name)
	}
	original := *r
	m.state = &ShutdownState{IP: ip, LoopbackIndex: lo}
	if err := m.store.UpdateState(m.state); err != nil {
		return fmt.Errorf("record local route intent: %w", err)
	}
	if err := m.store.PersistState(context.Background()); err != nil {
		return fmt.Errorf("persist local route intent: %w", err)
	}
	current, err := m.ops.lookup(ip)
	if err != nil {
		return fmt.Errorf("recheck original local route: %w", err)
	}
	if current == nil || *current != original {
		return errors.Join(errors.New("local route changed while persisting intent"), m.Close())
	}
	if err := m.ops.move(ip); err != nil {
		return errors.Join(fmt.Errorf("move local route: %w", err), m.Close())
	}
	r, err = m.ops.lookup(ip)
	if err != nil {
		return fmt.Errorf("verify local route: %w", err)
	}
	if !m.state.owns(r) {
		return errors.Join(errors.New("kernel did not install the owned loopback route"), m.Close())
	}
	return nil
}

// Close forgets the intent only after its owned route has gone. Failure retains
// the state so a later shutdown or daemon start can retry safely.
func (m *Manager) Close() error {
	if m == nil || m.state == nil {
		return nil
	}
	if err := m.state.cleanup(m.ops); err != nil {
		return err
	}
	if err := m.store.DeleteState(m.state); err != nil {
		return fmt.Errorf("forget local route intent: %w", err)
	}
	if err := m.store.PersistState(context.Background()); err != nil {
		return fmt.Errorf("persist local route cleanup: %w", err)
	}
	m.state = nil
	return nil
}
