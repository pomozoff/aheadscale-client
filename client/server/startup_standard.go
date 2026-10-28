//go:build (!linux || android) && (!windows || !windowsvpnprototype)

package server

func connectOnServiceStart() bool { return true }
