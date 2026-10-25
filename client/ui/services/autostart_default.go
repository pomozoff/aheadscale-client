//go:build (!windows || !windowsvpnprototype) && !android && !ios && !freebsd && !js

package services

import "github.com/wailsapp/wails/v3/pkg/application"

func newAutostartBackend(mgr *application.AutostartManager) autostartBackend { return mgr }
