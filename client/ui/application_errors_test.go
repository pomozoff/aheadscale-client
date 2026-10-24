//go:build !android && !ios && !freebsd && !js

package main

import (
	"bytes"
	"errors"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

func TestApplicationErrorsReachClientLog(t *testing.T) {
	app := newApplication(func() {})
	handler := app.Config().ErrorHandler
	require.NotNil(t, handler, "WebView startup failures must reach the client log")

	var output bytes.Buffer
	logger := logrus.StandardLogger()
	previous := logger.Out
	logger.SetOutput(&output)
	t.Cleanup(func() { logger.SetOutput(previous) })

	handler(errors.New("webview startup probe"))
	require.Contains(t, output.String(), "webview startup probe")
}
