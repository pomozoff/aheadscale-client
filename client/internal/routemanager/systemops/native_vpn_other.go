//go:build !linux && (!windows || !windowsvpnprototype) && (!darwin || !macosvpnprototype)

package systemops

func nativeVPNRouting() bool { return false }
