//go:build darwin && !ios && macosvpnprototype

package device

import (
 "context"
 "fmt"
 "net"
 "os"
 "github.com/google/uuid"
 "github.com/netbirdio/netbird/client/iface/bind"
 "github.com/netbirdio/netbird/client/iface/configurer"
 "github.com/netbirdio/netbird/client/iface/udpmux"
 "github.com/netbirdio/netbird/client/iface/wgaddr"
 "github.com/netbirdio/netbird/client/internal/macosvpn"
 "github.com/netbirdio/netbird/client/internal/windowsvpn"
 nbnet "github.com/netbirdio/netbird/client/net"
 log "github.com/sirupsen/logrus"
 "golang.zx2c4.com/wireguard/device"
 "golang.zx2c4.com/wireguard/tun/netstack"
)

// MacVPNDevice uses only the descriptor inherited from the root supervisor.
// It never creates a second utun or invokes ifconfig, route, scutil or a UAPI socket.
type MacVPNDevice struct{
 ctx context.Context;name string;address wgaddr.Address;port int;key string
 iceBind *bind.ICEBind;device *device.Device;filteredDevice *FilteredDevice
 udpMux *udpmux.UniversalUDPMuxDefault;configurer WGConfigurer;session *macosvpn.EngineSession
}
func NewMacVPNDevice(ctx context.Context,address wgaddr.Address,port int,key string,iceBind *bind.ICEBind)*MacVPNDevice{return &MacVPNDevice{ctx:ctx,address:address,port:port,key:key,iceBind:iceBind}}
func(t *MacVPNDevice)Create()(WGConfigurer,error){
 if os.Geteuid()!=0{return nil,fmt.Errorf("native VPN engine requires the root supervisor")}
 dns,err:=nbnet.GetLastIPFromNetwork(t.address.Network,1);if err!=nil{return nil,err}
 domains,err:=windowsvpn.NativeDNSDomains(t.address.Prefix());if err!=nil{return nil,err}
 control,err:=macosvpn.NewInheritedControl(os.Stdin);if err!=nil{return nil,fmt.Errorf("native supervisor control: %w",err)}
 session,err:=macosvpn.NewEngineSession(control,macosvpn.SessionConfiguration{Version:1,Generation:uuid.NewString(),Address:t.address.Prefix().String(),MTU:1280,DNSServers:[]string{dns.String()},DNSDomains:domains})
 if err!=nil{control.Close();return nil,err};t.session=session
 conn,err:=session.Open(t.ctx);if err!=nil{return nil,err}
 t.name=session.InterfaceName()
 adapter,err:=net.InterfaceByName(t.name);if err!=nil{session.Close();return nil,err}
 addresses,err:=adapter.Addrs();if err!=nil{session.Close();return nil,err}
 matches:=0;for _,a:=range addresses{ip,_,e:=net.ParseCIDR(a.String());if e==nil && ip.Equal(t.address.IP.AsSlice()){matches++}}
 if matches!=1{session.Close();return nil,fmt.Errorf("native utun does not own assigned VPN address")}
 raw,err:=macosvpn.NewConnTun(conn,t.name,1280);if err!=nil{session.Close();return nil,err}
 t.filteredDevice=newDeviceFilter(raw)
 t.device=device.NewDevice(t.filteredDevice,t.iceBind,&device.Logger{Verbosef:func(f string,a ...any){log.Debugf("native WireGuard: "+f,a...)},Errorf:func(f string,a ...any){log.Errorf("native WireGuard: "+f,a...)}})
 t.configurer=configurer.NewUSPConfigurerNoUAPI(t.device,t.name,t.iceBind.ActivityRecorder())
 if err=t.configurer.ConfigureInterface(t.key,t.port);err!=nil{t.Close();return nil,err};return t.configurer,nil
}
func(t *MacVPNDevice)Up()(*udpmux.UniversalUDPMuxDefault,error){if err:=t.device.Up();err!=nil{return nil,err};mux,err:=t.iceBind.GetICEMux();if err==nil{t.udpMux=mux};return mux,err}
func(t *MacVPNDevice)UpdateAddr(a wgaddr.Address)error{if a.Prefix()!=t.address.Prefix(){return fmt.Errorf("native VPN address change requires a new session")};return nil}
func(t *MacVPNDevice)Close()error{if t.configurer!=nil{t.configurer.Close()};if t.device!=nil{t.device.Close()};if t.session!=nil{t.session.Close()};if t.udpMux!=nil{return t.udpMux.Close()};return nil}
func(t *MacVPNDevice)WgAddress()wgaddr.Address{return t.address}
func(t *MacVPNDevice)MTU()uint16{return 1280}
func(t *MacVPNDevice)DeviceName()string{return t.name}
func(t *MacVPNDevice)FilteredDevice()*FilteredDevice{return t.filteredDevice}
func(t *MacVPNDevice)Device()*device.Device{return t.device}
func(t *MacVPNDevice)GetNet()*netstack.Net{return nil}
func(t *MacVPNDevice)GetICEBind()EndpointManager{return t.iceBind}
func(t *MacVPNDevice)PlatformOwnsInterface()bool{return true}
func(t *MacVPNDevice)NativeReady()error{if t.session==nil{return fmt.Errorf("native session missing")};return t.session.EngineReady()}
func(t *MacVPNDevice)NativeDisconnected()<-chan struct{}{if t.session==nil{return nil};return t.session.Disconnected()}
