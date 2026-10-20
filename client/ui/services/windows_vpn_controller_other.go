//go:build (!windows || !windowsvpnprototype) && !android && !ios && !freebsd && !js

package services

func newPlatformVPNController() nativeVPNController { return nil }
