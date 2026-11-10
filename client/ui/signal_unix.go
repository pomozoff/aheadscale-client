//go:build !linux && !windows && !android && !ios && !freebsd && !js

package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	log "github.com/sirupsen/logrus"
)

// listenForShowSignal retains the non-Linux Unix wake-up interface.
// Linux leaves this signal to WebKit and uses SingleInstance/tray instead.
func listenForShowSignal(ctx context.Context, tray *Tray) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGUSR1)

	go func() {
		for {
			select {
			case <-ctx.Done():
				signal.Stop(sigCh)
				return
			case <-sigCh:
				log.Debug("SIGUSR1 received, showing window")
				tray.ShowWindow()
			}
		}
	}()
}
