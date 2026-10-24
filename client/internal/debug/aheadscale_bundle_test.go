package debug

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/netbirdio/netbird/client/anonymize"
	mgmProto "github.com/netbirdio/netbird/shared/management/proto"
)

func TestAheadscaleDebugBundleFilename(t *testing.T) {
	g := NewBundleGenerator(GeneratorDependencies{TempDir: t.TempDir()}, BundleConfig{})
	path, err := g.Generate()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Remove(path) })
	require.Contains(t, filepath.Base(path), "aheadscale.debug.")
}

func TestStrictBundleHidesPeerAddressPrefix(t *testing.T) {
	a := anonymize.NewAnonymizer(anonymize.DefaultAddresses())
	a.SetLevel(anonymize.LevelStrict)
	peer := &mgmProto.PeerConfig{Address: "100.88.158.229/16"}
	anonymizePeerConfig(peer, a)
	require.Equal(t, a.AnonymizeIPString("100.88.158.229/16"), peer.Address)
	require.NotContains(t, peer.Address, "100.88.158.229")
}
