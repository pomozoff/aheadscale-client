//go:build !android && !ios && (!windows || !windowsvpnprototype)

package internal

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestWGIfaceMonitorKeepsWatchingDaemonOwnedInterface(t *testing.T) {
	interfaces, err := net.Interfaces()
	if err != nil {
		t.Fatal(err)
	}
	var name string
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp != 0 {
			name = iface.Name
			break
		}
	}
	if name == "" {
		t.Fatal("нет рабочего интерфейса для проверки")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	monitor := NewWGIfaceMonitor()
	type result struct {
		restart bool
		err     error
	}
	finished := make(chan result, 1)
	go func() { restart, err := monitor.Start(ctx, name); finished <- result{restart, err} }()
	select {
	case actual := <-finished:
		t.Fatalf("наблюдение за обычным интерфейсом завершилось до отмены: %+v", actual)
	case <-time.After(100 * time.Millisecond):
	}
	cancel()
	select {
	case actual := <-finished:
		if actual.restart {
			t.Fatal("отмена запустила повторное подключение")
		}
	case <-time.After(time.Second):
		t.Fatal("наблюдатель не завершился после отмены")
	}
}
