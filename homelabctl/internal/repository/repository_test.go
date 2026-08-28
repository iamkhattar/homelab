package repository

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func TestRootFindsRepositoryWithoutGitExecutable(t *testing.T) {
	root, _ := initGitRepository(t)
	nested := filepath.Join(root, "one", "two")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(nested)

	got, err := Root(context.Background(), "")
	if err != nil {
		t.Fatalf("Root() error = %v", err)
	}
	if got != root {
		t.Fatalf("Root() = %q, want %q", got, root)
	}
}

func TestRootValidatesExplicitRepository(t *testing.T) {
	root, _ := initGitRepository(t)
	nested := filepath.Join(root, "nested")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Root(context.Background(), nested); err == nil || !strings.Contains(err.Error(), "repository root") {
		t.Fatalf("Root() accepted nested explicit path, error = %v", err)
	}
	if _, err := Root(context.Background(), t.TempDir()); err == nil || !strings.Contains(err.Error(), "not a readable Git repository") {
		t.Fatalf("Root() accepted non-repository path, error = %v", err)
	}
}

func TestRootFailsOutsideRepository(t *testing.T) {
	t.Chdir(t.TempDir())
	if _, err := Root(context.Background(), ""); err == nil || !strings.Contains(err.Error(), "no .git directory or file found") {
		t.Fatalf("Root() error = %v, want discovery failure", err)
	}
}

func TestHeadSHAReadsCommittedHead(t *testing.T) {
	root, initialHash := initGitRepository(t)

	got, err := HeadSHA(root)
	if err != nil {
		t.Fatalf("HeadSHA() error = %v", err)
	}
	if got != initialHash.String() {
		t.Fatalf("HeadSHA() = %q, want %q", got, initialHash)
	}
}

func TestHeadCommitDateUsesCommittedTimestamp(t *testing.T) {
	root, _ := initGitRepository(t)

	got, err := HeadCommitDate(root)
	if err != nil {
		t.Fatalf("HeadCommitDate() error = %v", err)
	}
	if got != "1970-01-01T00:00:01Z" {
		t.Fatalf("HeadCommitDate() = %q, want deterministic commit timestamp", got)
	}
}

func TestHeadSHARejectsUnbornRepository(t *testing.T) {
	root := t.TempDir()
	if _, err := git.PlainInit(root, false); err != nil {
		t.Fatal(err)
	}
	if _, err := HeadSHA(root); err == nil || !strings.Contains(err.Error(), "resolving Git HEAD") {
		t.Fatalf("HeadSHA() error = %v, want unborn HEAD failure", err)
	}
}

func TestChangedServiceNamesUsesMergeBaseAndIgnoresWorktree(t *testing.T) {
	root, baseHash := initGitRepository(t)
	repository, err := git.PlainOpen(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.Storer.SetReference(plumbing.NewHashReference("refs/remotes/origin/main", baseHash)); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "services", "api", "main.go"))
	writeFile(t, filepath.Join(root, "services", "worker", "Dockerfile"))
	writeFile(t, filepath.Join(root, "docs", "guide.md"))
	commitPaths(t, repository, "change services", "services/api/main.go", "services/worker/Dockerfile", "docs/guide.md")
	writeFile(t, filepath.Join(root, "services", "uncommitted", "Dockerfile"))

	got, err := ChangedServiceNames(root, "origin/main")
	if err != nil {
		t.Fatalf("ChangedServiceNames() error = %v", err)
	}
	want := []string{"api", "worker"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ChangedServiceNames() = %v, want %v", got, want)
	}
}

func TestChangedServiceNamesRejectsUnknownBase(t *testing.T) {
	root, _ := initGitRepository(t)
	if _, err := ChangedServiceNames(root, "missing-ref"); err == nil {
		t.Fatal("ChangedServiceNames() unexpectedly accepted an unknown base")
	}
}

func TestChangedServiceNamesIncludesTopLevelButler(t *testing.T) {
	root, baseHash := initGitRepository(t)
	repository, err := git.PlainOpen(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.Storer.SetReference(plumbing.NewHashReference("refs/remotes/origin/main", baseHash)); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "butler", "cmd", "butler", "main.go"))
	commitPaths(t, repository, "change butler", "butler/cmd/butler/main.go")
	got, err := ChangedServiceNames(root, "origin/main")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []string{"butler"}) {
		t.Fatalf("ChangedServiceNames() = %v, want butler", got)
	}
}

func TestChangedServiceNamesIncludesDeletedAndRenamedServices(t *testing.T) {
	root, _ := initGitRepository(t)
	repository, err := git.PlainOpen(root)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "services", "retired", "Dockerfile"))
	writeFile(t, filepath.Join(root, "services", "old-name", "Dockerfile"))
	baseHash := commitPaths(t, repository, "add services", "services/retired/Dockerfile", "services/old-name/Dockerfile")
	if err := repository.Storer.SetReference(plumbing.NewHashReference("refs/remotes/origin/main", baseHash)); err != nil {
		t.Fatal(err)
	}
	worktree, err := repository.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := worktree.Remove("services/retired/Dockerfile"); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(root, "services", "old-name"), filepath.Join(root, "services", "new-name")); err != nil {
		t.Fatal(err)
	}
	if _, err := worktree.Remove("services/old-name/Dockerfile"); err != nil {
		t.Fatal(err)
	}
	if _, err := worktree.Add("services/new-name/Dockerfile"); err != nil {
		t.Fatal(err)
	}
	if _, err := worktree.Commit("remove and rename services", &git.CommitOptions{Author: &object.Signature{
		Name: "repository test", Email: "test@example.invalid", When: time.Unix(2, 0),
	}}); err != nil {
		t.Fatal(err)
	}

	got, err := ChangedServiceNames(root, "origin/main")
	if err != nil {
		t.Fatalf("ChangedServiceNames() error = %v", err)
	}
	want := []string{"new-name", "old-name", "retired"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ChangedServiceNames() = %v, want %v", got, want)
	}
}

func TestGoModulesSkipsGeneratedDirectories(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "homelabctl", "go.mod"))
	writeFile(t, filepath.Join(root, "services", "butler", "go.mod"))
	writeFile(t, filepath.Join(root, "node_modules", "ignored", "go.mod"))

	modules, err := GoModules(root)
	if err != nil {
		t.Fatalf("GoModules() error = %v", err)
	}
	if len(modules) != 2 {
		t.Fatalf("GoModules() returned %d modules, want 2: %v", len(modules), modules)
	}
}

func TestGoFilesFindsSourceAndSkipsGeneratedDirectories(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "homelabctl", "main.go"))
	writeFile(t, filepath.Join(root, "services", "api", "api_test.go"))
	writeFile(t, filepath.Join(root, "vendor", "ignored.go"))
	writeFile(t, filepath.Join(root, "docs", "example.txt"))

	files, err := GoFiles(root)
	if err != nil {
		t.Fatalf("GoFiles() error = %v", err)
	}
	want := []string{
		filepath.Join(root, "homelabctl", "main.go"),
		filepath.Join(root, "services", "api", "api_test.go"),
	}
	if !reflect.DeepEqual(files, want) {
		t.Fatalf("GoFiles() = %v, want %v", files, want)
	}
}

func TestServicesFindsDockerfiles(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "butler", "Dockerfile"))
	writeFile(t, filepath.Join(root, "services", "notes", "README.md"))

	services, err := Services(root)
	if err != nil {
		t.Fatalf("Services() error = %v", err)
	}
	if len(services) != 1 || services[0].Name != "butler" {
		t.Fatalf("Services() = %v, want butler", services)
	}
}

func TestServicesReturnsEmptyWhenDirectoryDoesNotExist(t *testing.T) {
	services, err := Services(t.TempDir())
	if err != nil || len(services) != 0 {
		t.Fatalf("Services() = %v, %v; want empty result", services, err)
	}
}

func TestServicesFindsTopLevelButlerWithoutServicesDirectory(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "butler", "Dockerfile"))
	services, err := Services(root)
	if err != nil || len(services) != 1 || services[0].Name != "butler" {
		t.Fatalf("Services() = %v, %v; want top-level Butler", services, err)
	}
}

func writeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("test"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func initGitRepository(t *testing.T) (string, plumbing.Hash) {
	t.Helper()
	root := t.TempDir()
	repository, err := git.PlainInit(root, false)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, ".repository-marker"))
	hash := commitPaths(t, repository, "initial commit", ".repository-marker")
	return root, hash
}

func commitPaths(t *testing.T, repository *git.Repository, message string, paths ...string) plumbing.Hash {
	t.Helper()
	worktree, err := repository.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		if _, err := worktree.Add(filepath.ToSlash(path)); err != nil {
			t.Fatal(err)
		}
	}
	hash, err := worktree.Commit(message, &git.CommitOptions{Author: &object.Signature{
		Name: "repository test", Email: "test@example.invalid", When: time.Unix(1, 0),
	}})
	if err != nil {
		t.Fatal(err)
	}
	return hash
}
