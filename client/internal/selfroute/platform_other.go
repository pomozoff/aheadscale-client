//go:build !darwin || ios

package selfroute

import (
	"errors"
	"net/netip"

	"github.com/netbirdio/netbird/client/internal/statemanager"
)

// RegisterState is intentionally empty outside desktop macOS.
func RegisterState(*statemanager.Manager) {}

type unsupportedOps struct{}

func newRouteOps() routeOps { return unsupportedOps{} }
func (unsupportedOps) lookup(netip.Addr) (*routeInfo, error) {
	return nil, errors.New("local route recovery requires macOS")
}
func (unsupportedOps) interfaceIndex(string, netip.Addr) (int, error) {
	return 0, errors.New("local route recovery requires macOS")
}
func (unsupportedOps) move(netip.Addr) error {
	return errors.New("local route recovery requires macOS")
}
func (unsupportedOps) remove(netip.Addr) error {
	return errors.New("local route recovery requires macOS")
}
