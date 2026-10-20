//go:build !windows || !windowsvpnprototype

package server

func connectOnServiceStart() bool { return true }
