package ui

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

// captureStdout runs fn and returns whatever it printed to stdout.
func captureStdout(fn func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	fn()

	_ = w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	return buf.String()
}

func TestStep(t *testing.T) {
	out := captureStdout(func() { Step("Installing packages") })
	if !strings.Contains(out, "🔧") {
		t.Errorf("Step output should contain wrench emoji, got: %s", out)
	}
	if !strings.Contains(out, "Installing packages") {
		t.Errorf("Step output should contain the step name, got: %s", out)
	}
}

func TestStepDone(t *testing.T) {
	out := captureStdout(func() { StepDone("Task complete") })
	if !strings.Contains(out, "✅") {
		t.Errorf("StepDone output should contain check mark emoji, got: %s", out)
	}
	if !strings.Contains(out, "Task complete") {
		t.Errorf("StepDone output should contain the message, got: %s", out)
	}
}

func TestStepFail(t *testing.T) {
	out := captureStdout(func() { StepFail("Task failed") })
	if !strings.Contains(out, "❌") {
		t.Errorf("StepFail output should contain cross emoji, got: %s", out)
	}
	if !strings.Contains(out, "Task failed") {
		t.Errorf("StepFail output should contain the message, got: %s", out)
	}
}

func TestKeyValue(t *testing.T) {
	out := captureStdout(func() { KeyValue("Namespace", "default") })
	if !strings.Contains(out, "Namespace:") {
		t.Errorf("KeyValue output should contain key with colon, got: %s", out)
	}
	if !strings.Contains(out, "default") {
		t.Errorf("KeyValue output should contain value, got: %s", out)
	}
}
