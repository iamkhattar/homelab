package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newDocsCommand(s *state) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "docs",
		Short: "Develop and run the internal documentation site",
	}

	setupCmd := &cobra.Command{
		Use:   "setup",
		Short: "Install pinned documentation dependencies",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return setupDocs(cmd, s)
		},
	}
	devCmd := npmDocsCommand(s, "dev", "Start the VitePress development server")
	buildCmd := npmDocsCommand(s, "build", "Build the static documentation site")
	previewCmd := npmDocsCommand(s, "preview", "Preview the production documentation build")

	var image string
	var port int
	serveCmd := &cobra.Command{
		Use:   "serve",
		Short: "Run a built documentation container locally",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if port < 1 || port > 65535 {
				return fmt.Errorf("port must be between 1 and 65535")
			}
			if err := validateContainerValue(image, "documentation image"); err != nil {
				return err
			}
			return s.run(cmd.Context(), s.root, "docker", "run", "--rm", "--name", "homelab-docs", "--publish", fmt.Sprintf("%d:8080", port), image)
		},
	}
	serveCmd.Flags().StringVar(&image, "image", "iamkhattar/homelab-docs:dev", "documentation image to run")
	serveCmd.Flags().IntVar(&port, "port", 8080, "local port to publish")

	cmd.AddCommand(setupCmd, devCmd, buildCmd, previewCmd, serveCmd)
	return cmd
}

func npmDocsCommand(s *state, script, description string) *cobra.Command {
	return &cobra.Command{
		Use:   script,
		Short: description,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return s.run(cmd.Context(), s.dir("docs"), "npm", "run", script)
		},
	}
}
