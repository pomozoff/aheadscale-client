//go:build linux && !android

package dns

import (
	"context"
	"testing"
)

func TestNativeDNSUsesResolvedAndCleansOwnLink(t *testing.T) {
	if NativeNetworkManagerEnabled(context.Background()) {
		t.Fatal("standalone became native")
	}
	if !NativeNetworkManagerEnabled(WithNativeNetworkManager(context.Background())) {
		t.Fatal("lost installed native mode")
	}
	for _, tc := range []struct{ resolved, nss, stub, allowed bool }{{true, true, false, true}, {true, false, true, true}, {false, true, true, false}, {true, false, false, false}} {
		manager, err := nativeDNSManager(tc.resolved, tc.nss, tc.stub)
		if tc.allowed {
			if err != nil || manager != systemdManager {
				t.Fatal("native DNS must use resolved")
			}
		} else if err == nil {
			t.Fatal("silently accepted unsupported global resolver")
		}
	}
}
