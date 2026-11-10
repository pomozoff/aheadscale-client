//go:build linux && !android

package main

import "context"

// WebKit owns its thread-suspension signal. Opening the Linux window uses the
// existing launcher/SingleInstance/tray path, never a process signal handler.
func listenForShowSignal(_ context.Context, _ *Tray) {}
