package lint

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/iamkhattar/homelab/hl/internal/ui"
)

// AnsibleLinter runs ansible-lint if available, otherwise falls back to
// ansible-playbook --syntax-check on each playbook.
type AnsibleLinter struct{}

func (a *AnsibleLinter) Name() string { return "ansible" }
func (a *AnsibleLinter) CanFix() bool { return false }

func (a *AnsibleLinter) Fix(_ string) error { return nil }

func (a *AnsibleLinter) Lint(root string) error {
	dir := filepath.Join(root, "ansible")
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil
	}

	// Prefer ansible-lint if installed.
	if _, err := exec.LookPath("ansible-lint"); err == nil {
		ui.KeyValue("tool", "ansible-lint")
		c := exec.Command("ansible-lint")
		c.Dir = dir
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		return c.Run()
	}

	// Fall back to ansible-playbook --syntax-check on each playbook.
	if _, err := exec.LookPath("ansible-playbook"); err == nil {
		ui.KeyValue("tool", "ansible-playbook --syntax-check (ansible-lint not found)")
		return a.syntaxCheck(dir)
	}

	ui.StepFail("ansible (no ansible-lint or ansible-playbook found, skipping)")
	return nil
}

// syntaxCheck runs ansible-playbook --syntax-check against all playbooks found
// in the playbooks/ directory.
func (a *AnsibleLinter) syntaxCheck(dir string) error {
	playbookDir := filepath.Join(dir, "playbooks")
	if _, err := os.Stat(playbookDir); os.IsNotExist(err) {
		return nil
	}

	entries, err := os.ReadDir(playbookDir)
	if err != nil {
		return fmt.Errorf("reading playbooks dir: %w", err)
	}

	var failed []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := filepath.Ext(e.Name())
		if ext != ".yml" && ext != ".yaml" {
			continue
		}

		playbook := filepath.Join("playbooks", e.Name())
		c := exec.Command("ansible-playbook", "--syntax-check", playbook)
		c.Dir = dir
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		if err := c.Run(); err != nil {
			failed = append(failed, e.Name())
		}
	}

	if len(failed) > 0 {
		return fmt.Errorf("%d playbook(s) failed syntax check: %v", len(failed), failed)
	}
	return nil
}
