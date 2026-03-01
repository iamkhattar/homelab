package lint

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNames_ReturnsAllLinters(t *testing.T) {
	names := Names()
	expected := []string{"go", "terraform", "helm", "helmfile", "ansible", "yaml"}
	if len(names) != len(expected) {
		t.Fatalf("expected %d linters, got %d: %v", len(expected), len(names), names)
	}
	for i, name := range expected {
		if names[i] != name {
			t.Errorf("linter %d: expected %q, got %q", i, name, names[i])
		}
	}
}

func TestNewRunner_SkipNormalization(t *testing.T) {
	r := NewRunner([]string{" Go ", "YAML"})
	if !r.skip["go"] {
		t.Error("skip should contain 'go' (lowercased, trimmed)")
	}
	if !r.skip["yaml"] {
		t.Error("skip should contain 'yaml' (lowercased)")
	}
}

func TestNewRunner_EmptySkip(t *testing.T) {
	r := NewRunner(nil)
	if len(r.skip) != 0 {
		t.Errorf("expected empty skip map, got %v", r.skip)
	}
	if len(r.linters) == 0 {
		t.Error("runner should have linters registered")
	}
}

// TestYAMLLinter_ValidFiles tests that valid YAML passes.
func TestYAMLLinter_ValidFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "good.yaml"), []byte("key: value\n"), 0644)
	os.WriteFile(filepath.Join(dir, "also-good.yml"), []byte("items:\n  - one\n  - two\n"), 0644)

	y := &YAMLLinter{}
	if err := y.Lint(dir); err != nil {
		t.Errorf("expected no error for valid YAML, got: %v", err)
	}
}

// TestYAMLLinter_InvalidFiles tests that broken YAML is detected.
func TestYAMLLinter_InvalidFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "bad.yaml"), []byte("key: [unclosed\n"), 0644)

	y := &YAMLLinter{}
	if err := y.Lint(dir); err == nil {
		t.Error("expected error for invalid YAML, got nil")
	}
}

// TestYAMLLinter_SkipsDotGit tests that .git directories are skipped.
func TestYAMLLinter_SkipsDotGit(t *testing.T) {
	dir := t.TempDir()
	gitDir := filepath.Join(dir, ".git")
	os.MkdirAll(gitDir, 0755)
	os.WriteFile(filepath.Join(gitDir, "bad.yaml"), []byte("key: [unclosed\n"), 0644)
	os.WriteFile(filepath.Join(dir, "good.yaml"), []byte("key: value\n"), 0644)

	y := &YAMLLinter{}
	if err := y.Lint(dir); err != nil {
		t.Errorf("should skip .git dir, got: %v", err)
	}
}

// TestYAMLLinter_EmptyDir tests that an empty directory passes.
func TestYAMLLinter_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	y := &YAMLLinter{}
	if err := y.Lint(dir); err != nil {
		t.Errorf("empty dir should pass, got: %v", err)
	}
}

// TestTerraformLinter_FormattedFiles tests that properly formatted HCL passes.
func TestTerraformLinter_FormattedFiles(t *testing.T) {
	dir := t.TempDir()
	infraDir := filepath.Join(dir, "infra")
	os.MkdirAll(infraDir, 0755)
	os.WriteFile(filepath.Join(infraDir, "main.tf"), []byte(`variable "name" {
  type    = string
  default = "test"
}
`), 0644)

	tf := &TerraformLinter{}
	if err := tf.Lint(dir); err != nil {
		t.Errorf("expected no error for formatted HCL, got: %v", err)
	}
}

// TestTerraformLinter_UnformattedFiles tests that poorly formatted HCL is caught.
func TestTerraformLinter_UnformattedFiles(t *testing.T) {
	dir := t.TempDir()
	infraDir := filepath.Join(dir, "infra")
	os.MkdirAll(infraDir, 0755)
	// Intentionally bad formatting: extra spaces.
	os.WriteFile(filepath.Join(infraDir, "main.tf"), []byte(`variable "name" {
  type =     string
  default =     "test"
}
`), 0644)

	tf := &TerraformLinter{}
	if err := tf.Lint(dir); err == nil {
		t.Error("expected error for unformatted HCL, got nil")
	}
}

// TestTerraformLinter_Fix tests that fix rewrites files.
func TestTerraformLinter_Fix(t *testing.T) {
	dir := t.TempDir()
	infraDir := filepath.Join(dir, "infra")
	os.MkdirAll(infraDir, 0755)
	file := filepath.Join(infraDir, "main.tf")
	os.WriteFile(file, []byte(`variable "name" {
  type =     string
  default =     "test"
}
`), 0644)

	tf := &TerraformLinter{}
	if err := tf.Fix(dir); err != nil {
		t.Fatalf("fix failed: %v", err)
	}

	// After fix, lint should pass.
	if err := tf.Lint(dir); err != nil {
		t.Errorf("lint should pass after fix, got: %v", err)
	}
}

// TestTerraformLinter_NoInfraDir tests that missing infra/ is a no-op.
func TestTerraformLinter_NoInfraDir(t *testing.T) {
	dir := t.TempDir()
	tf := &TerraformLinter{}
	if err := tf.Lint(dir); err != nil {
		t.Errorf("missing infra dir should be no-op, got: %v", err)
	}
	if err := tf.Fix(dir); err != nil {
		t.Errorf("missing infra dir fix should be no-op, got: %v", err)
	}
}

// TestTerraformLinter_ValidateSkipsWithoutInit tests that terraform validate
// is gracefully skipped when .terraform/ doesn't exist.
func TestTerraformLinter_ValidateSkipsWithoutInit(t *testing.T) {
	dir := t.TempDir()
	infraDir := filepath.Join(dir, "infra")
	os.MkdirAll(infraDir, 0755)
	// Well-formatted file, but no .terraform/ directory.
	os.WriteFile(filepath.Join(infraDir, "main.tf"), []byte(`variable "name" {
  type    = string
  default = "test"
}
`), 0644)

	tf := &TerraformLinter{}
	// Should pass: format is correct, validate is skipped.
	if err := tf.Lint(dir); err != nil {
		t.Errorf("should pass when .terraform doesn't exist, got: %v", err)
	}
}

// TestGoLinter_Properties tests basic linter metadata.
func TestGoLinter_Properties(t *testing.T) {
	g := &GoLinter{}
	if g.Name() != "go" {
		t.Errorf("expected name 'go', got '%s'", g.Name())
	}
	if !g.CanFix() {
		t.Error("go linter should support fix")
	}
	if g.Tool() != "go" {
		t.Errorf("expected tool 'go', got '%s'", g.Tool())
	}
}

// TestHelmLinter_Properties tests basic linter metadata.
func TestHelmLinter_Properties(t *testing.T) {
	h := &HelmLinter{}
	if h.Name() != "helm" {
		t.Errorf("expected name 'helm', got '%s'", h.Name())
	}
	if h.CanFix() {
		t.Error("helm linter should not support fix")
	}
}

// TestHelmfileLinter_Properties tests basic linter metadata.
func TestHelmfileLinter_Properties(t *testing.T) {
	h := &HelmfileLinter{}
	if h.Name() != "helmfile" {
		t.Errorf("expected name 'helmfile', got '%s'", h.Name())
	}
	if h.CanFix() {
		t.Error("helmfile linter should not support fix")
	}
}

// TestAnsibleLinter_Properties tests basic linter metadata.
func TestAnsibleLinter_Properties(t *testing.T) {
	a := &AnsibleLinter{}
	if a.Name() != "ansible" {
		t.Errorf("expected name 'ansible', got '%s'", a.Name())
	}
	if a.CanFix() {
		t.Error("ansible linter should not support fix")
	}
}

// TestAnsibleLinter_NoAnsibleDir tests that missing ansible/ is a no-op.
func TestAnsibleLinter_NoAnsibleDir(t *testing.T) {
	dir := t.TempDir()
	a := &AnsibleLinter{}
	if err := a.Lint(dir); err != nil {
		t.Errorf("missing ansible dir should be no-op, got: %v", err)
	}
}

// TestAnsibleLinter_IsAlwaysAvailable tests that ansible linter handles its own
// tool detection (no Tool() method), so toolAvailable always returns true.
func TestAnsibleLinter_IsAlwaysAvailable(t *testing.T) {
	a := &AnsibleLinter{}
	if !toolAvailable(a) {
		t.Error("ansible linter should pass toolAvailable (handles detection internally)")
	}
}

// TestFindCharts discovers Chart.yaml files.
func TestFindCharts(t *testing.T) {
	dir := t.TempDir()
	chart1 := filepath.Join(dir, "core", "cert-manager")
	chart2 := filepath.Join(dir, "apps", "vault")
	os.MkdirAll(chart1, 0755)
	os.MkdirAll(chart2, 0755)
	os.WriteFile(filepath.Join(chart1, "Chart.yaml"), []byte("name: cert-manager"), 0644)
	os.WriteFile(filepath.Join(chart2, "Chart.yaml"), []byte("name: vault"), 0644)

	charts, err := findCharts(dir)
	if err != nil {
		t.Fatalf("findCharts failed: %v", err)
	}
	if len(charts) != 2 {
		t.Errorf("expected 2 charts, got %d", len(charts))
	}
}

// TestFindCharts_Empty returns empty for no charts.
func TestFindCharts_Empty(t *testing.T) {
	dir := t.TempDir()
	charts, err := findCharts(dir)
	if err != nil {
		t.Fatalf("findCharts failed: %v", err)
	}
	if len(charts) != 0 {
		t.Errorf("expected 0 charts, got %d", len(charts))
	}
}

// TestToolAvailable_PureGo tests that pure-Go linters are always available.
func TestToolAvailable_PureGo(t *testing.T) {
	y := &YAMLLinter{}
	if !toolAvailable(y) {
		t.Error("YAML linter (pure-Go) should always be available")
	}
	tf := &TerraformLinter{}
	if !toolAvailable(tf) {
		t.Error("Terraform linter (pure-Go) should always be available")
	}
}
