package lint

import (
	"os"
	"os/exec"
)

// GitleaksLinter scans the repository for hardcoded secrets using gitleaks.
type GitleaksLinter struct{}

func (g *GitleaksLinter) Name() string { return "gitleaks" }
func (g *GitleaksLinter) Tool() string { return "gitleaks" }
func (g *GitleaksLinter) CanFix() bool { return false }

func (g *GitleaksLinter) Fix(_ string) error { return nil }

func (g *GitleaksLinter) Lint(root string) error {
	c := exec.Command("gitleaks", "dir", root)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}
