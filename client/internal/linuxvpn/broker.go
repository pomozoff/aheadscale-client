//go:build linux && !android

package linuxvpn

import (
	"context"
	"fmt"
)

type Message struct {
	Version    int
	Operation  string
	Generation uint64
	Phase      Phase
}

// Handle is called only after the Unix transport obtains the kernel peer UID.
func (c *Controller) Handle(peerUID uint32, message Message) (State, error) {
	if peerUID != 0 {
		return State{}, fmt.Errorf("служебный канал Aheadscale доступен только root")
	}
	if message.Version != 2 {
		return State{}, fmt.Errorf("неподдерживаемая версия служебного протокола")
	}
	c.gate <- struct{}{}
	defer func() { <-c.gate }()
	c.mu.Lock()
	defer c.mu.Unlock()
	switch message.Operation {
	case "Begin":
		// A retransmission cannot silently replace an in-flight activation.
		if message.Generation != 0 && message.Generation == c.saved.Intent.Generation && c.saved.Intent.Connected {
			return c.state, nil
		}
		if message.Generation != 0 {
			return State{}, fmt.Errorf("неизвестное поколение подключения")
		}
		next := c.saved
		next.Intent = Intent{Connected: true, Generation: next.Intent.Generation + 1}
		next.PendingCancel = false
		next.ActivationOwner = c.owner
		next.ActivationGeneration = next.Intent.Generation
		automatic := false
		if c.track != nil {
			c.mu.Unlock()
			owner, err := c.track(context.Background(), next.Intent.Generation, c.saved.Intent)
			c.mu.Lock()
			if err != nil {
				return State{}, err
			}
			next.ActivationOwner = owner.owner
			automatic = owner.automatic
		}
		next.ActivationNonInteractive = automatic
		if err := saveIntent(c.path, next); err != nil {
			return State{}, err
		}
		c.saved = next
		c.state = State{Generation: next.Intent.Generation, Phase: Connecting, NonInteractive: automatic}
		if c.prepare != nil {
			c.mu.Unlock()
			err := c.prepare(context.Background(), next.Intent.Generation)
			c.mu.Lock()
			if err != nil {
				c.state.Phase = Failed
				c.state.Error = err.Error()
				return State{}, err
			}
		}
	case "Cancel":
		if message.Generation != c.saved.Intent.Generation {
			return State{}, fmt.Errorf("устаревшая отмена подключения")
		}
		phase := Connecting
		if c.state.Phase == NeedsLogin {
			phase = NeedsLogin
		}
		next := c.saved
		// Disconnect() also runs while NetworkManager itself shuts down.
		// Invalidate the attempt, but only a verified NM user event changes intent.
		next.PendingCancel = true
		next.Intent.Generation++
		if err := saveIntent(c.path, next); err != nil {
			return State{}, err
		}
		c.saved = next
		c.state = State{Generation: next.Intent.Generation, Phase: phase, NonInteractive: c.state.NonInteractive}
		if c.down != nil {
			c.mu.Unlock()
			err := c.down(context.Background())
			c.mu.Lock()
			if err != nil {
				c.state = State{Generation: next.Intent.Generation, Phase: Failed, Error: err.Error()}
				return State{}, err
			}
		}
		if phase != NeedsLogin {
			c.state.Phase = Disconnected
		}
	case "Complete":
		if message.Phase == Connected && c.ready != nil {
			c.mu.Unlock()
			err := c.ready(context.Background())
			c.mu.Lock()
			if err != nil {
				return State{}, err
			}
		}
		if message.Generation != c.saved.Intent.Generation || !c.saved.Intent.Connected || c.saved.PendingCancel {
			return State{}, fmt.Errorf("устаревшее завершение подключения")
		}
		switch message.Phase {
		case Connecting, Connected, NeedsLogin, Failed:
		default:
			return State{}, fmt.Errorf("неизвестное состояние подключения")
		}
		c.state = State{Generation: message.Generation, Phase: message.Phase, NonInteractive: c.state.NonInteractive}
	default:
		return State{}, fmt.Errorf("неизвестная служебная операция")
	}
	return c.state, nil
}
