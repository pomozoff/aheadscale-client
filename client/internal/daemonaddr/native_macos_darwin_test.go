//go:build darwin && macosvpnprototype

package daemonaddr

import (
	"path/filepath"
	"testing"
)

func TestNativeMacOSAddressNeverDiscoversLegacySocket(t *testing.T) {
	original := scanDir
	setScanDir(t.TempDir())
	t.Cleanup(func() { setScanDir(original) })
	createSockFile(t, filepath.Join(scanDir, "legacy.sock"))
	native := NativeMacOSAddress("unix:///var/run/aheadscale.sock")
	if got := ResolveUnixDaemonAddr(native); got != native {
		t.Fatalf("native attached to unrelated socket: %s", got)
	}
}
