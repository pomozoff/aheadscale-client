//go:build !darwin || ios || !macosvpnprototype

package internal

func nativeMacOSVPN() bool { return false }
