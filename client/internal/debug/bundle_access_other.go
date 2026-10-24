//go:build !windows

package debug

import "github.com/netbirdio/netbird/client/internal/ipcauth"

func PrepareBundleForCaller(_ string, _ ipcauth.Identity, _ bool) error {
	return nil
}

func ExportBundle(path string) (string, error) {
	return path, nil
}
