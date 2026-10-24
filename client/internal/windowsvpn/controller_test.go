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
