//go:build windows && integration

package windowsstartup

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
	"unicode/utf16"

	"golang.org/x/sys/windows"
)

const installedUI = `C:\Program Files\Aheadscale\native\aheadscale-ui.exe`

func powershell(t *testing.T, script string) string {
	t.Helper()
	dir, err := windows.GetSystemDirectory()
	if err != nil {
		t.Fatal(err)
	}
	words := utf16.Encode([]rune("$ProgressPreference='SilentlyContinue';" + script))
	data := make([]byte, len(words)*2)
	for i, w := range words {
		binary.LittleEndian.PutUint16(data[2*i:], w)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, filepath.Join(dir, `WindowsPowerShell\v1.0\powershell.exe`), "-NoProfile", "-NonInteractive", "-EncodedCommand", base64.StdEncoding.EncodeToString(data))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("PowerShell failed: %v: %s", err, out)
	}
	return string(out)
}
func fixture(t *testing.T) *Manager {
	t.Helper()
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	m := New(installedUI)
	m.suffix = "-test-" + hex.EncodeToString(b)
	t.Cleanup(func() {
		powershell(t, `$ErrorActionPreference='Stop';$s=New-Object -ComObject Schedule.Service;$s.Connect();$n='Aheadscale-Startup-'+[Security.Principal.WindowsIdentity]::GetCurrent().User.Value+'`+m.suffix+`';try{$s.GetFolder('\').DeleteTask($n,0)}catch{if($_.Exception.GetBaseException().HResult-ne-2147024894){throw}};Remove-ItemProperty 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Run' -Name 'Aheadscale`+m.suffix+`' -ErrorAction SilentlyContinue;Remove-ItemProperty 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run' -Name 'Aheadscale`+m.suffix+`' -ErrorAction SilentlyContinue;exit 0`)
	})
	return m
}
func seedRun(t *testing.T, m *Manager) {
	t.Helper()
	powershell(t, `$null=New-Item 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Run' -Force;Set-ItemProperty 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Run' -Name 'Aheadscale`+m.suffix+`' -Value '"`+installedUI+`"'`)
}
func TestStartupMigratesOwnRunAndRemovesDuplicate(t *testing.T) {
	m := fixture(t)
	seedRun(t, m)
	s, err := m.State(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !s.Exists || !s.Enabled {
		t.Fatal("enabled legacy autostart did not become an enabled startup task")
	}
	var got struct {
		Path   string
		Level  int
		Logon  int
		Legacy bool
	}
	out := powershell(t, `$ErrorActionPreference='Stop';$s=New-Object -ComObject Schedule.Service;$s.Connect();$task=$s.GetFolder('\').GetTask('Aheadscale-Startup-'+[Security.Principal.WindowsIdentity]::GetCurrent().User.Value+'`+m.suffix+`');@{Path=$task.Definition.Actions.Item(1).Path;Level=$task.Definition.Principal.RunLevel;Logon=$task.Definition.Principal.LogonType;Legacy=([bool](Get-ItemProperty 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Run' -Name 'Aheadscale`+m.suffix+`' -ErrorAction SilentlyContinue))}|ConvertTo-Json -Compress`)
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatal(err)
	}
	if got.Path != installedUI || got.Level != 0 || got.Logon != 3 || got.Legacy {
		t.Fatalf("wrong OS startup contract: %+v", got)
	}
	if _, err := m.State(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := m.SetEnabled(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	s, err = m.State(context.Background())
	if err != nil || s.Enabled || s.Exists {
		t.Fatalf("disabled startup remains registered: %+v %v", s, err)
	}
	if err := m.SetEnabled(context.Background(), true); err != nil {
		t.Fatal(err)
	}
}
func TestStartupDoesNotEnableAbsentOrDisabledLegacy(t *testing.T) {
	m := fixture(t)
	s, err := m.State(context.Background())
	if err != nil || s.Enabled || s.Exists {
		t.Fatalf("absent startup enabled: %+v %v", s, err)
	}
	seedRun(t, m)
	powershell(t, `$null=New-Item 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run' -Force;Set-ItemProperty 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run' -Name 'Aheadscale`+m.suffix+`' -Value ([byte[]]@(3,0,0,0,0,0,0,0,0,0,0,0))`)
	s, err = m.State(context.Background())
	if err != nil || s.Enabled || s.Exists {
		t.Fatalf("disabled startup enabled: %+v %v", s, err)
	}
}
func TestStartupRefusesModifiedTask(t *testing.T) {
	m := fixture(t)
	if err := m.SetEnabled(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	powershell(t, `$ErrorActionPreference='Stop';$s=New-Object -ComObject Schedule.Service;$s.Connect();$root=$s.GetFolder('\');$n='Aheadscale-Startup-'+[Security.Principal.WindowsIdentity]::GetCurrent().User.Value+'`+m.suffix+`';$d=$root.GetTask($n).Definition;$d.Actions.Item(1).Path='C:\Windows\notepad.exe';$null=$root.RegisterTaskDefinition($n,$d,4,$d.Principal.UserId,$null,3)`)
	if err := m.SetEnabled(context.Background(), false); err == nil {
		t.Fatal("modified startup task was accepted for removal")
	}
	out := powershell(t, `$s=New-Object -ComObject Schedule.Service;$s.Connect();$s.GetFolder('\').GetTask('Aheadscale-Startup-'+[Security.Principal.WindowsIdentity]::GetCurrent().User.Value+'`+m.suffix+`').Definition.Actions.Item(1).Path`)
	if len(out) == 0 {
		t.Fatal("foreign task lost")
	}
}

func TestStartupExplicitEnableSurvivesUpgrade(t *testing.T) {
	m := fixture(t)
	seedRun(t, m)
	powershell(t, `$null=New-Item 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run' -Force;Set-ItemProperty 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run' -Name 'Aheadscale`+m.suffix+`' -Value ([byte[]]@(3,0,0,0,0,0,0,0,0,0,0,0))`)
	if err := m.SetEnabled(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if _, err := m.run(context.Background(), "remove"); err != nil {
		t.Fatal(err)
	}
	seedRun(t, m) // Установщик возвращает включённый выбор новой версии клиента.
	state, err := m.State(context.Background())
	if err != nil || !state.Enabled {
		t.Fatalf("explicit enable lost after upgrade round-trip: %+v %v", state, err)
	}
}
