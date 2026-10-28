//go:build !ios && !android

package cmd

import "runtime"

var nativeNetworkManager bool

func init() {
	if runtime.GOOS == "linux" {
		serviceCmd.PersistentFlags().BoolVar(&nativeNetworkManager, "native-networkmanager", false, "Передать управление Desktop VPN системному NetworkManager Aheadscale")
	}
}
