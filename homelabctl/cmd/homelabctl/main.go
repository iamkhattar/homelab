package main

import (
	"context"
	"fmt"
	"os"

	"github.com/iamkhattar/homelab/homelabctl/internal/cli"
	"github.com/iamkhattar/homelab/homelabctl/internal/command"
	"github.com/iamkhattar/homelab/homelabctl/internal/ui"
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
		ui.New(os.Stderr).Error(fmt.Sprint(err))
		os.Exit(1)
	}
}
