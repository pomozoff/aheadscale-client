//go:build windows && windowsvpnprototype

package services

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"
	"golang.org/x/sys/windows"
)

func newPlatformVPNController(conn DaemonConn) nativeVPNController {
	return newNativeVPNController(conn, runSystemVPN)
}

func runSystemVPN(ctx context.Context, connected bool) error {
	folder, err := windows.KnownFolderPath(windows.FOLDERID_LocalAppData, 0)
	if err != nil {
		return fmt.Errorf("не удалось найти контроллер системного VPN: %w", err)
	}
	// Не ищем исполняемый файл в PATH и не меняем конфигурацию службы.
	controller := filepath.Join(folder, "Microsoft", "WindowsApps", "aheadscale-vpn-prototype.exe")
	operation, action := "disconnect", "отключить"
	if connected {
		operation, action = "connect", "подключить"
	}
	command := exec.CommandContext(ctx, controller, operation)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	command.WaitDelay = 5 * time.Second
	// Контроллер удерживает PowerShell/rasdial в собственном Job Object.
	// Отмена убивает только эту команду и её потомков; SessionController
	// затем выполняет Disconnect прежде, чем пропустит следующее намерение.
	output, err := command.CombinedOutput()
	if err != nil {
		log.Debugf("Windows VPN %s failed: %v; controller: %s", operation, err, output)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("не удалось %s системный VPN Aheadscale: %w", action, err)
	}
	return nil
}
