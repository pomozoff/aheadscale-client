//go:build darwin && !ios && macosvpnprototype

package internal

func platformOwnsWGIfaceLifecycle() bool { return true }
