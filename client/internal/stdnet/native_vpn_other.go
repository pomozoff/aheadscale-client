//go:build (!windows || !windowsvpnprototype) && (!darwin || !macosvpnprototype)

package stdnet

func nativeVPNInterface(string) bool { return false }
