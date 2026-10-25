//go:build windows && windowsvpnprototype

package internal

import (
	"context"
	"net"
	"testing"
	"time"
)

// Системный VPN закрывает интерфейс до завершения Down посредника.
// Наблюдатель движка не должен конкурировать с владельцем этого интерфейса.
func TestNativeWGIfaceMonitorLeavesLifecycleToWindows(t *testing.T) {
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
		if actual.restart || actual.err != nil {
			t.Fatalf("Windows владеет интерфейсом: restart=%v err=%v", actual.restart, actual.err)
		}
	case <-time.After(500 * time.Millisecond):
		cancel()
		select {
		case <-finished:
		case <-time.After(time.Second):
			t.Fatal("наблюдатель не завершился после отмены")
		}
		t.Fatal("движок продолжает следить за жизненным циклом системного VPN")
	}
	select {
	case <-monitor.done:
	default:
		t.Fatal("наблюдатель не освободил ожидание завершения")
	}
}
