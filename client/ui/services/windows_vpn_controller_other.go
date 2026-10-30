//go:build !linux && (!windows || !windowsvpnprototype) && !android && !ios && !freebsd && !js

package services

func newPlatformVPNController(_ DaemonConn) nativeVPNController { return nil }
