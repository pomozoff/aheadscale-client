//go:build linux && !android

package linuxvpn

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestNativeRuntimeRequiresRoot(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("requires unprivileged process")
	}
	c, err := NewController(filepath.Join(t.TempDir(), "state"), "boot", false)
	if err != nil {
		t.Fatal(err)
	}
	if err = c.Run(context.Background()); err == nil {
		t.Fatal("started system controller without root")
	}
}
