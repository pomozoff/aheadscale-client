//go:build !linux && (!windows || !windowsvpnprototype)

package systemops

func nativeVPNRouting() bool { return false }
