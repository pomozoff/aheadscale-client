//go:build !android && !ios && !freebsd && !js

package services

import (
	"context"
	"errors"
	"fmt"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type autostartBackend interface {
	IsEnabled() (bool, error)
	Enable() error
	Disable() error
}

// Autostart controls the platform startup registration without duplicating it in preferences.
type Autostart struct {
	mgr autostartBackend
}

// NewAutostart selects the platform backend for the supplied desktop manager.
func NewAutostart(mgr *application.AutostartManager) *Autostart {
	return &Autostart{mgr: newAutostartBackend(mgr)}
}

func (a *Autostart) Supported(_ context.Context) bool {
	_, err := a.mgr.IsEnabled()
	return !errors.Is(err, application.ErrAutostartNotSupported)
}

// IsEnabled returns false without error on unsupported platforms.
func (a *Autostart) IsEnabled(_ context.Context) (bool, error) {
	enabled, err := a.mgr.IsEnabled()
	if err != nil {
		if errors.Is(err, application.ErrAutostartNotSupported) {
			return false, nil
		}
		return false, fmt.Errorf("read autostart state: %w", err)
	}
	return enabled, nil
}

// SetEnabled takes effect on the next login, not immediately.
func (a *Autostart) SetEnabled(_ context.Context, enabled bool) error {
	if enabled {
		if err := a.mgr.Enable(); err != nil {
			return fmt.Errorf("enable autostart: %w", err)
		}
		return nil
	}
	if err := a.mgr.Disable(); err != nil {
		return fmt.Errorf("disable autostart: %w", err)
	}
	return nil
}
