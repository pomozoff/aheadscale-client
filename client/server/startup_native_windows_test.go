//go:build windows && windowsvpnprototype

package server

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/netbirdio/netbird/client/internal"
	"github.com/netbirdio/netbird/client/internal/profilemanager"
)

// Запуск службы с сохранённой регистрацией не запускает движок раньше
// системного VPN; иначе он ждёт канал и мешает первому запросу GUI.
func TestNativeServiceWaitsForSystemVPN(t *testing.T) {
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
	s := New(ctx, "console", "", true, false, false, false)
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
