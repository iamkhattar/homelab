package lint

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/iamkhattar/homelab/hl/internal/ui"
)

// Linter is an interface for a single lint target.
type Linter interface {
	// Name returns the human-readable name used in --skip and output.
	Name() string
	// Lint checks for issues and returns an error if any are found.
	Lint(root string) error
	// Fix auto-corrects issues where possible. Returns nil if nothing to fix
	// or fix is not supported.
	Fix(root string) error
	// CanFix reports whether this linter supports auto-fixing.
	CanFix() bool
}

// Runner orchestrates multiple linters.
type Runner struct {
	linters []Linter
	skip    map[string]bool
}

// NewRunner creates a runner with all registered linters.
func NewRunner(skip []string) *Runner {
	skipMap := make(map[string]bool, len(skip))
	for _, s := range skip {
		skipMap[strings.ToLower(strings.TrimSpace(s))] = true
	}
	return &Runner{
		linters: allLinters(),
		skip:    skipMap,
	}
}

// allLinters returns all available linters in order.
func allLinters() []Linter {
	return []Linter{
		&GoLinter{},
		&TerraformLinter{},
		&HelmLinter{},
		&HelmfileLinter{},
		&AnsibleLinter{},
		&YAMLLinter{},
	}
}

// Names returns the names of all registered linters.
func Names() []string {
	var names []string
	for _, l := range allLinters() {
		names = append(names, l.Name())
	}
	return names
}

// Lint runs all non-skipped linters and collects errors.
func (r *Runner) Lint(root string) error {
	var failed []string
	for _, l := range r.linters {
		if r.skip[l.Name()] {
			ui.SubtleStyle.Render(fmt.Sprintf("  skipping %s", l.Name()))
			continue
		}
		if !toolAvailable(l) {
			ui.StepFail(fmt.Sprintf("%s (tool not found, skipping)", l.Name()))
			continue
		}
		ui.Step(l.Name())
		if err := l.Lint(root); err != nil {
			ui.StepFail(fmt.Sprintf("%s failed", l.Name()))
			failed = append(failed, l.Name())
			continue
		}
		ui.StepDone(l.Name())
	}
	if len(failed) > 0 {
		return fmt.Errorf("linting failed: %s", strings.Join(failed, ", "))
	}
	return nil
}

// Fix runs all non-skipped linters that support auto-fix.
func (r *Runner) Fix(root string) error {
	var failed []string
	for _, l := range r.linters {
		if r.skip[l.Name()] {
			continue
		}
		if !l.CanFix() {
			continue
		}
		if !toolAvailable(l) {
			ui.StepFail(fmt.Sprintf("%s (tool not found, skipping)", l.Name()))
			continue
		}
		ui.Step(fmt.Sprintf("Fixing %s", l.Name()))
		if err := l.Fix(root); err != nil {
			ui.StepFail(fmt.Sprintf("%s fix failed", l.Name()))
			failed = append(failed, l.Name())
			continue
		}
		ui.StepDone(fmt.Sprintf("%s fixed", l.Name()))
	}
	if len(failed) > 0 {
		return fmt.Errorf("fix failed: %s", strings.Join(failed, ", "))
	}
	return nil
}

// toolAvailable checks if the external tool required by a linter is on PATH.
// Linters that are pure-Go always return true.
func toolAvailable(l Linter) bool {
	type checker interface {
		Tool() string
	}
	c, ok := l.(checker)
	if !ok {
		// Pure-Go linter, always available.
		return true
	}
	_, err := exec.LookPath(c.Tool())
	return err == nil
}

// RepoRoot returns the root of the git repository, or an error.
func RepoRoot() (string, error) {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", fmt.Errorf("failed to detect repo root: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}
