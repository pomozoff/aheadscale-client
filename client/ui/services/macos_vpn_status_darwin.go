//go:build darwin && macosvpnprototype

package services

import "github.com/netbirdio/netbird/client/internal/macosvpn"

func reconcileMacOSStatus(st Status,_ macosvpn.ControlSnapshot,_ error) Status {return st}
