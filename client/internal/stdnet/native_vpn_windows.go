//go:build windows && windowsvpnprototype

package stdnet

import "github.com/netbirdio/netbird/client/internal/windowsvpn"

func nativeVPNInterface(name string) bool { return name != "" && name == windowsvpn.AdapterName() }
