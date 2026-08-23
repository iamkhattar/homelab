package main

import (
	"context"
	"fmt"
	"os"

	"github.com/iamkhattar/homelab/homelabctl/internal/cli"
	"github.com/iamkhattar/homelab/homelabctl/internal/command"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	runner := command.NewRunner(os.Stdin, os.Stdout, os.Stderr)
	root := cli.New(cli.BuildInfo{
		Version: version,
		Commit:  commit,
		Date:    date,
	}, runner)

	if err := root.ExecuteContext(context.Background()); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
