package lint

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/iamkhattar/homelab/hl/internal/ui"
)

// HelmLinter discovers Helm charts and runs helm lint on each.
type HelmLinter struct{}

func (h *HelmLinter) Name() string { return "helm" }
func (h *HelmLinter) Tool() string { return "helm" }
func (h *HelmLinter) CanFix() bool { return false }

func (h *HelmLinter) Fix(_ string) error { return nil }

func (h *HelmLinter) Lint(root string) error {
	clusterDir := filepath.Join(root, "cluster")
	if _, err := os.Stat(clusterDir); os.IsNotExist(err) {
		return nil
	}

	charts, err := findCharts(clusterDir)
	if err != nil {
		return err
	}

	if len(charts) == 0 {
		ui.KeyValue("charts", "none found")
		return nil
	}

	var failed []string
	for _, chartDir := range charts {
		rel, _ := filepath.Rel(root, chartDir)
		c := exec.Command("helm", "lint", chartDir)
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		if err := c.Run(); err != nil {
			failed = append(failed, rel)
		}
	}

	if len(failed) > 0 {
		return fmt.Errorf("%d chart(s) failed lint: %v", len(failed), failed)
	}
	return nil
}

// findCharts walks a directory tree and returns paths to directories containing Chart.yaml.
func findCharts(dir string) ([]string, error) {
	var charts []string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && d.Name() == "Chart.yaml" {
			charts = append(charts, filepath.Dir(path))
		}
		return nil
	})
	return charts, err
}
