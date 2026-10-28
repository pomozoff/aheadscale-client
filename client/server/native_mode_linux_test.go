//go:build linux && !android

package server

import (
	"context"
	"github.com/netbirdio/netbird/client/proto"
	"path/filepath"
	"testing"

	"github.com/netbirdio/netbird/client/internal"
	"github.com/netbirdio/netbird/client/internal/profilemanager"
)

// Запуск службы с сохранённой регистрацией не запускает движок раньше
// системного VPN; иначе он ждёт канал и мешает первому запросу GUI.
func TestNativeModeServesRPCWithoutStartingEngine(t *testing.T) {
	dir := t.TempDir()
	oldDir, oldConfig, oldActive, oldOverride := profilemanager.DefaultConfigPathDir, profilemanager.DefaultConfigPath, profilemanager.ActiveProfileStatePath, profilemanager.ConfigDirOverride
	profilemanager.DefaultConfigPathDir, profilemanager.DefaultConfigPath, profilemanager.ActiveProfileStatePath, profilemanager.ConfigDirOverride = dir, filepath.Join(dir, "default.json"), filepath.Join(dir, "active_profile.json"), dir
	t.Cleanup(func() {
		profilemanager.DefaultConfigPathDir, profilemanager.DefaultConfigPath, profilemanager.ActiveProfileStatePath, profilemanager.ConfigDirOverride = oldDir, oldConfig, oldActive, oldOverride
	})
	_, err := profilemanager.UpdateOrCreateConfig(profilemanager.ConfigInput{ConfigPath: profilemanager.DefaultConfigPath, ManagementURL: "https://startup-test.invalid:443"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(internal.CtxInitState(context.Background()))
	defer cancel()
	s := New(ctx, "console", "", true, false, false, false, WithNativeNetworkManager(true))
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	s.mutex.Lock()
	started := s.clientRunning || s.clientRunningChan != nil
	config := s.config
	s.mutex.Unlock()
	if started {
		t.Fatal("служба запустила движок до подключения системного VPN")
	}
	state, err := internal.CtxGetState(ctx).Status()
	if err != nil || state != internal.StatusIdle {
		t.Fatalf("состояние после старта: %s %v", state, err)
	}
	if config == nil || config.DisableAutoConnect {
		t.Fatal("опытный режим изменил настройку автоподключения общего профиля")
	}
}

func TestStandaloneKeepsAutoConnect(t *testing.T) {
	s := &Server{rootCtx: context.Background()}
	WithNativeNetworkManager(false)(s)
	if !s.connectOnStart() {
		t.Fatal("standalone startup was disabled")
	}
	WithNativeNetworkManager(true)(s)
	if s.connectOnStart() {
		t.Fatal("native mode started engine independently")
	}
	features, err := s.GetFeatures(context.Background(), &proto.GetFeaturesRequest{})
	if err != nil || !features.GetNativeNetworkmanager() {
		t.Fatalf("native mode not advertised: %v", err)
	}
}
