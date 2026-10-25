//go:build windows && windowsvpnprototype

package services

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
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
	if !connected {
		tokenUser, err := windows.GetCurrentProcessToken().GetTokenUser()
		if err != nil {
			return fmt.Errorf("не удалось определить владельца системного VPN: %w", err)
		}
		// RAS Disconnected не означает, что посредник уже закончил свой Down.
		// Он держит этот mutex до полного cleanup; новый Login ждёт его здесь.
		waitCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		name := `Local\Aheadscale.VpnPrototype.NativeBroker.` + tokenUser.User.Sid.String()
		if err := waitNativeBrokerCleanup(waitCtx, name); err != nil {
			return fmt.Errorf("завершение предыдущего системного VPN: %w", err)
		}
	}
	return nil
}

// waitNativeBrokerCleanup ждёт завершения старого посредника, включая его Down.
func waitNativeBrokerCleanup(ctx context.Context, name string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	handle, err := windows.OpenMutex(windows.SYNCHRONIZE|windows.MUTEX_MODIFY_STATE, false, windows.StringToUTF16Ptr(name))
	if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
		return nil
	}
	if err != nil {
		return err
	}
	defer func() { _ = windows.CloseHandle(handle) }()
	// Windows mutex принадлежит потоку, а не goroutine.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		result, err := windows.WaitForSingleObject(handle, 100)
		if err != nil {
			return err
		}
		switch result {
		case windows.WAIT_OBJECT_0:
			return windows.ReleaseMutex(handle)
		case windows.WAIT_ABANDONED:
			_ = windows.ReleaseMutex(handle)
			return fmt.Errorf("посредник завершился без подтверждения очистки")
		case uint32(windows.WAIT_TIMEOUT):
			continue
		default:
			return fmt.Errorf("неожиданный результат ожидания посредника: %d", result)
		}
	}
}
