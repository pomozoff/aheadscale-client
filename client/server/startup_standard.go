//go:build (!linux || android) && (!windows || !windowsvpnprototype) && (!darwin || !macosvpnprototype)

package server

func connectOnServiceStart() bool { return true }
