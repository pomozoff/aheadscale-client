package windowsstartup

import (
	"context"
	_ "embed"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"
	"unicode/utf16"

	"golang.org/x/sys/windows"
)

//go:embed task.ps1
var taskScript string
var operations sync.Mutex

// State reports the current user's validated startup registration.
type State struct {
	Exists  bool `json:"exists"`
	Enabled bool `json:"enabled"`
}

// Manager controls only Aheadscale's non-elevated logon task.
type Manager struct {
	executable string
	suffix     string
}

// New selects the installed executable to start at the current user's logon.
func New(executable string) *Manager { return &Manager{executable: executable} }

// State migrates an enabled legacy Run entry and reads the resulting task.
func (m *Manager) State(ctx context.Context) (State, error) { return m.run(ctx, "state") }

// SetEnabled changes startup for subsequent logons without starting the app.
func (m *Manager) SetEnabled(ctx context.Context, enabled bool) error {
	operation := "disable"
	if enabled {
		operation = "enable"
	}
	_, err := m.run(ctx, operation)
	return err
}

func (m *Manager) run(ctx context.Context, operation string) (State, error) {
	operations.Lock()
	defer operations.Unlock()
	dir, err := windows.GetSystemDirectory()
	if err != nil {
		return State{}, err
	}
	request, err := json.Marshal(map[string]string{"operation": operation, "executable": m.executable, "suffix": m.suffix})
	if err != nil {
		return State{}, err
	}
	script := "$ProgressPreference='SilentlyContinue';[Console]::OutputEncoding=[Text.UTF8Encoding]::new($false);$p=$env:AHEADSCALE_STARTUP_REQUEST|ConvertFrom-Json; & {\n" + taskScript + "\n} -Operation $p.operation -Executable $p.executable -TestSuffix $p.suffix"
	words := utf16.Encode([]rune(script))
	data := make([]byte, len(words)*2)
	for i, w := range words {
		binary.LittleEndian.PutUint16(data[2*i:], w)
	}
	timeout, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(timeout, filepath.Join(dir, `WindowsPowerShell\v1.0\powershell.exe`), "-NoProfile", "-NonInteractive", "-EncodedCommand", base64.StdEncoding.EncodeToString(data))
	cmd.Env = append(os.Environ(), "AHEADSCALE_STARTUP_REQUEST="+string(request))
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	cmd.WaitDelay = 3 * time.Second
	out, err := cmd.CombinedOutput()
	if err != nil {
		return State{}, fmt.Errorf("операция автозапуска %s: %w: %s", operation, err, out)
	}
	var state State
	if err := json.Unmarshal(out, &state); err != nil {
		return State{}, fmt.Errorf("ответ автозапуска: %w", err)
	}
	return state, nil
}
