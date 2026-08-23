package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

const (
	gotestsumVersion = "v1.13.0"
	gosecVersion     = "v2.28.0"
)

func newSetupCommand(s *state) *cobra.Command {
	var resetAnsible bool
	var uninstallAnsible bool
	cmd := &cobra.Command{
		Use:   "setup [all|ansible|docs|reports]",
		Short: "Install pinned workstation dependencies",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := "all"
			if len(args) == 1 {
				target = args[0]
			}
			if resetAnsible && uninstallAnsible {
				return fmt.Errorf("--reset and --uninstall are mutually exclusive")
			}
			if (resetAnsible || uninstallAnsible) && target != "ansible" {
				return fmt.Errorf("--reset and --uninstall require the ansible setup target")
			}
			switch target {
			case "all":
				if err := setupAnsible(cmd, s); err != nil {
					return err
				}
				if err := setupDocs(cmd, s); err != nil {
					return err
				}
				return setupReportTools(cmd, s)
			case "ansible":
				if uninstallAnsible {
					return removeAnsibleRuntime(s)
				}
				if resetAnsible {
					if err := removeAnsibleRuntime(s); err != nil {
						return err
					}
				}
				return setupAnsible(cmd, s)
			case "docs":
				return setupDocs(cmd, s)
			case "reports":
				return setupReportTools(cmd, s)
			default:
				return fmt.Errorf("unknown setup target %q; expected all, ansible, docs, or reports", target)
			}
		},
	}
	cmd.Flags().BoolVar(&resetAnsible, "reset", false, "remove generated Ansible runtime state, then reinstall pinned dependencies")
	cmd.Flags().BoolVar(&uninstallAnsible, "uninstall", false, "remove generated Ansible runtime state without reinstalling it")
	return cmd
}

func removeAnsibleRuntime(s *state) error {
	for _, relative := range []string{".venv", ".ansible", "collections"} {
		path := s.dir("ansible", relative)
		if s.dryRun {
			s.info("would remove generated Ansible path " + path)
			continue
		}
		if err := os.RemoveAll(path); err != nil {
			return fmt.Errorf("removing generated Ansible path %s: %w", path, err)
		}
		s.success("removed generated Ansible path " + path)
	}
	return nil
}

func setupAnsible(cmd *cobra.Command, s *state) error {
	dir := s.dir("ansible")
	venv := filepath.Join(dir, ".venv")
	if err := s.run(cmd.Context(), dir, "python3", "-m", "venv", venv); err != nil {
		return err
	}
	python := filepath.Join(venv, "bin", "python")
	if err := s.run(cmd.Context(), dir, python, "-m", "pip", "install", "--requirement", "requirements.txt"); err != nil {
		return err
	}
	galaxy := filepath.Join(venv, "bin", "ansible-galaxy")
	return s.runEnv(cmd.Context(), dir, s.ansibleEnvironment(), galaxy, "collection", "install", "--requirement", "requirements.yml", "--collections-path", "collections")
}

func setupDocs(cmd *cobra.Command, s *state) error {
	return s.run(cmd.Context(), s.dir("docs"), "npm", "ci")
}

func setupReportTools(cmd *cobra.Command, s *state) error {
	tools := []string{
		"gotest.tools/gotestsum@" + gotestsumVersion,
		"github.com/securego/gosec/v2/cmd/gosec@" + gosecVersion,
	}
	for _, tool := range tools {
		if err := s.run(cmd.Context(), s.root, "go", "install", tool); err != nil {
			return err
		}
	}
	return nil
}
