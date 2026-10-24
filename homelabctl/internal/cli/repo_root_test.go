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
)

// TestCommandOutsideRepositoryUsesRememberedRoot proves that an operator can
// run a command from any directory once the CLI has succeeded from the
// repository once.
func TestCommandOutsideRepositoryUsesRememberedRoot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))

	root := initCLIRepository(t)
	if err := repository.SaveRoot(root); err != nil {
		t.Fatalf("SaveRoot() error = %v", err)
	}
	t.Chdir(t.TempDir())

	var stdout bytes.Buffer
	runner := command.NewRunner(strings.NewReader(""), &stdout, &bytes.Buffer{})
	cobraRoot := New(BuildInfo{Version: "1.2.3", Commit: "abc123", Date: "today"}, runner)
	cobraRoot.SetArgs([]string{"node", "connect", "titan", "--dry-run"})

	err := cobraRoot.Execute()
	if err == nil {
		t.Fatal("expected the command to fail for a missing inventory, not to resolve the repository")
	}
	if strings.Contains(err.Error(), "no .git directory or file found") {
		t.Fatalf("the remembered repository root was not used: %v", err)
	}
}

// TestCommandOutsideRepositoryExplainsHowToFixIt proves the failure is
// actionable instead of merely stating that discovery failed.
func TestCommandOutsideRepositoryExplainsHowToFixIt(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Chdir(t.TempDir())

	var stdout bytes.Buffer
	runner := command.NewRunner(strings.NewReader(""), &stdout, &bytes.Buffer{})
	cobraRoot := New(BuildInfo{Version: "1.2.3", Commit: "abc123", Date: "today"}, runner)
	cobraRoot.SetArgs([]string{"node", "connect", "titan", "--dry-run"})

	err := cobraRoot.Execute()
	if err == nil {
		t.Fatal("expected an error outside a repository")
	}
	message := err.Error()
	for _, want := range []string{"repository root", "--repo-root"} {
		if !strings.Contains(message, want) {
			t.Fatalf("error message %q does not mention %q", message, want)
		}
	}
}

// initCLIRepository creates a committed Git repository the CLI accepts as a
// repository root.
func initCLIRepository(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	repositoryUnderTest, err := git.PlainInit(root, false)
	if err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(root, "marker")
	if err := os.WriteFile(marker, []byte("marker\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	worktree, err := repositoryUnderTest.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := worktree.Add("marker"); err != nil {
		t.Fatal(err)
	}
	if _, err := worktree.Commit("initial", &git.CommitOptions{Author: &object.Signature{
		Name: "cli test", Email: "test@example.invalid", When: time.Unix(1, 0),
	}}); err != nil {
		t.Fatal(err)
	}
	return root
}
