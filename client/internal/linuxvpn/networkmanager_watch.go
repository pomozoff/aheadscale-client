//go:build linux && !android

package linuxvpn

import (
	"context"
	"fmt"
	"time"

	"github.com/godbus/dbus/v5"
)

type sessionKey struct {
	owner string
	path  dbus.ObjectPath
}
type NMEvent struct {
	Owner         string
	Generation    uint64
	UserRequested bool
}

func parseNMStop(signal *dbus.Signal, owner string, path dbus.ObjectPath, generation uint64) (NMEvent, bool) {
	if signal == nil || signal.Sender != owner || signal.Path != path || signal.Name != activeInterface+".StateChanged" || len(signal.Body) != 2 {
		return NMEvent{}, false
	}
	state, stateOK := signal.Body[0].(uint32)
	reason, reasonOK := signal.Body[1].(uint32)
	if !stateOK || !reasonOK || (state != 4 && !(state == 3 && reason == 2)) {
		return NMEvent{}, false
	}
	return NMEvent{Owner: owner, Generation: generation, UserRequested: reason == 2}, true
}
func (m *Manager) trackCurrentSession(ctx context.Context, generation uint64, intent Intent) (string, error) {
	return m.trackSession(ctx, generation, intent, false)
}
func (m *Manager) reattachCurrentSession(ctx context.Context, generation uint64, intent Intent) (string, error) {
	return m.trackSession(ctx, generation, intent, true)
}
func (m *Manager) trackSession(ctx context.Context, generation uint64, intent Intent, reattach bool) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	conn, err := dbus.ConnectSystemBus(dbus.WithContext(ctx))
	if err != nil {
		return "", err
	}
	defer conn.Close()
	owner, err := getOwner(ctx, conn)
	if err != nil {
		return "", err
	}
	if _, err = m.profile(ctx, conn, owner); err != nil {
		return "", err
	}
	path, state, err := m.active(ctx, conn, owner)
	if err != nil {
		return "", err
	}
	if path == "" {
		return "", fmt.Errorf("нет собственного активируемого NM-сеанса Aheadscale")
	}
	if state != 1 && state != 2 && !(reattach && state == 3) {
		return "", fmt.Errorf("NM-сеанс Aheadscale уже отключается; новый Begin отклонён")
	}
	m.trackingMu.Lock()
	defer m.trackingMu.Unlock()
	if m.tracked == nil {
		m.tracked = make(map[sessionKey]uint64)
	}
	for key := range m.tracked {
		if key.owner != owner {
			delete(m.tracked, key)
			delete(m.automatic, key)
		}
	}
	key := sessionKey{owner, path}
	if source, automatic := m.automatic[key]; automatic {
		if !intent.Connected || source != intent.Generation {
			return "", fmt.Errorf("отменённая автоматическая активация Aheadscale")
		}
		delete(m.automatic, key)
	}
	m.tracked[sessionKey{owner, path}] = generation
	return owner, nil
}
func (m *Manager) Watch(ctx context.Context) (<-chan NMEvent, error) {
	conn, err := dbus.ConnectSystemBus(dbus.WithContext(ctx))
	if err != nil {
		return nil, err
	}
	signals := make(chan *dbus.Signal, 64)
	conn.Signal(signals)
	if err = conn.AddMatchSignalContext(ctx, dbus.WithMatchSender(nmDest), dbus.WithMatchInterface(activeInterface), dbus.WithMatchMember("StateChanged")); err != nil {
		_ = conn.Close()
		return nil, err
	}
	events := make(chan NMEvent, 32)
	go func() {
		defer close(events)
		defer conn.RemoveSignal(signals)
		defer conn.Close()
		for {
			select {
			case <-ctx.Done():
				return
			case signal, ok := <-signals:
				if !ok {
					return
				}
				if signal == nil {
					continue
				}
				queryCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
				owner, err := getOwner(queryCtx, conn)
				cancel()
				if err != nil || signal.Sender != owner {
					continue
				}
				key := sessionKey{owner, signal.Path}
				m.trackingMu.Lock()
				generation, tracked := m.tracked[key]
				if !tracked {
					generation, tracked = m.automatic[key]
				}
				m.trackingMu.Unlock()
				if !tracked {
					continue
				}
				event, ok := parseNMStop(signal, owner, signal.Path, generation)
				if !ok {
					continue
				}
				m.trackingMu.Lock()
				delete(m.tracked, key)
				delete(m.automatic, key)
				m.trackingMu.Unlock()
				select {
				case events <- event:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return events, nil
}
