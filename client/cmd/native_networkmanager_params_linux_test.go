//go:build linux && !android

package cmd

import (
	"github.com/netbirdio/netbird/client/configs"
	"testing"
)

func TestNativeServiceParamsPreserveMode(t *testing.T) {
	originalDir, originalMode := configs.StateDir, nativeNetworkManager
	flag := serviceCmd.PersistentFlags().Lookup("native-networkmanager")
	if flag == nil {
		t.Fatal("native service flag missing")
	}
	originalChanged := flag.Changed
	t.Cleanup(func() {
		configs.StateDir = originalDir
		nativeNetworkManager = originalMode
		flag.Changed = originalChanged
	})
	configs.StateDir = t.TempDir()
	flag.Changed = false
	nativeNetworkManager = true
	if err := saveServiceParams(currentServiceParams()); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadServiceParams()
	if err != nil {
		t.Fatal(err)
	}
	nativeNetworkManager = false
	applyServiceParams(serviceCmd, loaded)
	if !nativeNetworkManager {
		t.Fatal("service restart lost saved native mode")
	}
	flag.Changed = true
	nativeNetworkManager = false
	applyServiceParams(serviceCmd, loaded)
	if nativeNetworkManager {
		t.Fatal("explicit CLI package standalone mode was overwritten")
	}
}
