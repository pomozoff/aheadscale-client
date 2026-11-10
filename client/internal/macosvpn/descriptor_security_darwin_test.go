package macosvpn

import (
	"context"
	"os"
	"os/exec"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sys/unix"
)

func TestRejectedDescriptorMessagesDoNotLeakFDs(t *testing.T) {
	receiver, sender := inheritedPair(t)
	a, b := socketFiles(t, unix.SOCK_STREAM)
	before := openDescriptorCount(t)
	for i := 0; i < 100; i++ {
		sendDescriptor(t, sender, handoffGeneration, a, b)
		if c, err := ReceivePacketChannel(context.Background(), receiver, handoffGeneration); err == nil {
			c.Close()
			t.Fatal("extra descriptors accepted")
		}
	}
	after := openDescriptorCount(t)
	if after > before {
		t.Fatalf("rejected handoffs leaked descriptors: %d -> %d", before, after)
	}
	// Rejecting a received duplicate must not close the sender's original.
	if _, err := a.Stat(); err != nil {
		t.Fatal(err)
	}
}
func TestDescriptorRejectsTruncatedControlAndMalformedEnvelope(t *testing.T) {
	for _, name := range []string{"truncated-rights", "too-long", "bad-magic", "no-fd"} {
		t.Run(name, func(t *testing.T) {
			receiver, sender := inheritedPair(t)
			a, _ := socketFiles(t, unix.SOCK_STREAM)
			id := uuid.MustParse(handoffGeneration)
			payload := append([]byte("ASD1"), id[:]...)
			rights := []int{int(a.Fd())}
			switch name {
			case "truncated-rights":
				for len(rights) < 32 {
					rights = append(rights, int(a.Fd()))
				}
			case "too-long":
				payload = append(payload, 0)
			case "bad-magic":
				payload[0] = 0
			case "no-fd":
				rights = nil
			}
			var ancillary []byte
			if len(rights) > 0 {
				ancillary = unix.UnixRights(rights...)
			}
			if _, _, err := sender.WriteMsgUnix(payload, ancillary, nil); err != nil {
				t.Fatal(err)
			}
			if c, err := ReceivePacketChannel(context.Background(), receiver, handoffGeneration); err == nil {
				c.Close()
				t.Fatal("invalid message accepted")
			}
		})
	}
}

// A context hook makes the actual fork boundary deterministic: on the second
// cancellation check a real child executes while the received FD is raw.
type execProbeContext struct {
	context.Context
	calls atomic.Int32
	probe func()
}

func (c *execProbeContext) Err() error {
	if c.calls.Add(1) == 2 {
		c.probe()
	}
	return c.Context.Err()
}

func TestReceivedPacketDescriptorCannotEscapeIntoExecChild(t *testing.T) {
	receiver, sender := inheritedPair(t)
	a, b := socketFiles(t, unix.SOCK_STREAM)
	unix.CloseOnExec(int(a.Fd()))
	unix.CloseOnExec(int(b.Fd()))
	// Mark only our packet socket; macOS may create unrelated sockets in a child.
	if err := unix.SetsockoptInt(int(a.Fd()), unix.SOL_SOCKET, unix.SO_RCVLOWAT, 37); err != nil {
		t.Fatal(err)
	}
	sendDescriptor(t, sender, handoffGeneration, a)
	var childError error
	var output []byte
	probe := func() {
		childCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		cmd := exec.CommandContext(childCtx, os.Args[0], "-test.run=^TestHandoffExecChild$")
		cmd.Env = append(os.Environ(), "AHEADSCALE_FD_EXEC_PROBE=1")
		output, childError = cmd.CombinedOutput()
	}
	probe()
	if childError != nil {
		t.Fatalf("baseline fixture already inherits a socket before receive: %v: %s", childError, output)
	}
	ctx := &execProbeContext{Context: context.Background(), probe: func() {
		childCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		cmd := exec.CommandContext(childCtx, os.Args[0], "-test.run=^TestHandoffExecChild$")
		cmd.Env = append(os.Environ(), "AHEADSCALE_FD_EXEC_PROBE=1")
		output, childError = cmd.CombinedOutput()
	}}
	channel, err := ReceivePacketChannel(ctx, receiver, handoffGeneration)
	if err != nil {
		t.Fatal(err)
	}
	defer channel.Close()
	if childError != nil {
		t.Fatalf("packet descriptor escaped into exec child: %v: %s", childError, output)
	}
	if ctx.calls.Load() < 2 {
		t.Fatal("exec probe did not run")
	}
}
func TestHandoffExecChild(t *testing.T) {
	if os.Getenv("AHEADSCALE_FD_EXEC_PROBE") != "1" {
		return
	}
	for fd := 3; fd < 1024; fd++ {
		var stat unix.Stat_t
		if unix.Fstat(fd, &stat) == nil && stat.Mode&unix.S_IFMT == unix.S_IFSOCK {
			kind, _ := unix.GetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_TYPE)
			marker, _ := unix.GetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_RCVLOWAT)
			if kind == unix.SOCK_STREAM && marker == 37 {
				t.Fatalf("inherited private packet socket FD %d", fd)
			}
		}
	}
}

func openDescriptorCount(t *testing.T) int {
	t.Helper()
	var limit unix.Rlimit
	if err := unix.Getrlimit(unix.RLIMIT_NOFILE, &limit); err != nil {
		t.Fatal(err)
	}
	bound := limit.Cur
	if bound > 65536 {
		bound = 65536
	}
	count := 0
	for fd := uint64(0); fd < bound; fd++ {
		if _, err := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0); err == nil {
			count++
		}
	}
	return count
}
