//go:build !darwin || !macosvpnprototype

package daemonaddr

func NativeMacOSAddress(addr string) string {return addr}
