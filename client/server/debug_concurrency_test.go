//go:build !android && !ios

package server

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/netbirdio/netbird/client/internal"
	"github.com/netbirdio/netbird/client/internal/profilemanager"
	"github.com/netbirdio/netbird/client/proto"
)

// A slow log reader models filesystem I/O without introducing a production test hook.
// The archive must not retain the daemon mutex while waiting for that reader.
func TestDebugBundleDoesNotBlockControlRPCs(t *testing.T) {
	for _, operation := range []string{"status", "disconnect"} {
		t.Run(operation, func(t *testing.T) {
			ctx := internal.CtxInitState(context.Background())
			s := New(ctx, filepath.Join(t.TempDir(), "absent.log"), "", false, false, false, false)
			s.config = &profilemanager.Config{}
			s.uiLogPath = filepath.Join(t.TempDir(), "gui-client.log")
			if err := os.WriteFile(s.uiLogPath, []byte("synthetic UI log\n"), 0600); err != nil {
				t.Fatal(err)
			}
			entered, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			done := make(chan error, 1)
			go func() {
				path, _, err := s.generateDebugBundle(&proto.DebugBundleRequest{}, func(path string) (*os.File, error) {
					once.Do(func() { close(entered) })
					<-release
					return os.Open(path)
				})
				if path != "" {
					_ = os.Remove(path)
				}
				done <- err
			}()
			t.Cleanup(func() {
				close(release)
				select {
				case err := <-done:
					if err != nil {
						t.Errorf("bundle: %v", err)
					}
				case <-time.After(20 * time.Second):
					t.Error("bundle did not finish after releasing log reader")
				}
			})
			select {
			case <-entered:
			case <-time.After(20 * time.Second):
				t.Fatal("bundle did not reach log reader")
			}
			replied := make(chan error, 1)
			go func() {
				if operation == "status" {
					_, err := s.Status(ctx, &proto.StatusRequest{})
					replied <- err
				} else {
					// Disconnect is idempotent on this offline synthetic server.
					_, err := s.Down(ctx, &proto.DownRequest{})
					replied <- err
				}
			}()
			select {
			case err := <-replied:
				if err != nil && operation == "status" {
					t.Fatalf("Status: %v", err)
				}
			case <-time.After(time.Second):
				t.Fatal("control RPC blocked by unfinished diagnostic archive")
			}
		})
	}
}
