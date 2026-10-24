package windowsvpn

import (
	"bytes"
	"encoding/hex"
	"errors"
	"io"
	"testing"
)

func ipv4Packet(t *testing.T) []byte {
	t.Helper()
	packet, err := hex.DecodeString("4500001c00010000400166de0a0000010a0000020800f7ff00000000")
	if err != nil {
		t.Fatal(err)
	}
	return packet
}

type chunkReader struct{ io.Reader }

func (r chunkReader) Read(p []byte) (int, error) {
	if len(p) > 1 {
		p = p[:1]
	}
	return r.Reader.Read(p)
}

type chunkWriter struct{ bytes.Buffer }

func (w *chunkWriter) Write(p []byte) (int, error) {
	if len(p) > 3 {
		p = p[:3]
	}
	return w.Buffer.Write(p)
}

func TestFramePreservesPacketAcrossPartialIO(t *testing.T) {
	packet := ipv4Packet(t)
	writer := new(chunkWriter)
	if err := WriteFrame(writer, packet); err != nil {
		t.Fatal(err)
	}
	want := append([]byte{28, 0, 0, 0}, packet...)
	if !bytes.Equal(writer.Bytes(), want) {
		t.Fatalf("wire bytes = %x, want %x", writer.Bytes(), want)
	}
	output := make([]byte, 65535)
	n, err := ReadFrame(chunkReader{bytes.NewReader(writer.Bytes())}, output)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(output[:n], packet) {
		t.Fatalf("packet = %x, want %x", output[:n], packet)
	}
}

func TestReadFrameRejectsMalformedOrOversizedInput(t *testing.T) {
	cases := []struct {
		name     string
		wire     []byte
		capacity int
		want     error
	}{
		{"zero", []byte{0, 0, 0, 0}, 65535, ErrInvalidFrameLength},
		{"65536", []byte{0, 0, 1, 0}, 65535, ErrInvalidFrameLength},
		{"short header", []byte{1, 0}, 65535, io.ErrUnexpectedEOF},
		{"short payload", []byte{4, 0, 0, 0, 0x45, 0}, 65535, io.ErrUnexpectedEOF},
		{"destination bound", []byte{4, 0, 0, 0, 1, 2, 3, 4}, 3, io.ErrShortBuffer},
		{"end of stream", nil, 65535, io.EOF},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ReadFrame(bytes.NewReader(tc.wire), make([]byte, tc.capacity))
			if !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestWriteFrameRejectsUnboundedLengthsBeforeWriting(t *testing.T) {
	for _, size := range []int{0, 65536} {
		var output bytes.Buffer
		if err := WriteFrame(&output, make([]byte, size)); !errors.Is(err, ErrInvalidFrameLength) {
			t.Fatalf("length %d error = %v", size, err)
		}
		if output.Len() != 0 {
			t.Fatalf("invalid length %d wrote %d bytes", size, output.Len())
		}
	}
}

func TestFrameAcceptsMaximumLength(t *testing.T) {
	packet := bytes.Repeat([]byte{0x45}, 65535)
	var wire bytes.Buffer
	if err := WriteFrame(&wire, packet); err != nil {
		t.Fatal(err)
	}
	output := make([]byte, 65535)
	n, err := ReadFrame(&wire, output)
	if err != nil || n != len(packet) || !bytes.Equal(output, packet) {
		t.Fatalf("maximum frame n=%d err=%v", n, err)
	}
}
