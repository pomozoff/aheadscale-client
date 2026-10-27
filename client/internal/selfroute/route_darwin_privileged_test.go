//go:build privileged && !ios

package selfroute

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"golang.zx2c4.com/wireguard/tun"

	"github.com/netbirdio/netbird/client/internal/statemanager"
)

func TestDarwinLocalRouteLifecycle(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Fatal("run the isolated UTUN regression with sudo")
	}
	ip := netip.MustParseAddr("192.0.2.217")
	ops := darwinOps{}
	r, err := ops.lookup(ip)
	if !errors.Is(err, errRouteNotFound) {
		t.Fatalf("fixture host route check: %+v %v", r, err)
	}
	if r != nil {
		t.Fatal("fixture address already has a host route; refusing to change it")
	}
	dev, err := tun.CreateTUN("utun", 1280)
	if err != nil {
		t.Fatal(err)
	}
	defer dev.Close()
	name, err := dev.Name()
	if err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("/sbin/ifconfig", name, "inet", ip.String(), ip.String(), "up").CombinedOutput()
	if err != nil {
		t.Fatalf("assign fixture address: %s: %v", out, err)
	}
	statePath := filepath.Join(t.TempDir(), "state.json")
	sm := statemanager.New(statePath)
	RegisterState(sm)
	m := New(sm)
	// Always clean a successful kernel change even if a following assertion fails.
	defer m.Close()
	if err := m.Set(ip, name); err != nil {
		t.Fatal(err)
	}
	testLocalHTTP(t, ip)
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	if r, err := ops.lookup(ip); !errors.Is(err, errRouteNotFound) || r != nil {
		t.Fatalf("graceful Close left host route: %+v %v", r, err)
	}
	// Recreate only the fixture's native host route, then simulate loss of UTUN
	// without Manager.Close (the state file must be sufficient for recovery).
	out, err = exec.Command("/sbin/ifconfig", name, "inet", ip.String(), ip.String(), "up").CombinedOutput()
	if err != nil {
		t.Fatalf("reassign fixture address: %s: %v", out, err)
	}
	if err := m.Set(ip, name); err != nil {
		t.Fatal(err)
	}
	if err := dev.Close(); err != nil {
		t.Fatal(err)
	}
	r, err = ops.lookup(ip)
	if err != nil || !m.state.owns(r) {
		t.Fatalf("missing crash fixture route: %+v %v", r, err)
	}
	recovered := statemanager.New(statePath)
	RegisterState(recovered)
	if err := recovered.PerformCleanup(); err != nil {
		t.Fatal(err)
	}
	if err := recovered.PersistState(context.Background()); err != nil {
		t.Fatal(err)
	}
	if r, err := ops.lookup(ip); !errors.Is(err, errRouteNotFound) || r != nil {
		t.Fatalf("crash recovery left host route: %+v %v", r, err)
	}
	if err := recovered.PerformCleanup(); err != nil {
		t.Fatal(err)
	}
}

func testLocalHTTP(t *testing.T, ip netip.Addr) {
	t.Helper()
	listener, err := net.Listen("tcp4", net.JoinHostPort(ip.String(), "0"))
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{ReadHeaderTimeout: time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, _ := net.SplitHostPort(r.RemoteAddr)
		_, _ = io.WriteString(w, host)
	})}
	go func() { _ = server.Serve(listener) }()
	defer server.Close()
	transport := &http.Transport{Proxy: nil, DisableKeepAlives: true}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 2 * time.Second}
	response, err := client.Get("http://" + listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil || response.StatusCode != 200 || string(body) != ip.String() {
		t.Fatalf("self HTTP: status=%d body=%s error=%v", response.StatusCode, body, err)
	}
}
