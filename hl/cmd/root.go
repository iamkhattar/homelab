package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/iamkhattar/homelab/hl/cmd/app"
	"github.com/iamkhattar/homelab/hl/cmd/build"
	"github.com/iamkhattar/homelab/hl/cmd/cluster"
	"github.com/iamkhattar/homelab/hl/cmd/config"
	"github.com/iamkhattar/homelab/hl/cmd/deploy"
	"github.com/iamkhattar/homelab/hl/cmd/infra"
)

var rootCmd = &cobra.Command{
	Use:   "hl",
	Short: "Homelab CLI — manage your homelab cluster",
	Long: `hl is the single entry point for managing your homelab.

It wraps Terraform, Ansible, Helmfile, and kubectl behind ergonomic
subcommands for provisioning infrastructure, bootstrapping nodes,
deploying services, and interacting with running applications.`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.AddCommand(build.Cmd)
	rootCmd.AddCommand(cluster.Cmd)
	rootCmd.AddCommand(deploy.Cmd)
	rootCmd.AddCommand(infra.Cmd)
	rootCmd.AddCommand(app.Cmd)
	rootCmd.AddCommand(config.Cmd)
	rootCmd.AddCommand(versionCmd)
}

func initConfig() {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	configDir := filepath.Join(home, ".homelab")
	viper.AddConfigPath(configDir)
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")

	viper.SetEnvPrefix("HL")
	viper.AutomaticEnv()

	// Read config file if it exists; ignore if not found
	_ = viper.ReadInConfig()
}
