package main

import (
	"strings"
	"testing"
)

func TestCleanupRequestRejectsOtherFilesAndShellInput(t *testing.T) {
	base := ".font-service-remove-4294967295.exe"
	if pid, err := cleanupRequest("1234", base); err != nil || pid != 1234 {
		t.Fatalf("valid generated request: %d, %v", pid, err)
	}
	for _, pid := range []string{"", "0", "-1", "+1", "01", " 1", "1&whoami", "4294967296"} {
		if _, err := cleanupBatchScript(pid, base); err == nil {
			t.Errorf("accepted invalid PID %q", pid)
		}
	}
	for _, bad := range []string{
		"font-service.exe", ".font-service-remove-.exe", ".font-service-remove-12345678901.exe",
		`..\.font-service-remove-1.exe`, `/tmp/.font-service-remove-1.exe`,
		`.font-service-remove-1.exe:stream`, `.font-service-remove-1&whoami.exe`,
		`.font-service-remove-1%TEMP%.exe`, `.font-service-remove-1!TEMP!.exe`,
		`.font-service-remove-1".exe`, ".font-service-remove-1\r\nwhoami.exe",
		`.font-service-remove-1^2.exe`, `.font-service-remove-1 2.exe`, `.font-service-remove-１２.exe`,
	} {
		if _, err := cleanupBatchScript("1234", bad); err == nil {
			t.Errorf("accepted arbitrary basename or shell input %q", bad)
		}
	}
}

func TestCleanupShellRemovesOnlyFixedArtifacts(t *testing.T) {
	script, err := cleanupBatchScript("1234", ".font-service-remove-123.exe")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(string(script), "\r\n"), "\r\n")
	if len(lines) != 6 {
		t.Fatalf("unexpected shell statements: %q", script)
	}
	if lines[1] != "setlocal EnableExtensions DisableDelayedExpansion" ||
		lines[2] != `"%~dp0worker.exe" --cleanup 1234 ".font-service-remove-123.exe"` ||
		lines[4] != `del /q "%~dp0worker.exe"` ||
		lines[5] != `del /q "%~f0" & rmdir "%~dp0" & exit /b %FIGMA_FONT_CLEANUP_RESULT%` {
		t.Fatalf("shell deletion scope/quoting changed: %q", script)
	}
	for _, unsafe := range []string{"call ", "enabledelayedexpansion", "rmdir /s", "del /s", "*", `font-service.exe"`} {
		if strings.Contains(strings.ToLower(string(script)), unsafe) {
			t.Errorf("unsafe shell construction %q", unsafe)
		}
	}
	if !strings.Contains(cleanupCommandTail, `/d /s /q /e:on /v:off /c ""%FIGMA_FONT_UNINSTALL_SCRIPT%""`) {
		t.Fatal("shell must disable AutoRun/delayed expansion and preserve the quoted path")
	}
}

func TestCleanupPathOnlyEntersDedicatedEnvironment(t *testing.T) {
	path := `C:\Users\한글 & 이름! %TEMP% ^(x)\figma-font-cleanup-123\cleanup.cmd`
	env, err := cleanupEnvironment([]string{"PATH=unchanged", "figma_font_uninstall_script=bad", "FIGMA_FONT_UNINSTALL_SCRIPT=worse", `ErRoRlEvEl=" & arbitrary command`}, path)
	if err != nil {
		t.Fatal(err)
	}
	if len(env) != 2 || env[0] != "PATH=unchanged" || env[1] != cleanupScriptVariable+"="+path {
		t.Fatalf("path changed or inherited override survived: %q", env)
	}
	script, _ := cleanupBatchScript("1234", ".font-service-remove-123.exe")
	if strings.Contains(string(script), path) || strings.Contains(cleanupCommandTail, path) {
		t.Fatal("a user path entered shell source")
	}
	for _, bad := range []string{"", "bad\x00path", "bad\rpath", "bad\npath", `bad"path`} {
		if _, err := cleanupEnvironment(nil, bad); err == nil {
			t.Errorf("accepted invalid environment path %q", bad)
		}
	}
}
