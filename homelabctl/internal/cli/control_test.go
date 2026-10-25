package cli

import (
	"bytes"
	"net/http"
	"net/http/httptest"
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

func TestControlReconcileUsesNormalAPI(t *testing.T) {
	t.Setenv("BUTLER_TOKEN", "test-token")
	var stderr bytes.Buffer
	runner := command.NewRunner(strings.NewReader(""), &bytes.Buffer{}, &stderr)
	root := New(BuildInfo{}, runner)
	root.SetArgs([]string{"--repo-root", testRepository(t), "--dry-run", "control", "reconcile", "--port-forward"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"port-forward service/butler", "8080:8080", "--address 127.0.0.1"} {
		if !strings.Contains(stderr.String(), expected) {
			t.Fatalf("output %q does not contain %q", stderr.String(), expected)
		}
	}
}

func TestControlOrphansPrintsReadOnlyReportAndExitsWithDrift(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/orphans" {
			t.Errorf("request = %s %s, want GET /api/v1/orphans", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"orphaned":[{"kind":"pocket-id-client","name":"retired","why":"declared in no PocketIDClient resource"}],"missing":[],"sources":[{"name":"pocket-id-clients","status":"ok"}]}`))
	}))
	defer server.Close()

	t.Setenv("BUTLER_TOKEN", "test-token")
	var stdout bytes.Buffer
	runner := command.NewRunner(strings.NewReader(""), &stdout, &bytes.Buffer{})
	root := New(BuildInfo{}, runner)
	root.SetArgs([]string{"--repo-root", testRepository(t), "control", "orphans", "--address", server.URL})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected a drift error")
	}
	exitCoder, ok := err.(interface{ ExitCode() int })
	if !ok || exitCoder.ExitCode() != driftExitCode {
		t.Fatalf("error %v does not carry exit code %d", err, driftExitCode)
	}
	for _, expected := range []string{`"kind": "pocket-id-client"`, `"name": "retired"`, `"status": "ok"`} {
		if !strings.Contains(stdout.String(), expected) {
			t.Fatalf("output %q does not contain %q", stdout.String(), expected)
		}
	}
}

func TestControlOrphansCleanReportExitsZero(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"orphaned":[],"missing":[],"sources":[]}`))
	}))
	defer server.Close()

	t.Setenv("BUTLER_TOKEN", "test-token")
	runner := command.NewRunner(strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	root := New(BuildInfo{}, runner)
	root.SetArgs([]string{"--repo-root", testRepository(t), "control", "orphans", "--address", server.URL})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
}

func TestControlOrphansReportFailureIsNotDrift(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusBadGateway)
	}))
	defer server.Close()

	t.Setenv("BUTLER_TOKEN", "test-token")
	runner := command.NewRunner(strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	root := New(BuildInfo{}, runner)
	root.SetArgs([]string{"--repo-root", testRepository(t), "control", "orphans", "--address", server.URL})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected an error")
	}
	if exitCoder, ok := err.(interface{ ExitCode() int }); ok {
		t.Fatalf("report failure must not report drift exit code %d", exitCoder.ExitCode())
	}
}

func TestControlReconcilePostsToReconcileEndpoint(t *testing.T) {
	var method, path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(`{"kind":"reconcile","state":"running"}`))
	}))
	defer server.Close()

	t.Setenv("BUTLER_TOKEN", "test-token")
	runner := command.NewRunner(strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	root := New(BuildInfo{}, runner)
	root.SetArgs([]string{"--repo-root", testRepository(t), "control", "reconcile", "--address", server.URL})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if method != http.MethodPost || path != "/api/v1/reconcile" {
		t.Fatalf("request = %s %s, want POST /api/v1/reconcile", method, path)
	}
}

func TestControlRecoveryHasNoUICommand(t *testing.T) {
	recovery, _, err := newControlCommand(&state{}).Find([]string{"recovery"})
	if err != nil {
		t.Fatal(err)
	}
	for _, subcommand := range recovery.Commands() {
		if subcommand.Name() == "ui" {
			t.Fatal("control recovery ui must not exist")
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
