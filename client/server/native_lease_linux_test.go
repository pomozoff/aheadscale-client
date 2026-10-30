//go:build linux && !android

package server

import (
	"context"
	"github.com/netbirdio/netbird/client/internal"
	"github.com/netbirdio/netbird/client/internal/ipcauth"
	"github.com/netbirdio/netbird/client/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"sync"
	"testing"
	"time"
)

func leaseContext(generation string, uid uint32) context.Context {
	return metadata.NewIncomingContext(ctxWithIdentity(ipcauth.Identity{UID: uid}), metadata.Pairs(nativeGenerationMetadataKey, generation))
}
func TestNativeLeaseRejectsOldAndUnprivilegedCommands(t *testing.T) {
	s := &Server{nativeNetworkManager: true, nativeGeneration: 7, nativeHighWater: 7}
	for _, tc := range []struct {
		generation string
		uid        uint32
		code       codes.Code
	}{{"6", 0, codes.FailedPrecondition}, {"7", 1000, codes.PermissionDenied}, {"", 0, codes.FailedPrecondition}} {
		if _, err := s.Down(leaseContext(tc.generation, tc.uid), &proto.DownRequest{}); status.Code(err) != tc.code {
			t.Fatalf("wrong Down rejection: %v", err)
		}
	}
	release, err := s.enterNativeRequest(leaseContext("7", 0))
	if err != nil {
		t.Fatal(err)
	}
	release()
	standalone := &Server{}
	release, err = standalone.enterNativeRequest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	release()
}
func TestNativeBeginRejectsOldGenerationBeforeCleanup(t *testing.T) {
	s := &Server{nativeNetworkManager: true, nativeGeneration: 7, nativeHighWater: 7}
	if _, err := s.NativeBegin(ctxWithIdentity(ipcauth.Identity{UID: 0}), &proto.NativeBeginRequest{Generation: 6}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("old begin changed current engine: %v", err)
	}
	if s.nativeGeneration != 7 {
		t.Fatal("stale begin revoked current lease")
	}
	if _, err := s.NativeStop(ctxWithIdentity(ipcauth.Identity{UID: 1000}), &proto.NativeStopRequest{}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("unprivileged force stop: %v", err)
	}
}

type waitObservedContext struct {
	context.Context
	waiting chan struct{}
	once    sync.Once
}

func (c *waitObservedContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.waiting) })
	return c.Context.Done()
}
func TestNativeUpWaitDoesNotBlockCancel(t *testing.T) {
	stateCtx := internal.CtxInitState(context.Background())
	internal.CtxGetState(stateCtx).Set(internal.StatusConnecting)
	s := &Server{rootCtx: stateCtx, nativeNetworkManager: true, nativeGeneration: 7, nativeHighWater: 7, clientRunning: true, clientGiveUpChan: make(chan struct{}), clientRunningChan: make(chan struct{})}
	base, cancel := context.WithCancel(leaseContext("7", 0))
	defer cancel()
	observed := &waitObservedContext{Context: base, waiting: make(chan struct{})}
	upDone := make(chan error, 1)
	go func() { _, err := s.Up(observed, &proto.UpRequest{}); upDone <- err }()
	select {
	case <-observed.waiting:
	case <-time.After(time.Second):
		t.Fatal("Up did not reach readiness wait")
	}
	acquired := make(chan struct{})
	go func() { s.nativeLeaseGate.Lock(); close(acquired); s.nativeLeaseGate.Unlock() }()
	select {
	case <-acquired:
	case <-time.After(time.Second):
		t.Fatal("readiness wait blocks native Cancel")
	}
	cancel()
	select {
	case <-upDone:
	case <-time.After(time.Second):
		t.Fatal("cancelled Up did not return")
	}
}

func TestNativeBeginWaitsForPreviousCleanup(t *testing.T) {
	pending := make(chan struct{})
	s := &Server{rootCtx: internal.CtxInitState(context.Background()), nativeNetworkManager: true, nativeGeneration: 7, nativeHighWater: 7, nativePendingCleanup: pending}
	ctx, cancel := context.WithTimeout(ctxWithIdentity(ipcauth.Identity{UID: 0}), 50*time.Millisecond)
	defer cancel()
	if _, err := s.NativeBegin(ctx, &proto.NativeBeginRequest{Generation: 8}); status.Code(err) != codes.DeadlineExceeded {
		t.Fatalf("granted lease before cleanup: %v", err)
	}
	if s.nativeGeneration != 0 || s.nativeHighWater != 7 {
		t.Fatal("published new engine owner before cleanup")
	}
	if _, err := s.enterNativeRequest(leaseContext("7", 0)); status.Code(err) != codes.Unavailable {
		t.Fatal("revoked owner can still start/stop engine")
	}
	close(pending)
	if _, err := s.NativeBegin(ctxWithIdentity(ipcauth.Identity{UID: 0}), &proto.NativeBeginRequest{Generation: 9}); err != nil {
		t.Fatal(err)
	}
	if s.nativeGeneration != 9 {
		t.Fatal("did not grant next lease after cleanup")
	}
}
