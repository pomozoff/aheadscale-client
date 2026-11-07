//go:build !ios && !android

package cmd

import "fmt"

func serviceControlError(operation string, err error, goos string, euid int) error {
	return fmt.Errorf("%s service: %w", operation, err)
}
