package configs

// SelfUpdatesEnabled reports whether a signed Aheadscale update feed is available.
// Preview builds must never install upstream packages over the Aheadscale client.
func SelfUpdatesEnabled() bool {
	return false
}
