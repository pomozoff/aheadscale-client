package cmd

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestAheadscaleIgnoresOtherProductConfigEnvironment(t *testing.T) {
	cmd := &cobra.Command{Use: "aheadscale"}
	path := cmd.PersistentFlags().String("config", "/etc/aheadscale/config.json", "config")
	t.Setenv("NB_CONFIG", "/etc/netbird/config.json")
	t.Setenv("WT_CONFIG", "/etc/wiretrustee/config.json")
	SetFlagsFromEnvVars(cmd)
	require.Equal(t, "/etc/aheadscale/config.json", *path)
}
