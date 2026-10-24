//go:build !windows || !windowsvpnprototype

package stdnet

func nativeVPNInterface(string) bool { return false }
