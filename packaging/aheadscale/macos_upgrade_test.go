//go:build !windows

package aheadscale

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMacOSPreinstallGUIExit(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash is required")
	}
	// The script's service branch must never touch a real installed Mac client.
	if _, err := os.Stat("/Applications/Aheadscale.app/Contents/MacOS/aheadscale"); !os.IsNotExist(err) {
		t.Skip("requires a host without an installed macOS client")
	}
	const gui = "/Applications/Aheadscale.app/Contents/MacOS/aheadscale-ui"
	const other = "/tmp/another-app/aheadscale-ui"
	for _, tc := range []struct {
		name, mode, listing, identity, signals string
		fails                                  bool
	}{
		{"installed GUI", "stop", "101 " + gui, "501 Mon Oct 26 01:00:00 2026 " + gui, "-TERM 101\n", false},
		{"no GUI", "stop", "", "", "", false},
		{"same name elsewhere", "stop", "101 " + other, "501 Mon Oct 26 01:00:00 2026 " + other, "", false},
		{"exited before lookup", "stop", "101 " + gui, "", "", false},
		{"PID reused before signal", "reuse", "101 " + gui, "501 Mon Oct 26 01:00:00 2026 " + gui, "", false},
		{"termination refused", "refuse", "101 " + gui, "501 Mon Oct 26 01:00:00 2026 " + gui, "-TERM 101\n", true},
		{"GUI ignores TERM", "stubborn", "101 " + gui, "501 Mon Oct 26 01:00:00 2026 " + gui, "-TERM 101\n", true},
		{"process enumeration fails", "ps-fails", "", "", "", true},
		{"foreign argv0 spoof", "spoof", "101 " + gui, "501 Mon Oct 26 01:00:00 2026 " + gui, "", false},
		{"native path query fails", "probe-fails", "101 " + gui, "501 Mon Oct 26 01:00:00 2026 " + gui, "", true},
		{"native path unavailable for live PID", "unavailable", "101 " + gui, "501 Mon Oct 26 01:00:00 2026 " + gui, "", true},
		{"unexpected path query reply", "bad-reply", "101 " + gui, "501 Mon Oct 26 01:00:00 2026 " + gui, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := t.TempDir()
			write := func(name, contents string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(fixture, name), []byte(contents), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			write("list", strings.ReplaceAll(strings.ReplaceAll(tc.listing, gui, "aheadscale-ui"), other, "aheadscale-ui"))
			if tc.identity != "" {
				write("identity.101", tc.identity)
			}
			// Mock only OS process primitives; execute the real preinstall script.
			write("commands", `ps() {
    if [[ "$1" == '-axo' ]]; then
        [[ "$MODE" != 'ps-fails' ]] || return 1
        if [[ ! -f "$FIXTURE/scanned" ]]; then
            : > "$FIXTURE/scanned"
            cat "$FIXTURE/list"
            return 0
        fi
        for entry in "$FIXTURE"/identity.*; do
            [[ -f "$entry" ]] || continue
            contents=$(cat "$entry")
            executable=${contents##* }
            printf '%s %s\n' "${entry##*.}" "${executable##*/}"
        done
    elif [[ "$1" == '-p' ]]; then
        [[ -f "$FIXTURE/identity.$2" ]] || return 1
        if [[ "$MODE" == 'reuse' && -f "$FIXTURE/read-once" ]]; then
            printf '%s\n' '502 Mon Oct 26 02:00:00 2026 /tmp/another-app/aheadscale-ui' > "$FIXTURE/identity.$2"
        fi
        : > "$FIXTURE/read-once"
        contents=$(cat "$FIXTURE/identity.$2")
        printf '%s\n' "${contents% *}"
    else
        return 97
    fi
}
osascript() {
    case "$MODE" in
        spoof) printf 'other\n'; return 0 ;;
        probe-fails) return 1 ;;
        unavailable) printf 'unavailable\n'; return 0 ;;
        bad-reply) printf 'invalid\n'; return 0 ;;
    esac
    if [[ ! -f "$FIXTURE/identity.$4" ]]; then
        printf 'unavailable\n'
        return 0
    fi
    contents=$(cat "$FIXTURE/identity.$4")
    if [[ "${contents##* }" == "$5" ]]; then printf 'match\n'; else printf 'other\n'; fi
}
kill() {
    printf '%s\n' "$*" >> "$FIXTURE/signals"
    [[ "$MODE" != 'refuse' ]] || return 1
    if [[ "$MODE" != 'stubborn' ]]; then
        rm -f "$FIXTURE/identity.$2"
    fi
}
sleep() { :; }
`)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, bash, "scripts/macos-preinstall")
			cmd.Env = append(os.Environ(), "BASH_ENV="+filepath.Join(fixture, "commands"), "FIXTURE="+fixture, "MODE="+tc.mode)
			output, err := cmd.CombinedOutput()
			if ctx.Err() != nil {
				t.Fatal("preinstall did not finish within the fixture deadline")
			}
			if (err != nil) != tc.fails {
				t.Fatalf("unexpected exit: %v; output: %s", err, output)
			}
			signals, readErr := os.ReadFile(filepath.Join(fixture, "signals"))
			if readErr != nil && !os.IsNotExist(readErr) {
				t.Fatal(readErr)
			}
			if string(signals) != tc.signals || strings.Contains(string(signals), "KILL") {
				t.Fatalf("unexpected process signals: got %q, want %q", signals, tc.signals)
			}
		})
	}
}
