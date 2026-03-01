package deploy

import (
	"fmt"

	"github.com/spf13/cobra"

	hlexec "github.com/iamkhattar/homelab/hl/internal/exec"
	"github.com/iamkhattar/homelab/hl/internal/ui"
)

var Cmd = &cobra.Command{
	Use:   "deploy",
	Short: "Helmfile deployment lifecycle",
	Long:  "Manage Helm releases via Helmfile. Sync, diff, and apply releases to the cluster.",
}

var syncCmd = &cobra.Command{
	Use:     "sync",
	Short:   "Deploy everything (helmfile sync)",
	Long:    "Synchronize all Helm releases to their desired state.",
	Example: "  hl deploy sync",
	RunE: func(cmd *cobra.Command, args []string) error {
		ui.Step("Syncing all releases")
		if err := hlexec.Helmfile("sync"); err != nil {
			ui.StepFail("Sync failed")
			return err
		}
		ui.StepDone("All releases synced")
		return nil
	},
}

var diffCmd = &cobra.Command{
	Use:     "diff",
	Short:   "Preview pending changes (helmfile diff)",
	Long:    "Show a diff of what would change if you ran apply.",
	Example: "  hl deploy diff",
	RunE: func(cmd *cobra.Command, args []string) error {
		ui.Step("Diffing releases")
		return hlexec.Helmfile("diff")
	},
}

var applyCmd = &cobra.Command{
	Use:   "apply [release]",
	Short: "Apply all or a specific release",
	Long:  "Apply changed releases. Optionally target a single release by name.",
	Example: `  hl deploy apply
  hl deploy apply vault
  hl deploy apply cert-manager`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 1 {
			release := args[0]
			ui.Step(fmt.Sprintf("Applying release %s", ui.Bold.Render(release)))
			if err := hlexec.Helmfile("apply", "-l", fmt.Sprintf("name=%s", release)); err != nil {
				ui.StepFail(fmt.Sprintf("Failed to apply %s", release))
				return err
			}
			ui.StepDone(fmt.Sprintf("Release %s applied", ui.Bold.Render(release)))
			return nil
		}

		ui.Step("Applying all changed releases")
		if err := hlexec.Helmfile("apply"); err != nil {
			ui.StepFail("Apply failed")
			return err
		}
		ui.StepDone("All releases applied")
		return nil
	},
}

func init() {
	Cmd.AddCommand(syncCmd)
	Cmd.AddCommand(diffCmd)
	Cmd.AddCommand(applyCmd)
}
