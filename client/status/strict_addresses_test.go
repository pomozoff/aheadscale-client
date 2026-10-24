package status

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/netbirdio/netbird/client/anonymize"
)

func TestStrictStatusHidesPeerIPv4(t *testing.T) {
	a := anonymize.NewAnonymizer(anonymize.DefaultAddresses())
	a.SetLevel(anonymize.LevelStrict)
	peer := PeerStateDetailOutput{IP: "100.88.19.208"}
	anonymizePeerDetail(a, &peer)
	require.Equal(t, a.AnonymizeIPString("100.88.19.208"), peer.IP)
	require.NotEqual(t, "100.88.19.208", peer.IP)
}
