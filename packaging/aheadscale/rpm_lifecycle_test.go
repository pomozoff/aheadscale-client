//go:build !windows

package aheadscale

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Старая .24 удаляется после установки нового варианта. Исправление должно
// пережить именно её disable/stop, а не только собственный новый preun.
func TestRPMTransactionPreservesServiceState(t *testing.T) {
	for _, tc := range []struct {
		name, enabled, active, wantEnabled, wantActive string
		installed                                      bool
	}{
		{"desktop to CLI", "enabled", "active", "enabled", "active", true},
		{"CLI to desktop", "enabled", "active", "enabled", "active", true},
		{"disabled and stopped", "disabled", "inactive", "disabled", "inactive", true},
		{"enabled but stopped", "enabled", "inactive", "enabled", "inactive", true},
		{"disabled but active", "disabled", "active", "disabled", "active", true},
		{"fresh installation", "disabled", "inactive", "enabled", "active", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := t.TempDir()
			if err := os.MkdirAll(filepath.Join(fixture, "systemd/system"), 0700); err != nil {
				t.Fatal(err)
			}
			for name, value := range map[string]string{"enabled": tc.enabled, "active": tc.active} {
				if err := os.WriteFile(filepath.Join(fixture, name), []byte(value+"\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if tc.installed {
				if err := os.WriteFile(filepath.Join(fixture, "installed"), nil, 0600); err != nil {
					t.Fatal(err)
				}
			}
			spec, err := os.ReadFile("aheadscale.spec.in")
			if err != nil {
				t.Fatal(err)
			}
			body := strings.ReplaceAll(string(spec), "%{_rundir}", fixture)
			body = strings.ReplaceAll(body, "/run/systemd/system", filepath.Join(fixture, "systemd/system"))
			body = strings.ReplaceAll(body, "%systemd_post aheadscale.service", ":")
			helper, readErr := os.ReadFile("scripts/rpm-lifecycle")
			if readErr != nil && !os.IsNotExist(readErr) {
				t.Fatal(readErr)
			}
			body = strings.ReplaceAll(body, "@RPM_LIFECYCLE@", string(helper))
			commands := `
rpm() { [ "$1" = '-q' ] || return 97; [ -f "$FIXTURE/installed" ]; }
systemctl() {
    operation=$1; shift
    case "$*" in *aheadscale.service*) ;; *) [ "$operation" = daemon-reload ] || return 98 ;; esac
    case "$operation" in
        is-enabled) cat "$FIXTURE/enabled"; [ "$(cat "$FIXTURE/enabled")" = enabled ] ;;
        is-active) cat "$FIXTURE/active"; [ "$(cat "$FIXTURE/active")" = active ] ;;
        enable) printf 'enabled\n' > "$FIXTURE/enabled"; case "$*" in *--now*) printf 'active\n' > "$FIXTURE/active" ;; esac ;;
        disable) printf 'disabled\n' > "$FIXTURE/enabled"; case "$*" in *--now*) printf 'inactive\n' > "$FIXTURE/active" ;; esac ;;
        restart|start) printf 'active\n' > "$FIXTURE/active" ;;
        stop) printf 'inactive\n' > "$FIXTURE/active" ;;
        daemon-reload) : ;;
        *) return 99 ;;
    esac
}
`
			script := commands + rpmTestHook(body, "pre") + "\n" + rpmTestHook(body, "post")
			if tc.installed {
				script += "\nsystemctl disable --now aheadscale.service\n"
			}
			script += "\n" + rpmTestHook(body, "posttrans")
			cmd := exec.Command("bash", "-e", "-c", script, "rpm-hook", "1")
			cmd.Env = append(os.Environ(), "FIXTURE="+fixture)
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("scriptlets: %v\n%s", err, output)
			}
			for name, want := range map[string]string{"enabled": tc.wantEnabled, "active": tc.wantActive} {
				value, err := os.ReadFile(filepath.Join(fixture, name))
				if err != nil {
					t.Fatal(err)
				}
				if got := strings.TrimSpace(string(value)); got != want {
					t.Errorf("after legacy removal %s = %q, want %q", name, got, want)
				}
			}
		})
	}
}

func rpmTestHook(spec, name string) string {
	_, tail, found := strings.Cut(spec, "\n%"+name+"\n")
	if !found {
		return ""
	}
	hook, _, _ := strings.Cut(tail, "\n%")
	return hook
}
