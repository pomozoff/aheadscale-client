//go:build !darwin || !macosvpnprototype

package daemonaddr

const NativeMacOS = false

func NativeMacOSAddress(addr string) string { return addr }
