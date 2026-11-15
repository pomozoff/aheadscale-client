//go:build darwin && macosvpnprototype

package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/netbirdio/netbird/client/internal/macosvpn"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func nativeDesktopCommand(cmd *cobra.Command, on bool) (bool, error) {
	// The authenticated root bridge alone starts the engine through the existing
	// RPC. Intercepting that root subprocess would recursively start the profile.
	if os.Geteuid() == 0 {
		return false, nil
	}
	var unsupported []string
	cmd.Flags().Visit(func(f *pflag.Flag) {
		switch f.Name {
		case "daemon-addr", "log-level", "log-file":
		default:
			unsupported = append(unsupported, f.Name)
		}
	})
	if len(unsupported) > 0 {
		return true, fmt.Errorf("системный VPN использует установленный профиль; параметры %v недоступны", unsupported)
	}
	if daemonAddr != macosvpn.DaemonAddress {
		return true, errors.New("native VPN требует штатный адрес службы")
	}
	ctx, cancel := signal.NotifyContext(cmd.Context(), os.Interrupt)
	defer cancel()
	action := "disconnect"
	if on {
		action = "connect"
	}
	if _, err := macosvpn.RunControl(ctx, action); err != nil {
		return true, err
	}
	if on {
		ready, stop := context.WithTimeout(ctx, 10*time.Second)
		defer stop()
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			state, err := getStatus(ready, false, false)
			if err == nil && state.GetStatus() == "Connected" {
				break
			}
			select {
			case <-ready.Done():
				return true, errors.New("системный VPN включён, но движок Aheadscale ещё не готов")
			case <-ticker.C:
			}
		}
		cmd.Println("Connected")
	} else {
		cmd.Println("Disconnected")
	}
	return true, nil
}
