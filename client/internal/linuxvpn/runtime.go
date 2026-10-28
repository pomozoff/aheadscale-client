//go:build linux && !android

package linuxvpn

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/netbirdio/netbird/client/proto"
)

const ControlSocket = "/run/aheadscale/networkmanager-control.sock"

// Configure binds the runtime once, before Run or serving broker requests.
func (c *Controller) Configure(manager *Manager, daemon proto.DaemonServiceClient) {
	c.prepare = func(ctx context.Context, generation uint64) error {
		ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		_, err := daemon.NativeBegin(ctx, &proto.NativeBeginRequest{Generation: generation})
		return err
	}
	c.manager = manager
	c.daemon = daemon
	c.track = manager.trackCurrentSession
	c.down = func(ctx context.Context) error {
		ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		_, err := daemon.NativeStop(ctx, &proto.NativeStopRequest{})
		return err
	}
	c.ready = func(ctx context.Context) error {
		observation := c.observeEngine(ctx, Observation{})
		if !observation.EngineReady {
			return fmt.Errorf("движок Aheadscale не подтвердил адрес собственного туннеля")
		}
		return nil
	}
}
func (c *Controller) observeEngine(ctx context.Context, observation Observation) Observation {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	status, err := c.daemon.Status(ctx, &proto.StatusRequest{GetFullPeerStatus: true})
	if err != nil {
		return observation
	}
	observation.EngineAvailable = true
	observation.NeedsLogin = strings.EqualFold(status.GetStatus(), "NeedsLogin") || strings.EqualFold(status.GetStatus(), "LoginFailed")
	if !strings.EqualFold(status.GetStatus(), "Connected") {
		return observation
	}
	iface, err := net.InterfaceByName("Aheadscale")
	if err != nil || iface.Flags&net.FlagUp == 0 {
		return observation
	}
	addresses, err := iface.Addrs()
	if err != nil {
		return observation
	}
	actual := make([]string, 0, len(addresses))
	for _, address := range addresses {
		actual = append(actual, address.String())
	}
	observation.EngineReady = readyInterfaceAddress(status.GetFullStatus().GetLocalPeerState().GetIP(), actual)
	return observation
}
func listenControlSocket() (net.Listener, error) {
	parent := filepath.Dir(ControlSocket)
	if err := os.MkdirAll(parent, 0700); err != nil {
		return nil, err
	}
	info, err := os.Lstat(parent)
	if err != nil {
		return nil, err
	}
	owner, ok := info.Sys().(*syscall.Stat_t)
	if !ok || owner.Uid != 0 || !info.IsDir() || info.Mode().Perm()&0022 != 0 {
		return nil, fmt.Errorf("небезопасный каталог служебного канала Aheadscale")
	}
	if existing, err := os.Lstat(ControlSocket); err == nil {
		stat, ok := existing.Sys().(*syscall.Stat_t)
		if !ok || stat.Uid != 0 || existing.Mode()&os.ModeSocket == 0 {
			return nil, fmt.Errorf("служебный путь Aheadscale занят чужим файлом")
		}
		connection, dialErr := net.DialTimeout("unix", ControlSocket, time.Second)
		if dialErr == nil {
			_ = connection.Close()
			return nil, fmt.Errorf("контроллер Aheadscale уже работает")
		}
		if !errors.Is(dialErr, syscall.ECONNREFUSED) {
			return nil, dialErr
		}
		if err = os.Remove(ControlSocket); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	listener, err := net.Listen("unix", ControlSocket)
	if err != nil {
		return nil, err
	}
	if err = os.Chmod(ControlSocket, 0600); err != nil {
		_ = listener.Close()
		return nil, err
	}
	return listener, nil
}
func (c *Controller) Run(ctx context.Context) error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("системный контроллер Aheadscale должен работать от root")
	}
	if c.manager == nil || c.daemon == nil {
		return fmt.Errorf("компоненты native VPN Aheadscale не настроены")
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	events, err := c.manager.Watch(ctx)
	if err != nil {
		return err
	}
	initial, err := c.manager.Snapshot(ctx)
	if err == nil {
		if err = c.recoverInitialObservation(initial); err != nil {
			return err
		}
		c.mu.Lock()
		generation := c.saved.ActivationGeneration
		c.mu.Unlock()
		if generation != 0 && (initial.VPNActive || initial.VPNConnecting) {
			_, _ = c.manager.trackCurrentSession(ctx, generation, c.Intent())
		}
	}
	listener, err := listenControlSocket()
	if err != nil {
		return err
	}
	brokerDone := make(chan error, 1)
	go func() { brokerDone <- c.ServeBroker(ctx, listener) }()
	brokerExited := false
	defer func() {
		cancel()
		if !brokerExited {
			<-brokerDone
		}
	}()
	poll := time.NewTicker(time.Second)
	defer poll.Stop()
	var nextActivation time.Time
	for {
		select {
		case <-ctx.Done():
			return nil
		case brokerErr := <-brokerDone:
			brokerExited = true
			if brokerErr == nil {
				brokerErr = fmt.Errorf("служебный канал Aheadscale остановлен")
			}
			return brokerErr
		case event, ok := <-events:
			if !ok {
				if ctx.Err() != nil {
					return nil
				}
				return fmt.Errorf("наблюдение NetworkManager остановлено")
			}
			if err = c.ObserveNMStop(ctx, event.Owner, event.Generation, event.UserRequested); err != nil {
				log.Printf("Aheadscale native VPN stop: %v", err)
			}
		case <-poll.C:
			queryCtx, stop := context.WithTimeout(ctx, 4*time.Second)
			generation := c.Intent().Generation
			observation, snapshotErr := c.manager.Snapshot(queryCtx)
			if snapshotErr != nil {
				observation = Observation{}
			}
			observation = c.observeEngine(queryCtx, observation)
			observation.Generation = generation
			action := c.Reconcile(observation)
			err = nil
			switch action {
			case Activate:
				if time.Now().After(nextActivation) {
					nextActivation = time.Now().Add(5 * time.Second)
					err = c.executeAction(queryCtx, generation, Activate)
				}
			case Deactivate:
				err = c.executeAction(queryCtx, generation, Deactivate)
			case StopEngine:
				err = c.executeAction(queryCtx, generation, StopEngine)
			default:
				err = nil
			}
			stop()
			if err != nil {
				log.Printf("Aheadscale native VPN reconcile: %v", err)
			}
		}
	}
}

func (c *Controller) executeAction(ctx context.Context, generation uint64, action Action) error {
	select {
	case c.gate <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-c.gate }()
	c.mu.Lock()
	intent := c.saved.Intent
	pending := c.saved.PendingCancel
	c.mu.Unlock()
	if intent.Generation != generation {
		return nil
	}
	switch action {
	case Activate:
		if !intent.Connected || pending {
			return nil
		}
		return c.manager.activateAutomatically(ctx, generation)
	case Deactivate:
		if intent.Connected {
			return c.manager.failConnection(ctx)
		}
		return c.manager.SetConnected(ctx, false)
	case StopEngine:
		return c.down(ctx)
	}
	return nil
}

// Run calls this once before opening the broker, so no new activation can race
// a recovery decision. A missed stop cannot leave a durable binding forever.
func (c *Controller) recoverInitialObservation(initial Observation) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.owner = initial.Owner
	if initial.VPNActive || initial.VPNConnecting {
		return nil
	}
	next := c.saved
	next.PendingCancel = false
	next.ActivationGeneration = 0
	next.ActivationOwner = initial.Owner
	if err := saveIntent(c.path, next); err != nil {
		return err
	}
	c.saved = next
	return nil
}
