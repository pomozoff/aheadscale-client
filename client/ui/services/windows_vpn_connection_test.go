//go:build !android && !ios && !freebsd && !js

package services

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netbirdio/netbird/client/internal/windowsvpn"
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
