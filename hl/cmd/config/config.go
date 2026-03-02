package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"

	"github.com/iamkhattar/homelab/hl/internal/ui"
)

var Cmd = &cobra.Command{
	Use:   "config",
	Short: "Manage CLI configuration",
	Long:  "Manage the hl CLI configuration stored at ~/.homelab/config.yaml.",
}

var initCmd = &cobra.Command{
	Use:     "init",
	Short:   "Initialize CLI configuration",
	Long:    "Create a default configuration file at ~/.homelab/config.yaml with sensible defaults.",
	Example: "  hl config init",
	RunE: func(cmd *cobra.Command, args []string) error {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}

		configDir := filepath.Join(home, ".homelab")
		configFile := filepath.Join(configDir, "config.yaml")

		if _, err := os.Stat(configFile); err == nil {
			ui.StepDone(fmt.Sprintf("Config file already exists: %s", ui.SubtleStyle.Render(configFile)))
			return nil
		}

		if err := os.MkdirAll(configDir, 0755); err != nil {
			return err
		}

		defaults := map[string]interface{}{
			"cluster": map[string]interface{}{
				"kubeconfig": filepath.Join(home, ".kube", "config"),
				"context":    "",
				"namespace":  "default",
			},
			"infra": map[string]interface{}{
				"dir": "",
			},
			"helmfile": map[string]interface{}{
				"dir": "",
			},
			"services": map[string]interface{}{
				"domain": "",
			},
		}

		data, err := yaml.Marshal(defaults)
		if err != nil {
			return err
		}

		if err := os.WriteFile(configFile, data, 0644); err != nil {
			return err
		}

		ui.StepDone(fmt.Sprintf("Config initialized: %s", ui.SubtleStyle.Render(configFile)))
		return nil
	},
}

var showCmd = &cobra.Command{
	Use:     "show",
	Short:   "Print current configuration",
	Long:    "Display all current configuration values.",
	Example: "  hl config show",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println(ui.Heading.Render("Configuration"))
		fmt.Println()
		settings := viper.AllSettings()
		data, err := yaml.Marshal(settings)
		if err != nil {
			return err
		}
		fmt.Print(string(data))
		return nil
	},
}

var setCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a configuration value",
	Long:  "Set a configuration value using dot-notation keys (e.g. cluster.namespace).",
	Example: `  hl config set cluster.namespace applications
  hl config set helmfile.dir /path/to/cluster`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		viper.Set(args[0], args[1])
		if err := viper.WriteConfig(); err != nil {
			return fmt.Errorf("failed to write config: %w", err)
		}
		ui.StepDone(fmt.Sprintf("%s = %s", ui.Bold.Render(args[0]), args[1]))
		return nil
	},
}

func init() {
	Cmd.AddCommand(initCmd)
	Cmd.AddCommand(showCmd)
	Cmd.AddCommand(setCmd)
}
