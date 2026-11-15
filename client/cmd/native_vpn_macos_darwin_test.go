//go:build darwin && macosvpnprototype

package cmd

import (
	"github.com/spf13/cobra"
	"testing"
)

func TestMacOSNativeUpRejectsAlternateConfiguration(t *testing.T) {
	c := &cobra.Command{}
	c.Flags().String("management-url", "", "")
	if err := c.Flags().Set("management-url", "https://other.invalid"); err != nil {
		t.Fatal(err)
	}
	handled, err := nativeDesktopCommand(c, true)
	if !handled || err == nil {
		t.Fatalf("native CLI fell through to direct Up: handled=%v err=%v", handled, err)
	}
}
