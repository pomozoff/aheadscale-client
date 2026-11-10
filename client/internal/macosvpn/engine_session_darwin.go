package macosvpn

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"golang.org/x/sys/unix"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

// EngineSession is private to one engine generation. Its control endpoint must
// be inherited from the authenticated root supervisor, never opened by name.
// Open consumes the endpoint; Close releases it and the packet stream.
type EngineSession struct {
	control                        *net.UnixConn
	config                         SessionConfiguration
	mu                             sync.Mutex
	started, opened, ready, closed bool
	name                           string
	packet                         net.Conn
	done                           chan struct{}
	once                           sync.Once
}

func NewEngineSession(control *net.UnixConn, config SessionConfiguration) (*EngineSession, error) {
	if control == nil {
		return nil, errors.New("missing native supervisor")
	}
	if _, err := config.Marshal(); err != nil {
		return nil, err
	}
	return &EngineSession{control: control, config: config, done: make(chan struct{})}, nil
}
func (s *EngineSession) Open(ctx context.Context) (channel net.Conn, err error) {
	s.mu.Lock()
	if s.started || s.closed {
		s.mu.Unlock()
		return nil, errors.New("native session already consumed")
	}
	s.started = true
	s.mu.Unlock()
	defer func() {
		if err != nil {
			s.Close()
		}
	}()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	deadline, _ := ctx.Deadline()
	if err = s.control.SetDeadline(deadline); err != nil {
		return nil, err
	}
	interrupted := make(chan struct{})
	stop := context.AfterFunc(ctx, func() { s.control.SetDeadline(time.Now()); close(interrupted) })
	defer func() {
		if !stop() {
			<-interrupted
		}
		s.control.SetDeadline(time.Time{})
	}()
	data, err := s.config.Marshal()
	if err != nil {
		return nil, err
	}
	if _, err = s.control.Write(data); err != nil {
		return nil, err
	}
	channel, err = ReceivePacketChannel(ctx, s.control, s.config.Generation)
	if err != nil {
		return nil, err
	}
	// ReceivePacketChannel restores its read deadline. Reapply the session budget.
	if err = s.control.SetReadDeadline(deadline); err != nil {
		channel.Close()
		return nil, err
	}
	payload := make([]byte, 4097)
	oob := make([]byte, unix.CmsgSpace(32))
	n, on, flags, _, readErr := s.control.ReadMsgUnix(payload, oob)
	// This acknowledgement never carries descriptors. Dispose of all rights even
	// on malformed/truncated data instead of letting a failed setup leak them.
	messages, parseErr := unix.ParseSocketControlMessage(oob[:on])
	for _, m := range messages {
		if m.Header.Level == unix.SOL_SOCKET && m.Header.Type == unix.SCM_RIGHTS {
			fds, _ := unix.ParseUnixRights(&m)
			for _, fd := range fds {
				unix.Close(fd)
			}
		}
	}
	if readErr != nil || parseErr != nil || on != 0 || flags&(unix.MSG_TRUNC|unix.MSG_CTRUNC) != 0 || n > 4096 {
		channel.Close()
		if readErr != nil {
			return nil, readErr
		}
		return nil, errors.New("invalid native settings acknowledgement")
	}
	var ack struct {
		Kind       string `json:"kind"`
		Generation string `json:"generation"`
		Interface  string `json:"interface"`
	}
	dec := json.NewDecoder(bytes.NewReader(payload[:n]))
	dec.DisallowUnknownFields()
	err = dec.Decode(&ack)
	var extra any
	if err != nil || dec.Decode(&extra) != io.EOF || ack.Kind != "settings" || ack.Generation != s.config.Generation || !validUTunName(ack.Interface) {
		channel.Close()
		return nil, errors.New("stale or invalid native settings acknowledgement")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || ctx.Err() != nil {
		channel.Close()
		return nil, errors.New("native session cancelled")
	}
	s.name = ack.Interface
	s.opened = true
	s.packet = &sessionConn{Conn: channel, onClose: func() { s.once.Do(func() { close(s.done) }) }}
	return s.packet, nil
}
func validUTunName(name string) bool {
	if !strings.HasPrefix(name, "utun") || len(name) > 20 {
		return false
	}
	n, e := strconv.ParseUint(strings.TrimPrefix(name, "utun"), 10, 32)
	return e == nil && name == "utun"+strconv.FormatUint(n, 10)
}
func (s *EngineSession) InterfaceName() string         { s.mu.Lock(); defer s.mu.Unlock(); return s.name }
func (s *EngineSession) Disconnected() <-chan struct{} { return s.done }
func (s *EngineSession) EngineReady() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.opened || s.closed || s.ready {
		return errors.New("native engine is not ready or already acknowledged")
	}
	data, _ := json.Marshal(map[string]string{"kind": "engine", "generation": s.config.Generation})
	s.control.SetWriteDeadline(time.Now().Add(time.Second))
	defer s.control.SetWriteDeadline(time.Time{})
	if _, err := s.control.Write(data); err != nil {
		return err
	}
	s.ready = true
	return nil
}
func (s *EngineSession) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	p := s.packet
	s.mu.Unlock()
	if p != nil {
		p.Close()
	}
	s.once.Do(func() { close(s.done) })
	return s.control.Close()
}

type sessionConn struct {
	net.Conn
	onClose func()
	once    sync.Once
}

func (c *sessionConn) Close() error { err := c.Conn.Close(); c.once.Do(c.onClose); return err }
