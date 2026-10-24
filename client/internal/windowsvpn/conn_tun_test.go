package windowsvpn

import (
	"bytes"
	"errors"
	"net"
	"sync"
	"testing"
	"time"
)

func TestConnTunTransfersRealPacketBothWaysWithOffsets(t *testing.T) {
	conn, peer := net.Pipe()
	defer peer.Close()
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	_ = peer.SetDeadline(time.Now().Add(3 * time.Second))
	device, err := NewConnTun(conn, "AheadscalePrototype", 1280)
	if err != nil {
		t.Fatal(err)
	}
	defer device.Close()
	packet := ipv4Packet(t)
	written := make(chan error, 1)
	go func() { written <- WriteFrame(peer, packet) }()
	buffer := bytes.Repeat([]byte{0xa5}, 64)
	sizes := make([]int, 1)
	n, err := device.Read([][]byte{buffer}, sizes, 7)
	if err != nil || n != 1 || sizes[0] != len(packet) {
		t.Fatalf("Read n=%d size=%d err=%v", n, sizes[0], err)
	}
	if !bytes.Equal(buffer[7:7+sizes[0]], packet) {
		t.Fatalf("received %x", buffer[7:7+sizes[0]])
	}
	if !bytes.Equal(buffer[:7], bytes.Repeat([]byte{0xa5}, 7)) || !bytes.Equal(buffer[7+sizes[0]:], bytes.Repeat([]byte{0xa5}, len(buffer)-7-sizes[0])) {
		t.Fatal("Read modified bytes outside packet")
	}
	if err := <-written; err != nil {
		t.Fatal(err)
	}
	go func() {
		n, err := device.Write([][]byte{append([]byte{0, 0, 0}, packet...)}, 3)
		if err == nil && n != 1 {
			err = errors.New("wrong packet count")
		}
		written <- err
	}()
	result := make([]byte, 65535)
	length, err := ReadFrame(peer, result)
	if err != nil || !bytes.Equal(result[:length], packet) {
		t.Fatalf("peer packet %x err=%v", result[:length], err)
	}
	if err := <-written; err != nil {
		t.Fatal(err)
	}
}

type observedConn struct {
	net.Conn
	readStarted, writeStarted chan struct{}
	readOnce, writeOnce       sync.Once
}

func (c *observedConn) Read(p []byte) (int, error) {
	c.readOnce.Do(func() { close(c.readStarted) })
	return c.Conn.Read(p)
}
func (c *observedConn) Write(p []byte) (int, error) {
	c.writeOnce.Do(func() { close(c.writeStarted) })
	return c.Conn.Write(p)
}

func TestConnTunCloseUnblocksStreamOperations(t *testing.T) {
	for _, direction := range []string{"read", "write"} {
		t.Run(direction, func(t *testing.T) {
			a, b := net.Pipe()
			defer b.Close()
			conn := &observedConn{Conn: a, readStarted: make(chan struct{}), writeStarted: make(chan struct{})}
			device, err := NewConnTun(conn, "AheadscalePrototype", 1280)
			if err != nil {
				t.Fatal(err)
			}
			defer device.Close()
			done := make(chan error, 1)
			var started <-chan struct{}
			if direction == "read" {
				started = conn.readStarted
				go func() { _, err := device.Read([][]byte{make([]byte, 1280)}, make([]int, 1), 0); done <- err }()
			} else {
				started = conn.writeStarted
				packet := ipv4Packet(t)
				go func() { _, err := device.Write([][]byte{packet}, 0); done <- err }()
			}
			select {
			case <-started:
			case <-time.After(time.Second):
				t.Fatal("operation did not enter stream")
			}
			if err := device.Close(); err != nil {
				t.Fatal(err)
			}
			if err := device.Close(); err != nil {
				t.Fatalf("repeated Close: %v", err)
			}
			select {
			case err := <-done:
				if err == nil {
					t.Fatal("closed stream returned success")
				}
			case <-time.After(time.Second):
				t.Fatal("Close did not unblock operation")
			}
		})
	}
}

func TestConnTunMalformedFrameEndsSession(t *testing.T) {
	conn, peer := net.Pipe()
	defer peer.Close()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	device, err := NewConnTun(conn, "AheadscalePrototype", 1280)
	if err != nil {
		t.Fatal(err)
	}
	defer device.Close()
	go func() { _, _ = peer.Write([]byte{0, 0, 1, 0}) }()
	_, err = device.Read([][]byte{make([]byte, 1280)}, make([]int, 1), 0)
	if !errors.Is(err, ErrInvalidFrameLength) {
		t.Fatalf("Read error = %v", err)
	}
	if err := peer.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		return
	}
	_, err = peer.Read(make([]byte, 1))
	if err == nil {
		t.Fatal("malformed frame left session open")
	}
	if timeout, ok := err.(net.Error); ok && timeout.Timeout() {
		t.Fatal("malformed frame did not close stream")
	}
}
