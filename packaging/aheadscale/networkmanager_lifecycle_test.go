//go:build linux

package aheadscale

import (
	"os/exec"
	"testing"
)

func TestNetworkManagerLifecycle(t *testing.T) {
	cmd := exec.Command("python3", "test_networkmanager_lifecycle.py")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("lifecycle: %v\n%s", err, out)
	}
}
