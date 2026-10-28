package server

import (
	"context"
	"testing"

	"github.com/netbirdio/netbird/client/internal"
	"github.com/stretchr/testify/require"
)

func TestSystemDisconnectStopsCurrentClient(t *testing.T) {
	s := newTestServer()
	s.rootCtx = internal.CtxInitState(context.Background())
	state := internal.CtxGetState(s.rootCtx)
	state.Set(internal.StatusConnected)
	ctx, cancel := context.WithCancel(s.rootCtx)
	defer cancel()
	client := newDummyConnectClient(ctx)
	s.actCancel, s.connectClient, s.clientRunning = cancel, client, true
	s.clientGiveUpChan = make(chan struct{})
	go func() { <-ctx.Done(); close(s.clientGiveUpChan) }()
	require.NoError(t, s.onSystemDisconnect(client, s.clientRunningChan))
	require.ErrorIs(t, ctx.Err(), context.Canceled)
	require.False(t, s.clientRunning, "system OFF must cancel reconnect intent")
	require.Nil(t, s.connectClient)
	got, err := state.Status()
	require.NoError(t, err)
	require.Equal(t, internal.StatusIdle, got)
}

func TestSystemDisconnectIgnoresOldClient(t *testing.T) {
	s := newTestServer()
	s.rootCtx = internal.CtxInitState(context.Background())
	internal.CtxGetState(s.rootCtx).Set(internal.StatusConnected)
	ctx, cancel := context.WithCancel(s.rootCtx)
	defer cancel()
	current, old := newDummyConnectClient(ctx), newDummyConnectClient(ctx)
	s.actCancel, s.connectClient, s.clientRunning = cancel, current, true
	require.NoError(t, s.onSystemDisconnect(old, s.clientRunningChan))
	require.NoError(t, ctx.Err())
	require.True(t, s.clientRunning)
	require.Same(t, current, s.connectClient)
	got, err := internal.CtxGetState(s.rootCtx).Status()
	require.NoError(t, err)
	require.Equal(t, internal.StatusConnected, got)
}

func TestSystemDisconnectDoesNotOverwriteConcurrentUp(t *testing.T) {
	s := newTestServer()
	s.rootCtx = internal.CtxInitState(context.Background())
	state := internal.CtxGetState(s.rootCtx)
	state.Set(internal.StatusConnected)
	ctx, cancel := context.WithCancel(s.rootCtx)
	defer cancel()
	old := newDummyConnectClient(ctx)
	oldExited := make(chan struct{})
	s.actCancel, s.connectClient, s.clientRunning = cancel, old, true
	s.clientGiveUpChan = oldExited
	done := make(chan error, 1)
	oldRunning := s.clientRunningChan
	go func() { done <- s.onSystemDisconnect(old, oldRunning) }()
	<-ctx.Done()
	// The new Up wins while the previous Down is waiting for teardown.
	s.mutex.Lock()
	current := newDummyConnectClient(context.Background())
	s.connectClient, s.clientRunning = current, true
	s.clientGiveUpChan = make(chan struct{})
	state.Set(internal.StatusConnecting)
	s.mutex.Unlock()
	close(oldExited)
	require.NoError(t, <-done)
	require.True(t, s.clientRunning)
	require.Same(t, current, s.connectClient)
	got, err := state.Status()
	require.NoError(t, err)
	require.Equal(t, internal.StatusConnecting, got)
}

func TestSystemDisconnectIgnoresPreviousUpBeforeClientReplacement(t *testing.T) {
	s := newTestServer()
	s.rootCtx = internal.CtxInitState(context.Background())
	state := internal.CtxGetState(s.rootCtx)
	state.Set(internal.StatusConnecting)
	ctx, cancel := context.WithCancel(s.rootCtx)
	defer cancel()
	old := newDummyConnectClient(context.Background())
	oldRunning := make(chan struct{})
	s.clientRunningChan = oldRunning
	// Up has installed its new intent, but its goroutine has not replaced
	// connectClient yet. The pointer alone still identifies the old client.
	s.actCancel, s.connectClient, s.clientRunning = cancel, old, true
	s.clientRunningChan = make(chan struct{})
	require.NoError(t, s.onSystemDisconnect(old, oldRunning))
	require.NoError(t, ctx.Err(), "a stale system event must not cancel the new Up")
	require.True(t, s.clientRunning)
	got, err := state.Status()
	require.NoError(t, err)
	require.Equal(t, internal.StatusConnecting, got)
}
