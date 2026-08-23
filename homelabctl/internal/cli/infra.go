package cli

import "github.com/spf13/cobra"

func newInfraCommand(s *state) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "infra",
		Short: "Validate and plan optional Terraform infrastructure",
		Long:  "Read-only Terraform operations for the future Hetzner layer. Apply and destroy are intentionally not exposed yet.",
	}

	fmtCmd := &cobra.Command{
		Use:   "fmt",
		Short: "Check Terraform formatting",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return s.run(cmd.Context(), s.dir("infra"), "terraform", "fmt", "-check", "-recursive")
		},
	}

	validateCmd := &cobra.Command{
		Use:   "validate",
		Short: "Initialise without a backend and validate Terraform",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := s.run(cmd.Context(), s.dir("infra"), "terraform", "init", "-backend=false", "-input=false"); err != nil {
				return err
			}
			return s.run(cmd.Context(), s.dir("infra"), "terraform", "validate")
		},
	}

	planCmd := &cobra.Command{
		Use:   "plan",
		Short: "Create a Terraform execution plan",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return s.run(cmd.Context(), s.dir("infra"), "terraform", "plan")
		},
	}

	cmd.AddCommand(fmtCmd, validateCmd, planCmd)
	return cmd
}
