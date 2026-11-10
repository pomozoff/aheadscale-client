//go:build darwin && !ios && macosvpnprototype

package server

// The system profile requests engine startup only after provider authentication.
func connectOnServiceStart() bool { return false }
