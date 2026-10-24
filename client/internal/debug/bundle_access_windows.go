package debug

import (
	"fmt"
	"io"
	"os"

	log "github.com/sirupsen/logrus"
	"golang.org/x/sys/windows"

	"github.com/netbirdio/netbird/client/internal/ipcauth"
)

// PrepareBundleForCaller releases a completed archive to its authenticated
// requester. Call only after upload: the service must never reopen a released file.
func PrepareBundleForCaller(path string, caller ipcauth.Identity, identified bool) error {
	if !identified || caller.SID == "" {
		return fmt.Errorf("debug bundle requester has no verified Windows identity")
	}
	sid, err := windows.StringToSid(caller.SID)
	if err != nil {
		return fmt.Errorf("debug bundle requester SID: %w", err)
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;;FA;;;SY)(A;;FA;;;BA)(A;;FRSD;;;" + sid.String() + ")")
	if err != nil {
		return fmt.Errorf("debug bundle security descriptor: %w", err)
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return fmt.Errorf("debug bundle DACL: %w", err)
	}
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	handle, err := windows.CreateFile(name, windows.READ_CONTROL|windows.WRITE_DAC|windows.FILE_READ_ATTRIBUTES,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return fmt.Errorf("open generated debug bundle: %w", err)
	}
	defer windows.CloseHandle(handle)
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil {
		return err
	}
	if info.FileAttributes&(windows.FILE_ATTRIBUTE_REPARSE_POINT|windows.FILE_ATTRIBUTE_DIRECTORY) != 0 {
		return fmt.Errorf("debug bundle must be a regular file")
	}
	return windows.SetSecurityInfo(handle, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil)
}

// ExportBundle copies the finished SYSTEM archive into the calling user's Temp.
// The source's parent remains private; no privileged write uses a user-chosen path.
func ExportBundle(path string) (result string, err error) {
	source, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("read generated debug bundle: %w", err)
	}
	defer source.Close()
	destination, err := os.CreateTemp("", "aheadscale.debug.*.zip")
	if err != nil {
		return "", fmt.Errorf("create user debug bundle: %w", err)
	}
	defer func() {
		_ = destination.Close()
		if err != nil {
			_ = os.Remove(destination.Name())
		}
	}()
	if _, err := io.Copy(destination, source); err != nil {
		return "", fmt.Errorf("copy debug bundle: %w", err)
	}
	if err := destination.Close(); err != nil {
		return "", fmt.Errorf("close user debug bundle: %w", err)
	}
	if err := source.Close(); err != nil {
		return "", fmt.Errorf("close generated debug bundle: %w", err)
	}
	if err := os.Remove(path); err != nil {
		log.Warnf("remove exported debug bundle source: %v", err)
	}
	return destination.Name(), nil
}
