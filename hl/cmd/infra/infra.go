package infra

import (
	"fmt"

	"github.com/spf13/cobra"

	hlexec "github.com/iamkhattar/homelab/hl/internal/exec"
	"github.com/iamkhattar/homelab/hl/internal/ui"
)

var Cmd = &cobra.Command{
	Use:   "infra",
	Short: "Terraform infrastructure lifecycle (optional, for external nodes)",
	Long:  "Manage external Hetzner infrastructure via Terraform. These commands are optional and only needed when provisioning cloud nodes.",
}

var initCmd = &cobra.Command{
	Use:     "init",
	Short:   "Initialize Terraform",
	Long:    "Run terraform init to initialize providers and the Terraform Cloud backend.",
	Example: "  hl infra init",
	RunE: func(cmd *cobra.Command, args []string) error {
		ui.Step("Initializing Terraform")
		if err := hlexec.Terraform("init"); err != nil {
			ui.StepFail("Terraform init failed")
			return err
		}
		ui.StepDone("Terraform initialized")
		return nil
	},
}

var planCmd = &cobra.Command{
	Use:     "plan",
	Short:   "Preview infrastructure changes",
	Long:    "Run terraform plan to preview what changes would be applied.",
	Example: "  hl infra plan",
	RunE: func(cmd *cobra.Command, args []string) error {
		ui.Step("Planning infrastructure changes")
		return hlexec.Terraform("plan")
	},
}

var applyCmd = &cobra.Command{
	Use:     "apply",
	Short:   "Apply infrastructure changes",
	Long:    "Run terraform apply to provision or update external infrastructure.",
	Example: "  hl infra apply",
	RunE: func(cmd *cobra.Command, args []string) error {
		ui.Step("Applying infrastructure changes")
		if err := hlexec.Terraform("apply"); err != nil {
			ui.StepFail("Terraform apply failed")
			return err
		}
		ui.StepDone("Infrastructure updated")
		return nil
	},
}

var destroyCmd = &cobra.Command{
	Use:     "destroy",
	Short:   "Destroy infrastructure (with confirmation)",
	Long:    "Run terraform destroy to tear down all external infrastructure. Terraform will prompt for confirmation.",
	Example: "  hl infra destroy",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println(ui.WarningStyle.Render("⚠ This will destroy all external infrastructure"))
		ui.Step("Destroying infrastructure")
		if err := hlexec.Terraform("destroy"); err != nil {
			ui.StepFail("Terraform destroy failed")
			return err
		}
		ui.StepDone("Infrastructure destroyed")
		return nil
	},
}

func init() {
	Cmd.AddCommand(initCmd)
	Cmd.AddCommand(planCmd)
	Cmd.AddCommand(applyCmd)
	Cmd.AddCommand(destroyCmd)
}
