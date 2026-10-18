//go:build windows

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"

	fontservice "figmapdf.local/fontservice"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var kernel = windows.NewLazySystemDLL("kernel32.dll")
var attachConsole = kernel.NewProc("AttachConsole")

func main() {
	// GUI subsystem prevents a console flash at login. Explicit CLI/Install.cmd
	// attaches to its existing parent console so fixed result codes stay visible.
	if len(os.Args) < 2 || os.Args[1] != "--serve" {
		attachConsole.Call(^uintptr(0))
		stdout, _ := windows.GetStdHandle(windows.STD_OUTPUT_HANDLE)
		os.Stdout = os.NewFile(uintptr(stdout), "stdout")
		stderr, _ := windows.GetStdHandle(windows.STD_ERROR_HANDLE)
		os.Stderr = os.NewFile(uintptr(stderr), "stderr")
	}
	code, err := run(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
	}
	os.Exit(code)
}
func run(args []string) (int, error) {
	if len(args) == 0 {
		return 2, errors.New("E_USAGE: use Install.cmd, or --install / --serve / --uninstall")
	}
	var err error
	switch args[0] {
	case "--install":
		if len(args) != 1 {
			return 2, errors.New("E_USAGE")
		}
		err = manage(true)
	case "--uninstall":
		if len(args) != 1 {
			return 2, errors.New("E_USAGE")
		}
		err = manage(false)
	case "--serve":
		if len(args) != 1 {
			return 2, errors.New("E_USAGE")
		}
		err = serve()
	case "--cleanup":
		if len(args) != 3 {
			return 2, errors.New("E_USAGE")
		}
		err = cleanup(args[1], args[2])
	default:
		return 2, errors.New("E_USAGE")
	}
	if err != nil {
		var op *net.OpError
		if errors.As(err, &op) {
			return 24, errors.New("E_PORT_IN_USE: local port 38474 is unavailable")
		}
		return 20, err
	}
	return 0, nil
}
func installedPath() (string, error) {
	// Known Folder API follows Windows profile redirection; environment variables
	// alone are not trusted to select uninstall targets.
	dir, err := windows.KnownFolderPath(windows.FOLDERID_LocalAppData, 0)
	if err != nil {
		return "", errors.New("E_LOCAL_APP_DATA")
	}
	return filepath.Join(dir, "FigmaPDFExporter", "FontService", "font-service.exe"), nil
}
func identity() (string, *windows.SecurityAttributes, error) {
	token, err := windows.OpenCurrentProcessToken()
	if err != nil {
		return "", nil, err
	}
	defer token.Close()
	user, err := token.GetTokenUser()
	if err != nil {
		return "", nil, err
	}
	sid := user.User.Sid.String()
	sd, err := windows.SecurityDescriptorFromString("O:" + sid + "D:P(A;;GA;;;" + sid + ")")
	if err != nil {
		return "", nil, err
	}
	return `Global\FigmaPDFExporter.FontService.` + sid, &windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}, nil
}

// Existing global objects must retain our exact user boundary. An object
// pre-created by another account must fail closed even if its DACL grants us access.
func validateIPC(h windows.Handle) error {
	token, err := windows.OpenCurrentProcessToken()
	if err != nil {
		return err
	}
	defer token.Close()
	user, err := token.GetTokenUser()
	if err != nil {
		return err
	}
	sd, err := windows.GetSecurityInfo(h, windows.SE_KERNEL_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return errors.New("E_IPC_IDENTITY")
	}
	owner, _, err := sd.Owner()
	if err != nil || owner == nil || !owner.Equals(user.User.Sid) {
		return errors.New("E_IPC_IDENTITY")
	}
	control, _, err := sd.Control()
	if err != nil || control&windows.SE_DACL_PROTECTED == 0 {
		return errors.New("E_IPC_IDENTITY")
	}
	acl, _, err := sd.DACL()
	if err != nil || acl == nil || acl.AceCount != 1 {
		return errors.New("E_IPC_IDENTITY")
	}
	var ace *windows.ACCESS_ALLOWED_ACE
	if err = windows.GetAce(acl, 0, &ace); err != nil || ace == nil || ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE || ace.Header.AceFlags != 0 {
		return errors.New("E_IPC_IDENTITY")
	}
	sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
	if !sid.Equals(user.User.Sid) {
		return errors.New("E_IPC_IDENTITY")
	}
	return nil
}
func singleton(name string, sa *windows.SecurityAttributes) (windows.Handle, error) {
	p, _ := windows.UTF16PtrFromString(name)
	h, err := windows.CreateMutex(sa, false, p)
	if h != 0 {
		if e := validateIPC(h); e != nil {
			windows.CloseHandle(h)
			return 0, e
		}
	}
	if err == windows.ERROR_ALREADY_EXISTS {
		windows.CloseHandle(h)
		return 0, errors.New("E_ALREADY_RUNNING")
	}
	return h, err
}
func event(name string, sa *windows.SecurityAttributes) (windows.Handle, error) {
	p, _ := windows.UTF16PtrFromString(name)
	h, err := windows.CreateEvent(sa, 1, 0, p)
	if h != 0 {
		if e := validateIPC(h); e != nil {
			windows.CloseHandle(h)
			return 0, e
		}
	}
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		err = nil
	}
	return h, err
}
func openEvent(name string, access uint32) (windows.Handle, error) {
	p, _ := windows.UTF16PtrFromString(name)
	h, err := windows.OpenEvent(access|windows.READ_CONTROL, false, p)
	if err != nil {
		return 0, err
	}
	if err = validateIPC(h); err != nil {
		windows.CloseHandle(h)
		return 0, err
	}
	return h, nil
}
func isRunning(name string) (bool, error) {
	p, _ := windows.UTF16PtrFromString(name)
	h, err := windows.OpenMutex(windows.SYNCHRONIZE|windows.READ_CONTROL, false, p)
	if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer windows.CloseHandle(h)
	if err = validateIPC(h); err != nil {
		return false, err
	}
	return true, nil
}
func stop(name string) error {
	alive, err := isRunning(name + ".serve")
	if err != nil || !alive {
		return err
	}
	h, err := openEvent(name+".stop", windows.EVENT_MODIFY_STATE)
	if err != nil {
		return errors.New("E_STOP_UNAVAILABLE")
	}
	err = windows.SetEvent(h)
	windows.CloseHandle(h)
	if err != nil {
		return err
	}
	end := time.Now().Add(12 * time.Second)
	for time.Now().Before(end) {
		alive, err = isRunning(name + ".serve")
		if err != nil {
			return err
		}
		if !alive {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return errors.New("E_STOP_TIMEOUT: existing bridge was not terminated")
}
func serve() error {
	name, sa, err := identity()
	if err != nil {
		return err
	}
	m, err := singleton(name+".serve", sa)
	if err != nil {
		if err.Error() == "E_ALREADY_RUNNING" {
			return nil
		}
		return err
	}
	defer windows.CloseHandle(m)
	s, err := event(name+".stop", sa)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(s)
	windows.ResetEvent(s)
	ready, err := event(name+".ready", sa)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(ready)
	windows.ResetEvent(ready)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	defer close(done)
	go func() {
		for {
			select {
			case <-done:
				return
			default:
			}
			v, _ := windows.WaitForSingleObject(s, 100)
			if v == windows.WAIT_OBJECT_0 {
				cancel()
				return
			}
		}
	}()
	go func() {
		for {
			select {
			case <-done:
				return
			default:
			}
			if healthy(os.Getpid()) {
				windows.SetEvent(ready)
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
	}()
	err = fontservice.Run(ctx)
	if err != nil {
		writeStatus("E_LISTENER_START: local port 38474 unavailable or listener failed")
	}
	return err
}
func healthy(pid int) bool {
	client := &http.Client{Transport: &http.Transport{Proxy: nil, DisableKeepAlives: true}, Timeout: 300 * time.Millisecond}
	req, _ := http.NewRequest("GET", "http://localhost:38474/health", nil)
	req.Header.Set("Origin", "null")
	r, err := client.Do(req)
	if err != nil {
		return false
	}
	defer r.Body.Close()
	var h struct {
		Service string `json:"service"`
		PID     int    `json:"pid"`
	}
	return r.StatusCode == 200 && json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&h) == nil && h.Service == "figma-local-font-service" && h.PID == pid
}
func writeStatus(message string) {
	p, err := installedPath()
	if err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(filepath.Dir(p), "last-error.txt"), []byte(message+"\r\n"), 0600)
}
func manage(install bool) (result error) {
	target, err := installedPath()
	if err != nil {
		return err
	}
	command, err := startupCommand(target)
	if err != nil {
		return err
	}
	name, sa, err := identity()
	if err != nil {
		return err
	}
	gate, err := singleton(name+".manage", sa)
	if err != nil {
		return errors.New("E_INSTALL_BUSY: another install/uninstall is active")
	}
	defer windows.CloseHandle(gate)
	var key registry.Key
	if install {
		key, _, err = registry.CreateKey(registry.CURRENT_USER, runKeyPath, registry.QUERY_VALUE|registry.SET_VALUE)
	} else {
		key, err = registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.QUERY_VALUE|registry.SET_VALUE)
	}
	if err != nil && (install || !errors.Is(err, registry.ErrNotExist)) {
		return errors.New("E_STARTUP_REGISTRY")
	}
	var old string
	var readErr error = registry.ErrNotExist
	if key != 0 {
		defer key.Close()
		old, _, readErr = key.GetStringValue(runValueName)
	}
	if readErr != nil && !errors.Is(readErr, registry.ErrNotExist) {
		return errors.New("E_STARTUP_REGISTRY")
	}
	if readErr == nil && old != command {
		return errors.New("E_STARTUP_ENTRY_CONFLICT: startup value belongs to another command")
	}

	marker := filepath.Join(filepath.Dir(target), ".installation-sha256")
	oldHash, markerErr := os.ReadFile(marker)
	_, statErr := os.Stat(target)
	existed := statErr == nil
	if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return errors.New("E_INSTALL_IDENTITY")
	}
	if existed {
		actual, e := hashFile(target)
		if markerErr != nil || e != nil || strings.TrimSpace(string(oldHash)) != actual {
			return errors.New("E_INSTALL_IDENTITY: installed file is not owned by this installer")
		}
	}
	wasRunning, err := isRunning(name + ".serve")
	if err != nil {
		return err
	}
	backup := ""
	keepBackup := false
	if install && existed {
		f, e := os.CreateTemp(filepath.Dir(target), ".font-service-backup-*.exe")
		if e != nil {
			return e
		}
		backup = f.Name()
		f.Close()
		if e = atomicCopy(target, backup); e != nil {
			os.Remove(backup)
			return e
		}
		defer func() {
			if !keepBackup {
				_ = os.Remove(backup)
			}
		}()
	}
	stopped, copied, markerWritten := false, false, false
	if install {
		defer func() {
			if result == nil {
				return
			}
			if !stopped {
				return
			}
			if e := stop(name); e != nil {
				result = fmt.Errorf("%w; E_ROLLBACK_FAILED: %v", result, e)
				return
			}
			var restoreErr error
			if existed && copied {
				restoreErr = atomicCopy(backup, target)
				if restoreErr == nil {
					restoreErr = os.WriteFile(marker, oldHash, 0600)
				}
			} else if !existed && copied {
				restoreErr = os.Remove(target)
				if errors.Is(restoreErr, os.ErrNotExist) {
					restoreErr = nil
				}
				_ = os.Remove(marker)
			} else if markerWritten && existed {
				restoreErr = os.WriteFile(marker, oldHash, 0600)
			}
			if !existed {
				_ = os.Remove(filepath.Join(filepath.Dir(target), "last-error.txt"))
				_ = os.Remove(filepath.Dir(target))
			}
			current, _, e := key.GetStringValue(runValueName)
			if e == nil && current == command {
				if readErr == nil {
					if e = key.SetStringValue(runValueName, old); restoreErr == nil {
						restoreErr = e
					}
				} else {
					if e = key.DeleteValue(runValueName); restoreErr == nil {
						restoreErr = e
					}
				}
			}
			if wasRunning && existed && restoreErr == nil {
				if p, e := hiddenStart(target, "--serve"); e == nil {
					_ = p.Release()
				} else {
					restoreErr = e
				}
			}
			if restoreErr != nil {
				keepBackup = true
				result = fmt.Errorf("%w; E_ROLLBACK_FAILED: %v", result, restoreErr)
			}
		}()
	}
	if err = stop(name); err != nil {
		return err
	}
	stopped = true
	if !install {
		if err = removeInstalled(target); err != nil {
			return err
		}
		if readErr == nil {
			if err = key.DeleteValue(runValueName); err != nil {
				return err
			}
		}
		_ = os.Remove(marker)
		_ = os.Remove(filepath.Join(filepath.Dir(target), "last-error.txt"))
		_ = os.Remove(filepath.Dir(target)) // succeeds only when empty; preserve other files
		fmt.Println("OK_UNINSTALLED: login startup removed; local bridge stopped")
		return nil
	}
	source, err := os.Executable()
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		return err
	}
	if !strings.EqualFold(filepath.Clean(source), filepath.Clean(target)) {
		if err = atomicCopy(source, target); err != nil {
			return fmt.Errorf("E_INSTALL_COPY: %w", err)
		}
		copied = true
	}
	digest, err := hashFile(target)
	if err != nil {
		return err
	}
	if err = os.WriteFile(marker, []byte(digest+"\n"), 0600); err != nil {
		return err
	}
	markerWritten = true
	current, _, nowErr := key.GetStringValue(runValueName)
	if (readErr == nil && (nowErr != nil || current != old)) || (readErr != nil && !errors.Is(nowErr, registry.ErrNotExist)) {
		return errors.New("E_STARTUP_ENTRY_CONFLICT: startup value changed during install")
	}
	if err = key.SetStringValue(runValueName, command); err != nil {
		return errors.New("E_STARTUP_REGISTRY")
	}
	proc, err := hiddenStart(target, "--serve")
	if err != nil {
		return errors.New("E_START_FAILED")
	}
	defer proc.Release()
	end := time.Now().Add(10 * time.Second)
	for time.Now().Before(end) {
		if h, e := openEvent(name+".ready", windows.SYNCHRONIZE); e == nil {
			v, _ := windows.WaitForSingleObject(h, 100)
			windows.CloseHandle(h)
			if v == windows.WAIT_OBJECT_0 && healthy(proc.Pid) {
				fmt.Println("OK_INSTALLED: bridge listening on localhost:38474; login startup enabled. Figma Desktop font readiness is checked when exporting.")
				return nil
			}
		}
		p, e := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(proc.Pid))
		if e == nil {
			v, _ := windows.WaitForSingleObject(p, 0)
			var status uint32
			windows.GetExitCodeProcess(p, &status)
			windows.CloseHandle(p)
			if v == windows.WAIT_OBJECT_0 {
				if status == 24 {
					return errors.New("E_PORT_IN_USE: local port 38474 unavailable")
				}
				return errors.New("E_START_FAILED: check last-error.txt")
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return errors.New("E_START_TIMEOUT: startup registered; bridge health not confirmed")
}
func hiddenStart(path string, args ...string) (*os.Process, error) {
	c := exec.Command(path, args...)
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	if err := c.Start(); err != nil {
		return nil, err
	}
	return c.Process, nil
}
func atomicCopy(source, target string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.CreateTemp(filepath.Dir(target), ".font-service-update-*.exe")
	if err != nil {
		return err
	}
	temp := out.Name()
	defer os.Remove(temp)
	_, err = io.Copy(out, in)
	if err == nil {
		err = out.Sync()
	}
	closeErr := out.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	a, _ := windows.UTF16PtrFromString(temp)
	b, _ := windows.UTF16PtrFromString(target)
	for i := 0; i < 30; i++ {
		err = windows.MoveFileEx(a, b, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
		if err == nil {
			return nil
		}
		if !errors.Is(err, windows.ERROR_SHARING_VIOLATION) && !errors.Is(err, windows.ERROR_ACCESS_DENIED) {
			return err
		}
		time.Sleep(100 * time.Millisecond)
	}
	return err
}
func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	_, err = io.Copy(h, f)
	return hex.EncodeToString(h.Sum(nil)), err
}
