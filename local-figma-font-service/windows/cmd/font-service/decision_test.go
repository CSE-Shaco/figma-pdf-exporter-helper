package main

import (
	"strings"
	"testing"
)

func TestStartupCommand(t *testing.T) {
	p := `C:\Users\한글 이름\AppData\Local\FigmaPDFExporter\FontService\font-service.exe`
	got, err := startupCommand(p)
	if err != nil || got != `"`+p+`" --serve` {
		t.Fatalf("%q %v", got, err)
	}
	for _, bad := range []string{"", `C:\bad"path`, "bad\npath", strings.Repeat("a", 251), strings.Repeat("😀", 126)} {
		if _, err := startupCommand(bad); err == nil {
			t.Fatalf("accepted invalid path %q", bad)
		}
	}
	if _, err := startupCommand(strings.Repeat("a", 250)); err != nil {
		t.Fatal(err)
	}
}
