//go:build linux && !android

package linuxvpn

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

type Phase string

const (
	Disconnected Phase = "Disconnected"
	Connecting   Phase = "Connecting"
	Connected    Phase = "Connected"
	NeedsLogin   Phase = "NeedsLogin"
	Failed       Phase = "Failed"
)

type Intent struct {
	Connected  bool
	Generation uint64
}
type State struct {
	NonInteractive bool
	Generation     uint64
	Phase          Phase
	Error          string
}
type savedIntent struct {
	ActivationNonInteractive bool
	PendingCancel            bool
	ActivationOwner          string
	ActivationGeneration     uint64
	Version                  int
	BootID                   string
	Intent                   Intent
}

func loadIntent(path string) (savedIntent, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return savedIntent{}, err
	}
	owner, owned := info.Sys().(*syscall.Stat_t)
	if !owned || owner.Uid != uint32(os.Geteuid()) || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return savedIntent{}, fmt.Errorf("небезопасный файл состояния NetworkManager")
	}
	f, err := os.Open(path)
	if err != nil {
		return savedIntent{}, err
	}
	defer f.Close()
	var saved savedIntent
	decoder := json.NewDecoder(f)
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&saved); err != nil {
		return savedIntent{}, err
	}
	if err = decoder.Decode(new(any)); err != io.EOF {
		return savedIntent{}, fmt.Errorf("лишние данные состояния NetworkManager")
	}
	if saved.Version != 1 || saved.BootID == "" {
		return savedIntent{}, fmt.Errorf("неподдерживаемое состояние NetworkManager")
	}
	return saved, nil
}
func saveIntent(path string, saved savedIntent) error {
	data, err := json.Marshal(saved)
	if err != nil {
		return err
	}
	// Parent directory belongs to the root service, never to an NM profile.
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".nm-state-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
