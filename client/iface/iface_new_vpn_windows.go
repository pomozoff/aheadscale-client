//go:build windows && windowsvpnprototype

package iface

import (
	"github.com/netbirdio/netbird/client/iface/bind"
	"github.com/netbirdio/netbird/client/iface/device"
	"github.com/netbirdio/netbird/client/iface/netstack"
	"github.com/netbirdio/netbird/client/iface/wgproxy"
)

func NewWGIFace(opts WGIFaceOpts) (*WGIface, error) {
	opts.MTU = 1280
	iceBind := bind.NewICEBind(opts.TransportNet, opts.Address, opts.MTU)
	var tun WGTunDevice
	if netstack.IsEnabled() {
		tun = device.NewNetstackDevice(opts.IFaceName, opts.Address, opts.WGPort, opts.WGPrivKey, opts.MTU, iceBind, netstack.ListenAddr())
	} else {
		tun = device.NewVpnTunDevice(opts.Context, opts.Address, opts.WGPort, opts.WGPrivKey, iceBind)
	}
	return &WGIface{userspaceBind: true, tun: tun, wgProxyFactory: wgproxy.NewUSPFactory(iceBind, opts.MTU)}, nil
}
