//go:build linux && !android

package linuxvpn

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

const maxBrokerMessage = 16 * 1024

type BrokerReply struct {
	State State
	Error string
}

func decodeBrokerMessage(reader io.Reader) (Message, error) {
	// Exactly one newline-delimited message; never allocate an arbitrary payload.
	line, err := bufio.NewReader(io.LimitReader(reader, maxBrokerMessage+1)).ReadBytes('\n')
	if len(line) > maxBrokerMessage {
		return Message{}, fmt.Errorf("служебное сообщение слишком большое")
	}
	if err != nil && err != io.EOF {
		return Message{}, err
	}
	var message Message
	decoder := json.NewDecoder(bytes.NewReader(line))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&message); err != nil {
		return Message{}, err
	}
	if err = decoder.Decode(new(any)); err != io.EOF {
		return Message{}, fmt.Errorf("лишние данные служебного сообщения")
	}
	return message, nil
}

func unixPeerUID(connection *net.UnixConn) (uint32, error) {
	raw, err := connection.SyscallConn()
	if err != nil {
		return 0, err
	}
	var credentials *unix.Ucred
	var credentialErr error
	if err = raw.Control(func(fd uintptr) {
		credentials, credentialErr = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
	}); err != nil {
		return 0, err
	}
	if credentialErr != nil {
		return 0, credentialErr
	}
	if credentials == nil {
		return 0, fmt.Errorf("нет kernel peer credentials")
	}
	return credentials.Uid, nil
}

// ServeBroker owns its listener. Shutdown closes active connections and waits
// for handlers, so no cancelled connection can mutate a replacement controller.
func (c *Controller) ServeBroker(ctx context.Context, listener net.Listener) error {
	var workers sync.WaitGroup
	defer workers.Wait()
	stop := context.AfterFunc(ctx, func() { _ = listener.Close() })
	defer stop()
	defer listener.Close()
	slots := make(chan struct{}, 16)
	for {
		connection, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		select {
		case slots <- struct{}{}:
		default:
			_ = connection.Close()
			continue
		}
		workers.Add(1)
		go func() {
			defer workers.Done()
			defer func() { <-slots }()
			defer connection.Close()
			stopConnection := context.AfterFunc(ctx, func() { _ = connection.Close() })
			defer stopConnection()
			_ = connection.SetDeadline(time.Now().Add(5 * time.Second))
			reply := BrokerReply{}
			unixConnection, ok := connection.(*net.UnixConn)
			if !ok {
				reply.Error = "требуется локальный Unix socket"
			} else {
				uid, peerErr := unixPeerUID(unixConnection)
				if peerErr != nil {
					reply.Error = peerErr.Error()
				} else if uid != 0 {
					reply.Error = "служебный канал Aheadscale доступен только root"
				} else {
					message, decodeErr := decodeBrokerMessage(connection)
					if decodeErr != nil {
						reply.Error = decodeErr.Error()
					} else {
						var handleErr error
						reply.State, handleErr = c.Handle(uid, message)
						if handleErr != nil {
							reply.Error = handleErr.Error()
						}
					}
				}
			}
			_ = json.NewEncoder(connection).Encode(reply)
		}()
	}
}
