package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newVersionCommand(s *state) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version and build information",
		Args:  cobra.NoArgs,
		Run: func(_ *cobra.Command, _ []string) {
			s.print("homelabctl %s (commit: %s, built: %s)\n", s.build.Version, s.build.Commit, s.build.Date)
		},
		Example: fmt.Sprintf("  %s", "homelabctl version"),
	}
}
