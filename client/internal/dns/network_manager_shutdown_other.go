//go:build !linux || android

package dns

// PrepareInterfaceRemoval has no NetworkManager integration on this platform.
func PrepareInterfaceRemoval(string) error { return nil }
