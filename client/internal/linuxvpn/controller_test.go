//go:build linux && !android

package linuxvpn

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCancelRejectsLateUp(t *testing.T) {
	c, err := NewController(filepath.Join(t.TempDir(), "state"), "boot", true)
	if err != nil {
		t.Fatal(err)
	}
	generation, err := c.Request(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.Request(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if err = c.Complete(generation, Connected); err == nil {
		t.Fatal("accepted cancelled generation")
	}
	if c.State().Phase != Disconnected {
		t.Fatal("cancelled connection revived")
	}
}
func TestRestartPreservesOff(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state")
	c, err := NewController(path, "boot", true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.Request(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	restarted, err := NewController(path, "boot", true)
	if err != nil {
		t.Fatal(err)
	}
	if restarted.Intent().Connected {
		t.Fatal("restart lost explicit OFF")
	}
	rebooted, err := NewController(path, "next-boot", true)
	if err != nil {
		t.Fatal(err)
	}
	if !rebooted.Intent().Connected {
		t.Fatal("reboot ignored autoconnect preference")
	}
}
func TestBrokerRejectsNonRootAndStaleGeneration(t *testing.T) {
	c, err := NewController(filepath.Join(t.TempDir(), "state"), "boot", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.Handle(1000, Message{Version: 2, Operation: "Begin"}); err == nil {
		t.Fatal("accepted unprivileged peer")
	}
	if _, err = c.Handle(0, Message{Version: 3, Operation: "Begin"}); err == nil {
		t.Fatal("accepted unknown protocol version")
	}
	started, err := c.Handle(0, Message{Version: 2, Operation: "Begin"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.Handle(0, Message{Version: 2, Operation: "Complete", Generation: started.Generation + 1}); err == nil {
		t.Fatal("accepted unknown generation")
	}
	if _, err = c.Handle(0, Message{Version: 2, Operation: "Cancel", Generation: started.Generation}); err != nil {
		t.Fatal(err)
	}
	if _, err = c.Handle(0, Message{Version: 2, Operation: "Complete", Generation: started.Generation}); err == nil {
		t.Fatal("accepted late completion")
	}
}
func TestDamagedStateFailsClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state")
	if err := os.WriteFile(path, []byte("not-json"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewController(path, "boot", true); err == nil {
		t.Fatal("silently replaced damaged state with ON")
	}
}

func TestConnectWaitsForPreviousDown(t *testing.T) {
	c, err := NewController(filepath.Join(t.TempDir(), "state"), "boot", true)
	if err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	c.down = func(context.Context) error { close(entered); <-release; return nil }
	downDone := make(chan error, 1)
	go func() { _, err := c.Request(context.Background(), false); downDone <- err }()
	<-entered
	upDone := make(chan error, 1)
	go func() { _, err := c.Request(context.Background(), true); upDone <- err }()
	select {
	case err := <-upDone:
		t.Fatalf("new Up overtook cleanup: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	close(release)
	if err := <-downDone; err != nil {
		t.Fatal(err)
	}
	if err := <-upDone; err != nil {
		t.Fatal(err)
	}
}
func TestBrokerTransportRejectsUnprivilegedPeer(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("test specifically exercises an unprivileged kernel peer")
	}
	c, err := NewController(filepath.Join(t.TempDir(), "state"), "boot", false)
	if err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(t.TempDir(), "broker")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- c.ServeBroker(ctx, listener) }()
	connection, err := net.Dial("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	_ = connection.SetDeadline(time.Now().Add(time.Second))
	// The server rejects the kernel UID before reading any request bytes.
	var reply BrokerReply
	if err = json.NewDecoder(connection).Decode(&reply); err != nil {
		t.Fatal(err)
	}
	if reply.Error == "" || c.Intent().Connected {
		t.Fatal("unprivileged peer changed intent")
	}
	_ = connection.Close()
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("broker did not stop")
	}
}
func TestBrokerRejectsOversizedAndUnknownMessages(t *testing.T) {
	for _, request := range []string{strings.Repeat("x", 16385), `{"Version":2,"Operation":"Begin","command":"exec"}`} {
		if _, err := decodeBrokerMessage(strings.NewReader(request)); err == nil {
			t.Fatal("accepted invalid protocol message")
		}
	}
}

func TestBrokerBeginWaitsForCancelledCleanup(t *testing.T) {
	c, err := NewController(filepath.Join(t.TempDir(), "state"), "boot", false)
	if err != nil {
		t.Fatal(err)
	}
	started, err := c.Handle(0, Message{Version: 2, Operation: "Begin"})
	if err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	c.down = func(context.Context) error { close(entered); <-release; return nil }
	cancelled := make(chan error, 1)
	go func() {
		_, err := c.Handle(0, Message{Version: 2, Operation: "Cancel", Generation: started.Generation})
		cancelled <- err
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("broker cancel did not clean engine")
	}
	next := make(chan error, 1)
	go func() { _, err := c.Handle(0, Message{Version: 2, Operation: "Begin"}); next <- err }()
	select {
	case <-next:
		t.Fatal("Begin overtook cleanup")
	case <-time.After(30 * time.Millisecond):
	}
	close(release)
	if err := <-cancelled; err != nil {
		t.Fatal(err)
	}
	if err := <-next; err != nil {
		t.Fatal(err)
	}
}

func TestReadyRequiresInterfaceAddress(t *testing.T) {
	for _, addresses := range [][]string{nil, {"192.168.1.100/24"}, {"100.88.19.209/16"}} {
		if readyInterfaceAddress("100.88.19.208", addresses) {
			t.Fatal("accepted missing/wrong VPN IP")
		}
	}
	if !readyInterfaceAddress("100.88.19.208/16", []string{"100.88.19.208/16"}) {
		t.Fatal("rejected expected VPN address")
	}
}
func TestOwnerReplacementReconcilesIntent(t *testing.T) {
	c, err := NewController(filepath.Join(t.TempDir(), "state"), "boot", true)
	if err != nil {
		t.Fatal(err)
	}
	if action := c.Reconcile(Observation{Owner: ":1.1", EngineReady: true, VPNActive: true}); action != NoAction {
		t.Fatalf("unexpected action: %v", action)
	}
	if action := c.Reconcile(Observation{Owner: "", EngineReady: true}); action != NoAction {
		t.Fatal("owner loss is not user OFF")
	}
	if action := c.Reconcile(Observation{Owner: ":1.2", EngineReady: true}); action != Activate {
		t.Fatal("did not restore ON for replacement owner")
	}
	if _, err = c.Request(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if action := c.Reconcile(Observation{Owner: ":1.3"}); action != NoAction {
		t.Fatal("owner replacement lost OFF")
	}
}
func TestDaemonLossClearsConnected(t *testing.T) {
	c, err := NewController(filepath.Join(t.TempDir(), "state"), "boot", true)
	if err != nil {
		t.Fatal(err)
	}
	generation := c.Intent().Generation
	if err = c.Complete(generation, Connected); err != nil {
		t.Fatal(err)
	}
	if action := c.Reconcile(Observation{Owner: ":1.1", VPNActive: true}); action != Deactivate {
		t.Fatal("did not clear stale native VPN")
	}
	if c.State().Phase == Connected {
		t.Fatal("kept Connected without engine")
	}
}

func TestNeedsLoginSurvivesPluginFailureCleanup(t *testing.T) {
	c, err := NewController(filepath.Join(t.TempDir(), "state"), "boot", true)
	if err != nil {
		t.Fatal(err)
	}
	_ = c.Reconcile(Observation{Owner: ":1.1", EngineAvailable: true})
	active, err := c.Handle(0, Message{Version: 2, Operation: "Begin"})
	if err != nil {
		t.Fatal(err)
	}
	if err = c.Complete(active.Generation, NeedsLogin); err != nil {
		t.Fatal(err)
	}
	if _, err = c.Handle(0, Message{Version: 2, Operation: "Cancel", Generation: active.Generation}); err != nil {
		t.Fatal(err)
	}
	if err = c.ObserveNMStop(context.Background(), ":1.1", active.Generation, false); err != nil {
		t.Fatal(err)
	}
	if action := c.Reconcile(Observation{Owner: ":1.1", EngineAvailable: true}); action != NoAction {
		t.Fatal("automatic retry resumed after an explicit NeedsLogin result")
	}
	if c.State().Phase != NeedsLogin || !c.Intent().Connected {
		t.Fatal("login requirement or saved ON intention was lost during cleanup")
	}
}

func TestPluginStopDuringNMRestartPreservesIntent(t *testing.T) {
	c, err := NewController(filepath.Join(t.TempDir(), "state"), "boot", true)
	if err != nil {
		t.Fatal(err)
	}
	_ = c.Reconcile(Observation{Owner: ":1.1", EngineReady: true, VPNActive: true})
	started, err := c.Handle(0, Message{Version: 2, Operation: "Begin"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.Handle(0, Message{Version: 2, Operation: "Cancel", Generation: started.Generation}); err != nil {
		t.Fatal(err)
	}
	if action := c.Reconcile(Observation{Owner: ":1.1", EngineAvailable: true}); action != NoAction {
		t.Fatal("restarted before NM classified the stop")
	}
	if action := c.Reconcile(Observation{Owner: ":1.2", EngineAvailable: true}); action != Activate {
		t.Fatal("service restart lost original ON intent")
	}
}

func TestOnlyCurrentNMSessionCanSetOff(t *testing.T) {
	c, err := NewController(filepath.Join(t.TempDir(), "state"), "boot", true)
	if err != nil {
		t.Fatal(err)
	}
	_ = c.Reconcile(Observation{Owner: ":1.1", EngineReady: true, VPNActive: true})
	active, err := c.Handle(0, Message{Version: 2, Operation: "Begin"})
	if err != nil {
		t.Fatal(err)
	}
	if err = c.ObserveNMStop(context.Background(), ":1.0", active.Generation, true); err != nil {
		t.Fatal(err)
	}
	if !c.Intent().Connected {
		t.Fatal("accepted old NM owner")
	}
	if err = c.ObserveNMStop(context.Background(), ":1.1", active.Generation+1, true); err != nil {
		t.Fatal(err)
	}
	if !c.Intent().Connected {
		t.Fatal("accepted foreign session")
	}
	if err = c.ObserveNMStop(context.Background(), ":1.1", active.Generation, true); err != nil {
		t.Fatal(err)
	}
	if c.Intent().Connected {
		t.Fatal("lost explicit OFF")
	}
	_ = c.Reconcile(Observation{Owner: ":1.2", EngineAvailable: true})
	if c.Intent().Connected {
		t.Fatal("restart revived explicit OFF")
	}
}

func TestOwnerReplacementRejectsLateCompletionWithoutPluginCancel(t *testing.T) {
	c, err := NewController(filepath.Join(t.TempDir(), "state"), "boot", true)
	if err != nil {
		t.Fatal(err)
	}
	_ = c.Reconcile(Observation{Owner: ":1.1", EngineAvailable: true})
	active, err := c.Handle(0, Message{Version: 2, Operation: "Begin"})
	if err != nil {
		t.Fatal(err)
	}
	_ = c.Reconcile(Observation{Owner: ":1.2", EngineReady: true})
	if err = c.Complete(active.Generation, Connected); err == nil {
		t.Fatal("accepted completion from previous NM owner")
	}
	if !c.Intent().Connected {
		t.Fatal("lost original ON intent")
	}
}

func TestSameOwnerStopWaitsForClassification(t *testing.T) {
	c, err := NewController(filepath.Join(t.TempDir(), "state"), "boot", true)
	if err != nil {
		t.Fatal(err)
	}
	_ = c.Reconcile(Observation{Owner: ":1.1", EngineAvailable: true})
	active, err := c.Handle(0, Message{Version: 2, Operation: "Begin"})
	if err != nil {
		t.Fatal(err)
	}
	_ = c.Reconcile(Observation{Owner: ":1.1", EngineReady: true, VPNActive: true})
	if action := c.Reconcile(Observation{Owner: ":1.1", EngineAvailable: true}); action != NoAction {
		t.Fatal("reconnected before stop classification")
	}
	if err = c.ObserveNMStop(context.Background(), ":1.1", active.Generation, false); err != nil {
		t.Fatal(err)
	}
	if action := c.Reconcile(Observation{Owner: ":1.1", EngineAvailable: true}); action != Activate {
		t.Fatal("failed to recover confirmed service failure")
	}
}
func TestStaleObservationCannotStopNewAttempt(t *testing.T) {
	c, err := NewController(filepath.Join(t.TempDir(), "state"), "boot", true)
	if err != nil {
		t.Fatal(err)
	}
	previous := c.Intent().Generation
	if _, err = c.Handle(0, Message{Version: 2, Operation: "Begin"}); err != nil {
		t.Fatal(err)
	}
	if action := c.Reconcile(Observation{Generation: previous, Owner: ":1.1", VPNActive: true}); action != NoAction {
		t.Fatal("accepted observation from preceding generation")
	}
}

func TestBeginPersistsBeforeEngineLease(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state")
	c, err := NewController(path, "boot", false)
	if err != nil {
		t.Fatal(err)
	}
	var failed uint64
	c.prepare = func(_ context.Context, generation uint64) error {
		saved, err := loadIntent(path)
		if err != nil {
			return err
		}
		if saved.Intent.Generation != generation {
			t.Fatal("engine lease precedes durable generation")
		}
		failed = generation
		return fmt.Errorf("engine unavailable")
	}
	if _, err = c.Handle(0, Message{Version: 2, Operation: "Begin"}); err == nil {
		t.Fatal("ignored engine preparation error")
	}
	c.prepare = nil
	next, err := c.Handle(0, Message{Version: 2, Operation: "Begin"})
	if err != nil {
		t.Fatal(err)
	}
	if next.Generation <= failed {
		t.Fatal("reused generation after lost preparation reply")
	}
}

func TestNeedsLoginDoesNotAutoRetryAfterCleanup(t *testing.T) {
	c, err := NewController(filepath.Join(t.TempDir(), "state"), "boot", true)
	if err != nil {
		t.Fatal(err)
	}
	_ = c.Reconcile(Observation{Owner: ":1.1", EngineAvailable: true})
	active, err := c.Handle(0, Message{Version: 2, Operation: "Begin"})
	if err != nil {
		t.Fatal(err)
	}
	if err = c.Complete(active.Generation, NeedsLogin); err != nil {
		t.Fatal(err)
	}
	if err = c.ObserveNMStop(context.Background(), ":1.1", active.Generation, false); err != nil {
		t.Fatal(err)
	}
	if action := c.Reconcile(Observation{Owner: ":1.1", EngineAvailable: true}); action != NoAction {
		t.Fatal("keeps starting new SSO instead of waiting for user")
	}
}

func TestStartupDropsMissedStopBindingWithoutChangingIntent(t *testing.T) {
	for _, on := range []bool{false, true} {
		c, err := NewController(filepath.Join(t.TempDir(), "state"), "boot", on)
		if err != nil {
			t.Fatal(err)
		}
		c.saved.ActivationOwner = ":1.5"
		c.saved.ActivationGeneration = 1
		c.saved.PendingCancel = true
		if err = c.recoverInitialObservation(Observation{Owner: ":1.5"}); err != nil {
			t.Fatal(err)
		}
		if c.Intent().Connected != on || c.saved.PendingCancel || c.saved.ActivationGeneration != 0 {
			t.Fatalf("missed stop stuck or intent lost: %+v", c.saved)
		}
	}
}

func TestBrokerCarriesNonInteractiveActivationPolicy(t *testing.T) {
	c, err := NewController(filepath.Join(t.TempDir(), "state"), "boot", true)
	if err != nil {
		t.Fatal(err)
	}
	c.track = func(context.Context, uint64, Intent) (activationSession, error) {
		return activationSession{owner: ":1.1", automatic: true}, nil
	}
	lease, err := c.Handle(0, Message{Version: 2, Operation: "Begin"})
	if err != nil || !lease.NonInteractive {
		t.Fatalf("automatic lease permits browser interaction: %+v %v", lease, err)
	}
}

func TestAutomaticLeasePolicySurvivesControllerRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state")
	c, err := NewController(path, "boot", true)
	if err != nil {
		t.Fatal(err)
	}
	c.track = func(context.Context, uint64, Intent) (activationSession, error) {
		return activationSession{owner: ":1.1", automatic: true}, nil
	}
	if _, err = c.Handle(0, Message{Version: 1, Operation: "Begin"}); err == nil {
		t.Fatal("accepted obsolete broker contract")
	}
	if _, err = c.Handle(0, Message{Version: 2, Operation: "Begin"}); err != nil {
		t.Fatal(err)
	}
	restored, err := NewController(path, "boot", true)
	if err != nil || !restored.State().NonInteractive {
		t.Fatalf("lost automatic policy at restart: %+v %v", restored, err)
	}
}
