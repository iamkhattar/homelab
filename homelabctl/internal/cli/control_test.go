package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/iamkhattar/homelab/homelabctl/internal/command"
)

func TestControlBootstrapRequiresConfirmation(t *testing.T) {
	runner := command.NewRunner(strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	root := New(BuildInfo{}, runner)
	root.SetArgs([]string{"--repo-root", testRepository(t), "control", "bootstrap"})
	if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "--confirm") {
		t.Fatalf("error = %v", err)
	}
}

func TestNormalControlCommandRequiresToken(t *testing.T) {
	t.Setenv("BUTLER_TOKEN", "")
	runner := command.NewRunner(strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	root := New(BuildInfo{}, runner)
	root.SetArgs([]string{"--repo-root", testRepository(t), "control", "status", "--session-file", t.TempDir() + "/missing-session.json"})
	if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "control login") {
		t.Fatalf("error = %v", err)
	}
}

func TestNormalControlDefaultsToHTTPSIngress(t *testing.T) {
	command := newControlCommand(&state{})
	address := command.PersistentFlags().Lookup("address")
	if address == nil || address.DefValue != defaultButlerAddress {
		t.Fatalf("address default = %v, want %q", address, defaultButlerAddress)
	}
	portForward := command.PersistentFlags().Lookup("port-forward")
	if portForward == nil || portForward.DefValue != "false" {
		t.Fatalf("port-forward default = %v, want false", portForward)
	}
}

func TestControlDryRunUsesShortLivedRecoveryToken(t *testing.T) {
	var stderr bytes.Buffer
	runner := command.NewRunner(strings.NewReader(""), &bytes.Buffer{}, &stderr)
	root := New(BuildInfo{}, runner)
	root.SetArgs([]string{"--repo-root", testRepository(t), "--dry-run", "control", "recovery"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"create token butler-recovery-client", "--audience=butler-recovery", "--duration=10m"} {
		if !strings.Contains(stderr.String(), expected) {
			t.Fatalf("output %q does not contain %q", stderr.String(), expected)
		}
	}
}

func TestControlRecoveryUIDryRunKeepsTokenOutOfOutput(t *testing.T) {
	var stdout, stderr bytes.Buffer
	runner := command.NewRunner(strings.NewReader(""), &stdout, &stderr)
	root := New(BuildInfo{}, runner)
	root.SetArgs([]string{"--repo-root", testRepository(t), "--dry-run", "control", "recovery", "ui"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	combined := stdout.String() + stderr.String()
	for _, expected := range []string{"create token butler-recovery-client", "--audience=butler-recovery", "loopback-only recovery console"} {
		if !strings.Contains(combined, expected) {
			t.Fatalf("output %q does not contain %q", combined, expected)
		}
	}
}

func TestNormalControlPortForwardIsExplicit(t *testing.T) {
	t.Setenv("BUTLER_TOKEN", "test-token")
	var stderr bytes.Buffer
	runner := command.NewRunner(strings.NewReader(""), &bytes.Buffer{}, &stderr)
	root := New(BuildInfo{}, runner)
	root.SetArgs([]string{"--repo-root", testRepository(t), "--dry-run", "control", "status", "--port-forward"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"port-forward service/butler", "8080:8080", "--address 127.0.0.1"} {
		if !strings.Contains(stderr.String(), expected) {
			t.Fatalf("output %q does not contain %q", stderr.String(), expected)
		}
	}
}

func TestNormalControlRejectsBlankAddress(t *testing.T) {
	t.Setenv("BUTLER_TOKEN", "test-token")
	runner := command.NewRunner(strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	root := New(BuildInfo{}, runner)
	root.SetArgs([]string{"--repo-root", testRepository(t), "control", "status", "--address="})
	if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "--port-forward") {
		t.Fatalf("error = %v", err)
	}
}

func TestControlRejectsUnsafePathIdentifiers(t *testing.T) {
	t.Setenv("BUTLER_TOKEN", "test-token")
	for _, args := range [][]string{
		{"control", "users", "update", "../../admin"},
		{"control", "users", "set-groups", "user?admin"},
		{"control", "clients", "rotate", "client/name"},
	} {
		runner := command.NewRunner(strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
		root := New(BuildInfo{}, runner)
		root.SetArgs(append([]string{"--repo-root", testRepository(t)}, args...))
		if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "must be 1-128") {
			t.Fatalf("args %v: error = %v", args, err)
		}
	}
}
