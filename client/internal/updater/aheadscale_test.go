package updater

import (
	"context"
	"testing"
)

func TestAheadscalePreviewDoesNotStartUpstreamFetcher(t *testing.T) {
	// No update provider is configured: starting a preview must not touch one.
	manager := &Manager{}
	manager.Start(context.Background())
	if manager.cancel != nil {
		t.Fatal("preview started an upstream update worker")
	}
}
