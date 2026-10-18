package main

import (
	"errors"
	"strconv"
	"strings"
)

const cleanupBasePrefix = ".font-service-remove-"
const cleanupBaseSuffix = ".exe"
const cleanupScriptVariable = "FIGMA_FONT_UNINSTALL_SCRIPT"

// /d ignores AutoRun, /s handles surrounding /c quotes, and command extensions
// and delayed expansion are explicitly controlled rather than user defaults.
const cleanupCommandTail = ` /d /s /q /e:on /v:off /c ""%` + cleanupScriptVariable + `%""`

func cleanupRequest(pidText, base string) (uint32, error) {
	pid, err := strconv.ParseUint(pidText, 10, 32)
	if err != nil || pid == 0 || strconv.FormatUint(pid, 10) != pidText {
		return 0, errors.New("E_USAGE")
	}
	if !strings.HasPrefix(base, cleanupBasePrefix) || !strings.HasSuffix(base, cleanupBaseSuffix) {
		return 0, errors.New("E_USAGE")
	}
	// os.CreateTemp uses a decimal uint32 suffix. Restrict the entire basename,
	// excluding separators, alternate data streams, and shell characters.
	number := strings.TrimSuffix(strings.TrimPrefix(base, cleanupBasePrefix), cleanupBaseSuffix)
	if len(number) == 0 || len(number) > 10 {
		return 0, errors.New("E_USAGE")
	}
	for _, r := range number {
		if r < '0' || r > '9' {
			return 0, errors.New("E_USAGE")
		}
	}
	return uint32(pid), nil
}

func cleanupBatchScript(pidText, base string) ([]byte, error) {
	if _, err := cleanupRequest(pidText, base); err != nil {
		return nil, err
	}
	// User paths never enter source. The native worker alone removes the
	// installed image. The batch waits for it, then removes its fixed files
	// and directory if empty, even when the native worker fails. Keep the last
	// commands on one line so deleting this script cannot prevent their read.
	// Do not add CALL or enable delayed expansion.
	script := "@echo off\r\nsetlocal EnableExtensions DisableDelayedExpansion\r\n" +
		`"%~dp0worker.exe" --cleanup ` + pidText + ` "` + base + `"` + "\r\n" +
		`set "FIGMA_FONT_CLEANUP_RESULT=%errorlevel%"` + "\r\n" +
		`del /q "%~dp0worker.exe"` + "\r\n" +
		`del /q "%~f0" & rmdir "%~dp0" & exit /b %FIGMA_FONT_CLEANUP_RESULT%` + "\r\n"
	return []byte(script), nil
}

func cleanupEnvironment(environ []string, scriptPath string) ([]string, error) {
	if scriptPath == "" || strings.ContainsAny(scriptPath, "\x00\r\n\"") {
		return nil, errors.New("E_CLEANUP_PATH")
	}
	result := make([]string, 0, len(environ)+1)
	for _, entry := range environ {
		name, _, _ := strings.Cut(entry, "=")
		// An inherited ERRORLEVEL variable shadows cmd's numeric pseudo-value.
		// Remove it before capturing the native worker's actual exit status.
		if !strings.EqualFold(name, cleanupScriptVariable) && !strings.EqualFold(name, "ERRORLEVEL") {
			result = append(result, entry)
		}
	}
	return append(result, cleanupScriptVariable+"="+scriptPath), nil
}
