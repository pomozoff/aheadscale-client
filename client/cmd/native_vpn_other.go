//go:build !linux || android

package cmd

import "github.com/spf13/cobra"

func nativeDesktopCommand(_ *cobra.Command, _ bool) (bool, error) { return false, nil }
