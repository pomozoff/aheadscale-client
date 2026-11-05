package windowsvpn

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func receiveResult(t *testing.T, result <-chan error) error {
	t.Helper()
	select {
	case err := <-result:
		return err
	case <-time.After(3 * time.Second):
		t.Fatal("operation did not finish")
		return nil
	}
}

func TestSessionControllerDisconnectCancelsLateConnect(t *testing.T) {
	entered := make(chan context.Context, 1)
	release := make(chan struct{})
	var connected atomic.Bool
	controller := NewSessionController(func(ctx context.Context, up bool) error {
		if up {
			entered <- ctx
			<-release
		}
		connected.Store(up)
		return nil
	})
	upDone := make(chan error, 1)
	go func() { upDone <- controller.SetConnected(context.Background(), true) }()
	first := <-entered
	downDone := make(chan error, 1)
	go func() { downDone <- controller.SetConnected(context.Background(), false) }()
	select {
	case <-first.Done():
	case <-time.After(time.Second):
		t.Fatal("disconnect did not cancel pending connect")
	}
	close(release)
	if err := receiveResult(t, upDone); !errors.Is(err, context.Canceled) {
		t.Fatalf("late connect returned %v", err)
	}
	if err := receiveResult(t, downDone); err != nil {
		t.Fatal(err)
	}
	if connected.Load() || controller.Snapshot().State != Disconnected {
		t.Fatalf("late result restored connection: %+v", controller.Snapshot())
	}
}

func TestSessionControllerOlderFailureCannotOverwriteNewConnection(t *testing.T) {
	entered := make(chan context.Context, 1)
	release := make(chan struct{})
	var calls atomic.Int32
	var connected atomic.Bool
	controller := NewSessionController(func(ctx context.Context, up bool) error {
		if up && calls.Add(1) == 1 {
			entered <- ctx
			<-release
			return errors.New("obsolete failure")
		}
		connected.Store(up)
		return nil
	})
	oldDone := make(chan error, 1)
	go func() { oldDone <- controller.SetConnected(context.Background(), true) }()
	first := <-entered
	latestDone := make(chan error, 1)
	go func() { latestDone <- controller.SetConnected(context.Background(), true) }()
	select {
	case <-first.Done():
	case <-time.After(time.Second):
		t.Fatal("new intent did not cancel older operation")
	}
	close(release)
	if err := receiveResult(t, oldDone); !errors.Is(err, context.Canceled) {
		t.Fatalf("obsolete result returned %v", err)
	}
	if err := receiveResult(t, latestDone); err != nil {
		t.Fatal(err)
	}
	if !connected.Load() || controller.Snapshot().State != Connected {
		t.Fatalf("obsolete failure won: %+v", controller.Snapshot())
	}
}

func TestSessionControllerFailedConnectClosesPartialConnection(t *testing.T) {
	failure := errors.New("engine not ready")
	var connected atomic.Bool
	controller := NewSessionController(func(_ context.Context, up bool) error {
		connected.Store(up)
		if up {
			return failure
		}
		return nil
	})
	if err := controller.SetConnected(context.Background(), true); !errors.Is(err, failure) {
		t.Fatalf("error=%v", err)
	}
	if connected.Load() || controller.Snapshot().State != Failed {
		t.Fatalf("partial connection survived: %+v", controller.Snapshot())
	}
}

func TestSessionControllerStartupDoesNotOverrideUserIntent(t *testing.T) {
	var connected bool
	c := NewSessionController(func(_ context.Context, up bool) error { connected = up; return nil })
	if err := c.SetConnected(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if err := c.StartOnLaunch(context.Background()); err != nil {
		t.Fatal(err)
	}
	if connected {
		t.Fatal("late startup overrode explicit Disconnect")
	}
}

func TestSessionControllerStartupConnectsOnlyOnce(t *testing.T) {
	calls := 0
	c := NewSessionController(func(_ context.Context, up bool) error {
		if up {
			calls++
		}
		return nil
	})
	if err := c.StartOnLaunch(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := c.StartOnLaunch(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("startup connect calls: %d", calls)
	}
}

// После потери демона сохраняется прежнее намерение Connect, но повторный
// запуск должен оставаться слабее более нового ручного Disconnect.
func TestSessionControllerResumesAfterDaemonRestart(t *testing.T) {
	calls := 0
	c := NewSessionController(func(_ context.Context, up bool) error {
		if up {
			calls++
		}
		return nil
	})
	if err := c.SetConnected(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	ticket := c.Snapshot().Generation
	if err := c.ResumeAfterDaemonRestart(context.Background(), ticket); err != nil {
		t.Fatal(err)
	}
	if calls != 2 || c.Snapshot().State != Connected {
		t.Fatalf("VPN не восстановлен: calls=%d state=%+v", calls, c.Snapshot())
	}
}

func TestSessionControllerRestartCannotOverrideNewDisconnect(t *testing.T) {
	connected := false
	c := NewSessionController(func(_ context.Context, up bool) error { connected = up; return nil })
	if err := c.SetConnected(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	ticket := c.Snapshot().Generation
	if err := c.SetConnected(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if err := c.ResumeAfterDaemonRestart(context.Background(), ticket); err != nil {
		t.Fatal(err)
	}
	if connected {
		t.Fatal("устаревшее восстановление отменило Disconnect")
	}
}

func TestSessionControllerRestartDoesNotResumeDisconnectedSession(t *testing.T) {
	calls := 0
	c := NewSessionController(func(_ context.Context, up bool) error {
		if up {
			calls++
		}
		return nil
	})
	if err := c.SetConnected(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if err := c.ResumeAfterDaemonRestart(context.Background(), c.Snapshot().Generation); err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatal("восстановление включило отключённый VPN")
	}
}

// Повторный Login во время native Up не должен заменять браузерную попытку.
func TestSessionControllerConnectJoinsPendingActivation(t *testing.T) {
	entered := make(chan context.Context, 2)
	release := make(chan struct{})
	var calls atomic.Int32
	c := NewSessionController(func(ctx context.Context, on bool) error {
		if on {
			calls.Add(1)
			entered <- ctx
			<-release
		}
		return nil
	})
	firstDone := make(chan error, 1)
	go func() { firstDone <- c.SetConnected(context.Background(), true) }()
	first := <-entered
	joinedDone := make(chan error, 1)
	go func() { joinedDone <- c.ConnectOrJoin(context.Background()) }()
	select {
	case <-first.Done():
		close(release)
		receiveResult(t, firstDone)
		receiveResult(t, joinedDone)
		t.Fatal("повторный Login отменил исходный вход")
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	if err := receiveResult(t, firstDone); err != nil {
		t.Fatal(err)
	}
	if err := receiveResult(t, joinedDone); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 || c.Snapshot().State != Connected {
		t.Fatalf("ожидался один успешный вход: calls=%d state=%+v", calls.Load(), c.Snapshot())
	}
}

func TestSessionControllerCancelJoinDoesNotCancelActivation(t *testing.T) {
	entered := make(chan context.Context, 1)
	release := make(chan struct{})
	c := NewSessionController(func(ctx context.Context, on bool) error {
		if on {
			entered <- ctx
			<-release
		}
		return nil
	})
	firstDone := make(chan error, 1)
	go func() { firstDone <- c.SetConnected(context.Background(), true) }()
	first := <-entered
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	joinedDone := make(chan error, 1)
	go func() { joinedDone <- c.ConnectOrJoin(ctx) }()
	time.Sleep(100 * time.Millisecond)
	cancel()
	if err := receiveResult(t, joinedDone); !errors.Is(err, context.Canceled) {
		t.Fatalf("join: %v", err)
	}
	select {
	case <-first.Done():
		t.Error("отмена ожидающего отменила владельца входа")
	default:
	}
	close(release)
	if err := receiveResult(t, firstDone); err != nil {
		t.Fatal(err)
	}
}

func TestSessionControllerDisconnectCancelsJoinedActivation(t *testing.T) {
	entered := make(chan context.Context, 1)
	var connected atomic.Bool
	c := NewSessionController(func(ctx context.Context, on bool) error {
		if on {
			entered <- ctx
			<-ctx.Done()
			return ctx.Err()
		}
		connected.Store(false)
		return nil
	})
	firstDone := make(chan error, 1)
	go func() { firstDone <- c.ConnectOrJoin(context.Background()) }()
	<-entered
	joinedDone := make(chan error, 1)
	go func() { joinedDone <- c.ConnectOrJoin(context.Background()) }()
	time.Sleep(100 * time.Millisecond)
	if err := c.SetConnected(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	for _, result := range []<-chan error{firstDone, joinedDone} {
		if err := receiveResult(t, result); !errors.Is(err, context.Canceled) {
			t.Fatalf("отмена должна завершить всех ожидающих: %v", err)
		}
	}
	if connected.Load() || c.Snapshot().State != Disconnected {
		t.Fatalf("VPN пережил Disconnect: %+v", c.Snapshot())
	}
}
