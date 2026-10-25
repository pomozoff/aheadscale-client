//go:build !android && !ios && !freebsd && !js

package services

import (
	"context"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	"github.com/netbirdio/netbird/client/internal/windowsvpn"
	"github.com/netbirdio/netbird/client/proto"
)

// Настоящий gRPC-поток разрывается как при остановке службы. Второе
// подключение получает Idle: одного восстановления подписки недостаточно.
func TestNativeRecoveryAfterStatusStreamRestart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	daemon := &recoveryDaemon{breakStream: make(chan struct{}), snapshots: make(chan string, 8)}
	listener := bufconn.Listen(1 << 20)
	server := grpc.NewServer()
	proto.RegisterDaemonServiceServer(server, daemon)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	rpc, err := grpc.NewClient("passthrough:///recovery", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	require.NoError(t, err)
	t.Cleanup(func() { _ = rpc.Close() })
	var connected atomic.Bool
	recovered := make(chan struct{}, 2)
	var connects atomic.Int32
	controller := windowsvpn.NewSessionController(func(_ context.Context, up bool) error {
		connected.Store(up)
		if up && connects.Add(1) > 1 {
			recovered <- struct{}{}
		}
		return nil
	})
	connection := &Connection{conn: stubConn{client: proto.NewDaemonServiceClient(rpc)}, nativeVPN: controller}
	require.NoError(t, connection.Up(ctx, UpParams{}))
	feed := NewDaemonFeed(connection.conn, recoveryEmitter{snapshots: daemon.snapshots}, nil, nil, connection)
	feed.streamWg.Add(1)
	go feed.statusStreamLoop(ctx)
	t.Cleanup(func() { cancel(); require.NoError(t, feed.ServiceShutdown()) })
	waitRecoveryStatus(t, daemon.snapshots, StatusConnected)
	connected.Store(false)
	close(daemon.breakStream)
	waitRecoveryStatus(t, daemon.snapshots, StatusDaemonUnavailable)
	waitRecoveryStatus(t, daemon.snapshots, StatusIdle)
	select {
	case <-recovered:
	case <-time.After(4 * time.Second):
		t.Fatal("подписка восстановилась, но VPN остался отключённым")
	}
	require.True(t, connected.Load(), "восстановление потока должно восстановить VPN")
	require.EqualValues(t, 2, connects.Load(), "один дополнительный Connect на возвращение службы")
}

type recoveryEmitter struct{ snapshots chan string }

func (e recoveryEmitter) Emit(name string, data ...any) bool {
	if name == EventStatusSnapshot {
		e.snapshots <- data[0].(Status).Status
	}
	return true
}
func waitRecoveryStatus(t *testing.T, ch <-chan string, want string) {
	t.Helper()
	timeout := time.After(4 * time.Second)
	for {
		select {
		case got := <-ch:
			if got == want {
				return
			}
		case <-timeout:
			t.Fatalf("не получен статус %s", want)
		}
	}
}

type recoveryDaemon struct {
	proto.UnimplementedDaemonServiceServer
	subscriptions atomic.Int32
	breakStream   chan struct{}
	snapshots     chan string
}

func (d *recoveryDaemon) SubscribeStatus(_ *proto.StatusRequest, stream proto.DaemonService_SubscribeStatusServer) error {
	current := StatusIdle
	if d.subscriptions.Add(1) == 1 {
		current = StatusConnected
	}
	if err := stream.Send(&proto.StatusResponse{Status: current}); err != nil {
		return err
	}
	if current == StatusConnected {
		select {
		case <-d.breakStream:
			return status.Error(codes.Unavailable, "служба остановлена")
		case <-stream.Context().Done():
			return stream.Context().Err()
		}
	}
	<-stream.Context().Done()
	return stream.Context().Err()
}
func (d *recoveryDaemon) GetActiveProfile(context.Context, *proto.GetActiveProfileRequest) (*proto.GetActiveProfileResponse, error) {
	return &proto.GetActiveProfileResponse{Id: "default", Username: "aheadtest"}, nil
}
func (d *recoveryDaemon) GetConfig(_ context.Context, req *proto.GetConfigRequest) (*proto.GetConfigResponse, error) {
	if req.ProfileName != "default" || req.Username != "aheadtest" {
		return nil, status.Error(codes.PermissionDenied, "неверный владелец профиля")
	}
	return &proto.GetConfigResponse{}, nil
}

func TestNativeRecoveryHonorsPreferenceAndNewDisconnect(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		disabled, disconnect bool
	}{
		{name: "enabled"}, {name: "disabled", disabled: true}, {name: "newer_disconnect", disconnect: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			controller := windowsvpn.NewSessionController(func(_ context.Context, up bool) error {
				if up {
					calls++
				}
				return nil
			})
			connection := &Connection{conn: stubConn{client: &startupProfileDaemon{disabled: tc.disabled}}, nativeVPN: controller}
			require.NoError(t, connection.Up(context.Background(), UpParams{}))
			resume := connection.nativeDaemonRecovery()
			require.NotNil(t, resume)
			if tc.disconnect {
				require.NoError(t, connection.Down(context.Background()))
			}
			require.NoError(t, resume(context.Background()))
			want := 2
			if tc.disabled || tc.disconnect {
				want = 1
			}
			require.Equal(t, want, calls, "восстановление учитывает настройку и более новое действие")
		})
	}
	require.Nil(t, (&Connection{}).nativeDaemonRecovery(), "обычный backend не запускает native RPC")
}
