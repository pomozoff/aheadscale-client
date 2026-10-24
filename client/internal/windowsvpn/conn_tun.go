package windowsvpn

import (
	"errors"
	"io"
	"net"
	"os"
	"sync"

	"golang.zx2c4.com/wireguard/tun"
)

type connTun struct {
	conn      net.Conn
	name      string
	mtu       int
	events    chan tun.Event
	readMu    sync.Mutex
	writeMu   sync.Mutex
	closeOnce sync.Once
	closeErr  error
}

var _ tun.Device = (*connTun)(nil)

// NewConnTun accepts an already authenticated transport; it never opens a socket
// or changes host interfaces, routes, DNS, or authentication policy.
func NewConnTun(conn net.Conn, name string, mtu int) (tun.Device, error) {
	if conn == nil {
		return nil, errors.New("VPN transport is nil")
	}
	if name == "" {
		return nil, errors.New("VPN device name is empty")
	}
	if mtu <= 0 || mtu > MaxFrameSize {
		return nil, errors.New("invalid VPN device MTU")
	}
	device := &connTun{conn: conn, name: name, mtu: mtu, events: make(chan tun.Event, 2)}
	device.events <- tun.EventUp
	return device, nil
}

func (d *connTun) File() *os.File           { return nil }
func (d *connTun) Name() (string, error)    { return d.name, nil }
func (d *connTun) MTU() (int, error)        { return d.mtu, nil }
func (d *connTun) Events() <-chan tun.Event { return d.events }
func (d *connTun) BatchSize() int           { return 1 }

func (d *connTun) Read(buffers [][]byte, sizes []int, offset int) (int, error) {
	if len(buffers) == 0 || len(sizes) < len(buffers) || offset < 0 || offset > len(buffers[0]) {
		return 0, io.ErrShortBuffer
	}
	d.readMu.Lock()
	defer d.readMu.Unlock()
	size, err := ReadFrame(d.conn, buffers[0][offset:])
	if err != nil {
		_ = d.Close()
		return 0, err
	}
	sizes[0] = size
	return 1, nil
}

func (d *connTun) Write(buffers [][]byte, offset int) (int, error) {
	if offset < 0 {
		return 0, io.ErrShortBuffer
	}
	d.writeMu.Lock()
	defer d.writeMu.Unlock()
	for index, buffer := range buffers {
		if offset > len(buffer) {
			return index, io.ErrShortBuffer
		}
		if err := WriteFrame(d.conn, buffer[offset:]); err != nil {
			_ = d.Close()
			return index, err
		}
	}
	return len(buffers), nil
}

func (d *connTun) Close() error {
	d.closeOnce.Do(func() {
		// Closing the connection unblocks both directions without acquiring IO locks.
		d.closeErr = d.conn.Close()
		d.events <- tun.EventDown
		close(d.events)
	})
	return d.closeErr
}
