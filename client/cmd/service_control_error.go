//go:build !ios && !android

package cmd

import "fmt"

func serviceControlError(operation string, err error, goos string, euid int) error {
	if goos == "linux" && euid > 0 {
		return fmt.Errorf("%s service: %w; administrative privileges may be required; retry with sudo aheadscale service %s", operation, err, operation)
	}
	return fmt.Errorf("%s service: %w", operation, err)
}
