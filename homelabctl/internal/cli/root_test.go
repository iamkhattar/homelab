package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/iamkhattar/homelab/homelabctl/internal/command"
	"github.com/iamkhattar/homelab/homelabctl/internal/repository"
	"github.com/spf13/cobra"
)

func TestVersionDoesNotRequireRepository(t *testing.T) {
	var stdout bytes.Buffer
	runner := command.NewRunner(strings.NewReader(""), &stdout, &bytes.Buffer{})
	root := New(BuildInfo{Version: "1.2.3", Commit: "abc123", Date: "today"}, runner)
	root.SetArgs([]string{"version"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got, want := stdout.String(), "homelabctl 1.2.3 (commit: abc123, built: today)\n"; got != want {
		t.Fatalf("version output = %q, want %q", got, want)
	}
}

func TestNodePrepareDryRunBuildsSafeCommand(t *testing.T) {
	var stderr bytes.Buffer
	runner := command.NewRunner(strings.NewReader(""), &bytes.Buffer{}, &stderr)
	root := New(BuildInfo{}, runner)
	root.SetArgs([]string{"--repo-root", testRepository(t), "--dry-run", "node", "prepare", "--limit", "titan", "--check"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	got := stderr.String()
	if !strings.Contains(got, "ansible-playbook playbooks/prepare.yml --limit titan --check --diff") {
		t.Fatalf("dry-run output did not contain expected command: %q", got)
	}
}

func TestNodeAuthorizeKeyDryRunUsesPublicKeyOnly(t *testing.T) {
	repo := testRepository(t)
	key := filepath.Join(repo, "operator.pub")
	if err := os.WriteFile(key, []byte("ssh-ed25519 AAAAAAAAAAAAAAAAAAAAAA== operator\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var stderr bytes.Buffer
	runner := command.NewRunner(strings.NewReader(""), &bytes.Buffer{}, &stderr)
	root := New(BuildInfo{}, runner)
	root.SetArgs([]string{"--repo-root", repo, "--dry-run", "node", "authorize-key", "titan", "--public-key", key})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got := stderr.String(); !strings.Contains(got, "ssh-copy-id -i "+key+" titan") {
		t.Fatalf("dry-run output did not contain expected key install: %q", got)
	}
}

func TestRunnableCommandsHaveDetailedHelpAndExamples(t *testing.T) {
	runner := command.NewRunner(strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	root := New(BuildInfo{}, runner)

	var visit func(*cobra.Command)
	visit = func(cmd *cobra.Command) {
		path := cmd.CommandPath()
		generated := cmd.Name() == "help" || strings.HasPrefix(path, "homelabctl completion")
		if !generated && (strings.TrimSpace(cmd.Long) == "" || cmd.Long == cmd.Short) {
			t.Errorf("%s must have detailed Long help", path)
		}
		if cmd.Runnable() && !generated {
			if strings.TrimSpace(cmd.Example) == "" {
				t.Errorf("%s must have at least one example", path)
			}
		}
		for _, child := range cmd.Commands() {
			visit(child)
		}
	}
	visit(root)
}

func TestInvalidInputsFailBeforeExternalCommands(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "empty context", args: []string{"--context", "", "doctor"}, want: "context"},
		{name: "invalid host", args: []string{"node", "connect", "bad host"}, want: "host must"},
		{name: "invalid release", args: []string{"deploy", "apply", "Bad_Name"}, want: "release must"},
		{name: "invalid image tag", args: []string{"build", "docs", "--tag", "bad tag"}, want: "invalid image tag"},
		{name: "blank image", args: []string{"docs", "serve", "--image", ""}, want: "image must not be empty"},
		{name: "blank Ansible limit", args: []string{"node", "prepare", "--limit", ""}, want: "limit must not be empty"},
		{name: "registry whitespace", args: []string{"build", "services", "--registry", "bad registry"}, want: "must not contain whitespace"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stderr bytes.Buffer
			runner := command.NewRunner(strings.NewReader(""), &bytes.Buffer{}, &stderr)
			root := New(BuildInfo{}, runner)
			args := append([]string{"--repo-root", testRepository(t), "--dry-run"}, test.args...)
			root.SetArgs(args)
			err := root.Execute()
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(test.want)) {
				t.Fatalf("Execute() error = %v, want substring %q", err, test.want)
			}
			if stderr.Len() != 0 {
				t.Fatalf("external command was prepared before validation: %q", stderr.String())
			}
		})
	}
}

func TestNodeAuthorizeKeyRejectsPrivateKey(t *testing.T) {
	repo := testRepository(t)
	key := filepath.Join(repo, "operator")
	if err := os.WriteFile(key, []byte("-----BEGIN OPENSSH PRIVATE KEY-----\nsecret\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	runner := command.NewRunner(strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	root := New(BuildInfo{}, runner)
	root.SetArgs([]string{"--repo-root", repo, "--dry-run", "node", "authorize-key", "titan", "--public-key", key})

	if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "public key") {
		t.Fatalf("Execute() error = %v, want public-key validation failure", err)
	}
}

func TestAnsibleExecutablePrefersLocalVirtualEnvironment(t *testing.T) {
	root := t.TempDir()
	local := root + "/.venv/bin/ansible-playbook"
	if err := writeEmpty(local); err != nil {
		t.Fatal(err)
	}
	if got := ansibleExecutable(root, "ansible-playbook"); got != local {
		t.Fatalf("ansibleExecutable() = %q, want %q", got, local)
	}
}

func TestBuildDocsDryRunUsesIsolatedContext(t *testing.T) {
	var stderr bytes.Buffer
	runner := command.NewRunner(strings.NewReader(""), &bytes.Buffer{}, &stderr)
	root := New(BuildInfo{}, runner)
	root.SetArgs([]string{"--repo-root", testRepository(t), "--dry-run", "build", "docs", "--image", "homelab-docs", "--tag", "test"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got := stderr.String(); !strings.Contains(got, "docker build --tag homelab-docs:test docs") {
		t.Fatalf("dry-run output did not contain the docs build: %q", got)
	}
}

func TestCIBuildDryRunBuildsServicesAndDocs(t *testing.T) {
	repo := testRepository(t)
	if err := writeEmpty(filepath.Join(repo, "services", "example", "Dockerfile")); err != nil {
		t.Fatal(err)
	}
	if err := writeEmpty(filepath.Join(repo, "docs", "Dockerfile")); err != nil {
		t.Fatal(err)
	}

	var stderr bytes.Buffer
	runner := command.NewRunner(strings.NewReader(""), &bytes.Buffer{}, &stderr)
	root := New(BuildInfo{}, runner)
	root.SetArgs([]string{"--repo-root", repo, "--dry-run", "ci", "build", "--tag", "revision"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	got := stderr.String()
	if !strings.Contains(got, "docker build --tag iamkhattar/example:revision .") {
		t.Fatalf("dry-run output did not contain the service build: %q", got)
	}
	if !strings.Contains(got, "docker build --tag iamkhattar/homelab-docs:revision docs") {
		t.Fatalf("dry-run output did not contain the docs build: %q", got)
	}
}

func TestCIPublishRequiresCI(t *testing.T) {
	t.Setenv("CI", "")
	runner := command.NewRunner(strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	root := New(BuildInfo{}, runner)
	root.SetArgs([]string{"--repo-root", testRepository(t), "--dry-run", "ci", "publish", "--tag", "revision"})

	if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "only allowed when CI is set") {
		t.Fatalf("Execute() error = %v, want CI guard", err)
	}
}

func TestCIPublishDefaultsToGitSHA(t *testing.T) {
	t.Setenv("CI", "true")
	repo := testRepository(t)
	sha, err := repository.HeadSHA(repo)
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	runner := command.NewRunner(strings.NewReader(""), &bytes.Buffer{}, &stderr)
	root := New(BuildInfo{}, runner)
	root.SetArgs([]string{"--repo-root", repo, "--dry-run", "ci", "publish"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	got := stderr.String()
	if strings.Contains(got, "git ") || !strings.Contains(got, "iamkhattar/homelab-docs:"+sha) {
		t.Fatalf("dry-run output did not use the Git SHA default: %q", got)
	}
}

func TestSetupAnsibleDryRunUsesLocalVirtualEnvironment(t *testing.T) {
	var stderr bytes.Buffer
	runner := command.NewRunner(strings.NewReader(""), &bytes.Buffer{}, &stderr)
	root := New(BuildInfo{}, runner)
	root.SetArgs([]string{"--repo-root", testRepository(t), "--dry-run", "setup", "ansible"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	got := stderr.String()
	if !strings.Contains(got, "python3 -m venv") || !strings.Contains(got, "ansible-galaxy collection install") {
		t.Fatalf("setup dry-run did not contain expected commands: %q", got)
	}
}

func TestInventoryInitCreatesPrivateFileWithoutOverwrite(t *testing.T) {
	repo := testRepository(t)
	example := repo + "/ansible/inventory/hosts.example.yml"
	if err := os.MkdirAll(filepath.Dir(example), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(example, []byte("example\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runner := command.NewRunner(strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	root := New(BuildInfo{}, runner)
	root.SetArgs([]string{"--repo-root", repo, "inventory", "init"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	destination := repo + "/ansible/inventory/hosts.yml"
	info, err := os.Stat(destination)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("inventory mode = %o, want 600", info.Mode().Perm())
	}

	root = New(BuildInfo{}, runner)
	root.SetArgs([]string{"--repo-root", repo, "inventory", "init"})
	if err := root.Execute(); err == nil {
		t.Fatal("second inventory init unexpectedly succeeded")
	}
}

func TestSnapshotSaveDryRunUsesValidatedPlaybook(t *testing.T) {
	var stderr bytes.Buffer
	runner := command.NewRunner(strings.NewReader(""), &bytes.Buffer{}, &stderr)
	root := New(BuildInfo{}, runner)
	root.SetArgs([]string{"--repo-root", testRepository(t), "--dry-run", "cluster", "snapshot", "save", "--name", "pre-upgrade"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got := stderr.String(); !strings.Contains(got, "playbooks/snapshot.yml") || !strings.Contains(got, "pre-upgrade") {
		t.Fatalf("snapshot dry-run did not contain expected command: %q", got)
	}
}

func TestRecoveryExportRejectsRepositoryDestination(t *testing.T) {
	repo := testRepository(t)
	var stderr bytes.Buffer
	runner := command.NewRunner(strings.NewReader(""), &bytes.Buffer{}, &stderr)
	root := New(BuildInfo{}, runner)
	root.SetArgs([]string{"--repo-root", repo, "--dry-run", "cluster", "recovery", "export", "--destination", filepath.Join(repo, "recovery")})

	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "outside the repository") {
		t.Fatalf("Execute() error = %v, want repository destination rejection", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("external command was prepared before validation: %q", stderr.String())
	}
}

func TestSnapshotNameLengthIsBounded(t *testing.T) {
	runner := command.NewRunner(strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	root := New(BuildInfo{}, runner)
	root.SetArgs([]string{"--repo-root", testRepository(t), "--dry-run", "cluster", "snapshot", "save", "--name", strings.Repeat("a", 65)})

	if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "1-64") {
		t.Fatalf("Execute() error = %v, want snapshot length validation", err)
	}
}

func TestDocsDevDryRunUsesIsolatedProject(t *testing.T) {
	var stderr bytes.Buffer
	runner := command.NewRunner(strings.NewReader(""), &bytes.Buffer{}, &stderr)
	root := New(BuildInfo{}, runner)
	root.SetArgs([]string{"--repo-root", testRepository(t), "--dry-run", "docs", "dev"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got := stderr.String(); !strings.Contains(got, "npm run dev") || !strings.Contains(got, "/docs") {
		t.Fatalf("docs dry-run did not contain expected command: %q", got)
	}
}

func TestNodeVersionSupported(t *testing.T) {
	tests := map[string]bool{
		"v20.11.1": false,
		"v22.12.0": false,
		"v24.0.0":  true,
		"v25.0.0":  true,
		"unknown":  false,
	}
	for version, want := range tests {
		if got := nodeVersionSupported(version); got != want {
			t.Errorf("nodeVersionSupported(%q) = %v, want %v", version, got, want)
		}
	}
}

func TestGoVersionSupported(t *testing.T) {
	tests := map[string]bool{
		"go version go1.26.3 darwin/arm64": false,
		"go version go1.27.0 linux/amd64":  true,
		"go version go1.28.1 linux/arm64":  true,
		"unknown":                          false,
	}
	for version, want := range tests {
		if got := goVersionSupported(version); got != want {
			t.Errorf("goVersionSupported(%q) = %v, want %v", version, got, want)
		}
	}
}

func testRepository(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	repository, err := git.PlainInit(root, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeEmpty(filepath.Join(root, ".repository-marker")); err != nil {
		t.Fatal(err)
	}
	worktree, err := repository.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := worktree.Add(".repository-marker"); err != nil {
		t.Fatal(err)
	}
	if _, err := worktree.Commit("initial test repository", &git.CommitOptions{Author: &object.Signature{
		Name: "homelabctl test", Email: "test@example.invalid", When: time.Unix(1, 0),
	}}); err != nil {
		t.Fatal(err)
	}
	return root
}
