//go:build (!darwin || !macosvpnprototype) && !android && !ios && !freebsd && !js

package services

import "context"

func platformVPNStatus(_ context.Context,st Status) Status {return st}
