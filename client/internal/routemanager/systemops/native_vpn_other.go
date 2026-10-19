//go:build !windows || !windowsvpnprototype

package systemops

func nativeVPNRouting() bool { return false }
