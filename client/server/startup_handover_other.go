//go:build !linux || android

package server

func clearStartupHandover() error { return nil }
