package configs

import (
	"os"
	"path/filepath"
	"runtime"
)

// UILogFile is the file name the desktop UI writes its log to. It is defined
// here so the UI (writer), the daemon's RegisterUILog validation, and the debug
// bundle collector all share one definition.
const UILogFile = "gui-client.log"

var StateDir string

func init() {
	StateDir = os.Getenv("AS_STATE_DIR")
	if StateDir != "" {
		return
	}
	switch runtime.GOOS {
	case "windows":
		StateDir = filepath.Join(os.Getenv("PROGRAMDATA"), "Aheadscale")
	case "darwin", "linux":
		StateDir = "/var/lib/aheadscale"
	case "freebsd", "openbsd", "netbsd", "dragonfly":
		StateDir = "/var/db/aheadscale"
	}
}
