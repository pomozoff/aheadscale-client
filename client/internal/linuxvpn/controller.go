//go:build linux && !android

package linuxvpn

import (
	"context"
	"errors"
	"fmt"
	"github.com/netbirdio/netbird/client/proto"
	"os"
	"sync"
)

// Controller holds the session intention independently of the desktop process.
// The gate keeps new attempts behind the previous engine cleanup.
type Controller struct {
	prepare func(context.Context, uint64) error
	manager *Manager
	daemon  proto.DaemonServiceClient
	ready   func(context.Context) error
	track   func(context.Context, uint64, Intent) (string, error)
	owner   string
	mu      sync.Mutex
	gate    chan struct{}
	path    string
	saved   savedIntent
	state   State
	down    func(context.Context) error
}

func NewController(path, bootID string, autoConnect bool) (*Controller, error) {
	saved, err := loadIntent(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("прочитать состояние Aheadscale: %w", err)
	}
	if errors.Is(err, os.ErrNotExist) || saved.BootID != bootID {
		saved = savedIntent{Version: 1, BootID: bootID, Intent: Intent{Connected: autoConnect, Generation: saved.Intent.Generation + 1}}
	} else {
		saved.Intent.Generation++
	}
	if bootID == "" {
		return nil, fmt.Errorf("пустой boot ID")
	}
	if err = saveIntent(path, saved); err != nil {
		return nil, err
	}
	c := &Controller{path: path, saved: saved, gate: make(chan struct{}, 1), state: State{Generation: saved.Intent.Generation, Phase: Disconnected}}
	if saved.Intent.Connected {
		c.state.Phase = Connecting
	}
	return c, nil
}
func (c *Controller) Intent() Intent { c.mu.Lock(); defer c.mu.Unlock(); return c.saved.Intent }
func (c *Controller) State() State   { c.mu.Lock(); defer c.mu.Unlock(); return c.state }
func (c *Controller) Request(ctx context.Context, connected bool) (uint64, error) {
	select {
	case c.gate <- struct{}{}:
	case <-ctx.Done():
		return 0, ctx.Err()
	}
	defer func() { <-c.gate }()
	c.mu.Lock()
	next := c.saved
	next.Intent = Intent{Connected: connected, Generation: c.saved.Intent.Generation + 1}
	next.PendingCancel = false
	if err := saveIntent(c.path, next); err != nil {
		c.mu.Unlock()
		return 0, err
	}
	c.saved = next
	c.state = State{Generation: next.Intent.Generation, Phase: Connecting}
	c.mu.Unlock()
	if !connected && c.down != nil {
		if err := c.down(ctx); err != nil {
			c.mu.Lock()
			c.state = State{Generation: next.Intent.Generation, Phase: Failed, Error: err.Error()}
			c.mu.Unlock()
			return next.Intent.Generation, err
		}
	}
	if !connected {
		c.mu.Lock()
		c.state.Phase = Disconnected
		c.mu.Unlock()
	}
	return next.Intent.Generation, nil
}
func (c *Controller) Complete(generation uint64, phase Phase) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if generation != c.saved.Intent.Generation || !c.saved.Intent.Connected || c.saved.PendingCancel {
		return fmt.Errorf("попытка подключения Aheadscale отменена или устарела")
	}
	switch phase {
	case Connecting, Connected, NeedsLogin, Failed:
	default:
		return fmt.Errorf("неизвестное состояние подключения")
	}
	c.state = State{Generation: generation, Phase: phase}
	return nil
}

// ObserveNMStop is called only for a system-bus event verified against our UUID,
// current unique NM owner, and the active path recorded at broker Begin.
func (c *Controller) ObserveNMStop(ctx context.Context, owner string, generation uint64, userRequested bool) error {
	select {
	case c.gate <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-c.gate }()
	c.mu.Lock()
	if owner != c.saved.ActivationOwner || generation != c.saved.ActivationGeneration {
		c.mu.Unlock()
		return nil
	}
	next := c.saved
	next.PendingCancel = false
	next.ActivationGeneration = 0
	if userRequested {
		next.Intent.Connected = false
		next.Intent.Generation++
	}
	if err := saveIntent(c.path, next); err != nil {
		c.mu.Unlock()
		return err
	}
	c.saved = next
	if userRequested {
		c.state = State{Generation: next.Intent.Generation, Phase: Connecting}
	}
	c.mu.Unlock()
	if c.down != nil {
		if err := c.down(ctx); err != nil {
			return err
		}
	}
	if userRequested {
		c.mu.Lock()
		c.state.Phase = Disconnected
		c.mu.Unlock()
	}
	return nil
}
