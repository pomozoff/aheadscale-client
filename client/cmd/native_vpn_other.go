//go:build (!linux || android) && (!darwin || !macosvpnprototype)

package cmd

import "github.com/spf13/cobra"

func nativeDesktopCommand(_ *cobra.Command, _ bool) (bool, error) { return false, nil }
