//go:build linux && !android

package main

import (
	"context"
	"github.com/wailsapp/wails/v3/pkg/application"
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// Policy regression: select actual Linux production files, not a particular
// filename. Never deliver SIGUSR1 to JavaScriptCore to test its removal.
func TestLinuxWindowReopenDoesNotClaimWebKitSignal(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		selected, err := build.Default.MatchFile(".", name)
		if err != nil {
			t.Fatal(err)
		}
		if !selected {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			if selector, ok := node.(*ast.SelectorExpr); ok && selector.Sel.Name == "SIGUSR1" {
				t.Errorf("%s: Linux UI must leave SIGUSR1 to WebKit; use SingleInstance/tray", name)
			}
			return true
		})
	}
}
func TestSecondInstanceUsesExistingWindowCallback(t *testing.T) {
	// Wails has a process-wide application singleton. Isolate this constructor
	// from TestApplicationErrorsReachClientLog instead of changing production.
	if os.Getenv("AHEADSCALE_WINDOW_CALLBACK_CHILD") != "1" {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestSecondInstanceUsesExistingWindowCallback$", "-test.v")
		command.Env = append(os.Environ(), "AHEADSCALE_WINDOW_CALLBACK_CHILD=1")
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("isolated Wails callback: %v\n%s", err, output)
		}
		return
	}

	calls := 0
	app := newApplication(func() { calls++ })
	options := app.Config().SingleInstance
	if options == nil || options.UniqueID != "ru.aheadscale.ui" || options.OnSecondInstanceLaunch == nil {
		t.Fatal("missing stable SingleInstance controller")
	}
	for i := 0; i < 3; i++ {
		options.OnSecondInstanceLaunch(application.SecondInstanceData{})
	}
	if calls != 3 {
		t.Fatalf("window callback called %d times, want 3", calls)
	}
}
