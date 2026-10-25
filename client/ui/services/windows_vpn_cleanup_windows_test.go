//go:build windows && windowsvpnprototype

package services

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

// Настоящий kernel mutex воспроизводит границу RAS Disconnected / broker Down.
func holdNativeBrokerLease(t *testing.T) (string, func()) {
	t.Helper()
	name := fmt.Sprintf(`Local\Aheadscale.CleanupTest.%d.%s`, os.Getpid(), t.Name())
	ready := make(chan error, 1)
	release := make(chan struct{})
	done := make(chan struct{})
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer close(done)
		h, err := windows.CreateMutex(nil, true, windows.StringToUTF16Ptr(name))
		ready <- err
		if err != nil {
			return
		}
		defer func() { _ = windows.CloseHandle(h) }()
		<-release
		_ = windows.ReleaseMutex(h)
	}()
	require.NoError(t, <-ready)
	stop := func() {
		select {
		case <-release:
		default:
			close(release)
		}
		<-done
	}
	t.Cleanup(stop)
	return name, stop
}

func TestNativeBrokerCleanupWaitsForLateDown(t *testing.T) {
	name, stop := holdNativeBrokerLease(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- waitNativeBrokerCleanup(ctx, name) }()
	select {
	case err := <-done:
		t.Fatalf("Login overtook broker cleanup: %v", err)
	case <-time.After(150 * time.Millisecond):
	}
	stop()
	require.NoError(t, <-done)
	// Повторное ожидание не удерживает lease и не задерживает следующий вход.
	require.NoError(t, waitNativeBrokerCleanup(ctx, name))
}

func TestNativeBrokerCleanupCancellationStopsLogin(t *testing.T) {
	name, _ := holdNativeBrokerLease(t)
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, waitNativeBrokerCleanup(ctx, name), context.DeadlineExceeded)
}

func TestNativeBrokerCleanupWithoutBroker(t *testing.T) {
	name := fmt.Sprintf(`Local\Aheadscale.AbsentCleanupTest.%d`, os.Getpid())
	require.NoError(t, waitNativeBrokerCleanup(context.Background(), name))
}
