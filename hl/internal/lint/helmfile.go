package lint

import (
	"os"
	"os/exec"
	"path/filepath"
)

// HelmfileLinter runs helmfile lint in the cluster directory.
type HelmfileLinter struct{}

func (h *HelmfileLinter) Name() string { return "helmfile" }
func (h *HelmfileLinter) Tool() string { return "helmfile" }
func (h *HelmfileLinter) CanFix() bool { return false }

func (h *HelmfileLinter) Fix(_ string) error { return nil }

func (h *HelmfileLinter) Lint(root string) error {
	dir := filepath.Join(root, "cluster")
	if _, err := os.Stat(filepath.Join(dir, "helmfile.yaml")); os.IsNotExist(err) {
		return nil
	}

	c := exec.Command("helmfile", "lint")
	c.Dir = dir
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}
