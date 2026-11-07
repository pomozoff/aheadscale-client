//go:build !ios && !android

package cmd

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestServiceControlError(t *testing.T) {
	cause := errors.New("service manager denied the operation")
	original := fmt.Errorf("exit status 1: %w", cause)
	for _, operation := range []string{"start", "stop", "restart"} {
		t.Run(operation, func(t *testing.T) {
			for _, user := range []struct {
				name string
				goos string
				euid int
				hint bool
			}{
				{"linux-user", "linux", 1000, true},
				{"linux-root", "linux", 0, false},
				{"windows-user", "windows", -1, false},
				{"macos-user", "darwin", 501, false},
			} {
				t.Run(user.name, func(t *testing.T) {
					err := serviceControlError(operation, original, user.goos, user.euid)
					if !errors.Is(err, cause) {
						t.Fatal("service error must preserve its original cause")
					}
					want := operation + " service: " + original.Error()
					if user.hint {
						if !strings.Contains(err.Error(), "sudo aheadscale service "+operation) {
							t.Errorf("Linux user needs an actionable privilege hint, got %q", err)
						}
						if !strings.Contains(err.Error(), original.Error()) {
							t.Errorf("original diagnostic must remain visible, got %q", err)
						}
					} else if err.Error() != want {
						t.Errorf("other platforms/root must keep the diagnostic: got %q, want %q", err, want)
					}
				})
			}
		})
	}
}
