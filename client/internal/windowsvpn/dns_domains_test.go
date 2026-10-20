package windowsvpn

import (
	"net/netip"
	"slices"
	"testing"
)

func TestNativeDNSDomainsFollowMeshPrefix(t *testing.T) {
	for _, tc := range []struct{ prefix, reverse string }{
		{"100.88.77.246/16", "88.100.in-addr.arpa"},
		{"10.2.3.4/24", "3.2.10.in-addr.arpa"},
	} {
		t.Run(tc.prefix, func(t *testing.T) {
			domains, err := NativeDNSDomains(netip.MustParsePrefix(tc.prefix))
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(domains, []string{"aheadscale.internal", tc.reverse}) {
				t.Fatalf("domains=%v", domains)
			}
		})
	}
}
func TestNativeDNSDomainsRejectUnsupportedPrefixes(t *testing.T) {
	for _, prefix := range []string{"0.0.0.0/0", "100.88.0.0/21", "fd00::/64"} {
		if _, err := NativeDNSDomains(netip.MustParsePrefix(prefix)); err == nil {
			t.Fatalf("accepted %s", prefix)
		}
	}
}
