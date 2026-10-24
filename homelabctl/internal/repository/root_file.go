package repository

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const rootFileName = "repo-root"

// rootFilePath returns the file that records the last repository root the CLI
// used successfully.
func rootFilePath() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolving user configuration directory: %w", err)
	}
	return filepath.Join(base, "homelabctl", rootFileName), nil
}

// SaveRoot records a repository root so later commands can run from any
// directory. The record is a convenience, never a source of authority: it is
// only used when the working directory does not contain a repository and the
// recorded path still is one.
func SaveRoot(root string) error {
	path, err := rootFilePath()
	if err != nil {
		return err
	}
	return saveRootFile(path, root)
}

func saveRootFile(path, root string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("creating configuration directory: %w", err)
	}
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, []byte(strings.TrimSpace(root)+"\n"), 0o600); err != nil {
		return fmt.Errorf("writing repository root: %w", err)
	}
	if err := os.Rename(temporary, path); err != nil {
		_ = os.Remove(temporary)
		return fmt.Errorf("writing repository root: %w", err)
	}
	return nil
}

// RememberedRoot returns the previously recorded repository root when it still
// is a usable Git worktree, and an empty string otherwise.
func RememberedRoot() string {
	path, err := rootFilePath()
	if err != nil {
		return ""
	}
	return rememberedRootFile(path)
}

func rememberedRootFile(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	candidate := strings.TrimSpace(string(raw))
	if candidate == "" {
		return ""
	}
	if err := validateRoot(candidate); err != nil {
		return ""
	}
	return candidate
}
