package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/iamkhattar/homelab/homelabctl/internal/command"
	"github.com/spf13/cobra"
)

type fakeSelfUpdateClient struct {
	release          availableRelease
	found            bool
	detectErr        error
	installErr       error
	requestedVersion string
	installedPath    string
	installCalls     int
}

func (f *fakeSelfUpdateClient) detect(_ context.Context, version string) (availableRelease, bool, error) {
	f.requestedVersion = version
	return f.release, f.found, f.detectErr
}

func (f *fakeSelfUpdateClient) install(_ context.Context, _ availableRelease, path string) error {
	f.installCalls++
	f.installedPath = path
	return f.installErr
}

func TestSelfUpdateChecksWithoutRepositoryOrMutation(t *testing.T) {
	client := &fakeSelfUpdateClient{release: availableRelease{version: "0.1.42", url: "https://example.invalid/release"}, found: true}
	stdout, root := selfUpdateRoot(t, BuildInfo{Version: "0.1.41"}, client, "/usr/local/bin/homelabctl", "linux", "amd64")
	root.SetArgs([]string{"self-update", "--check"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if client.installCalls != 0 {
		t.Fatalf("install calls = %d, want 0", client.installCalls)
	}
	for _, expected := range []string{"◆ homelabctl update", "Current version   v0.1.41", "Target version    v0.1.42", "update available"} {
		if !strings.Contains(stdout.String(), expected) {
			t.Errorf("output %q does not contain %q", stdout.String(), expected)
		}
	}
}

func TestSelfUpdateInstallsExactVersionAtExecutablePath(t *testing.T) {
	client := &fakeSelfUpdateClient{release: availableRelease{version: "0.1.40"}, found: true}
	stdout, root := selfUpdateRoot(t, BuildInfo{Version: "0.1.41"}, client, "/opt/bin/homelabctl", "linux", "arm64")
	root.SetArgs([]string{"self-update", "--version", "v0.1.40"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if client.requestedVersion != "0.1.40" {
		t.Fatalf("requested version = %q, want 0.1.40", client.requestedVersion)
	}
	if client.installCalls != 1 || client.installedPath != "/opt/bin/homelabctl" {
		t.Fatalf("install calls/path = %d/%q", client.installCalls, client.installedPath)
	}
	if !strings.Contains(stdout.String(), "updated /opt/bin/homelabctl to homelabctl v0.1.40") {
		t.Fatalf("unexpected output: %q", stdout.String())
	}
}

func TestSelfUpdateSkipsCurrentVersionUnlessForced(t *testing.T) {
	client := &fakeSelfUpdateClient{release: availableRelease{version: "0.1.42"}, found: true}
	_, root := selfUpdateRoot(t, BuildInfo{Version: "v0.1.42"}, client, "/bin/homelabctl", "darwin", "arm64")
	root.SetArgs([]string{"self-update"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if client.installCalls != 0 {
		t.Fatalf("install calls = %d, want 0", client.installCalls)
	}

	root.SetArgs([]string{"self-update", "--force"})
	if err := root.Execute(); err != nil {
		t.Fatalf("forced Execute() error = %v", err)
	}
	if client.installCalls != 1 {
		t.Fatalf("forced install calls = %d, want 1", client.installCalls)
	}
}

func TestSelfUpdateDryRunChecksButDoesNotInstall(t *testing.T) {
	client := &fakeSelfUpdateClient{release: availableRelease{version: "0.2.0"}, found: true}
	_, root := selfUpdateRoot(t, BuildInfo{Version: "dev"}, client, "/bin/homelabctl", "linux", "amd64")
	root.SetArgs([]string{"--dry-run", "self-update"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if client.installCalls != 0 {
		t.Fatalf("install calls = %d, want 0", client.installCalls)
	}
}

func TestSelfUpdateValidationAndFailures(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		client    *fakeSelfUpdateClient
		goos      string
		goarch    string
		pathError error
		want      string
	}{
		{name: "unsupported platform", client: &fakeSelfUpdateClient{}, goos: "windows", goarch: "amd64", want: "not supported"},
		{name: "invalid version", args: []string{"--version", "next"}, client: &fakeSelfUpdateClient{}, goos: "linux", goarch: "amd64", want: "semantic version"},
		{name: "missing release", client: &fakeSelfUpdateClient{}, goos: "linux", goarch: "amd64", want: "no compatible"},
		{name: "detect failure", client: &fakeSelfUpdateClient{detectErr: errors.New("offline")}, goos: "linux", goarch: "amd64", want: "checking GitHub releases"},
		{name: "path failure", client: &fakeSelfUpdateClient{release: availableRelease{version: "0.2.0"}, found: true}, goos: "linux", goarch: "amd64", pathError: errors.New("unknown path"), want: "locating"},
		{name: "install failure", client: &fakeSelfUpdateClient{release: availableRelease{version: "0.2.0"}, found: true, installErr: errors.New("permission denied")}, goos: "linux", goarch: "amd64", want: "rerun with sudo"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout bytes.Buffer
			runner := command.NewRunner(strings.NewReader(""), &stdout, &bytes.Buffer{})
			state := &state{runner: runner, build: BuildInfo{Version: "0.1.0"}}
			dependencies := selfUpdateDependencies{
				newClient: func() (selfUpdateClient, error) { return test.client, nil },
				executablePath: func() (string, error) {
					return "/usr/local/bin/homelabctl", test.pathError
				},
				goos: test.goos, goarch: test.goarch,
			}
			cmd := newSelfUpdateCommand(state, dependencies)
			cmd.SetArgs(test.args)
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Execute() error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestNormalizeRequestedVersion(t *testing.T) {
	for input, expected := range map[string]string{"": "", "v0.1.42": "0.1.42", "1.2.3": "1.2.3"} {
		actual, err := normalizeRequestedVersion(input)
		if err != nil || actual != expected {
			t.Errorf("normalizeRequestedVersion(%q) = %q, %v; want %q", input, actual, err, expected)
		}
	}
	for _, invalid := range []string{"1", "1.2", "1.2.3.4", "latest"} {
		if _, err := normalizeRequestedVersion(invalid); err == nil {
			t.Errorf("normalizeRequestedVersion(%q) unexpectedly succeeded", invalid)
		}
	}
}

func selfUpdateRoot(t *testing.T, build BuildInfo, client selfUpdateClient, path, goos, goarch string) (*bytes.Buffer, *cobra.Command) {
	t.Helper()
	var stdout bytes.Buffer
	runner := command.NewRunner(strings.NewReader(""), &stdout, &bytes.Buffer{})
	state := &state{runner: runner, build: build}
	dependencies := selfUpdateDependencies{
		newClient:      func() (selfUpdateClient, error) { return client, nil },
		executablePath: func() (string, error) { return path, nil },
		goos:           goos,
		goarch:         goarch,
	}
	root := &cobra.Command{Use: "homelabctl"}
	root.PersistentFlags().BoolVar(&state.dryRun, "dry-run", false, "")
	root.AddCommand(newSelfUpdateCommand(state, dependencies))
	return &stdout, root
}
