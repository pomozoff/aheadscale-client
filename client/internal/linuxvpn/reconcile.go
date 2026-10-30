//go:build linux && !android

package linuxvpn

import "net/netip"

type Action uint8

const (
	NoAction Action = iota
	Activate
	Deactivate
	StopEngine
)

type Observation struct {
	Generation      uint64
	Owner           string
	VPNActive       bool
	VPNConnecting   bool
	EngineAvailable bool
	EngineReady     bool
	NeedsLogin      bool
}

// Reconcile distinguishes lost service ownership from an explicit Cancel.
// A verified user command changes intention; owner loss never turns OFF on.
func (c *Controller) Reconcile(observation Observation) Action {
	c.gate <- struct{}{}
	defer func() { <-c.gate }()
	c.mu.Lock()
	defer c.mu.Unlock()
	if observation.Generation != 0 && observation.Generation != c.saved.Intent.Generation {
		return NoAction
	}
	c.owner = observation.Owner
	if c.saved.PendingCancel {
		if observation.Owner == "" || observation.Owner == c.saved.ActivationOwner {
			return NoAction
		}
		next := c.saved
		next.PendingCancel = false
		if err := saveIntent(c.path, next); err != nil {
			c.state = State{Generation: next.Intent.Generation, Phase: Failed, Error: err.Error()}
			return NoAction
		}
		c.saved = next
	}

	if observation.Owner != "" && c.saved.ActivationOwner != "" && observation.Owner != c.saved.ActivationOwner {
		next := c.saved
		next.Intent.Generation++
		next.ActivationOwner = observation.Owner
		next.ActivationGeneration = 0
		if err := saveIntent(c.path, next); err != nil {
			c.state = State{Generation: next.Intent.Generation, Phase: Failed, Error: err.Error()}
			return NoAction
		}
		c.saved = next
		c.state = State{Generation: next.Intent.Generation, Phase: Connecting}
		if observation.EngineReady {
			return StopEngine
		}
	}
	if !observation.EngineReady && c.state.Phase == Connected {
		c.state.Phase = Connecting
	}
	if observation.Owner == "" {
		return NoAction
	}
	if !c.saved.Intent.Connected {
		if observation.VPNActive || observation.VPNConnecting {
			return Deactivate
		}
		if observation.EngineReady {
			return StopEngine
		}
		c.state.Phase = Disconnected
		return NoAction
	}
	if observation.VPNConnecting {
		return NoAction
	}
	if observation.VPNActive {
		if !observation.EngineReady {
			return Deactivate
		}
		c.state.Phase = Connected
		return NoAction
	}
	if c.saved.ActivationGeneration != 0 {
		return NoAction
	}
	if observation.NeedsLogin || c.state.Phase == NeedsLogin {
		c.state.Phase = NeedsLogin
		return NoAction
	}
	if observation.EngineReady || observation.EngineAvailable {
		return Activate
	}
	return NoAction
}
func readyInterfaceAddress(expected string, addresses []string) bool {
	expectedAddress, err := netip.ParseAddr(expected)
	if err != nil {
		prefix, prefixErr := netip.ParsePrefix(expected)
		if prefixErr != nil {
			return false
		}
		expectedAddress = prefix.Addr()
	}
	for _, address := range addresses {
		prefix, err := netip.ParsePrefix(address)
		if err == nil && prefix.Addr() == expectedAddress {
			return true
		}
	}
	return false
}
