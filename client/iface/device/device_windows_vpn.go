//go:build windows && windowsvpnprototype

package device

import (
	"context"
	"fmt"
	"github.com/netbirdio/netbird/client/iface/bind"
	"github.com/netbirdio/netbird/client/iface/configurer"
	"github.com/netbirdio/netbird/client/iface/udpmux"
	"github.com/netbirdio/netbird/client/iface/wgaddr"
	"github.com/netbirdio/netbird/client/internal/windowsvpn"
	nbnet "github.com/netbirdio/netbird/client/net"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sys/windows"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun/netstack"
	"golang.zx2c4.com/wireguard/windows/tunnel/winipcfg"
	"net/netip"
)

type VpnTunDevice struct {
	ctx            context.Context
	name           string
	address        wgaddr.Address
	port           int
	key            string
	mtu            uint16
	iceBind        *bind.ICEBind
	device         *device.Device
	filteredDevice *FilteredDevice
	udpMux         *udpmux.UniversalUDPMuxDefault
	configurer     WGConfigurer
	luid           winipcfg.LUID
}

func NewVpnTunDevice(ctx context.Context, address wgaddr.Address, port int, key string, iceBind *bind.ICEBind) *VpnTunDevice {
	return &VpnTunDevice{ctx: ctx, name: windowsvpn.ProfileName, address: address, port: port, key: key, mtu: 1280, iceBind: iceBind}
}
func (t *VpnTunDevice) Create() (WGConfigurer, error) {
	dnsAddress, err := nbnet.GetLastIPFromNetwork(t.address.Network, 1)
	if err != nil {
		return nil, err
	}
	conf := windowsvpn.SessionConfiguration{Address: t.address.Prefix(), MTU: 1280, Routes: []netip.Prefix{t.address.Prefix().Masked()}, DNSDomains: []string{"aheadscale.internal"}, DNSServers: []netip.Addr{dnsAddress}}
	conn, err := windowsvpn.AcceptSession(t.ctx, conf)
	if err != nil {
		return nil, err
	}
	raw, err := windowsvpn.NewConnTun(conn, t.name, 1280)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	adapters, err := winipcfg.GetAdaptersAddresses(windows.AF_INET, 0)
	if err != nil {
		_ = raw.Close()
		return nil, err
	}
	var matches int
	for _, adapter := range adapters {
		for address := adapter.FirstUnicastAddress; address != nil; address = address.Next {
			if address.Address.IP().Equal(t.address.IP.AsSlice()) {
				t.luid = adapter.LUID
				t.name = adapter.FriendlyName()
				matches++
				break
			}
		}
	}
	if matches != 1 {
		_ = raw.Close()
		return nil, fmt.Errorf("native VPN address must belong to exactly one adapter, got %d", matches)
	}
	windowsvpn.SetAdapterName(t.name)
	t.filteredDevice = newDeviceFilter(raw)
	t.device = device.NewDevice(t.filteredDevice, t.iceBind, &device.Logger{
		Verbosef: func(format string, args ...any) { log.Debugf("native WireGuard: "+format, args...) },
		Errorf:   func(format string, args ...any) { log.Errorf("native WireGuard: "+format, args...) },
	})
	t.configurer = configurer.NewUSPConfigurerNoUAPI(t.device, t.name, t.iceBind.ActivityRecorder())
	if err = t.configurer.ConfigureInterface(t.key, t.port); err != nil {
		_ = t.Close()
		return nil, err
	}
	return t.configurer, nil
}
func (t *VpnTunDevice) Up() (*udpmux.UniversalUDPMuxDefault, error) {
	if err := t.device.Up(); err != nil {
		return nil, err
	}
	mux, err := t.iceBind.GetICEMux()
	if err == nil {
		t.udpMux = mux
	}
	return mux, err
}
func (t *VpnTunDevice) UpdateAddr(address wgaddr.Address) error {
	if address.Prefix() != t.address.Prefix() {
		return fmt.Errorf("native VPN address change requires a new channel")
	}
	return nil
}
func (t *VpnTunDevice) Close() error {
	if t.configurer != nil {
		t.configurer.Close()
	}
	if t.device != nil {
		t.device.Close()
	}
	if t.udpMux != nil {
		return t.udpMux.Close()
	}
	return nil
}
func (t *VpnTunDevice) WgAddress() wgaddr.Address       { return t.address }
func (t *VpnTunDevice) MTU() uint16                     { return t.mtu }
func (t *VpnTunDevice) DeviceName() string              { return t.name }
func (t *VpnTunDevice) FilteredDevice() *FilteredDevice { return t.filteredDevice }
func (t *VpnTunDevice) Device() *device.Device          { return t.device }
func (t *VpnTunDevice) GetNet() *netstack.Net           { return nil }
func (t *VpnTunDevice) GetICEBind() EndpointManager     { return t.iceBind }
func (t *VpnTunDevice) GetInterfaceGUIDString() (string, error) {
	if t.luid == 0 {
		return "", fmt.Errorf("native VPN adapter not ready")
	}
	guid, err := t.luid.GUID()
	if err != nil {
		return "", err
	}
	return guid.String(), nil
}
