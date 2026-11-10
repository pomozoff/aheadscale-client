//go:build darwin && !ios && macosvpnprototype

package stdnet

import "strings"

func nativeVPNInterface(name string) bool { return strings.HasPrefix(name,"utun") }
