package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	Version = "dev"
	Commit  = "none"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the CLI version",
	Long:  "Display the current version and commit hash of the hl CLI.",
	Example: "  hl version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("hl %s (commit: %s)\n", Version, Commit)
	},
}
