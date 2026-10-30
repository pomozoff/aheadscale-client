//go:build linux && !android

package linuxvpn

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"syscall"
)

const StandaloneHandoverPath = "/run/aheadscale/standalone-handover.json"

// The installer transfers only an acknowledged intent, never config or keys.
// It affects this boot until an explicit standalone Up; reboot resumes the
// ordinary persisted auto-connect preference.
func StandaloneBootConnect(path, bootID string) (bool, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	owner, ok := info.Sys().(*syscall.Stat_t)
	if !ok || owner.Uid != uint32(os.Geteuid()) || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return false, fmt.Errorf("небезопасный файл переноса намерения Aheadscale")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	var marker struct {
		Version   int    `json:"version"`
		BootID    string `json:"boot_id"`
		Connected bool   `json:"connected"`
	}
	if err = json.Unmarshal(data, &marker); err != nil {
		return false, err
	}
	if marker.Version != 1 || marker.BootID == "" {
		return false, fmt.Errorf("неподдерживаемый формат переноса Aheadscale")
	}
	if marker.BootID != bootID {
		return true, nil
	}
	return marker.Connected, nil
}
func ClearStandaloneHandover(path string) error {
	if _, err := StandaloneBootConnect(path, "validate"); err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
