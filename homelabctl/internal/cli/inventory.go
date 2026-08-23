package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

func newInventoryCommand(s *state) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "inventory",
		Short: "Create and validate the private node inventory",
	}

	initCmd := &cobra.Command{
		Use:   "init",
		Short: "Create hosts.yml from the committed example",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			source := s.dir("ansible", "inventory", "hosts.example.yml")
			destination := s.dir("ansible", "inventory", "hosts.yml")
			if s.dryRun {
				s.info(fmt.Sprintf("would create %s from %s", destination, source))
				return nil
			}
			if _, err := os.Stat(destination); err == nil {
				return fmt.Errorf("%s already exists; refusing to overwrite private inventory", destination)
			} else if !os.IsNotExist(err) {
				return err
			}
			return copyExclusive(source, destination)
		},
	}

	showCmd := &cobra.Command{
		Use:   "show",
		Short: "Render the effective inventory graph",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return s.runAnsible(cmd.Context(), "ansible-inventory", "--graph")
		},
	}

	var verbose bool
	checkCmd := &cobra.Command{
		Use:   "check",
		Short: "Render inventory and verify Ansible connectivity",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := s.runAnsible(cmd.Context(), "ansible-inventory", "--graph"); err != nil {
				return err
			}
			args := []string{"k3s_cluster", "--module-name", "ansible.builtin.ping"}
			if verbose {
				args = append(args, "-vv")
			}
			return s.runAnsible(cmd.Context(), "ansible", args...)
		},
	}
	checkCmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "show verbose Ansible connection details")

	cmd.AddCommand(initCmd, showCmd, checkCmd)
	return cmd
}

func copyExclusive(source, destination string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}
