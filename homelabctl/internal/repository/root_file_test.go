package repository

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRememberedRootRoundTripsAValidRepository(t *testing.T) {
	root, _ := initGitRepository(t)
	path := filepath.Join(t.TempDir(), "homelabctl", "repo-root")

	if err := saveRootFile(path, root); err != nil {
		t.Fatalf("saveRootFile() error = %v", err)
	}
	if got := rememberedRootFile(path); got != root {
		t.Fatalf("rememberedRootFile() = %q, want %q", got, root)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Fatalf("repository root file mode = %o, want 600", mode)
	}
}

func TestRememberedRootIgnoresUnusableRecords(t *testing.T) {
	directory := t.TempDir()

	cases := map[string]string{
		"missing file": filepath.Join(directory, "absent"),
		"empty file":   filepath.Join(directory, "empty"),
		"not a repo":   filepath.Join(directory, "not-a-repo"),
	}
	for name, path := range cases {
		t.Run(name, func(t *testing.T) {
			if name == "empty file" || name == "not a repo" {
				if err := os.WriteFile(path, []byte("\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				if name == "not a repo" {
					if err := os.WriteFile(path, []byte(directory+"\n"), 0o600); err != nil {
						t.Fatal(err)
					}
				}
			}
			if got := rememberedRootFile(path); got != "" {
				t.Fatalf("rememberedRootFile() = %q, want empty", got)
			}
		})
	}
}

func TestRememberedRootIgnoresARemovedRepository(t *testing.T) {
	root, _ := initGitRepository(t)
	path := filepath.Join(t.TempDir(), "repo-root")
	if err := saveRootFile(path, root); err != nil {
		t.Fatalf("saveRootFile() error = %v", err)
	}
	if got := rememberedRootFile(path); got != root {
		t.Fatalf("rememberedRootFile() = %q, want %q", got, root)
	}
	if err := os.RemoveAll(filepath.Join(root, ".git")); err != nil {
		t.Fatal(err)
	}
	if got := rememberedRootFile(path); got != "" {
		t.Fatalf("rememberedRootFile() = %q, want empty for a removed repository", got)
	}
}
