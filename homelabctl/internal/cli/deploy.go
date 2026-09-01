package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/iamkhattar/homelab/homelabctl/internal/repository"
)

func newDeployCommand(s *state) *cobra.Command {
	var imageTag string
	cmd := &cobra.Command{
		Use:   "deploy",
		Short: "Preview and apply the Helmfile desired state",
	}

	var diffStage string
	diffCmd := &cobra.Command{
		Use:   "diff [release]",
		Short: "Preview pending Helm changes, optionally selecting one release or stage",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			helmArgs, err := helmfileSelectionArgs("diff", args, diffStage)
			if err != nil {
				return err
			}
			return runHelmfileDeploy(cmd.Context(), s, imageTag, helmArgs...)
		},
	}
	diffCmd.Flags().StringVar(&diffStage, "stage", "", "preview releases with this Helmfile stage label")

	var applyStage string
	applyCmd := &cobra.Command{
		Use:   "apply [release]",
		Short: "Apply changed releases, optionally selecting one release",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			helmArgs, err := helmfileSelectionArgs("apply", args, applyStage)
			if err != nil {
				return err
			}
			return runHelmfileDeploy(cmd.Context(), s, imageTag, helmArgs...)
		},
	}
	applyCmd.Flags().StringVar(&applyStage, "stage", "", "apply releases with this Helmfile stage label")

	var syncStage string
	syncCmd := &cobra.Command{
		Use:   "sync [release]",
		Short: "Synchronise releases without diff gating, optionally selecting one release or stage",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			helmArgs, err := helmfileSelectionArgs("sync", args, syncStage)
			if err != nil {
				return err
			}
			return runHelmfileDeploy(cmd.Context(), s, imageTag, helmArgs...)
		},
	}
	syncCmd.Flags().StringVar(&syncStage, "stage", "", "synchronise releases with this Helmfile stage label")

	cmd.AddCommand(diffCmd, applyCmd, syncCmd, newDeployPlatformCommand(s, &imageTag))
	cmd.PersistentFlags().StringVar(&imageTag, "image-tag", "", "shared immutable image tag (default: current full Git commit SHA)")
	return cmd
}

func helmfileSelectionArgs(action string, args []string, stage string) ([]string, error) {
	if stage != "" && len(args) == 1 {
		return nil, fmt.Errorf("release and --stage cannot be used together")
	}
	helmArgs := []string{action}
	if len(args) == 1 {
		if err := validateReleaseName(args[0]); err != nil {
			return nil, err
		}
		helmArgs = append(helmArgs, "--selector", fmt.Sprintf("name=%s", args[0]), "--include-needs")
	}
	if stage != "" {
		if err := validateReleaseName(stage); err != nil {
			return nil, fmt.Errorf("invalid stage: %w", err)
		}
		helmArgs = append(helmArgs, "--selector", fmt.Sprintf("stage=%s", stage), "--include-needs")
	}
	return helmArgs, nil
}

var platformStages = []string{"foundation", "networking", "secrets", "identity", "data", "observability", "cicd", "applications", "smart-home"}

func newDeployPlatformCommand(s *state, imageTag *string) *cobra.Command {
	var through string
	var confirm bool
	command := &cobra.Command{
		Use:     "platform",
		Short:   "Apply the platform in its safe dependency order",
		Long:    "Apply reviewed Helmfile stages in dependency order. The workflow stops before shared data, observability, CI/CD, applications, or opt-in home automation until Butler's bootstrap ConfigMap records successful Pocket ID logins to both Butler and Vault.",
		Example: "  homelabctl deploy platform --through identity --confirm\n  homelabctl control verify-identity --confirm\n  homelabctl deploy platform --through observability --confirm",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !confirm {
				return fmt.Errorf("--confirm is required for dependency-ordered platform deployment")
			}
			last := -1
			for index, stage := range platformStages {
				if stage == through {
					last = index
					break
				}
			}
			if last < 0 {
				return fmt.Errorf("--through must be one of: %s", strings.Join(platformStages, ", "))
			}
			bootstrapChecked := false
			for index, stage := range platformStages[:last+1] {
				if index > 3 && !bootstrapChecked {
					if err := requireOperationalBootstrap(cmd.Context(), s); err != nil {
						return err
					}
					bootstrapChecked = true
				}
				s.heading("Deploy stage: " + stage)
				if err := runHelmfileDeploy(cmd.Context(), s, *imageTag, "apply", "--selector", "stage="+stage, "--include-needs"); err != nil {
					return fmt.Errorf("applying %s stage: %w", stage, err)
				}
			}
			if last == 3 {
				s.info("Identity workloads are applied. Complete Pocket ID enrollment and run homelabctl control bootstrap, control login, and control verify-identity before continuing to data or applications.")
			}
			return nil
		},
	}
	command.Flags().StringVar(&through, "through", "identity", "last dependency stage to apply")
	command.Flags().BoolVar(&confirm, "confirm", false, "confirm ordered platform mutation")
	return command
}

func requireOperationalBootstrap(ctx context.Context, s *state) error {
	args := []string{"--context", s.kubeContext, "--namespace", "security", "get", "configmap", "butler-bootstrap-state", "--output=jsonpath={.data.phase}"}
	if s.dryRun {
		return s.run(ctx, s.root, "kubectl", args...)
	}
	phase, err := s.output(ctx, s.root, "kubectl", args...)
	if err != nil {
		return fmt.Errorf("checking Butler bootstrap acceptance: %w", err)
	}
	if strings.TrimSpace(phase) != "operational" {
		return fmt.Errorf("Butler bootstrap phase is %q; complete Pocket ID enrollment and homelabctl control verify-identity before deploying dependent stages", strings.TrimSpace(phase))
	}
	return nil
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
	if len(args) > 0 && (args[0] == "diff" || args[0] == "apply") {
		// New releases may contain custom resources whose CRDs are installed by
		// an earlier release in the same dependency graph. Helmfile limits this
		// bypass to installations; upgrades retain API discovery validation.
		args = append(args, "--skip-diff-validation-on-install")
	}
	return s.runEnv(ctx, s.dir("cluster"), map[string]string{"HOMELAB_IMAGE_TAG": imageTag}, "helmfile", args...)
}
