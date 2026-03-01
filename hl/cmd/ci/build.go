package ci

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/iamkhattar/homelab/hl/internal/lint"
	"github.com/iamkhattar/homelab/hl/internal/ui"
)

var skipLinters []string

var (
	tags    []string
	registry string
	push     bool
	changed  bool
	base     string
)

var Cmd = &cobra.Command{
	Use:   "ci",
	Short: "CI/dev tooling — lint, test, fix, check, docker",
	Long:  "Commands for linting, testing, fixing, running checks, and building container images. Designed to be called both locally and from CI pipelines.",
}

var lintCmd = &cobra.Command{
	Use:   "lint",
	Short: "Run all linters",
	Long: fmt.Sprintf(`Run all configured linters against the codebase.

Linters: %s

Use --skip to exclude specific linters by name.`, strings.Join(lint.Names(), ", ")),
	Example: `  hl ci lint
  hl ci lint --skip helm,helmfile
  hl ci lint --skip ansible`,
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := lint.RepoRoot()
		if err != nil {
			return err
		}
		ui.Step("Running linters")
		fmt.Println()
		runner := lint.NewRunner(skipLinters)
		if err := runner.Lint(root); err != nil {
			fmt.Println()
			ui.StepFail("Linting failed")
			return err
		}
		fmt.Println()
		ui.StepDone(ui.SuccessStyle.Render("All linters passed"))
		return nil
	},
}

var fixCmd = &cobra.Command{
	Use:   "fix",
	Short: "Auto-fix linting issues where possible",
	Long: fmt.Sprintf(`Auto-fix issues detected by linters that support it.

Fixable linters: go (go fmt), terraform (hcl format)

Use --skip to exclude specific fixers by name. Available: %s`, strings.Join(lint.Names(), ", ")),
	Example: `  hl ci fix
  hl ci fix --skip terraform`,
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := lint.RepoRoot()
		if err != nil {
			return err
		}
		ui.Step("Auto-fixing")
		fmt.Println()
		runner := lint.NewRunner(skipLinters)
		if err := runner.Fix(root); err != nil {
			fmt.Println()
			ui.StepFail("Fix failed")
			return err
		}
		fmt.Println()
		ui.StepDone(ui.SuccessStyle.Render("All fixes applied"))
		return nil
	},
}

var testCmd = &cobra.Command{
	Use:   "test",
	Short: "Run all tests (Go + Terraform)",
	Long: `Run all test suites across the repository.

Suites: go (go test ./... per module), terraform (terraform test in infra/)`,
	Example: "  hl ci test",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Go tests — run per module.
		ui.Step("Go tests")
		if err := runGoTests(); err != nil {
			ui.StepFail("Go tests failed")
			return err
		}
		ui.StepDone("Go tests passed")

		// Terraform tests.
		if err := runTerraformTests(); err != nil {
			return err
		}

		return nil
	},
}

// testResultsDir returns the directory for JUnit XML test results.
// In CI it uses $GITHUB_WORKSPACE/test-results, locally it uses <root>/test-results.
func testResultsDir(root string) string {
	if ws := os.Getenv("GITHUB_WORKSPACE"); ws != "" {
		return filepath.Join(ws, "test-results")
	}
	return filepath.Join(root, "test-results")
}

// runGoTests discovers Go modules and runs tests in each.
// When gotestsum is available, it produces JUnit XML reports.
func runGoTests() error {
	root, err := lint.RepoRoot()
	if err != nil {
		return err
	}
	modules, err := lint.FindGoModules(root)
	if err != nil {
		return err
	}

	useGotestsum := false
	if _, err := exec.LookPath("gotestsum"); err == nil {
		useGotestsum = true
		resultsDir := testResultsDir(root)
		if err := os.MkdirAll(resultsDir, 0755); err != nil {
			return fmt.Errorf("creating test-results dir: %w", err)
		}
	}

	for _, dir := range modules {
		rel, _ := filepath.Rel(root, dir)
		ui.KeyValue("  module", rel)

		var c *exec.Cmd
		if useGotestsum {
			junitFile := filepath.Join(testResultsDir(root), strings.ReplaceAll(rel, "/", "-")+".xml")
			c = exec.Command("gotestsum", "--junitfile", junitFile, "--", "./...")
		} else {
			c = exec.Command("go", "test", "./...")
		}
		c.Dir = dir
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		if err := c.Run(); err != nil {
			return fmt.Errorf("%s: %w", rel, err)
		}
	}
	return nil
}

// runTerraformTests runs terraform test in the infra/ directory if
// .tftest.hcl files exist and the terraform CLI is available.
func runTerraformTests() error {
	root, err := lint.RepoRoot()
	if err != nil {
		return err
	}

	infraDir := filepath.Join(root, "infra")
	if _, err := os.Stat(infraDir); os.IsNotExist(err) {
		return nil
	}

	// Check for .tftest.hcl files.
	matches, _ := filepath.Glob(filepath.Join(infraDir, "*.tftest.hcl"))
	if len(matches) == 0 {
		ui.KeyValue("terraform test", "no .tftest.hcl files found, skipping")
		return nil
	}

	if _, err := exec.LookPath("terraform"); err != nil {
		ui.KeyValue("terraform test", "skipped (terraform not found)")
		return nil
	}

	if err := lint.EnsureTerraformInit(infraDir); err != nil {
		return err
	}

	ui.Step(fmt.Sprintf("Terraform tests (%d test file(s))", len(matches)))
	c := exec.Command("terraform", "test")
	c.Dir = infraDir
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		ui.StepFail("Terraform tests failed")
		return err
	}
	ui.StepDone("Terraform tests passed")
	return nil
}

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Run all checks (lint + test) — single CI entry point",
	Long: `Run lint and test sequentially. Exits on the first failure.
This is the intended single entry point for CI pipelines.`,
	Example: `  hl ci check
  hl ci check --skip ansible,helmfile`,
	RunE: func(cmd *cobra.Command, args []string) error {
		steps := []struct {
			name string
			fn   func(*cobra.Command, []string) error
		}{
			{"lint", lintCmd.RunE},
			{"test", testCmd.RunE},
		}
		for _, step := range steps {
			if err := step.fn(cmd, nil); err != nil {
				return fmt.Errorf("%s failed: %w", step.name, err)
			}
		}
		fmt.Println()
		ui.StepDone(ui.SuccessStyle.Render("All checks passed"))
		return nil
	},
}

var dockerCmd = &cobra.Command{
	Use:   "docker [service...]",
	Short: "Build (and optionally push) container images for services",
	Long: `Build Docker images for services under services/.

If no service names are given, all services with a Dockerfile are built.
Use --changed to only build services that have changed relative to --base ref.
Use --push to push images after building (intended for CI only).
Requires the CI environment variable to be set when using --push.`,
	Example: `  hl ci docker butler
  hl ci docker --tag v1.0.0
  hl ci docker --tag latest --tag abc1234
  hl ci docker --changed                     # only services changed vs origin/main
  hl ci docker --changed --base HEAD~1        # changed vs previous commit
  hl ci docker --push --changed --tag latest  # CI only`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if push && os.Getenv("CI") == "" {
			return fmt.Errorf("--push is only allowed in CI environments (CI env var not set)")
		}
		if changed && len(args) > 0 {
			return fmt.Errorf("--changed and explicit service names are mutually exclusive")
		}

		root, err := lint.RepoRoot()
		if err != nil {
			return err
		}

		// When --changed is set, detect services via git diff.
		if changed {
			names, err := changedServices(root, base)
			if err != nil {
				return fmt.Errorf("detecting changed services: %w", err)
			}
			args = names
		}

		services, err := discoverServices(root, args)
		if err != nil {
			return err
		}

		if len(services) == 0 {
			ui.KeyValue("docker", "no services found")
			return nil
		}

	for _, svc := range services {
			// Build docker build args with a -t flag per tag.
			buildArgs := []string{"build"}
			var images []string
			for _, t := range tags {
				img := fmt.Sprintf("%s/%s:%s", registry, svc.name, t)
				buildArgs = append(buildArgs, "-t", img)
				images = append(images, img)
			}
			buildArgs = append(buildArgs, ".")

			ui.Step(fmt.Sprintf("Building %s (%s)", svc.name, strings.Join(tags, ", ")))

			c := exec.Command("docker", buildArgs...)
			c.Dir = svc.dir
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			if err := c.Run(); err != nil {
				ui.StepFail(fmt.Sprintf("Build failed: %s", svc.name))
				return err
			}
			ui.StepDone(fmt.Sprintf("Built %s", svc.name))

			if push {
				for _, img := range images {
					ui.Step(fmt.Sprintf("Pushing %s", img))
					p := exec.Command("docker", "push", img)
					p.Stdout = os.Stdout
					p.Stderr = os.Stderr
					if err := p.Run(); err != nil {
						ui.StepFail(fmt.Sprintf("Push failed: %s", img))
						return err
					}
					ui.StepDone(fmt.Sprintf("Pushed %s", img))
				}
			}
		}

		return nil
	},
}

type service struct {
	name string
	dir  string
}

// changedServices returns service names that have changes relative to the
// given git base ref by inspecting git diff output for the services/ directory.
func changedServices(root, baseRef string) ([]string, error) {
	c := exec.Command("git", "diff", "--name-only", baseRef, "--", "services/")
	c.Dir = root
	out, err := c.Output()
	if err != nil {
		return nil, fmt.Errorf("git diff failed: %w", err)
	}

	seen := make(map[string]bool)
	var names []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		// lines are like "services/butler/main.go" — extract second segment.
		parts := strings.SplitN(line, "/", 3)
		if len(parts) < 2 {
			continue
		}
		name := parts[1]
		if seen[name] {
			continue
		}
		// Only include if it has a Dockerfile.
		if _, err := os.Stat(filepath.Join(root, "services", name, "Dockerfile")); err != nil {
			continue
		}
		seen[name] = true
		names = append(names, name)
	}

	if len(names) == 0 {
		ui.KeyValue("docker", "no changed services detected")
	} else {
		ui.KeyValue("docker", fmt.Sprintf("changed services: %s", strings.Join(names, ", ")))
	}
	return names, nil
}

// discoverServices finds services under services/ that have a Dockerfile.
// If filter is non-empty, only matching service names are returned.
func discoverServices(root string, filter []string) ([]service, error) {
	servicesDir := filepath.Join(root, "services")
	entries, err := os.ReadDir(servicesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	want := make(map[string]bool, len(filter))
	for _, f := range filter {
		want[f] = true
	}

	var svcs []service
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(servicesDir, e.Name())
		if _, err := os.Stat(filepath.Join(dir, "Dockerfile")); err != nil {
			continue
		}
		if len(want) > 0 && !want[e.Name()] {
			continue
		}
		svcs = append(svcs, service{name: e.Name(), dir: dir})
	}

	// Check for names that didn't match.
	for _, f := range filter {
		found := false
		for _, s := range svcs {
			if s.name == f {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("service %q not found or has no Dockerfile", f)
		}
	}

	return svcs, nil
}

func init() {
	// --skip flag shared by lint, fix, and check.
	for _, cmd := range []*cobra.Command{lintCmd, fixCmd, checkCmd} {
		cmd.Flags().StringSliceVar(&skipLinters, "skip", nil,
			fmt.Sprintf("Linters to skip (comma-separated: %s)", strings.Join(lint.Names(), ",")))
	}

	// docker flags.
	dockerCmd.Flags().StringSliceVar(&tags, "tag", []string{"latest"}, "Image tags (can be specified multiple times)")
	dockerCmd.Flags().StringVar(&registry, "registry", "iamkhattar", "Container registry prefix")
	dockerCmd.Flags().BoolVar(&push, "push", false, "Push images after building (CI only)")
	dockerCmd.Flags().BoolVar(&changed, "changed", false, "Only build services changed relative to --base")
	dockerCmd.Flags().StringVar(&base, "base", "origin/main", "Git ref to diff against (used with --changed)")

	Cmd.AddCommand(setupCmd)
	Cmd.AddCommand(lintCmd)
	Cmd.AddCommand(fixCmd)
	Cmd.AddCommand(testCmd)
	Cmd.AddCommand(checkCmd)
	Cmd.AddCommand(dockerCmd)
}
