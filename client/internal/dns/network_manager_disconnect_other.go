//go:build !linux || android

package dns

import "context"

// WatchNetworkManagerDisconnect has no NetworkManager integration on this platform.
func WatchNetworkManagerDisconnect(context.Context, string) (<-chan struct{}, error) { return nil, nil }
