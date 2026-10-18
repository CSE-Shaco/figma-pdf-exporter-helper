//go:build windows

package main

import (
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"strings"
	"testing"
	"time"
)

// This test uses unique native object names and never changes startup registry,
// installed files, HTTP ports, or an existing bridge instance.
func TestRestrictedCurrentUserIPC(t *testing.T) {
	name, sa, err := identity()
	if err != nil {
		t.Fatal(err)
	}
	if sa.InheritHandle != 0 {
		t.Fatal("IPC handles must not be inherited")
	}
	if !strings.Contains(sa.SecurityDescriptor.String(), "D:P") || strings.Contains(sa.SecurityDescriptor.String(), ";;;WD") {
		t.Fatal("DACL is not protected/current-user-only")
	}
	name += fmt.Sprintf(".test.%d", time.Now().UnixNano())
	mutex, err := singleton(name, sa)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(mutex)
	if h, err := singleton(name, sa); err == nil {
		windows.CloseHandle(h)
		t.Fatal("duplicate instance was admitted")
	}
	alive, err := isRunning(name)
	if err != nil || !alive {
		t.Fatalf("running=%v error=%v", alive, err)
	}
	h, err := event(name+".stop", sa)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(h)
	sender, err := openEvent(name+".stop", windows.EVENT_MODIFY_STATE)
	if err != nil {
		t.Fatal(err)
	}
	err = windows.SetEvent(sender)
	windows.CloseHandle(sender)
	if err != nil {
		t.Fatal(err)
	}
	status, err := windows.WaitForSingleObject(h, 100)
	if err != nil || status != windows.WAIT_OBJECT_0 {
		t.Fatalf("signal=%d err=%v", status, err)
	}
	if _, err := openEvent(name+".absent", windows.SYNCHRONIZE); !errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
		t.Fatalf("unexpected absent event error: %v", err)
	}
	owner, _, err := sa.SecurityDescriptor.Owner()
	if err != nil {
		t.Fatal(err)
	}
	broad, err := windows.SecurityDescriptorFromString("O:" + owner.String() + "D:P(A;;GA;;;" + owner.String() + ")(A;;GR;;;WD)")
	if err != nil {
		t.Fatal(err)
	}
	unsafeSA := *sa
	unsafeSA.SecurityDescriptor = broad
	broadName, _ := windows.UTF16PtrFromString(name + ".broad")
	untrusted, err := windows.CreateMutex(&unsafeSA, false, broadName)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(untrusted)
	if err := validateIPC(untrusted); err == nil {
		t.Fatal("accepted IPC granting another user access")
	}
	if h, err := singleton(name+".broad", sa); err == nil || !strings.Contains(err.Error(), "E_IPC_IDENTITY") {
		if h != 0 {
			windows.CloseHandle(h)
		}
		t.Fatalf("untrusted existing mutex error=%v", err)
	}
	reused, err := event(name+".stop", sa)
	if err != nil {
		t.Fatalf("reusing authenticated event: %v", err)
	}
	windows.CloseHandle(reused)

}
