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

// EnsureTerraformInit runs terraform init -backend=false if the .terraform/
// directory doesn't exist. This downloads providers without configuring
// the remote backend (which requires credentials).
func EnsureTerraformInit(dir string) error {
	// Set dummy provider variables so init/validate/test don't fail on
	// required variables that are only needed for real applies.
	setDummyTerraformVars()

	if _, err := os.Stat(filepath.Join(dir, ".terraform")); err == nil {
		return nil
	}

	ui.Step("terraform init -backend=false")
	c := exec.Command("terraform", "init", "-backend=false")
	c.Dir = dir
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		return fmt.Errorf("terraform init: %w", err)
	}
	return nil
}

// setDummyTerraformVars sets placeholder values for required Terraform
// variables when they aren't already set. This allows init, validate,
// and test to run without real credentials.
func setDummyTerraformVars() {
	dummyVars := map[string]string{
		"TF_VAR_hetzner_cloud_api_token": strings.Repeat("0", 64),
		"TF_VAR_ssh_public_key":          "ssh-ed25519 AAAA placeholder",
	}
	for k, v := range dummyVars {
		if os.Getenv(k) == "" {
			_ = os.Setenv(k, v)
		}
	}
}

// validate runs terraform validate if the CLI is on PATH, initialising
// providers first if needed.
func (t *TerraformLinter) validate(dir string) error {
	if _, err := exec.LookPath("terraform"); err != nil {
		ui.KeyValue("terraform validate", "skipped (terraform not found)")
		return nil
	}

	if err := EnsureTerraformInit(dir); err != nil {
		return err
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
