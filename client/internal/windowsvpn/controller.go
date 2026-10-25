package windowsvpn

import (
	"context"
	"errors"
	"sync"
	"time"
)

type ConnectionState string

const (
	Disconnected  ConnectionState = "Disconnected"
	Connecting    ConnectionState = "Connecting"
	Connected     ConnectionState = "Connected"
	Reconnecting  ConnectionState = "Reconnecting"
	Disconnecting ConnectionState = "Disconnecting"
	Failed        ConnectionState = "Failed"
)

type SessionSnapshot struct {
	Generation uint64
	State      ConnectionState
}

// SessionController serializes side effects; a newer intent cancels the older
// command and owns the only result that may update the visible state.
type SessionController struct {
	mu         sync.Mutex
	serial     chan struct{}
	generation uint64
	state      ConnectionState
	cancel     context.CancelFunc
	run        func(context.Context, bool) error
}

func NewSessionController(run func(context.Context, bool) error) *SessionController {
	c := &SessionController{serial: make(chan struct{}, 1), state: Disconnected, run: run}
	c.serial <- struct{}{}
	return c
}

func (c *SessionController) Snapshot() SessionSnapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	return SessionSnapshot{Generation: c.generation, State: c.state}
}

func (c *SessionController) SetConnected(ctx context.Context, connected bool) error {
	return c.setConnected(ctx, connected, false, nil)
}

// StartOnLaunch resumes a saved connection only before any explicit user intent.
func (c *SessionController) StartOnLaunch(ctx context.Context) error {
	return c.setConnected(ctx, true, true, nil)
}

// ResumeAfterDaemonRestart восстанавливает только сохранившееся намерение Connect.
func (c *SessionController) ResumeAfterDaemonRestart(ctx context.Context, generation uint64) error {
	return c.setConnected(ctx, true, false, &generation)
}

func (c *SessionController) setConnected(ctx context.Context, connected, startup bool, restartGeneration *uint64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	if (startup && c.generation != 0) || (restartGeneration != nil &&
		(c.generation != *restartGeneration || (c.state != Connected && c.state != Connecting))) {
		c.mu.Unlock()
		return nil
	}
	previous := c.state
	c.generation++
	generation := c.generation
	if c.cancel != nil {
		c.cancel()
	}
	operation, cancel := context.WithCancel(ctx)
	c.cancel = cancel
	if connected {
		c.state = Connecting
	} else {
		c.state = Disconnecting
	}
	c.mu.Unlock()
	defer cancel()

	select {
	case <-c.serial:
	case <-operation.Done():
		c.mu.Lock()
		if c.generation == generation {
			c.state = previous
			c.cancel = nil
		}
		c.mu.Unlock()
		return operation.Err()
	}
	defer func() { c.serial <- struct{}{} }()
	if err := operation.Err(); err != nil {
		c.mu.Lock()
		if c.generation == generation {
			c.state = previous
			c.cancel = nil
		}
		c.mu.Unlock()
		return err
	}

	err := c.run(operation, connected)
	var cleanupErr error
	cleanup := connected && (err != nil || operation.Err() != nil)
	if cleanup {
		// Still holding serial: an old partial connect is disconnected before a
		// new generation can start. Never reuse the cancelled command context.
		cleanupCtx, stop := context.WithTimeout(context.Background(), 15*time.Second)
		cleanupErr = c.run(cleanupCtx, false)
		stop()
		err = errors.Join(err, cleanupErr)
	}
	if cause := operation.Err(); cause != nil {
		err = errors.Join(cause, err)
	}
	c.mu.Lock()
	current := c.generation == generation
	if current {
		c.cancel = nil
		switch {
		case err == nil && connected:
			c.state = Connected
		case err == nil:
			c.state = Disconnected
		case cleanup && cleanupErr == nil && operation.Err() != nil:
			c.state = Disconnected
		default:
			c.state = Failed
		}
	}
	c.mu.Unlock()
	if !current {
		return errors.Join(context.Canceled, err)
	}
	return err
}
