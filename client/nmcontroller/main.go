//go:build linux && !android

// Системная служба полного Desktop-пакета. Не является вторым VPN-движком.
package main

import (
	"context"
	"fmt"
	"github.com/netbirdio/netbird/client/internal/daemonaddr"
	"github.com/netbirdio/netbird/client/internal/linuxvpn"
	"github.com/netbirdio/netbird/client/internal/profilemanager"
	"github.com/netbirdio/netbird/client/proto"
	"google.golang.org/grpc"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func main() {
	if err := run(); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}
func run() error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("контроллер Aheadscale требует root")
	}
	if len(os.Args) > 1 {
		if len(os.Args) != 2 || os.Args[1] != "--remove-profile" {
			return fmt.Errorf("неизвестная операция контроллера Aheadscale")
		}
		manager, err := linuxvpn.NewInstalledManager()
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return manager.RemoveProfile(ctx)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	target, options := daemonaddr.DialTarget("unix:///var/run/aheadscale.sock")
	options = append(options, grpc.WithBlock())
	start, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	connection, err := grpc.DialContext(start, target, options...)
	if err != nil {
		return fmt.Errorf("служба Aheadscale недоступна: %w", err)
	}
	defer connection.Close()
	daemon := proto.NewDaemonServiceClient(connection)
	features, err := daemon.GetFeatures(start, &proto.GetFeaturesRequest{})
	if err != nil {
		return err
	}
	if !features.GetNativeNetworkmanager() || !features.GetDisableProfiles() {
		return fmt.Errorf("полный Desktop native-режим службы Aheadscale не включён")
	}
	config, err := daemonConfig(start, daemon)
	if err != nil {
		return err
	}
	if config.GetInterfaceName() != "Aheadscale" {
		return fmt.Errorf("интерфейс службы не совпадает с собственным профилем Aheadscale")
	}
	manager, err := linuxvpn.NewInstalledManager()
	if err != nil {
		return err
	}
	if manager.ManagementURL() != config.GetManagementUrl() {
		return fmt.Errorf("адрес службы не совпадает с установленным профилем Aheadscale")
	}
	if err = manager.EnsureProfile(start); err != nil {
		return err
	}
	boot, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		return err
	}
	controller, err := linuxvpn.NewController("/run/aheadscale/networkmanager-state.json", strings.TrimSpace(string(boot)), !config.GetDisableAutoConnect())
	if err != nil {
		return err
	}
	controller.Configure(manager, daemon)
	return controller.Run(ctx)
}

func daemonConfig(ctx context.Context, daemon proto.DaemonServiceClient) (*proto.GetConfigResponse, error) {
	return daemon.GetConfig(ctx, &proto.GetConfigRequest{ProfileName: profilemanager.DefaultProfileName})
}
