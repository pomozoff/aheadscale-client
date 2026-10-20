package iface

import "fmt"

// GetInterfaceGUIDString returns the GUID supplied by the active Windows backend.
func (w *WGIface) GetInterfaceGUIDString() (string, error) {
	provider, ok := w.tun.(interface{ GetInterfaceGUIDString() (string, error) })
	if !ok {
		return "", fmt.Errorf("Windows interface %T does not provide a GUID", w.tun)
	}
	return provider.GetInterfaceGUIDString()
}
