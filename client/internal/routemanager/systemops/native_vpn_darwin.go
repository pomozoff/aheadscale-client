//go:build darwin && !ios && macosvpnprototype

package systemops

func nativeVPNRouting() bool { return true }
