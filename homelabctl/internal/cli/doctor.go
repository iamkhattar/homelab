package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/iamkhattar/homelab/homelabctl/internal/ui"
)

func newDoctorCommand(s *state) *cobra.Command {
	var strict bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check the repository and local operator toolchain",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s.heading("Operator environment")
			tools := []string{"go", "node", "npm", "ssh", "ssh-copy-id", "ansible-playbook", "ansible-lint", "kubectl", "helm", "helmfile", "terraform", "docker"}
			missing := 0
			for _, tool := range tools {
				path, err := s.runner.LookPath(tool)
				if err != nil {
					s.status(ui.Failure, "missing", tool)
					missing++
					continue
				}
				if tool == "go" {
					version, err := s.output(cmd.Context(), s.root, path, "version")
					if err != nil || !goVersionSupported(version) {
						s.status(ui.Warning, "old", fmt.Sprintf("%-20s %s (need >=1.27)", tool, version))
						missing++
						continue
					}
					s.status(ui.Success, "ok", fmt.Sprintf("%-20s %s (%s)", tool, path, version))
					continue
				}
				if tool == "node" {
					version, err := s.output(cmd.Context(), s.root, path, "--version")
					if err != nil || !nodeVersionSupported(version) {
						s.status(ui.Warning, "old", fmt.Sprintf("%-20s %s (need >=24)", tool, version))
						missing++
						continue
					}
					s.status(ui.Success, "ok", fmt.Sprintf("%-20s %s (%s)", tool, path, version))
					continue
				}
				s.status(ui.Success, "ok", fmt.Sprintf("%-20s %s", tool, path))
			}

			files := []string{
				"ansible/inventory/hosts.yml",
				"ansible/playbooks/site.yml",
				"cluster/helmfile.yaml",
				"infra/main.tf",
				"docs/package-lock.json",
			}
			for _, file := range files {
				if _, err := os.Stat(filepath.Join(s.root, file)); err != nil {
					s.status(ui.Failure, "missing", file)
					missing++
					continue
				}
				s.status(ui.Success, "ok", file)
			}

			if strict && missing > 0 {
				return fmt.Errorf("doctor found %d missing requirement(s)", missing)
			}
			if missing > 0 {
				s.warning(fmt.Sprintf("%d optional or required item(s) are missing; use --strict to fail", missing))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&strict, "strict", false, "return a failure when any check is missing")
	return cmd
}

func nodeVersionSupported(version string) bool {
	parts := strings.Split(strings.TrimPrefix(strings.TrimSpace(version), "v"), ".")
	if len(parts) < 1 {
		return false
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return false
	}
	return major >= 24
}

func goVersionSupported(version string) bool {
	for _, field := range strings.Fields(version) {
		if !strings.HasPrefix(field, "go1.") {
			continue
		}
		parts := strings.Split(strings.TrimPrefix(field, "go"), ".")
		if len(parts) < 2 {
			return false
		}
		major, majorErr := strconv.Atoi(parts[0])
		minor, minorErr := strconv.Atoi(parts[1])
		return majorErr == nil && minorErr == nil && (major > 1 || major == 1 && minor >= 27)
	}
	return false
}
