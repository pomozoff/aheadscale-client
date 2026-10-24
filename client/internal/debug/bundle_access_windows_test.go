//go:build windows

package debug

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"

	"github.com/netbirdio/netbird/client/internal/ipcauth"
)

func protectedBundle(t *testing.T) (string, ipcauth.Identity) {
	t.Helper()
	id, err := ipcauth.CurrentProcessIdentity()
	require.NoError(t, err)
	if id.IsPrivileged() {
		t.Skip("access checks require an ordinary Windows user token")
	}
	path := filepath.Join(t.TempDir(), "bundle.zip")
	require.NoError(t, os.WriteFile(path, []byte("private diagnostic archive"), 0600))
	// Keep management rights for the fixture owner, with an explicit data-read
	// denial. The handoff must replace this with read/delete and remove write.
	sd, err := windows.SecurityDescriptorFromString("D:P(D;;0x1;;;" + id.SID + ")(A;;FA;;;SY)(A;;FA;;;BA)(A;;FA;;;" + id.SID + ")")
	require.NoError(t, err)
	dacl, _, err := sd.DACL()
	require.NoError(t, err)
	require.NoError(t, windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil, nil, dacl, nil))
	_, err = os.ReadFile(path)
	require.ErrorIs(t, err, os.ErrPermission, "the initial private file must be unreadable")
	return path, id
}

func TestBundleAccessWindowsVerifiedCallerCanReadAndDelete(t *testing.T) {
	path, self := protectedBundle(t)
	require.NoError(t, PrepareBundleForCaller(path, self, true))
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "private diagnostic archive", string(data))
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if f != nil {
		_ = f.Close()
	}
	require.ErrorIs(t, err, os.ErrPermission, "export must not grant write access")
	require.NoError(t, os.Remove(path))
}

func TestBundleAccessWindowsForeignSIDDoesNotGrantCurrentUserRead(t *testing.T) {
	path, _ := protectedBundle(t)
	foreign := ipcauth.Identity{SID: "S-1-5-21-9-9-9-9999"}
	require.NoError(t, PrepareBundleForCaller(path, foreign, true))
	_, err := os.ReadFile(path)
	require.ErrorIs(t, err, os.ErrPermission)
	require.NoError(t, os.Remove(path)) // the test remains the owner of its fixture
}

func TestBundleAccessWindowsUnverifiedCallerIsRefused(t *testing.T) {
	path, self := protectedBundle(t)
	require.Error(t, PrepareBundleForCaller(path, self, false))
	_, err := os.ReadFile(path)
	require.ErrorIs(t, err, os.ErrPermission)
	require.NoError(t, os.Remove(path))
}

func TestExportBundleWindowsCopiesIntoCallerTemp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "source.zip")
	require.NoError(t, os.WriteFile(path, []byte("archive bytes"), 0600))
	exported, err := ExportBundle(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Remove(exported) })
	require.NotEqual(t, path, exported)
	require.Equal(t, filepath.Clean(os.TempDir()), filepath.Dir(exported))
	require.Contains(t, filepath.Base(exported), "aheadscale.debug.")
	data, err := os.ReadFile(exported)
	require.NoError(t, err)
	require.Equal(t, "archive bytes", string(data))
	require.NoFileExists(t, path)
}

func TestExportBundleWindowsMissingSourceIsReported(t *testing.T) {
	_, err := ExportBundle(filepath.Join(t.TempDir(), "missing.zip"))
	require.ErrorIs(t, err, os.ErrNotExist)
}
