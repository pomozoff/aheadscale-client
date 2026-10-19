//go:build windows && windowsvpnprototype

package windowsvpn

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/Microsoft/go-winio"
	"github.com/netbirdio/netbird/client/internal/ipcauth"
	"golang.org/x/sys/windows"
)

const PrototypeFamily = "Aheadscale.VpnPrototype_9kyyx27wet2q6"
const PrototypePipe = `\\.\pipe\ProtectedPrefix\Administrators\aheadscale-vpn-prototype`
const ProfileName = "Aheadscale Prototype"

// OwnerSID is explicitly selected by the VM administrator, never by a pipe message.
var OwnerSID string

type SessionConfiguration struct {
	SessionID  string         `json:"sessionId"`
	Address    netip.Prefix   `json:"address"`
	MTU        uint16         `json:"mtu"`
	Routes     []netip.Prefix `json:"routes"`
	DNSDomains []string       `json:"dnsDomains"`
	DNSServers []netip.Addr   `json:"dnsServers"`
}

// AcceptSession transfers no WireGuard key. Only the configured user in the signed
// prototype package may receive packets. Windows, rather than message fields,
// supplies the peer's token and package identity.
func AcceptSession(ctx context.Context, configuration SessionConfiguration) (net.Conn, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if OwnerSID == "" {
		return nil, errors.New("native VPN requires --vpn-prototype-owner-sid")
	}
	if _, err := windows.StringToSid(OwnerSID); err != nil {
		return nil, fmt.Errorf("invalid VPN owner: %w", err)
	}
	if !configuration.Address.IsValid() || !configuration.Address.Addr().Is4() || configuration.MTU != 1280 {
		return nil, errors.New("native VPN prototype requires IPv4 and MTU 1280")
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, err
	}
	configuration.SessionID = hex.EncodeToString(nonce[:])
	payload, err := json.Marshal(configuration)
	if err != nil {
		return nil, err
	}
	if len(payload) > 4096 {
		return nil, errors.New("native VPN configuration too large")
	}
	listener, err := winio.ListenPipe(PrototypePipe, &winio.PipeConfig{
		// Pin the service owner: LocalSystem otherwise defaults to Administrators,
		// which correctly fails the broker's narrower LocalSystem ownership check.
		SecurityDescriptor: "O:SYD:P(A;;GA;;;SY)(A;;GA;;;" + OwnerSID + ")",
		InputBufferSize:    65539, OutputBufferSize: 65539,
	})
	if err != nil {
		return nil, err
	}
	defer listener.Close()
	stopAccept := context.AfterFunc(ctx, func() { _ = listener.Close() })
	defer stopAccept()
	timer := time.AfterFunc(90*time.Second, func() { _ = listener.Close() })
	defer timer.Stop()
	for attempt := 0; attempt < 16; attempt++ {
		conn, err := listener.Accept()
		if err != nil {
			return nil, err
		}
		stopConn := context.AfterFunc(ctx, func() { _ = conn.Close() })
		if err = authorizeNativeBroker(conn); err != nil {
			stopConn()
			_ = conn.Close()
			continue
		}
		if err = conn.SetDeadline(time.Now().Add(30 * time.Second)); err != nil {
			_ = conn.Close()
			return nil, err
		}
		// Control prelude is separate from the IP stream: magic, bounded JSON frame,
		// then an acknowledgement of this exact generation before any TUN packets.
		err = writeAll(conn, []byte("ASV1"))
		if err == nil {
			err = WriteFrame(conn, payload)
		}
		var ack [40]byte
		if err == nil {
			_, err = io.ReadFull(conn, ack[:])
		}
		if err == nil && string(ack[:]) != "ASREADY1"+configuration.SessionID {
			err = errors.New("stale native VPN generation")
		}
		if err == nil {
			err = conn.SetDeadline(time.Time{})
		}
		if err != nil {
			_ = conn.Close()
			return nil, err
		}
		return &nativeConn{Conn: conn, stop: stopConn}, nil
	}
	return nil, errors.New("native VPN broker identity rejected")
}

func authorizeNativeBroker(conn net.Conn) error {
	identity, err := ipcauth.ConnIdentity(conn)
	if err != nil {
		return err
	}
	if identity.SID != OwnerSID {
		return errors.New("native VPN broker owner mismatch")
	}
	fd, ok := conn.(interface{ Fd() uintptr })
	if !ok {
		return errors.New("missing native pipe handle")
	}
	var pid uint32
	if err = windows.GetNamedPipeClientProcessId(windows.Handle(fd.Fd()), &pid); err != nil {
		return err
	}
	process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(process)
	proc := windows.NewLazySystemDLL("kernel32.dll").NewProc("GetPackageFamilyName")
	var length uint32
	result, _, _ := proc.Call(uintptr(process), uintptr(unsafe.Pointer(&length)), 0)
	if result != uintptr(windows.ERROR_INSUFFICIENT_BUFFER) || length == 0 || length > 512 {
		return errors.New("native VPN broker has no package identity")
	}
	family := make([]uint16, length)
	result, _, _ = proc.Call(uintptr(process), uintptr(unsafe.Pointer(&length)), uintptr(unsafe.Pointer(&family[0])))
	if result != 0 || windows.UTF16ToString(family) != PrototypeFamily {
		return errors.New("native VPN broker package rejected")
	}
	return nil
}

type nativeConn struct {
	net.Conn
	stop func() bool
}

func (c *nativeConn) Close() error { c.stop(); return c.Conn.Close() }

var nativeAdapterName atomic.Value

func SetAdapterName(name string) { nativeAdapterName.Store(name) }
func AdapterName() string {
	value := nativeAdapterName.Load()
	if value == nil {
		return ""
	}
	return value.(string)
}
