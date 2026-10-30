//go:build linux && !android

package cmd

import (
	"context"
	"errors"
	"fmt"
	"github.com/netbirdio/netbird/client/internal/linuxvpn"
	"github.com/netbirdio/netbird/client/proto"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"os"
	"os/signal"
	"time"
)

type desktopVPNManager interface {
	Connect(context.Context) error
	SetConnected(context.Context, bool) error
}

func routeDesktopVPN(ctx context.Context, daemon proto.DaemonServiceClient, factory func() (desktopVPNManager, error), on bool) (bool, error) {
	features, err := daemon.GetFeatures(ctx, &proto.GetFeaturesRequest{})
	if err != nil {
		return true, err
	}
	if !features.GetNativeNetworkmanager() {
		return false, nil
	}
	manager, err := factory()
	if err != nil {
		return true, err
	}
	if on {
		return true, manager.Connect(ctx)
	}
	return true, manager.SetConnected(ctx, false)
}
func nativeDesktopCommand(cmd *cobra.Command, on bool) (bool, error) {
	// Standalone packages do not ship this descriptor. Avoid probing a service
	// for foreground/headless CLI runs; a corrupt Desktop descriptor fails closed.
	if _, err := os.Lstat(linuxvpn.PublicReceiptPath); errors.Is(err, os.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return true, err
	}
	ctx, cancel := signal.NotifyContext(cmd.Context(), os.Interrupt)
	defer cancel()
	connection, err := DialClientGRPCServer(ctx, daemonAddr)
	if err != nil {
		if _, installed := os.Stat(linuxvpn.PublicReceiptPath); installed == nil {
			return true, fmt.Errorf("служба Desktop Aheadscale недоступна: %w", err)
		}
		return false, nil
	}
	defer connection.Close()
	daemon := proto.NewDaemonServiceClient(connection)
	queryCtx, stop := context.WithTimeout(ctx, 5*time.Second)
	features, err := daemon.GetFeatures(queryCtx, &proto.GetFeaturesRequest{})
	stop()
	if err != nil {
		return true, err
	}
	if !features.GetNativeNetworkmanager() {
		return false, nil
	}
	var unsupported []string
	cmd.Flags().Visit(func(flag *pflag.Flag) {
		switch flag.Name {
		case "daemon-addr", "log-level", "log-file":
		default:
			unsupported = append(unsupported, flag.Name)
		}
	})
	if len(unsupported) > 0 {
		return true, fmt.Errorf("Desktop VPN использует установленный профиль; параметры %v доступны в самостоятельном CLI-пакете", unsupported)
	}
	handled, err := routeDesktopVPN(ctx, daemon, func() (desktopVPNManager, error) { return linuxvpn.NewDesktopManager() }, on)
	if on && errors.Is(err, context.Canceled) {
		cleanup, stop := context.WithTimeout(context.Background(), 15*time.Second)
		defer stop()
		if manager, loadErr := linuxvpn.NewDesktopManager(); loadErr == nil {
			err = errors.Join(err, manager.SetConnected(cleanup, false))
		}
	}
	if err == nil && handled {
		if on {
			cmd.Println("Connected")
		} else {
			cmd.Println("Disconnected")
		}
	}
	return handled, err
}
