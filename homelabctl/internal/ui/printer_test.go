package ui

import (
	"bytes"
	"strings"
	"testing"
)

func TestPrinterUsesReadablePlainOutputForNonTTY(t *testing.T) {
	t.Setenv("CLICOLOR_FORCE", "0")
	var output bytes.Buffer
	printer := New(&output)
	printer.Heading("Repository checks")
	printer.Status(Success, "pass", "go-test")
	printer.KeyValue("Version", "v0.1.42")
	printer.Command("/repo", "go test ./...")

	want := "◆ Repository checks\n✓ PASS  go-test\n  Version           v0.1.42\n+ (/repo) go test ./...\n"
	if output.String() != want {
		t.Fatalf("output = %q, want %q", output.String(), want)
	}
	if strings.Contains(output.String(), "\x1b[") {
		t.Fatalf("plain output contains ANSI: %q", output.String())
	}
}

func TestPrinterCanForceColor(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("CLICOLOR", "")
	t.Setenv("CLICOLOR_FORCE", "1")
	var output bytes.Buffer
	New(&output).Status(Failure, "fail", "go-test")
	if !strings.Contains(output.String(), "\x1b[") {
		t.Fatalf("forced-color output contains no ANSI: %q", output.String())
	}
}
