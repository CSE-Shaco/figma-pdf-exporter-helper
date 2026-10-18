//go:build windows

package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCleanupVerifiedDeletionPreservesOtherFiles(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, ".font-service-remove-123.exe")
	installed := filepath.Join(dir, "font-service.exe")
	for _, name := range []string{old, installed} {
		if err := os.WriteFile(name, []byte("same original payload"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	digest, err := hashFile(old)
	if err != nil {
		t.Fatal(err)
	}
	if err = removeVerifiedCleanupFile(old, "wrong digest"); err == nil {
		t.Fatal("deleted an unverified file")
	}
	if _, err = os.Stat(old); err != nil {
		t.Fatalf("mismatch removed file: %v", err)
	}
	if err = removeVerifiedCleanupFile(old, digest); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(old); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("verified old image still exists: %v", err)
	}
	if got, err := os.ReadFile(installed); err != nil || string(got) != "same original payload" {
		t.Fatalf("reinstalled image was changed: %q, %v", got, err)
	}
}

func TestCleanupHandleBlocksReplacement(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.exe")
	if err := os.WriteFile(path, []byte("owned"), 0600); err != nil {
		t.Fatal(err)
	}
	f, err := openCleanupFile(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err = moveCleanupImage(path, path+".replaced"); err == nil {
		t.Fatal("verification handle allowed a rename")
	}
	if err = os.WriteFile(path, []byte("replacement"), 0600); err == nil {
		t.Fatal("verification handle allowed a write")
	}
}

func TestCleanupRejectsHardLinkAndLiveParent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "old.exe")
	if err := os.WriteFile(path, []byte("owned"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(path, filepath.Join(dir, "unrelated.exe")); err != nil {
		t.Fatal(err)
	}
	if f, err := openCleanupFile(path); err == nil {
		f.Close()
		t.Fatal("accepted a multiply linked image")
	}
	if err := waitForCleanupParent(uint32(os.Getpid()), 0); err == nil {
		t.Fatal("accepted a parent that has not exited")
	}
}

func TestCleanupRenameDoesNotOverwrite(t *testing.T) {
	dir := t.TempDir()
	source, target := filepath.Join(dir, "source.exe"), filepath.Join(dir, "target.exe")
	if err := os.WriteFile(source, []byte("owned"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("sentinel"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := moveCleanupImage(source, target); err == nil {
		t.Fatal("rename overwrote an existing file")
	}
	if got, err := os.ReadFile(target); err != nil || string(got) != "sentinel" {
		t.Fatalf("sentinel changed: %q, %v", got, err)
	}
}
