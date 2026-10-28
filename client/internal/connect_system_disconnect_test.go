package internal

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSystemDisconnectCancelsRunAndNotifiesOwner(t *testing.T) {
	ownerCalled := make(chan struct{}, 1)
	c := NewConnectClient(context.Background(), nil, nil, WithSystemDisconnect(func() { ownerCalled <- struct{}{} }))
	engineCtx, cancel := context.WithCancel(c.ctx)
	defer cancel()
	event := make(chan struct{}, 1)
	event <- struct{}{}
	require.True(t, c.waitForEngine(engineCtx, event))
	require.ErrorIs(t, c.ctx.Err(), context.Canceled, "must cancel every retry, not just the engine")
	select {
	case <-ownerCalled:
	case <-time.After(time.Second):
		t.Fatal("daemon owner not notified")
	}
}

func TestSystemDisconnectIgnoresCancelledEngine(t *testing.T) {
	c := NewConnectClient(context.Background(), nil, nil, WithSystemDisconnect(func() { t.Error("ordinary engine stop must not change daemon intent") }))
	defer c.runCancel()
	engineCtx, cancel := context.WithCancel(c.ctx)
	cancel()
	event := make(chan struct{}, 1)
	event <- struct{}{}
	require.False(t, c.waitForEngine(engineCtx, event))
	require.NoError(t, c.ctx.Err(), "ordinary engine restart must keep reconnect intent")
}
