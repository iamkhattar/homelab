package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/iamkhattar/homelab/homelabctl/internal/repository"
)

func newDeployCommand(s *state) *cobra.Command {
	var stage string
	var imageTag string
	cmd := &cobra.Command{
		Use:   "deploy",
		Short: "Preview and apply the Helmfile desired state",
	}

	diffCmd := &cobra.Command{
		Use:   "diff",
		Short: "Preview pending Helm changes",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runHelmfileDeploy(cmd.Context(), s, imageTag, "diff")
		},
	}

	applyCmd := &cobra.Command{
		Use:   "apply [release]",
		Short: "Apply changed releases, optionally selecting one release",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if stage != "" && len(args) == 1 {
				return fmt.Errorf("release and --stage cannot be used together")
			}
			helmArgs := []string{"apply"}
			if len(args) == 1 {
				if err := validateReleaseName(args[0]); err != nil {
					return err
				}
				helmArgs = append(helmArgs, "--selector", fmt.Sprintf("name=%s", args[0]), "--include-needs")
			}
			if stage != "" {
				if err := validateReleaseName(stage); err != nil {
					return fmt.Errorf("invalid stage: %w", err)
				}
				helmArgs = append(helmArgs, "--selector", fmt.Sprintf("stage=%s", stage), "--include-needs")
			}
			return runHelmfileDeploy(cmd.Context(), s, imageTag, helmArgs...)
		},
	}
	applyCmd.Flags().StringVar(&stage, "stage", "", "apply releases with this Helmfile stage label")

	syncCmd := &cobra.Command{
		Use:   "sync",
		Short: "Synchronise every release without diff gating",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runHelmfileDeploy(cmd.Context(), s, imageTag, "sync")
		},
	}

	cmd.AddCommand(diffCmd, applyCmd, syncCmd)
	cmd.PersistentFlags().StringVar(&imageTag, "image-tag", "", "shared immutable image tag (default: current full Git commit SHA)")
	return cmd
}

func runHelmfileDeploy(ctx context.Context, s *state, imageTag string, args ...string) error {
	if imageTag == "" {
		sha, err := repository.HeadSHA(s.root)
		if err != nil {
			return fmt.Errorf("resolving default deploy image tag from Git: %w", err)
		}
		imageTag = sha
	}
	if err := validateTags([]string{imageTag}); err != nil {
		return err
	}
	return s.runEnv(ctx, s.dir("cluster"), map[string]string{"HOMELAB_IMAGE_TAG": imageTag}, "helmfile", args...)
}
