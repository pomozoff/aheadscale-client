//go:build !ios

package selfroute

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"os/exec"
	"time"

	"golang.org/x/net/route"
	"golang.org/x/sys/unix"

	"github.com/netbirdio/netbird/client/internal/statemanager"
)

type darwinOps struct{}

func newRouteOps() routeOps { return darwinOps{} }

// RegisterState enables the normal daemon startup recovery path.
func RegisterState(sm *statemanager.Manager) { sm.RegisterState(&ShutdownState{}) }

func (darwinOps) interfaceIndex(name string, ip netip.Addr) (int, error) {
	intf, err := net.InterfaceByName(name)
	if err != nil {
		return 0, err
	}
	if !ip.IsValid() {
		return intf.Index, nil
	}
	addrs, err := intf.Addrs()
	if err != nil {
		return 0, err
	}
	for _, addr := range addrs {
		prefix, err := netip.ParsePrefix(addr.String())
		if err == nil && prefix.Addr().Unmap() == ip {
			return intf.Index, nil
		}
	}
	return 0, fmt.Errorf("address %s is not assigned to %s", ip, name)
}

func (darwinOps) lookup(ip netip.Addr) (*routeInfo, error) {
	rib, err := route.FetchRIB(unix.AF_INET, route.RIBTypeRoute, 0)
	if err != nil {
		return nil, fmt.Errorf("fetch local route: %w", err)
	}
	messages, err := route.ParseRIB(route.RIBTypeRoute, rib)
	if err != nil {
		return nil, fmt.Errorf("parse local route: %w", err)
	}
	for _, msg := range messages {
		r, ok := msg.(*route.RouteMessage)
		if !ok || len(r.Addrs) <= unix.RTAX_GATEWAY || r.Flags&unix.RTF_HOST == 0 || r.Flags&unix.RTF_IFSCOPE != 0 {
			continue
		}
		dst, ok := r.Addrs[unix.RTAX_DST].(*route.Inet4Addr)
		if !ok || netip.AddrFrom4(dst.IP) != ip {
			continue
		}
		gateway := netip.Addr{}
		if addr, ok := r.Addrs[unix.RTAX_GATEWAY].(*route.Inet4Addr); ok {
			gateway = netip.AddrFrom4(addr.IP)
		}
		return &routeInfo{destination: ip, gateway: gateway, index: r.Index, host: true,
			local: r.Flags&unix.RTF_LOCAL != 0}, nil
	}
	return nil, errRouteNotFound
}

func runRoute(args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "/sbin/route", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("route: %s: %w", out, err)
	}
	return nil
}

func (darwinOps) move(ip netip.Addr) error {
	return runRoute("-n", "change", "-host", ip.String(), "-ifp", "lo0", "-ifa", ip.String())
}

func (darwinOps) remove(ip netip.Addr) error {
	return runRoute("-n", "delete", "-host", ip.String(), "-ifp", "lo0", "-ifa", ip.String())
}
