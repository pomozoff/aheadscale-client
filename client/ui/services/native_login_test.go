//go:build !android && !ios && !freebsd && !js

package services

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netbirdio/netbird/client/internal/windowsvpn"
	"github.com/netbirdio/netbird/client/proto"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

type nativeLoginDaemon struct {
	startupProfileDaemon
	login func() error
}

func (d *nativeLoginDaemon) Login(context.Context, *proto.LoginRequest, ...grpc.CallOption) (*proto.LoginResponse, error) {
	if err := d.login(); err != nil {
		return nil, err
	}
	return &proto.LoginResponse{NeedsSSOLogin: true}, nil
}

func TestNativeLoginStopsPendingConnectBeforeStartingSSO(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	entered := make(chan struct{})
	var stopped atomic.Bool
	daemon := &nativeLoginDaemon{login: func() error {
		if !stopped.Load() {
			return errors.New("previous native Connect is still active during Login")
		}
		return nil
	}}
	controller := windowsvpn.NewSessionController(func(ctx context.Context, up bool) error {
		if up {
			close(entered)
			<-ctx.Done()
			return ctx.Err()
		}
		stopped.Store(true)
		return nil
	})
	connection := &Connection{conn: stubConn{client: daemon}, nativeVPN: controller}
	done := make(chan error, 1)
	go func() { done <- connection.Up(ctx, UpParams{}) }()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("native Connect did not start")
	}
	result, err := connection.Login(context.Background(), LoginParams{})
	require.NoError(t, err)
	require.True(t, result.NeedsSSOLogin)
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(3 * time.Second):
		t.Fatal("old Connect still owns the login")
	}
}

func TestNativeLoginDoesNotStartSSOIfStopFails(t *testing.T) {
	var calls atomic.Int32
	daemon := &nativeLoginDaemon{login: func() error { calls.Add(1); return nil }}
	stopped := errors.New("native cleanup failed")
	connection := &Connection{conn: stubConn{client: daemon}, nativeVPN: windowsvpn.NewSessionController(func(context.Context, bool) error { return stopped })}
	_, err := connection.Login(context.Background(), LoginParams{})
	require.ErrorIs(t, err, stopped)
	require.Zero(t, calls.Load())
}

func TestNativeLoginPreventsLateAutomaticResume(t *testing.T) {
	var connects atomic.Int32
	daemon := &nativeLoginDaemon{login: func() error { return nil }}
	connection := &Connection{conn: stubConn{client: daemon}, nativeVPN: windowsvpn.NewSessionController(func(_ context.Context, up bool) error {
		if up {
			connects.Add(1)
		}
		return nil
	})}
	_, err := connection.Login(context.Background(), LoginParams{})
	require.NoError(t, err)
	require.NoError(t, connection.ResumeOnLaunch(context.Background()))
	require.Zero(t, connects.Load(), "automatic resume must not open another login after GUI Login")
}

func TestStandardLoginKeepsExistingFlow(t *testing.T) {
	var calls atomic.Int32
	connection := &Connection{conn: stubConn{client: &nativeLoginDaemon{login: func() error { calls.Add(1); return nil }}}}
	result, err := connection.Login(context.Background(), LoginParams{})
	require.NoError(t, err)
	require.True(t, result.NeedsSSOLogin)
	require.EqualValues(t, 1, calls.Load())
}
