//go:build !windows || !windowsvpnprototype

package internal

func platformOwnsWGIfaceLifecycle() bool { return false }
