package ci

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"

	"github.com/iamkhattar/homelab/hl/internal/ui"
)

var docsDir string

var docsCmd = &cobra.Command{
	Use:     "docs",
	Short:   "Generate CLI documentation in markdown",
	Long:    "Generate markdown documentation for all CLI commands into the specified directory (default: ../docs/cli).",
	Example: "  hl ci docs\n  hl ci docs --output ../docs/cli",
	RunE: func(cmd *cobra.Command, args []string) error {
		ui.Step(fmt.Sprintf("Generating docs to %s", ui.Bold.Render(docsDir)))

		if err := os.MkdirAll(docsDir, 0755); err != nil {
			return fmt.Errorf("failed to create docs directory: %w", err)
		}

		root := cmd.Root()
		if err := doc.GenMarkdownTree(root, docsDir); err != nil {
			ui.StepFail("Doc generation failed")
			return err
		}

		absDir, _ := filepath.Abs(docsDir)
		ui.StepDone(fmt.Sprintf("Documentation generated at %s", ui.SubtleStyle.Render(absDir)))
		return nil
	},
}

func init() {
	docsCmd.Flags().StringVar(&docsDir, "output", "../docs/cli", "Output directory for generated docs")
	Cmd.AddCommand(docsCmd)
}
