package windowsvpn

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

const MaxFrameSize = 65535

var ErrInvalidFrameLength = errors.New("invalid VPN frame length")

// ReadFrame reads one bounded packet. The session must end after a framing error.
func ReadFrame(reader io.Reader, packet []byte) (int, error) {
	var header [4]byte
	if _, err := io.ReadFull(reader, header[:]); err != nil {
		return 0, fmt.Errorf("read VPN frame header: %w", err)
	}
	length := binary.LittleEndian.Uint32(header[:])
	if length == 0 || length > MaxFrameSize {
		return 0, fmt.Errorf("%w: %d", ErrInvalidFrameLength, length)
	}
	if length > uint32(len(packet)) {
		return 0, io.ErrShortBuffer
	}
	n, err := io.ReadFull(reader, packet[:int(length)])
	if err != nil {
		return 0, fmt.Errorf("read VPN frame payload: %w", err)
	}
	return n, nil
}

// WriteFrame writes a complete packet; callers serialize concurrent writes.
func WriteFrame(writer io.Writer, packet []byte) error {
	if len(packet) == 0 || len(packet) > MaxFrameSize {
		return fmt.Errorf("%w: %d", ErrInvalidFrameLength, len(packet))
	}
	var header [4]byte
	binary.LittleEndian.PutUint32(header[:], uint32(len(packet)))
	if err := writeAll(writer, header[:]); err != nil {
		return fmt.Errorf("write VPN frame header: %w", err)
	}
	if err := writeAll(writer, packet); err != nil {
		return fmt.Errorf("write VPN frame payload: %w", err)
	}
	return nil
}

func writeAll(writer io.Writer, data []byte) error {
	for len(data) > 0 {
		n, err := writer.Write(data)
		if n < 0 || n > len(data) {
			return io.ErrShortWrite
		}
		data = data[n:]
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrNoProgress
		}
	}
	return nil
}
