//go:build windows

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

// Removing the running image needs an external process. Rename only our fixed
// installed image first; a subsequent install can never be deleted by the helper.
func removeInstalled(target string) (result error) {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	if !strings.EqualFold(filepath.Clean(self), filepath.Clean(target)) {
		err = os.Remove(target)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(target), cleanupBasePrefix+"*"+cleanupBaseSuffix)
	if err != nil {
		return err
	}
	renamed := temp.Name()
	if err = temp.Close(); err != nil {
		_ = os.Remove(renamed)
		return err
	}
	if err = os.Remove(renamed); err != nil {
		return err
	}
	if err = moveCleanupImage(target, renamed); err != nil {
		return errors.New("E_UNINSTALL_LOCKED: run Uninstall.cmd from the extracted ZIP")
	}
	started := false
	defer func() {
		if result != nil && !started {
			if e := moveCleanupImage(renamed, target); e != nil {
				result = fmt.Errorf("%w; E_UNINSTALL_RESTORE_FAILED", result)
			}
		}
	}()
	// Use the profile's Known Folder rather than inherited TEMP/TMP. The
	// unique temporary directory inherits the user profile's permissions.
	localAppData, err := windows.KnownFolderPath(windows.FOLDERID_LocalAppData, 0)
	if err != nil {
		return err
	}
	helperDir, err := os.MkdirTemp(localAppData, "figma-font-cleanup-")
	if err != nil {
		return err
	}
	helperPath := filepath.Join(helperDir, "worker.exe")
	scriptPath := filepath.Join(helperDir, "cleanup.cmd")
	defer func() {
		if !started {
			// Never recursively remove a directory, even on setup failure.
			_ = os.Remove(helperPath)
			_ = os.Remove(scriptPath)
			_ = os.Remove(helperDir)
		}
	}()
	if err = atomicCopy(renamed, helperPath); err != nil {
		return err
	}
	script, err := cleanupBatchScript(strconv.Itoa(os.Getpid()), filepath.Base(renamed))
	if err != nil {
		return err
	}
	if err = os.WriteFile(scriptPath, script, 0600); err != nil {
		return err
	}
	systemDir, err := windows.GetSystemDirectory()
	if err != nil {
		return err
	}
	cmdPath := filepath.Join(systemDir, "cmd.exe")
	c := exec.Command(cmdPath)
	c.Env, err = cleanupEnvironment(os.Environ(), scriptPath)
	if err != nil {
		return err
	}
	// Windows cannot remove a directory used as a current directory.
	c.Dir = systemDir
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW,
		CmdLine: syscall.EscapeArg(cmdPath) + cleanupCommandTail}
	if err = c.Start(); err != nil {
		return err
	}
	started = true
	// Cleanup now belongs to the started helper. A local Release failure must
	// not race it by rolling the image back to the installation filename.
	_ = c.Process.Release()
	return nil
}

// Unlike os.Rename, never overwrite a newly appeared destination file.
func moveCleanupImage(source, target string) error {
	a, err := windows.UTF16PtrFromString(source)
	if err != nil {
		return err
	}
	b, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(a, b, windows.MOVEFILE_WRITE_THROUGH)
}

func cleanup(pidText, base string) error {
	pid, err := cleanupRequest(pidText, base)
	if err != nil {
		return err
	}
	target, err := installedPath()
	if err != nil {
		return err
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	digest, err := hashFile(self)
	if err != nil {
		return err
	}
	if err = waitForCleanupParent(pid, 15000); err != nil {
		return err
	}
	path := filepath.Join(filepath.Dir(target), base)
	for i := 0; i < 30; i++ {
		err = removeVerifiedCleanupFile(path, digest)
		if err == nil || errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
			// A reinstalled image or unrelated file keeps this directory alive.
			_ = os.Remove(filepath.Dir(target))
			return nil
		}
		if !errors.Is(err, windows.ERROR_SHARING_VIOLATION) && !errors.Is(err, windows.ERROR_ACCESS_DENIED) {
			return err
		}
		time.Sleep(100 * time.Millisecond)
	}
	return errors.New("E_UNINSTALL_LOCKED")
}

func waitForCleanupParent(pid, milliseconds uint32) error {
	p, err := windows.OpenProcess(windows.SYNCHRONIZE, false, pid)
	if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
		// The validated nonzero parent PID already exited. Deletion still
		// requires independently verifying the renamed file's contents.
		return nil
	}
	if err != nil {
		return fmt.Errorf("E_CLEANUP_PARENT: %w", err)
	}
	defer windows.CloseHandle(p)
	state, err := windows.WaitForSingleObject(p, milliseconds)
	if err != nil {
		return fmt.Errorf("E_CLEANUP_PARENT: %w", err)
	}
	if state != windows.WAIT_OBJECT_0 {
		return errors.New("E_CLEANUP_PARENT_TIMEOUT")
	}
	return nil
}

func openCleanupFile(path string) (*os.File, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	// Deny write/delete sharing while verifying and deleting the same object.
	// OPEN_REPARSE_POINT prevents following a substituted final symlink.
	h, err := windows.CreateFile(p, windows.GENERIC_READ|windows.DELETE, windows.FILE_SHARE_READ,
		nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return nil, err
	}
	var info windows.ByHandleFileInformation
	if err = windows.GetFileInformationByHandle(h, &info); err != nil {
		windows.CloseHandle(h)
		return nil, err
	}
	if info.FileAttributes&(windows.FILE_ATTRIBUTE_DIRECTORY|windows.FILE_ATTRIBUTE_REPARSE_POINT) != 0 || info.NumberOfLinks != 1 {
		windows.CloseHandle(h)
		return nil, errors.New("E_CLEANUP_IDENTITY")
	}
	return os.NewFile(uintptr(h), path), nil
}

func removeVerifiedCleanupFile(path, expected string) (result error) {
	f, err := openCleanupFile(path)
	if err != nil {
		return err
	}
	defer func() {
		if err := f.Close(); result == nil {
			result = err
		}
	}()
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return err
	}
	if hex.EncodeToString(h.Sum(nil)) != expected {
		return errors.New("E_CLEANUP_IDENTITY")
	}
	// FILE_DISPOSITION_INFO is one BOOLEAN. Delete through the verified
	// handle, not a second path lookup that could select a replacement file.
	deleteFile := byte(1)
	return windows.SetFileInformationByHandle(windows.Handle(f.Fd()), windows.FileDispositionInfo, &deleteFile, 1)
}
