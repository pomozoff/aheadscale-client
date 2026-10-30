package dns

import "context"

type nativeNetworkManagerContextKey struct{}

// WithNativeNetworkManager carries service mode through engine DNS without process globals.
func WithNativeNetworkManager(ctx context.Context) context.Context {
	return context.WithValue(ctx, nativeNetworkManagerContextKey{}, true)
}
func NativeNetworkManagerEnabled(ctx context.Context) bool {
	enabled, _ := ctx.Value(nativeNetworkManagerContextKey{}).(bool)
	return enabled
}
