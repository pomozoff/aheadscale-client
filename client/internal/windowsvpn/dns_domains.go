package windowsvpn

import (
	"fmt"
	"net/netip"
	"strconv"
	"strings"
)

// NativeDNSDomains returns only the private forward zone and the reverse zone
// for a byte-aligned IPv4 mesh. Broader and IPv6 policies require a new session.
func NativeDNSDomains(prefix netip.Prefix) ([]string, error) {
	if !prefix.IsValid() || !prefix.Addr().Is4() || prefix.Bits() == 0 || prefix.Bits()%8 != 0 {
		return nil, fmt.Errorf("native VPN DNS requires a byte-aligned IPv4 mesh prefix")
	}
	address := prefix.Masked().Addr().As4()
	labels := make([]string, 0, prefix.Bits()/8+2)
	for index := prefix.Bits()/8 - 1; index >= 0; index-- {
		labels = append(labels, strconv.Itoa(int(address[index])))
	}
	labels = append(labels, "in-addr", "arpa")
	return []string{"aheadscale.internal", strings.Join(labels, ".")}, nil
}
