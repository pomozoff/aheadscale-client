//go:build (!windows || !windowsvpnprototype) && (!darwin || !macosvpnprototype)

package internal

func platformOwnsWGIfaceLifecycle() bool { return false }
