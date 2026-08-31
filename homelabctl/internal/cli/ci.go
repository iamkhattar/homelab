package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/iamkhattar/homelab/homelabctl/internal/repository"
	"github.com/iamkhattar/homelab/homelabctl/internal/ui"
	"github.com/iamkhattar/homelab/homelabctl/internal/workflow"
)

type checkStep struct {
	name string
	run  func(*cobra.Command) error
}

func newCICommand(s *state) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ci",
		Short: "Run the same repository checks used by GitHub Actions",
	}

	var skip []string
	var only []string
	var reports bool
	checkCmd := &cobra.Command{
		Use:   "check",
		Short: "Run formatting, tests, docs, workflow, Ansible and Terraform checks",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s.heading("Repository checks")
			if len(skip) > 0 && len(only) > 0 {
				return fmt.Errorf("--only and --skip cannot be used together")
			}
			skipped := make(map[string]bool, len(skip))
			for _, name := range skip {
				skipped[strings.TrimSpace(name)] = true
			}

			steps := []checkStep{
				{name: "go-format", run: func(cmd *cobra.Command) error { return checkGoFormat(cmd, s) }},
				{name: "go-test", run: func(cmd *cobra.Command) error { return checkGoTests(cmd, s, reports) }},
				{name: "docs", run: func(cmd *cobra.Command) error { return s.run(cmd.Context(), s.dir("docs"), "npm", "run", "build") }},
				{name: "workflows", run: func(_ *cobra.Command) error { return workflow.ValidateDirectory(s.root) }},
				{name: "ansible", run: func(cmd *cobra.Command) error { return checkAnsible(cmd, s) }},
				{name: "terraform", run: func(cmd *cobra.Command) error { return checkTerraform(cmd, s) }},
			}
			if reports {
				steps = append(steps,
					checkStep{name: "gosec", run: func(cmd *cobra.Command) error { return generateGoSecurityReports(cmd, s) }},
					checkStep{name: "trivy", run: func(cmd *cobra.Command) error { return generateTrivySecurityReport(cmd, s) }},
					checkStep{name: "sbom", run: func(cmd *cobra.Command) error { return generateSBOM(cmd, s) }},
				)
			}

			known := map[string]bool{}
			for _, step := range steps {
				known[step.name] = true
			}
			for name := range skipped {
				if !known[name] {
					return fmt.Errorf("unknown check in --skip: %s", name)
				}
			}
			selected := make(map[string]bool, len(only))
			for _, name := range only {
				name = strings.TrimSpace(name)
				if !known[name] {
					return fmt.Errorf("unknown check in --only: %s", name)
				}
				selected[name] = true
			}

			var failed []string
			for _, step := range steps {
				if skipped[step.name] || (len(selected) > 0 && !selected[step.name]) {
					s.status(ui.Skipped, "skip", step.name)
					continue
				}
				s.status(ui.Running, "run", step.name)
				if err := step.run(cmd); err != nil {
					s.status(ui.Failure, "fail", fmt.Sprintf("%s: %v", step.name, err))
					failed = append(failed, step.name)
					continue
				}
				s.status(ui.Success, "pass", step.name)
			}
			if len(failed) > 0 {
				return fmt.Errorf("checks failed: %s", strings.Join(failed, ", "))
			}
			return nil
		},
	}
	checkCmd.Flags().StringSliceVar(&skip, "skip", nil, "checks to skip; reporting mode also adds gosec,trivy,sbom")
	checkCmd.Flags().StringSliceVar(&only, "only", nil, "run only selected checks; reporting mode also adds gosec,trivy,sbom")
	checkCmd.Flags().BoolVar(&reports, "reports", false, "write JUnit, test JSON, SARIF and SPDX reports and run security scans")
	cmd.AddCommand(checkCmd, newCIImageCommand(s, false), newCIImageCommand(s, true))
	return cmd
}

func newCIImageCommand(s *state, publish bool) *cobra.Command {
	verb := "build"
	short := "Build all repository container images"
	if publish {
		verb = "publish"
		short = "Build and publish all repository container images"
	}

	var tags []string
	var registry string
	var docsImage string
	var homelabctlImage string
	var changed bool
	var base string
	cmd := &cobra.Command{
		Use:   verb,
		Short: short,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if publish {
				if os.Getenv("CI") == "" {
					return fmt.Errorf("ci publish is only allowed when CI is set")
				}
			}
			if err := validateContainerValue(registry, "registry namespace"); err != nil {
				return err
			}
			if err := validateContainerValue(docsImage, "documentation image"); err != nil {
				return err
			}
			if err := validateContainerValue(homelabctlImage, "homelabctl image"); err != nil {
				return err
			}
			if err := validateTags(tags); err != nil {
				return err
			}
			if changed && strings.TrimSpace(base) == "" {
				return fmt.Errorf("--base is required with --changed")
			}
			resolvedTags, err := resolveImageTags(cmd.Context(), s, tags)
			if err != nil {
				return err
			}
			if err := buildServices(cmd, s, nil, serviceBuildOptions{
				tags: resolvedTags, registry: registry, push: publish, changed: changed, base: base,
			}); err != nil {
				return err
			}
			if err := buildHomelabctl(cmd.Context(), s, homelabctlBuildOptions{
				tags: resolvedTags, image: homelabctlImage, push: publish,
			}); err != nil {
				return err
			}
			return buildDocs(cmd.Context(), s, docsBuildOptions{tags: resolvedTags, image: docsImage, push: publish})
		},
	}
	cmd.Flags().StringSliceVar(&tags, "tag", nil, "image tag; repeat for multiple tags; first tag is the shared build version (default: current Git SHA)")
	cmd.Flags().StringVar(&registry, "registry", "iamkhattar", "service image registry namespace")
	cmd.Flags().StringVar(&docsImage, "docs-image", "iamkhattar/homelab-docs", "documentation image name")
	cmd.Flags().StringVar(&homelabctlImage, "homelabctl-image", "iamkhattar/homelabctl", "homelabctl image name")
	cmd.Flags().BoolVar(&changed, "changed", false, "build only services changed from --base; homelabctl and docs are always built")
	cmd.Flags().StringVar(&base, "base", "", "Git base revision used with --changed")
	return cmd
}

func checkGoFormat(cmd *cobra.Command, s *state) error {
	files, err := repository.GoFiles(s.root)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return nil
	}
	args := append([]string{"-l"}, files...)
	out, err := s.output(cmd.Context(), s.root, "gofmt", args...)
	if err != nil {
		return err
	}
	if out != "" {
		lines := strings.Split(out, "\n")
		sort.Strings(lines)
		return fmt.Errorf("Go files need formatting:\n%s", strings.Join(lines, "\n"))
	}
	return nil
}

func checkGoTests(cmd *cobra.Command, s *state, reports bool) error {
	modules, err := repository.GoModules(s.root)
	if err != nil {
		return err
	}
	if reports {
		return generateGoTestReports(cmd, s, modules)
	}
	for _, module := range modules {
		if err := s.run(cmd.Context(), module, "go", "test", "./..."); err != nil {
			return err
		}
	}
	return nil
}

func checkAnsible(cmd *cobra.Command, s *state) error {
	ansibleDir := s.dir("ansible")
	lint := ansibleExecutable(ansibleDir, "ansible-lint")
	playbookCommand := ansibleExecutable(ansibleDir, "ansible-playbook")
	environment := map[string]string{"ANSIBLE_HOME": filepath.Join(ansibleDir, ".ansible")}
	if err := s.runEnv(cmd.Context(), ansibleDir, environment, lint, "--offline", "."); err != nil {
		return err
	}
	for _, playbook := range []string{"prepare.yml", "site.yml", "upgrade.yml", "reboot.yml", "reboot-node.yml", "diagnose-node.yml", "diagnose-cluster.yml", "snapshot.yml", "recovery-export.yml"} {
		if err := s.runEnv(cmd.Context(), ansibleDir, environment, playbookCommand, "--syntax-check", filepath.Join("playbooks", playbook), "-i", "inventory/hosts.example.yml"); err != nil {
			return err
		}
	}
	if err := s.runEnv(cmd.Context(), ansibleDir, environment, playbookCommand, filepath.Join("tests", "homelab-base-fstab.yml"), "-i", "localhost,"); err != nil {
		return err
	}
	return nil
}

func checkTerraform(cmd *cobra.Command, s *state) error {
	infraDir := s.dir("infra")
	commands := [][]string{
		{"fmt", "-check", "-recursive"},
		{"init", "-backend=false", "-input=false"},
		{"validate"},
		{"test"},
	}
	for _, args := range commands {
		if err := s.run(cmd.Context(), infraDir, "terraform", args...); err != nil {
			return err
		}
	}
	return nil
}
