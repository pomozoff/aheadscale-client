//go:build !android && !ios && !freebsd && !js

package services

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// Wails exposes a window before its WebView2 controller exists. Its native
// background setter panics during that interval, as this external-window fake does.
type pendingThemeWindow struct {
	ready   bool
	colours []application.RGBA
	hooks   map[events.WindowEventType]func(*application.WindowEvent)
}

func (w *pendingThemeWindow) ID() uint { return 1 }

func (w *pendingThemeWindow) OnWindowEvent(event events.WindowEventType, callback func(*application.WindowEvent)) func() {
	if w.hooks == nil {
		w.hooks = make(map[events.WindowEventType]func(*application.WindowEvent))
	}
	w.hooks[event] = callback
	return func() { delete(w.hooks, event) }
}

func (w *pendingThemeWindow) SetBackgroundColour(colour application.RGBA) application.Window {
	if !w.ready {
		panic("WebView2 controller is not ready")
	}
	w.colours = append(w.colours, colour)
	return nil
}

func TestThemeDoesNotTouchWindowBeforeRuntimeReady(t *testing.T) {
	theme := &Theme{}
	window := &pendingThemeWindow{}
	colour := windowBackgroundColour(false)
	theme.watchWindow(window, func() { theme.applyWindowBackground(window, colour) })
	require.NotPanics(t, func() { theme.applyWindowBackground(window, colour) })
	require.Empty(t, window.colours)

	window.ready = true
	window.hooks[events.Common.WindowRuntimeReady](nil)
	require.Equal(t, []application.RGBA{colour}, window.colours)
}

func TestThemeRuntimeReadyUsesLatestAppearance(t *testing.T) {
	theme := &Theme{}
	window := &pendingThemeWindow{}
	colour := windowBackgroundColour(false)
	theme.watchWindow(window, func() { theme.applyWindowBackground(window, colour) })
	require.NotPanics(t, func() { theme.applyWindowBackground(window, colour) })
	colour = windowBackgroundColour(true)
	require.NotPanics(t, func() { theme.applyWindowBackground(window, colour) })

	window.ready = true
	window.hooks[events.Common.WindowRuntimeReady](nil)
	require.Equal(t, []application.RGBA{windowBackgroundColour(true)}, window.colours)
	colour = windowBackgroundColour(false)
	theme.applyWindowBackground(window, colour)
	require.Equal(t, []application.RGBA{windowBackgroundColour(true), colour}, window.colours)
}

func TestThemeClosedWindowCannotBeRevivedByLateRuntimeReady(t *testing.T) {
	theme := &Theme{}
	window := &pendingThemeWindow{}
	colour := windowBackgroundColour(true)
	theme.watchWindow(window, func() { theme.applyWindowBackground(window, colour) })
	window.hooks[events.Common.WindowClosing](nil)
	window.hooks[events.Common.WindowRuntimeReady](nil)
	require.NotPanics(t, func() { theme.applyWindowBackground(window, colour) })
	require.Empty(t, window.colours)
}
