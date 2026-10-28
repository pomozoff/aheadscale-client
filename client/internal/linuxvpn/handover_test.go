//go:build linux && !android

package linuxvpn

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStandaloneHandoverPreservesOffOnlyThisBoot(t *testing.T) {
	p := filepath.Join(t.TempDir(), "handover")
	if err := os.WriteFile(p, []byte(`{"version":1,"boot_id":"boot","connected":false}`), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		boot string
		want bool
	}{{"boot", false}, {"next", true}} {
		on, err := StandaloneBootConnect(p, tc.boot)
		if err != nil || on != tc.want {
			t.Fatalf("%s=%v %v", tc.boot, on, err)
		}
	}
	if err := ClearStandaloneHandover(p); err != nil {
		t.Fatal(err)
	}
	on, err := StandaloneBootConnect(p, "boot")
	if err != nil || !on {
		t.Fatal("explicit user Up did not release handover")
	}
	if err = os.WriteFile(p, []byte(`{"version":1,"boot_id":"boot","connected":false}`), 0666); err != nil {
		t.Fatal(err)
	}
	if _, err = StandaloneBootConnect(p, "boot"); err == nil {
		t.Fatal("accepted unsafe marker")
	}
}
