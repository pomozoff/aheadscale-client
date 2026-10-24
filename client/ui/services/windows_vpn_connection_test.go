//go:build !android && !ios && !freebsd && !js

package services

import (
	"context"
	"errors"
	"os/user"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netbirdio/netbird/client/internal/windowsvpn"
	"github.com/netbirdio/netbird/client/proto"
	"google.golang.org/grpc"
)

// Проверяем намерение пользователя через публичные методы Connection:
// поздний результат Connect не должен включать VPN после Disconnect.
func TestNativeConnectionDisconnectCancelsLateGUIConnect(t *testing.T) {
	entered := make(chan context.Context, 1)
	release := make(chan struct{})
	var connected atomic.Bool
	controller := windowsvpn.NewSessionController(func(ctx context.Context, up bool) error {
		if up {
			entered <- ctx
			<-release
		}
		connected.Store(up)
		return nil
	})
	connection := &Connection{nativeVPN: controller}
	upDone := make(chan error, 1)
	go func() { upDone <- connection.Up(context.Background(), UpParams{}) }()
	var first context.Context
	select {
	case first = <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("GUI Connect обошёл системный контроллер")
	}
	downDone := make(chan error, 1)
	go func() { downDone <- connection.Down(context.Background()) }()
	select {
	case <-first.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("GUI Disconnect не отменил Connect")
	}
	close(release)
	select {
	case err := <-upDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("ошибка отмены: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Connect не завершился")
	}
	select {
	case err := <-downDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Disconnect не завершился")
	}
	if connected.Load() || controller.Snapshot().State != windowsvpn.Disconnected {
		t.Fatalf("поздний Connect восстановил VPN: %+v", controller.Snapshot())
	}
}

// Default — общий профиль службы: GetActiveProfile возвращает пустого
// владельца, а GUI передаёт текущего пользователя ОС. Другую учётку не принимаем.
func TestNativeConnectionActiveProfile(t *testing.T) {
	current, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, activeID, activeUser, requestedID, requestedUser string
		allowed                                                bool
	}{
		{"default_current_user", "default", "", "default", current.Username, true},
		{"default_other_user", "default", "", "default", current.Username + "-other", false},
		{"different_profile", "default", "", "other-profile", current.Username, false},
		{"named_current_owner", "named", current.Username, "named", current.Username, true},
		{"named_other_owner", "named", current.Username + "-other", "named", current.Username, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls int
			daemon := &nativeProfileDaemon{active: &proto.GetActiveProfileResponse{Id: tc.activeID, ProfileName: tc.activeID, Username: tc.activeUser}}
			controller := windowsvpn.NewSessionController(func(context.Context, bool) error { calls++; return nil })
			connection := &Connection{conn: stubConn{client: daemon}, nativeVPN: controller}
			err := connection.Up(context.Background(), UpParams{ProfileName: tc.requestedID, Username: tc.requestedUser})
			if tc.allowed {
				if err != nil || calls != 1 {
					t.Fatalf("активный профиль отклонён: err=%v calls=%d", err, calls)
				}
			} else if err == nil || calls != 0 {
				t.Fatalf("другой профиль принят: err=%v calls=%d", err, calls)
			}
		})
	}
}

type nativeProfileDaemon struct {
	proto.DaemonServiceClient
	active *proto.GetActiveProfileResponse
}

func (d *nativeProfileDaemon) GetActiveProfile(context.Context, *proto.GetActiveProfileRequest, ...grpc.CallOption) (*proto.GetActiveProfileResponse, error) {
	return d.active, nil
}

// Windows может уже считать профиль отключённым до готовности канала,
// пока движок ещё выполняет Up. Disconnect обязан остановить оба состояния.
func TestNativeConnectionDisconnectStopsPartialEngine(t *testing.T) {
	for _, failedProfile := range []bool{false, true} {
		name := "profile_already_off"
		if failedProfile {
			name = "profile_command_failed"
		}
		t.Run(name, func(t *testing.T) {
			var enginePending atomic.Bool
			enginePending.Store(true)
			profileErr := errors.New("profile disconnect failed")
			daemon := &nativeDownDaemon{pending: &enginePending}
			controller := newNativeVPNController(stubConn{client: daemon}, func(context.Context, bool) error {
				if failedProfile {
					return profileErr
				}
				return nil
			})
			connection := &Connection{nativeVPN: controller}
			err := connection.Down(context.Background())
			if failedProfile && !errors.Is(err, profileErr) {
				t.Fatalf("потеряна ошибка системной команды: %v", err)
			}
			if !failedProfile && err != nil {
				t.Fatal(err)
			}
			if enginePending.Load() {
				t.Fatal("профиль выключен, но движок продолжает подключение")
			}
		})
	}
}

type nativeDownDaemon struct {
	proto.DaemonServiceClient
	pending *atomic.Bool
}

func (d *nativeDownDaemon) Down(context.Context, *proto.DownRequest, ...grpc.CallOption) (*proto.DownResponse, error) {
	d.pending.Store(false)
	return &proto.DownResponse{}, nil
}
