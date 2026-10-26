//go:build !linux || android

package dns

// PrepareInterfaceRemoval has no NetworkManager integration on this platform.
func PrepareInterfaceRemoval(string) error { return nil }

// PrepareNetworkManagerInterface has no NetworkManager policy on this platform.
func PrepareNetworkManagerInterface(string) (func() error, error) {
	return func() error { return nil }, nil
}

// ConfigureNetworkManagerInterface has no NetworkManager integration on this platform.
func ConfigureNetworkManagerInterface(string) error { return nil }
