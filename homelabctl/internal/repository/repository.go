package repository

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

type Service struct {
	Name string
	Dir  string
}

func Root(_ context.Context, explicit string) (string, error) {
	if explicit != "" {
		root, err := filepath.Abs(explicit)
		if err != nil {
			return "", fmt.Errorf("resolving repository root: %w", err)
		}
		if err := validateRoot(root); err != nil {
			return "", err
		}
		return root, nil
	}

	start, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("reading current directory: %w", err)
	}
	for candidate := start; ; candidate = filepath.Dir(candidate) {
		if _, err := os.Stat(filepath.Join(candidate, ".git")); err == nil {
			if err := validateRoot(candidate); err != nil {
				return "", err
			}
			return candidate, nil
		}
		parent := filepath.Dir(candidate)
		if parent == candidate {
			break
		}
	}
	return "", fmt.Errorf("finding repository root from %s: no .git directory or file found", start)
}

func HeadSHA(root string) (string, error) {
	repository, err := open(root)
	if err != nil {
		return "", err
	}
	reference, err := repository.Head()
	if err != nil {
		return "", fmt.Errorf("resolving Git HEAD: %w", err)
	}
	hash := reference.Hash()
	if hash.IsZero() {
		return "", fmt.Errorf("resolving Git HEAD: commit hash is empty")
	}
	return hash.String(), nil
}

// HeadCommitDate returns the HEAD commit timestamp as deterministic RFC 3339
// build metadata. Image builds use the commit time instead of wall-clock time
// so every artifact for one revision reports the same date.
func HeadCommitDate(root string) (string, error) {
	repository, err := open(root)
	if err != nil {
		return "", err
	}
	reference, err := repository.Head()
	if err != nil {
		return "", fmt.Errorf("resolving Git HEAD: %w", err)
	}
	commit, err := repository.CommitObject(reference.Hash())
	if err != nil {
		return "", fmt.Errorf("loading Git HEAD commit: %w", err)
	}
	return commit.Committer.When.UTC().Format(time.RFC3339), nil
}

func ChangedServiceNames(root, base string) ([]string, error) {
	repository, err := open(root)
	if err != nil {
		return nil, err
	}
	baseHash, err := repository.ResolveRevision(plumbing.Revision(base))
	if err != nil {
		return nil, fmt.Errorf("resolving Git base %q: %w", base, err)
	}
	headHash, err := repository.ResolveRevision(plumbing.Revision("HEAD"))
	if err != nil {
		return nil, fmt.Errorf("resolving Git HEAD: %w", err)
	}
	baseCommit, err := repository.CommitObject(*baseHash)
	if err != nil {
		return nil, fmt.Errorf("loading Git base commit: %w", err)
	}
	headCommit, err := repository.CommitObject(*headHash)
	if err != nil {
		return nil, fmt.Errorf("loading Git HEAD commit: %w", err)
	}
	mergeBases, err := headCommit.MergeBase(baseCommit)
	if err != nil {
		return nil, fmt.Errorf("finding Git merge base: %w", err)
	}
	if len(mergeBases) != 1 {
		return nil, fmt.Errorf("expected one Git merge base between %q and HEAD, found %d", base, len(mergeBases))
	}
	baseTree, err := mergeBases[0].Tree()
	if err != nil {
		return nil, fmt.Errorf("loading Git merge-base tree: %w", err)
	}
	headTree, err := headCommit.Tree()
	if err != nil {
		return nil, fmt.Errorf("loading Git HEAD tree: %w", err)
	}
	changes, err := object.DiffTree(baseTree, headTree)
	if err != nil {
		return nil, fmt.Errorf("comparing Git trees: %w", err)
	}

	serviceNames := map[string]struct{}{}
	for _, change := range changes {
		for _, path := range []string{change.From.Name, change.To.Name} {
			parts := strings.Split(filepath.ToSlash(path), "/")
			if len(parts) >= 2 && parts[0] == "services" && parts[1] != "" {
				serviceNames[parts[1]] = struct{}{}
			}
			if len(parts) >= 1 && parts[0] == "butler" {
				serviceNames["butler"] = struct{}{}
			}
		}
	}
	names := make([]string, 0, len(serviceNames))
	for name := range serviceNames {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

func validateRoot(root string) error {
	repository, err := git.PlainOpen(root)
	if err != nil {
		return fmt.Errorf("%s is not a readable Git repository root: %w", root, err)
	}
	if _, err := repository.Worktree(); err != nil {
		return fmt.Errorf("%s is not a Git worktree: %w", root, err)
	}
	return nil
}

func open(root string) (*git.Repository, error) {
	repository, err := git.PlainOpen(root)
	if err != nil {
		return nil, fmt.Errorf("opening Git repository at %s: %w", root, err)
	}
	return repository, nil
}

func GoModules(root string) ([]string, error) {
	var modules []string
	err := walk(root, func(path string, entry fs.DirEntry) error {
		if !entry.IsDir() && entry.Name() == "go.mod" {
			modules = append(modules, filepath.Dir(path))
		}
		return nil
	})
	sort.Strings(modules)
	return modules, err
}

func GoFiles(root string) ([]string, error) {
	var files []string
	err := walk(root, func(path string, entry fs.DirEntry) error {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".go") {
			files = append(files, path)
		}
		return nil
	})
	sort.Strings(files)
	return files, err
}

func Services(root string) ([]Service, error) {
	serviceRoot := filepath.Join(root, "services")
	entries, err := os.ReadDir(serviceRoot)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("reading services directory: %w", err)
	}

	var services []Service
	butlerDir := filepath.Join(root, "butler")
	if _, err := os.Stat(filepath.Join(butlerDir, "Dockerfile")); err == nil {
		services = append(services, Service{Name: "butler", Dir: butlerDir})
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(serviceRoot, entry.Name())
		if _, err := os.Stat(filepath.Join(dir, "Dockerfile")); err == nil {
			services = append(services, Service{Name: entry.Name(), Dir: dir})
		}
	}
	sort.Slice(services, func(i, j int) bool { return services[i].Name < services[j].Name })
	return services, nil
}

func walk(root string, visit func(string, fs.DirEntry) error) error {
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() && path != root && skippedDirectory(entry.Name()) {
			return filepath.SkipDir
		}
		return visit(path, entry)
	})
}

func skippedDirectory(name string) bool {
	switch name {
	case ".git", ".terraform", ".venv", "collections", "node_modules", "vendor", "dist", "cache":
		return true
	default:
		return false
	}
}
