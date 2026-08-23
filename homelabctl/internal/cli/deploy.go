package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newDeployCommand(s *state) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deploy",
		Short: "Preview and apply the Helmfile desired state",
	}

	diffCmd := &cobra.Command{
		Use:   "diff",
		Short: "Preview pending Helm changes",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return s.run(cmd.Context(), s.dir("cluster"), "helmfile", "diff")
		},
	}

	applyCmd := &cobra.Command{
		Use:   "apply [release]",
		Short: "Apply changed releases, optionally selecting one release",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			helmArgs := []string{"apply"}
			if len(args) == 1 {
				if err := validateReleaseName(args[0]); err != nil {
					return err
				}
				helmArgs = append(helmArgs, "--selector", fmt.Sprintf("name=%s", args[0]))
			}
			return s.run(cmd.Context(), s.dir("cluster"), "helmfile", helmArgs...)
		},
	}

	syncCmd := &cobra.Command{
		Use:   "sync",
		Short: "Synchronise every release without diff gating",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return s.run(cmd.Context(), s.dir("cluster"), "helmfile", "sync")
		},
	}

	cmd.AddCommand(diffCmd, applyCmd, syncCmd)
	return cmd
}
