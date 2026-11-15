//go:build darwin && macosvpnprototype

package daemonaddr

const NativeMacOS = true

// Native candidates must never discover or silently attach to the legacy engine.
func NativeMacOSAddress(addr string) string {
	if addr == "unix:///var/run/aheadscale.sock" {
		return "unix:///var/run/aheadscale-native-prototype.sock"
	}
	return addr
}
