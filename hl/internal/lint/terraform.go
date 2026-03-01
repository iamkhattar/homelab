package lint

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/iamkhattar/homelab/hl/internal/ui"
)

// TerraformLinter checks .tf files for formatting (native hclwrite) and
// runs terraform validate when the terraform CLI is available.
type TerraformLinter struct{}

func (t *TerraformLinter) Name() string { return "terraform" }
func (t *TerraformLinter) CanFix() bool { return true }

func (t *TerraformLinter) Lint(root string) error {
	dir := filepath.Join(root, "infra")
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil
	}

	// 1. Native HCL format check.
	if err := t.checkFormat(root, dir); err != nil {
		return err
	}

	// 2. terraform validate (requires terraform CLI + init).
	if err := t.validate(dir); err != nil {
		return err
	}

	return nil
}

// checkFormat uses hclwrite.Format to detect unformatted .tf files.
func (t *TerraformLinter) checkFormat(root, dir string) error {
	var unformatted []string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".tf") {
			return nil
		}

		src, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading %s: %w", path, err)
		}

		formatted := hclwrite.Format(src)
		if !bytes.Equal(src, formatted) {
			rel, _ := filepath.Rel(root, path)
			unformatted = append(unformatted, rel)
		}
		return nil
	})
	if err != nil {
		return err
	}

	if len(unformatted) > 0 {
		for _, f := range unformatted {
			ui.KeyValue("unformatted", f)
		}
return fmt.Errorf("%d file(s) need formatting — run hl ci fix", len(unformatted))
	}
	return nil
}

// validate runs terraform validate if the CLI is on PATH and the working
// directory has been initialised (.terraform/ exists).
func (t *TerraformLinter) validate(dir string) error {
	if _, err := exec.LookPath("terraform"); err != nil {
		ui.KeyValue("terraform validate", "skipped (terraform not found)")
		return nil
	}

	// Skip validate if terraform init hasn't been run yet.
	if _, err := os.Stat(filepath.Join(dir, ".terraform")); os.IsNotExist(err) {
		ui.KeyValue("terraform validate", "skipped (run terraform init first)")
		return nil
	}

	ui.Step("terraform validate")
	c := exec.Command("terraform", "validate")
	c.Dir = dir
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}

func (t *TerraformLinter) Fix(root string) error {
	dir := filepath.Join(root, "infra")
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil
	}

	var fixed int
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".tf") {
			return nil
		}

		src, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading %s: %w", path, err)
		}

		formatted := hclwrite.Format(src)
		if !bytes.Equal(src, formatted) {
			if err := os.WriteFile(path, formatted, 0644); err != nil {
				return fmt.Errorf("writing %s: %w", path, err)
			}
			rel, _ := filepath.Rel(root, path)
			ui.KeyValue("formatted", rel)
			fixed++
		}
		return nil
	})
	if err != nil {
		return err
	}

	if fixed > 0 {
		ui.KeyValue("files fixed", fmt.Sprintf("%d", fixed))
	}
	return nil
}
