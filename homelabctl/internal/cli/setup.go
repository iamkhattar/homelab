package cli

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"
)

func newSetupCommand(s *state) *cobra.Command {
	return &cobra.Command{
		Use:   "setup [all|ansible|docs]",
		Short: "Install pinned workstation dependencies",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := "all"
			if len(args) == 1 {
				target = args[0]
			}
			switch target {
			case "all":
				if err := setupAnsible(cmd, s); err != nil {
					return err
				}
				return setupDocs(cmd, s)
			case "ansible":
				return setupAnsible(cmd, s)
			case "docs":
				return setupDocs(cmd, s)
			default:
				return fmt.Errorf("unknown setup target %q; expected all, ansible, or docs", target)
			}
		},
	}
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
