package lint

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/iamkhattar/homelab/hl/internal/ui"
)

// GoLinter lints Go code using go vet and fixes with go fmt.
// It discovers all Go modules (directories containing go.mod) in the repo.
type GoLinter struct{}

func (g *GoLinter) Name() string { return "go" }
func (g *GoLinter) Tool() string { return "go" }
func (g *GoLinter) CanFix() bool { return true }

func (g *GoLinter) Lint(root string) error {
	hasGolangciLint := false
	if _, err := exec.LookPath("golangci-lint"); err == nil {
		hasGolangciLint = true
	}

	return g.forEachModule(root, func(dir string) error {
		if hasGolangciLint {
			c := exec.Command("golangci-lint", "run", "./...")
			c.Dir = dir
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			return c.Run()
		}
		// Fallback to go vet when golangci-lint is not installed.
		ui.KeyValue("  note", "golangci-lint not found, falling back to go vet")
		c := exec.Command("go", "vet", "./...")
		c.Dir = dir
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		return c.Run()
	})
}

func (g *GoLinter) Fix(root string) error {
	return g.forEachModule(root, func(dir string) error {
		c := exec.Command("go", "fmt", "./...")
		c.Dir = dir
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		return c.Run()
	})
}

// forEachModule finds all directories containing a go.mod file under root
// and runs fn in each. Returns the first error encountered.
func (g *GoLinter) forEachModule(root string, fn func(dir string) error) error {
	modules, err := FindGoModules(root)
	if err != nil {
		return err
	}
	for _, dir := range modules {
		rel, _ := filepath.Rel(root, dir)
		ui.KeyValue("  module", rel)
		if err := fn(dir); err != nil {
			return fmt.Errorf("%s: %w", rel, err)
		}
	}
	return nil
}

// FindGoModules walks the repo and returns directories containing go.mod.
func FindGoModules(root string) ([]string, error) {
	var modules []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == "vendor" || d.Name() == ".git" || d.Name() == "node_modules") {
			return filepath.SkipDir
		}
		if !d.IsDir() && d.Name() == "go.mod" {
			modules = append(modules, filepath.Dir(path))
		}
		return nil
	})
	return modules, err
}
