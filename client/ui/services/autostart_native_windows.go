//go:build windows && windowsvpnprototype

package services

import (
	"context"
	"os"
	"sync"
	"time"

	"github.com/netbirdio/netbird/client/internal/windowsstartup"
	log "github.com/sirupsen/logrus"
	"github.com/wailsapp/wails/v3/pkg/application"
)

var startupMigration sync.Once

type nativeAutostart struct {
	manager *windowsstartup.Manager
	err     error
}

func newAutostartBackend(_ *application.AutostartManager) autostartBackend {
	exe, err := os.Executable()
	backend := &nativeAutostart{manager: windowsstartup.New(exe), err: err}
	if err == nil {
		// Конструктор вызывается также до ApplicationStarted. Миграция не задерживает UI.
		startupMigration.Do(func() {
			go func(manager *windowsstartup.Manager) {
				ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()
				if _, err := manager.State(ctx); err != nil {
					log.WithError(err).Warn("initialize Aheadscale startup task")
				}
			}(backend.manager)
		})
	}
	return backend
}
func (a *nativeAutostart) IsEnabled() (bool, error) {
	if a.err != nil {
		return false, a.err
	}
	state, err := a.manager.State(context.Background())
	return state.Enabled, err
}
func (a *nativeAutostart) Enable() error {
	if a.err != nil {
		return a.err
	}
	return a.manager.SetEnabled(context.Background(), true)
}
func (a *nativeAutostart) Disable() error {
	if a.err != nil {
		return a.err
	}
	return a.manager.SetEnabled(context.Background(), false)
}
