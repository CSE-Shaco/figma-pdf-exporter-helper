package main

import (
	"errors"
	"strings"
	"unicode/utf16"
)

const runValueName = "FigmaPDFExporterFontService"
const runKeyPath = `Software\Microsoft\Windows\CurrentVersion\Run`

// Run keys have a 260-character command-line limit. Count UTF-16 code units,
// not bytes, and reject controls/quotes rather than letting them alter arguments.
func startupCommand(path string) (string, error) {
	if path == "" || strings.ContainsAny(path, "\x00\r\n\"") {
		return "", errors.New("E_INSTALL_PATH")
	}
	command := `"` + path + `" --serve`
	if len(utf16.Encode([]rune(command))) > 260 {
		return "", errors.New("E_STARTUP_PATH_TOO_LONG")
	}
	return command, nil
}
