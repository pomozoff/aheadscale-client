// Package macosvpn provides the packet side of the native macOS VPN bridge.
package macosvpn

import (
	"encoding/binary"
	"errors"
	"io"
	"net"
	"os"
	"sync"
	"time"

	"golang.zx2c4.com/wireguard/tun"
)

const frameTimeout = 2 * time.Second

// connTun owns an already authenticated stream. It never creates an OS tunnel
// or changes addresses, routes or DNS: Network Extension owns those resources.
type connTun struct {
	conn            net.Conn
	name            string
	mtu             int
	events          chan tun.Event
	readMu, writeMu sync.Mutex
	closeOnce       sync.Once
	closeErr        error
}

var _ tun.Device = (*connTun)(nil)

// NewConnTun takes ownership only on success. Authentication and generation
// binding must have completed before the caller supplies the stream.
func NewConnTun(conn net.Conn, name string, mtu int) (tun.Device, error) {
	if conn == nil || name == "" || mtu <= 0 || mtu > 65535 {
		return nil, errors.New("invalid native VPN transport, name or MTU")
	}
	d := &connTun{conn: conn, name: name, mtu: mtu, events: make(chan tun.Event, 2)}
	d.events <- tun.EventUp
	return d, nil
}
func (d *connTun) File() *os.File           { return nil }
func (d *connTun) Name() (string, error)    { return d.name, nil }
func (d *connTun) MTU() (int, error)        { return d.mtu, nil }
func (d *connTun) Events() <-chan tun.Event { return d.events }
func (d *connTun) BatchSize() int           { return 1 }

func (d *connTun) Read(buffers [][]byte, sizes []int, offset int) (int, error) {
	if len(buffers) == 0 || len(sizes) == 0 || offset < 0 || offset > len(buffers[0]) {
		return 0, io.ErrShortBuffer
	}
	d.readMu.Lock()
	defer d.readMu.Unlock()
	n, err := d.readPacket(buffers[0][offset:])
	if err != nil {
		_ = d.Close()
		return 0, err
	}
	sizes[0] = n
	return 1, nil
}
func (d *connTun) readPacket(packet []byte) (int, error) {
	var h [4]byte
	// Idle is not a failed VPN. Bound the remainder only once a frame starts.
	if err := d.conn.SetReadDeadline(time.Time{}); err != nil {
		return 0, err
	}
	if _, err := io.ReadFull(d.conn, h[:1]); err != nil {
		return 0, err
	}
	if err := d.conn.SetReadDeadline(time.Now().Add(frameTimeout)); err != nil {
		return 0, err
	}
	if _, err := io.ReadFull(d.conn, h[1:]); err != nil {
		return 0, err
	}
	length := binary.LittleEndian.Uint32(h[:])
	if length == 0 || length > uint32(d.mtu) {
		return 0, errors.New("native VPN frame exceeds session MTU or is empty")
	}
	if length > uint32(len(packet)) {
		return 0, io.ErrShortBuffer
	}
	n, err := io.ReadFull(d.conn, packet[:int(length)])
	if err == nil {
		err = d.conn.SetReadDeadline(time.Time{})
	}
	return n, err
}
func (d *connTun) Write(buffers [][]byte, offset int) (int, error) {
	if offset < 0 {
		return 0, io.ErrShortBuffer
	}
	d.writeMu.Lock()
	defer d.writeMu.Unlock()
	for i, b := range buffers {
		if offset > len(b) {
			return i, io.ErrShortBuffer
		}
		packet := b[offset:]
		if len(packet) == 0 || len(packet) > d.mtu {
			return i, errors.New("invalid native VPN packet length")
		}
		if err := d.writePacket(packet); err != nil {
			_ = d.Close()
			return i, err
		}
	}
	return len(buffers), nil
}
func (d *connTun) writePacket(packet []byte) error {
	if err := d.conn.SetWriteDeadline(time.Now().Add(frameTimeout)); err != nil {
		return err
	}
	var h [4]byte
	binary.LittleEndian.PutUint32(h[:], uint32(len(packet)))
	for _, part := range [][]byte{h[:], packet} {
		for len(part) > 0 {
			n, err := d.conn.Write(part)
			if n < 0 || n > len(part) {
				return io.ErrShortWrite
			}
			if err != nil {
				return err
			}
			if n == 0 {
				return io.ErrNoProgress
			}
			part = part[n:]
		}
	}
	return d.conn.SetWriteDeadline(time.Time{})
}
func (d *connTun) Close() error {
	d.closeOnce.Do(func() {
		// No IO lock: Close must unblock a reader and writer concurrently.
		d.closeErr = d.conn.Close()
		d.events <- tun.EventDown
		close(d.events)
	})
	return d.closeErr
}
