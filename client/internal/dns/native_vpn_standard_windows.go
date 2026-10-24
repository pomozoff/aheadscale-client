//go:build windows && !windowsvpnprototype

package dns

func nativeVPNPlatform() bool { return false }
